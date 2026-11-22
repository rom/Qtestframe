<script setup lang="ts">
// Chat channels of a project: Slack, Microsoft Teams, Mattermost,
// Rocket.Chat, Google Chat, Matrix and Discord. The webhook URL / access
// token is write-only; events come from the server's event registry, so
// events added later show up here without a change.
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Checkbox from 'primevue/checkbox'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Select from 'primevue/select'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { api, asItems, errorMessage } from '@/lib/api'
import {
  CHAT_PROVIDERS,
  PROVIDER_NAMES,
  channelState,
  chatBody,
  chatFormProblems,
  deliverySeverity,
  eventOptionGroups,
  eventsText,
  type ChatChannel,
  type ChatDelivery,
  type ChatEvent,
  type ChatForm,
  type ChatGroup,
} from '@/lib/chat'
import { formatDateTime } from '@/lib/format'
import { useNotify } from '@/composables/notify'
import type { Release } from '@/lib/types'

const props = defineProps<{ projectId: string }>()
const { t, locale } = useI18n()
const notify = useNotify()

const channels = ref<ChatChannel[]>([])
const events = ref<ChatEvent[]>([])
const groups = ref<ChatGroup[]>([])
const releases = ref<Release[]>([])
const restricted = ref(false)
const canAllowRestricted = ref(false)
const hasBaseURL = ref(true)
const error = ref('')

async function load() {
  error.value = ''
  try {
    const res = await api.get<{ items: ChatChannel[]; restricted: boolean; can_allow_restricted: boolean; base_url: boolean }>(`/projects/${props.projectId}/chat-channels`)
    channels.value = asItems(res)
    restricted.value = res.restricted
    canAllowRestricted.value = res.can_allow_restricted
    hasBaseURL.value = res.base_url
    if (!events.value.length) {
      const ev = await api.get<{ items: ChatEvent[]; groups: ChatGroup[] }>('/chat-events')
      events.value = ev.items
      groups.value = ev.groups
    }
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)

const lang = computed(() => (locale.value === 'sv' ? 'sv' : 'en'))
const eventGroups = computed(() => eventOptionGroups(events.value, groups.value, lang.value))
const providerOptions = CHAT_PROVIDERS.map((p) => ({ label: PROVIDER_NAMES[p], value: p }))
const priorityOptions = computed(() => [{ label: t('chat.anyPriority'), value: '' }, ...['critical', 'high', 'medium', 'low'].map((p) => ({ label: t(`enums.priority.${p}`), value: p }))])
const releaseOptions = computed(() => [{ label: t('chat.anyRelease'), value: '' }, ...releases.value.map((r) => ({ label: r.version ? `${r.name} ${r.version}` : r.name, value: r.id }))])
const languageOptions = [
  { label: 'English', value: 'en' },
  { label: 'Svenska', value: 'sv' },
]
function eventLabel(ev: string): string {
  if (ev === 'digest') return t('chat.digestEvent')
  if (ev === 'chat.test') return t('chat.testEvent')
  const e = events.value.find((x) => x.event === ev)
  return e ? (e.labels[lang.value] ?? e.labels.en) : ev
}

// ---------- edit ----------
const editing = ref<ChatForm | null>(null)
const allEvents = ref(true)
const tried = ref(false)
const busy = ref(false)
const creating = computed(() => !editing.value?.id)
const problems = computed(() => (editing.value ? chatFormProblems(editing.value, creating.value) : {}))
const invalid = (k: string) => tried.value && !!problems.value[k]

async function loadReleases() {
  if (releases.value.length) return
  try {
    releases.value = asItems(await api.get<{ items: Release[] }>(`/projects/${props.projectId}/releases`))
  } catch {
    /* the release filter just offers no releases */
  }
}
function create() {
  tried.value = false
  allEvents.value = true
  editing.value = { name: '', provider: 'slack', url: '', token: '', homeserver: '', room: '', events: ['*'], filters: { failed_only: false, min_priority: '', release_id: '' }, language: lang.value, digest_minutes: 0, enabled: true, restricted_content: false }
  void loadReleases()
}
function edit(c: ChatChannel) {
  tried.value = false
  allEvents.value = c.events.includes('*')
  editing.value = { ...c, url: '', token: '', events: [...c.events], filters: { ...c.filters } }
  void loadReleases()
}
function setAllEvents(v: boolean) {
  allEvents.value = v
  if (editing.value) editing.value.events = v ? ['*'] : editing.value.events?.filter((e) => e !== '*') ?? []
}

async function save() {
  const f = editing.value
  tried.value = true
  if (!f || Object.keys(problems.value).length) return
  busy.value = true
  try {
    const body = chatBody(f)
    if (f.id) await api.patch(`/chat-channels/${f.id}`, body)
    else await api.post(`/projects/${props.projectId}/chat-channels`, { ...body, provider: f.provider })
    editing.value = null
    notify.ok(t('chat.saved'))
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}

async function test(c: ChatChannel) {
  try {
    const res = await api.post<{ ok: boolean; status: number; error?: string }>(`/chat-channels/${c.id}/test`)
    if (res.ok) notify.ok(t('chat.testOk', { status: res.status }))
    else notify.failText(t('chat.testFailed'), res.error ?? String(res.status))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}

async function toggle(c: ChatChannel) {
  try {
    await api.patch(`/chat-channels/${c.id}`, { enabled: !c.enabled })
    await load()
  } catch (e) {
    notify.fail(e)
  }
}

function remove(c: ChatChannel) {
  notify.confirmDanger(t('chat.confirmDelete', { name: c.name }), async () => {
    try {
      await api.del(`/chat-channels/${c.id}`)
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- delivery log ----------
const logOf = ref<ChatChannel | null>(null)
const log = ref<ChatDelivery[]>([])
const logBusy = ref(false)
async function openLog(c: ChatChannel) {
  logOf.value = c
  logBusy.value = true
  try {
    log.value = asItems(await api.get<{ items: ChatDelivery[] }>(`/chat-channels/${c.id}/deliveries`))
  } catch (e) {
    notify.fail(e)
  } finally {
    logBusy.value = false
  }
}

function filtersText(c: ChatChannel): string {
  const out: string[] = []
  if (c.filters.failed_only) out.push(t('chat.failedOnlyShort'))
  if (c.filters.min_priority) out.push(t('chat.minPriorityShort', { priority: t(`enums.priority.${c.filters.min_priority}`) }))
  if (c.filters.release_id) out.push(t('chat.releaseShort'))
  if (c.digest_minutes > 0) out.push(t('chat.digestShort', { n: c.digest_minutes }))
  return out.join(' · ')
}
</script>

<template>
  <section class="card chats" aria-labelledby="chat-title">
    <div class="row head">
      <h2 id="chat-title" class="plain-heading grow"><i class="pi pi-comments" aria-hidden="true" /> {{ t('chat.title') }}</h2>
      <Button :label="t('chat.add')" icon="pi pi-plus" size="small" aria-haspopup="dialog" @click="create" />
    </div>
    <p class="small muted">{{ t('chat.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <Message v-if="!hasBaseURL" severity="warn" :closable="false">{{ t('chat.noBaseURL') }}</Message>
    <Message v-if="restricted" severity="info" :closable="false">{{ t('chat.restrictedProject') }}</Message>
    <DataTable :value="channels" data-key="id" size="small">
      <Column field="name" :header="t('common.name')">
        <template #body="{ data }">
          <b>{{ data.name }}</b>
          <div class="small muted target">{{ PROVIDER_NAMES[data.provider as keyof typeof PROVIDER_NAMES] }} · {{ data.target }}</div>
        </template>
      </Column>
      <Column :header="t('chat.events')">
        <template #body="{ data }">
          <span class="small">{{ eventsText(data.events, events, lang, t('chat.allEvents')) }}</span>
          <div v-if="filtersText(data)" class="small muted">{{ filtersText(data) }}</div>
          <div v-if="data.pending" class="small muted">{{ t('chat.pending', { n: data.pending }, data.pending) }}</div>
        </template>
      </Column>
      <Column :header="t('chat.status')" style="width: 210px">
        <template #body="{ data }">
          <Tag v-if="channelState(data) === 'disabled'" :value="t('chat.autoDisabled')" severity="danger" />
          <Tag v-else-if="channelState(data) === 'off'" :value="t('chat.off')" severity="secondary" />
          <Tag v-else-if="channelState(data) === 'ok'" :value="t('chat.ok', { status: data.last_status })" severity="success" />
          <Tag v-else-if="channelState(data) === 'failed'" :value="t('chat.failed')" severity="danger" />
          <span v-else class="small muted">{{ t('chat.never') }}</span>
          <div v-if="data.disabled_reason" class="small error-text reason">{{ data.disabled_reason }}</div>
          <div v-else-if="data.last_error" class="small muted reason">{{ data.last_error }}</div>
          <div v-if="data.last_delivery_at" class="small muted">{{ formatDateTime(data.last_delivery_at) }}</div>
        </template>
      </Column>
      <Column style="width: 210px">
        <template #header><span class="sr-only">{{ t('common.actions') }}</span></template>
        <template #body="{ data }">
          <div class="row actions">
            <Button icon="pi pi-play" text rounded size="small" :aria-label="t('chat.testNamed', { name: data.name })" v-tooltip="t('chat.test')" @click="test(data)" />
            <Button icon="pi pi-list" text rounded size="small" :aria-label="t('chat.logNamed', { name: data.name })" v-tooltip="t('chat.log')" aria-haspopup="dialog" @click="openLog(data)" />
            <Button :icon="data.enabled ? 'pi pi-pause' : 'pi pi-power-off'" text rounded size="small" :aria-label="(data.enabled ? t('chat.turnOffNamed', { name: data.name }) : t('chat.turnOnNamed', { name: data.name }))" v-tooltip="data.enabled ? t('chat.turnOff') : t('chat.turnOn')" @click="toggle(data)" />
            <Button icon="pi pi-pencil" text rounded size="small" :aria-label="t('chat.editNamed', { name: data.name })" v-tooltip="t('common.edit')" aria-haspopup="dialog" @click="edit(data)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('chat.deleteNamed', { name: data.name })" v-tooltip="t('common.delete')" @click="remove(data)" />
          </div>
        </template>
      </Column>
      <template #empty><div class="empty">{{ t('chat.none') }}</div></template>
    </DataTable>

    <Dialog :visible="!!editing" modal :header="creating ? t('chat.add') : t('chat.edit')" :style="{ width: 'min(620px, 96vw)' }" @update:visible="(v: boolean) => !v && (editing = null)">
      <div v-if="editing" class="form">
        <label for="ch-provider">{{ t('chat.provider') }}</label>
        <Select v-model="editing!.provider" input-id="ch-provider" :options="providerOptions" option-label="label" option-value="value" fluid :disabled="!creating" aria-describedby="ch-provider-help" />
        <small id="ch-provider-help" class="muted">{{ t(`chat.help.${editing!.provider}`) }}</small>

        <label for="ch-name">{{ t('common.name') }}</label>
        <InputText id="ch-name" v-model="editing!.name" fluid :invalid="invalid('name')" :placeholder="t('chat.namePlaceholder')" />

        <template v-if="editing!.provider === 'matrix'">
          <label for="ch-hs">{{ t('chat.homeserver') }}</label>
          <InputText id="ch-hs" v-model="editing!.homeserver" fluid inputmode="url" placeholder="https://matrix.example.org" :invalid="invalid('homeserver')" />
          <label for="ch-room">{{ t('chat.room') }}</label>
          <InputText id="ch-room" v-model="editing!.room" fluid placeholder="!roomid:example.org" :invalid="invalid('room')" aria-describedby="ch-room-help" />
          <small id="ch-room-help" class="muted">{{ t('chat.roomHelp') }}</small>
          <label for="ch-token">{{ t('chat.token') }}</label>
          <InputText id="ch-token" v-model="editing!.token" type="password" autocomplete="off" fluid :invalid="invalid('token')" :placeholder="creating ? '' : t('chat.keepSecret')" aria-describedby="ch-secret-help" />
        </template>
        <template v-else>
          <label for="ch-url">{{ t('chat.url') }}</label>
          <InputText id="ch-url" v-model="editing!.url" type="password" autocomplete="off" fluid inputmode="url" :invalid="invalid('url')" :placeholder="creating ? 'https://' : t('chat.keepSecret')" aria-describedby="ch-secret-help" />
        </template>
        <small id="ch-secret-help" :class="invalid('url') || invalid('token') ? 'error-text' : 'muted'">{{ creating ? t('chat.secretHelp') : t('chat.secretKept', { target: editing!.target }) }}</small>

        <fieldset class="group">
          <legend>{{ t('chat.events') }}</legend>
          <div class="row toggle">
            <ToggleSwitch :model-value="allEvents" input-id="ch-all" @update:model-value="setAllEvents" />
            <label for="ch-all">{{ t('chat.allEventsHelp') }}</label>
          </div>
          <template v-if="!allEvents">
            <label for="ch-events" class="sr-only">{{ t('chat.events') }}</label>
            <MultiSelect v-model="editing!.events" input-id="ch-events" :options="eventGroups" option-label="label" option-value="value" option-group-label="label" option-group-children="items" filter display="chip" fluid :invalid="invalid('events')" :placeholder="t('chat.chooseEvents')" />
          </template>
        </fieldset>

        <fieldset class="group">
          <legend>{{ t('chat.filters') }}</legend>
          <div class="row toggle">
            <Checkbox v-model="editing!.filters!.failed_only" input-id="ch-failed" binary />
            <label for="ch-failed">{{ t('chat.failedOnly') }}</label>
          </div>
          <label id="ch-prio-label" for="ch-prio">{{ t('chat.minPriority') }}</label>
          <Select v-model="editing!.filters!.min_priority" input-id="ch-prio" :options="priorityOptions" option-label="label" option-value="value" fluid :placeholder="t('chat.anyPriority')" aria-labelledby="ch-prio-label" />
          <label id="ch-release-label" for="ch-release">{{ t('chat.release') }}</label>
          <Select v-model="editing!.filters!.release_id" input-id="ch-release" :options="releaseOptions" option-label="label" option-value="value" fluid :placeholder="t('chat.anyRelease')" aria-labelledby="ch-release-label" aria-describedby="ch-filter-help" />
          <small id="ch-filter-help" class="muted">{{ t('chat.filtersHelp') }}</small>
        </fieldset>

        <label for="ch-lang">{{ t('chat.language') }}</label>
        <Select v-model="editing!.language" input-id="ch-lang" :options="languageOptions" option-label="label" option-value="value" fluid />
        <label for="ch-digest">{{ t('chat.digest') }}</label>
        <InputNumber v-model="editing!.digest_minutes" input-id="ch-digest" :min="0" :max="1440" show-buttons :invalid="invalid('digest')" aria-describedby="ch-digest-help" />
        <small id="ch-digest-help" class="muted">{{ t('chat.digestHelp') }}</small>

        <div class="row toggle">
          <ToggleSwitch v-model="editing!.enabled" input-id="ch-enabled" />
          <label for="ch-enabled">{{ t('chat.enabled') }}</label>
        </div>
        <template v-if="restricted">
          <div class="row toggle">
            <ToggleSwitch v-model="editing!.restricted_content" input-id="ch-restricted" :disabled="!canAllowRestricted && !editing!.restricted_content" aria-describedby="ch-restricted-help" />
            <label for="ch-restricted">{{ t('chat.restrictedContent') }}</label>
          </div>
          <small id="ch-restricted-help" class="muted">{{ canAllowRestricted ? t('chat.restrictedHelp') : t('chat.restrictedAdminOnly') }}</small>
        </template>
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" severity="secondary" text @click="editing = null" />
        <Button :label="t('common.save')" icon="pi pi-check" :loading="busy" @click="save" />
      </template>
    </Dialog>

    <Dialog :visible="!!logOf" modal :header="t('chat.logTitle', { name: logOf?.name ?? '' })" :style="{ width: 'min(820px, 96vw)' }" @update:visible="(v: boolean) => !v && (logOf = null)">
      <p class="small muted">{{ t('chat.logHelp') }}</p>
      <DataTable :value="log" data-key="id" size="small" :loading="logBusy" scrollable scroll-height="60vh">
        <Column :header="t('chat.time')" style="width: 150px">
          <template #body="{ data }"><span class="small">{{ formatDateTime(data.sent_at ?? data.created_at) }}</span></template>
        </Column>
        <Column :header="t('chat.event')">
          <template #body="{ data }">
            <span class="small">{{ eventLabel(data.event) }}</span>
            <span v-if="data.count > 1" class="small muted"> ({{ data.count }})</span>
          </template>
        </Column>
        <Column :header="t('chat.status')" style="width: 120px">
          <template #body="{ data }"><Tag :value="t(`chat.deliveryStatus.${data.status}`)" :severity="deliverySeverity(data.status)" /></template>
        </Column>
        <Column :header="t('chat.attempts')" style="width: 80px">
          <template #body="{ data }"><span class="small">{{ data.attempts }}</span></template>
        </Column>
        <Column :header="t('chat.httpStatus')" style="width: 80px">
          <template #body="{ data }"><span class="small">{{ data.status_code || '–' }}</span></template>
        </Column>
        <Column :header="t('chat.error')">
          <template #body="{ data }">
            <span class="small reason">{{ data.error }}</span>
            <div v-if="data.status === 'retry' && data.next_attempt_at" class="small muted">{{ t('chat.nextAttempt', { at: formatDateTime(data.next_attempt_at) }) }}</div>
          </template>
        </Column>
        <template #empty><div class="empty">{{ t('chat.noDeliveries') }}</div></template>
      </DataTable>
      <template #footer><Button :label="t('common.close')" @click="logOf = null" /></template>
    </Dialog>
  </section>
</template>

<style scoped>
.chats {
  margin-top: 16px;
}
.head {
  gap: 8px;
}
.target,
.reason {
  overflow-wrap: anywhere;
}
.actions {
  gap: 0;
  flex-wrap: nowrap;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.form label,
.group legend {
  font-weight: 600;
  font-size: 13px;
  margin-top: 6px;
}
.group {
  border: 1px solid var(--p-content-border-color);
  border-radius: 6px;
  padding: 6px 12px 10px;
  margin: 8px 0 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.toggle {
  gap: 8px;
  margin-top: 6px;
}
.toggle label {
  margin: 0;
  font-weight: 400;
}
</style>
