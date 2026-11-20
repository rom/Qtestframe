<script setup lang="ts">
// Outgoing webhooks of a project: URL, events, signing secret (shown once),
// test delivery and the outcome of the latest delivery.
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Button from 'primevue/button'
import Column from 'primevue/column'
import DataTable from 'primevue/datatable'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { api, asItems, errorMessage } from '@/lib/api'
import { deliveryState, validWebhookUrl } from '@/lib/notifications'
import { formatDateTime } from '@/lib/format'
import { valuesBefore } from '@/lib/bulkRuns'
import { entry, recordUndo } from '@/lib/undoActions'
import { useNotify } from '@/composables/notify'
import type { Webhook } from '@/lib/types'

const props = defineProps<{ projectId: string }>()
const { t } = useI18n()
const notify = useNotify()
const hooks = ref<Webhook[]>([])
const events = ref<string[]>([])
const error = ref('')
const editing = ref<Partial<Webhook> | null>(null)
const busy = ref(false)
const secret = ref('')
const tried = ref(false)

const eventOptions = computed(() => [{ label: t('webhooks.allEvents'), value: '*' }, ...events.value.map((e) => ({ label: e, value: e }))])
const urlOk = computed(() => validWebhookUrl(editing.value?.url ?? ''))

async function load() {
  error.value = ''
  try {
    hooks.value = asItems(await api.get<{ items: Webhook[] }>(`/projects/${props.projectId}/webhooks`))
    if (!events.value.length) events.value = asItems(await api.get<{ items: string[] }>('/webhook-events'))
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(load)

function create() {
  tried.value = false
  editing.value = { name: '', url: '', events: ['*'], enabled: true }
}
function edit(w: Webhook) {
  tried.value = false
  editing.value = { ...w, events: [...w.events] }
}

async function save() {
  const w = editing.value
  tried.value = true
  if (!w || !w.name?.trim() || !urlOk.value || !w.events?.length) return
  busy.value = true
  try {
    if (w.id) {
      await api.patch(`/webhooks/${w.id}`, { name: w.name, url: w.url, events: w.events, enabled: w.enabled })
    } else {
      const out = await api.post<Webhook>(`/projects/${props.projectId}/webhooks`, { name: w.name, url: w.url, events: w.events, enabled: w.enabled })
      secret.value = out.secret ?? ''
    }
    editing.value = null
    notify.ok(t('webhooks.saved'))
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    busy.value = false
  }
}

async function test(w: Webhook) {
  try {
    const res = await api.post<{ ok: boolean; status: number; error?: string }>(`/webhooks/${w.id}/test`)
    if (res.ok) notify.ok(t('webhooks.testOk', { status: res.status }))
    else notify.failText(t('webhooks.testFailed'), res.error ?? String(res.status))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}

async function rotate(w: Webhook) {
  try {
    const out = await api.patch<Webhook>(`/webhooks/${w.id}`, { rotate_secret: true })
    secret.value = out.secret ?? ''
  } catch (e) {
    notify.fail(e)
  }
}

function remove(w: Webhook) {
  notify.confirmDanger(t('webhooks.confirmDelete', { name: w.name }), async () => {
    try {
      await api.del(`/webhooks/${w.id}`)
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

// ---------- bulk ----------
const selection = ref<Webhook[]>([])
const selectedIds = computed(() => selection.value.map((w) => w.id))
const eventsDialog = ref(false)
const eventsMode = ref<'add' | 'remove'>('add')
const eventsPick = ref<string[]>([])
const bulkBusy = ref('')
const testResults = ref<{ id: string; name: string; ok: boolean; status: number; error?: string }[]>([])
watch(hooks, (list) => {
  const ids = new Set(list.map((w) => w.id))
  selection.value = selection.value.filter((s) => ids.has(s.id))
})
async function bulkUpdate(body: Record<string, unknown>) {
  const ids = [...selectedIds.value]
  const before = valuesBefore(hooks.value, ids, ['enabled', 'events'])
  const path = `/projects/${props.projectId}/webhooks/bulk`
  try {
    await api.post(path, { ids, ...body })
    recordUndo(
      entry(
        t('webhooks.bulk.undo', { n: ids.length }),
        async () => {
          for (const b of before) await api.patch(`/webhooks/${b.id}`, b.before)
        },
        () => api.post(path, { ids, ...body }),
      ),
    )
    notify.ok(t('webhooks.bulk.updated', { n: ids.length }, ids.length))
    await load()
  } catch (e) {
    notify.fail(e)
  }
}
function openEvents(mode: 'add' | 'remove') {
  eventsMode.value = mode
  eventsPick.value = []
  eventsDialog.value = true
}
async function applyEvents() {
  eventsDialog.value = false
  if (eventsPick.value.length) await bulkUpdate(eventsMode.value === 'add' ? { add_events: eventsPick.value } : { remove_events: eventsPick.value })
}
async function bulkTest() {
  bulkBusy.value = 'test'
  testResults.value = []
  try {
    const res = await api.post<{ results: typeof testResults.value }>(`/projects/${props.projectId}/webhooks/test`, { ids: selectedIds.value })
    testResults.value = res.results
    await load()
  } catch (e) {
    notify.fail(e)
  } finally {
    bulkBusy.value = ''
  }
}
function bulkDelete() {
  const ids = [...selectedIds.value]
  notify.confirmDanger(t('webhooks.bulk.confirmDelete', { n: ids.length }, ids.length), async () => {
    try {
      await api.del(`/projects/${props.projectId}/webhooks/bulk`, { ids })
      notify.ok(t('webhooks.bulk.deleted', { n: ids.length }, ids.length))
      selection.value = []
      await load()
    } catch (e) {
      notify.fail(e)
    }
  })
}

async function copySecret() {
  try {
    await navigator.clipboard.writeText(secret.value)
    notify.ok(t('webhooks.copied'))
  } catch {
    /* the secret stays visible to copy by hand */
  }
}
</script>

<template>
  <section class="card webhooks" aria-labelledby="wh-title">
    <div class="row head">
      <h2 id="wh-title" class="plain-heading grow"><i class="pi pi-send" aria-hidden="true" /> {{ t('webhooks.title') }}</h2>
      <Button :label="t('webhooks.add')" icon="pi pi-plus" size="small" @click="create" />
    </div>
    <p class="small muted">{{ t('webhooks.intro') }}</p>
    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-if="selection.length" class="bulkbar" role="toolbar" :aria-label="t('webhooks.bulk.toolbar')">
      <b>{{ t('common.selected', { count: selection.length }) }}</b>
      <Button :label="t('webhooks.bulk.enable')" icon="pi pi-check-circle" size="small" severity="secondary" outlined @click="bulkUpdate({ enabled: true })" />
      <Button :label="t('webhooks.bulk.disable')" icon="pi pi-ban" size="small" severity="secondary" outlined @click="bulkUpdate({ enabled: false })" />
      <Button :label="t('webhooks.bulk.addEvents')" icon="pi pi-plus" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openEvents('add')" />
      <Button :label="t('webhooks.bulk.removeEvents')" icon="pi pi-minus" size="small" severity="secondary" outlined aria-haspopup="dialog" @click="openEvents('remove')" />
      <Button :label="t('webhooks.bulk.test')" icon="pi pi-play" size="small" severity="secondary" outlined :loading="bulkBusy === 'test'" @click="bulkTest" />
      <Button :label="t('common.delete')" icon="pi pi-trash" size="small" severity="danger" outlined @click="bulkDelete" />
      <Button icon="pi pi-times" size="small" text rounded severity="secondary" :aria-label="t('workspace.clearSelection')" v-tooltip.bottom="t('workspace.clearSelection')" @click="selection = []" />
    </div>
    <Message v-if="testResults.length" :severity="testResults.every((r) => r.ok) ? 'success' : 'warn'" closable style="margin-bottom: 10px" @close="testResults = []">
      <b>{{ t('webhooks.bulk.testResults', { ok: testResults.filter((r) => r.ok).length, n: testResults.length }) }}</b>
      <ul class="results">
        <li v-for="r in testResults" :key="r.id" class="small">
          <i :class="r.ok ? 'pi pi-check-circle' : 'pi pi-times-circle'" aria-hidden="true" />
          <b>{{ r.name }}</b>:
          <template v-if="r.ok">{{ t('webhooks.ok', { status: r.status }) }}</template>
          <template v-else>{{ r.error || t('webhooks.failed') }}</template>
        </li>
      </ul>
    </Message>
    <DataTable v-model:selection="selection" :value="hooks" data-key="id" size="small">
      <Column selection-mode="multiple" style="width: 2.6rem" />
      <Column field="name" :header="t('common.name')">
        <template #body="{ data }"><b>{{ data.name }}</b><div class="small muted url">{{ data.url }}</div></template>
      </Column>
      <Column :header="t('webhooks.events')">
        <template #body="{ data }"><span class="small">{{ data.events.includes('*') ? t('webhooks.allEvents') : data.events.join(', ') }}</span></template>
      </Column>
      <Column :header="t('webhooks.lastDelivery')" style="width: 200px">
        <template #body="{ data }">
          <Tag v-if="!data.enabled" :value="t('webhooks.disabled')" severity="secondary" />
          <template v-else>
            <Tag v-if="deliveryState(data) === 'ok'" :value="t('webhooks.ok', { status: data.last_status })" severity="success" />
            <Tag v-else-if="deliveryState(data) === 'failed'" :value="t('webhooks.failed')" severity="danger" v-tooltip="data.last_error" />
            <span v-else class="small muted">{{ t('webhooks.never') }}</span>
          </template>
          <div v-if="data.last_delivery_at" class="small muted">{{ formatDateTime(data.last_delivery_at) }}</div>
        </template>
      </Column>
      <Column style="width: 170px">
        <template #header><span class="sr-only">{{ t('common.actions') }}</span></template>
        <template #body="{ data }">
          <div class="row actions">
            <Button icon="pi pi-play" text rounded size="small" :aria-label="t('webhooks.testNamed', { name: data.name })" v-tooltip="t('webhooks.test')" @click="test(data)" />
            <Button icon="pi pi-pencil" text rounded size="small" :aria-label="t('webhooks.editNamed', { name: data.name })" v-tooltip="t('common.edit')" @click="edit(data)" />
            <Button icon="pi pi-key" text rounded size="small" :aria-label="t('webhooks.rotateNamed', { name: data.name })" v-tooltip="t('webhooks.rotate')" @click="rotate(data)" />
            <Button icon="pi pi-trash" text rounded size="small" severity="danger" :aria-label="t('webhooks.deleteNamed', { name: data.name })" v-tooltip="t('common.delete')" @click="remove(data)" />
          </div>
        </template>
      </Column>
      <template #empty><div class="empty">{{ t('webhooks.none') }}</div></template>
    </DataTable>

    <Dialog :visible="!!editing" modal :header="editing?.id ? t('webhooks.edit') : t('webhooks.add')" :style="{ width: 'min(560px, 96vw)' }" @update:visible="(v: boolean) => !v && (editing = null)">
      <div v-if="editing" class="form">
        <label for="wh-name">{{ t('common.name') }}</label>
        <InputText id="wh-name" v-model="editing!.name" fluid :invalid="tried && !editing!.name?.trim()" />
        <label for="wh-url">{{ t('webhooks.url') }}</label>
        <InputText id="wh-url" v-model="editing!.url" fluid inputmode="url" placeholder="https://" :invalid="tried && !urlOk" aria-describedby="wh-url-help" />
        <small id="wh-url-help" :class="tried && !urlOk ? 'error-text' : 'muted'">{{ t('webhooks.urlHelp') }}</small>
        <label for="wh-events">{{ t('webhooks.events') }}</label>
        <MultiSelect v-model="editing!.events" input-id="wh-events" :options="eventOptions" option-label="label" option-value="value" filter display="chip" fluid :invalid="tried && !editing!.events?.length" />
        <div class="row toggle">
          <ToggleSwitch v-model="editing!.enabled" input-id="wh-enabled" />
          <label for="wh-enabled">{{ t('webhooks.enabled') }}</label>
        </div>
        <p class="small muted">{{ t('webhooks.signatureHelp') }}</p>
      </div>
      <template #footer>
        <Button :label="t('common.cancel')" severity="secondary" text @click="editing = null" />
        <Button :label="t('common.save')" icon="pi pi-check" :loading="busy" @click="save" />
      </template>
    </Dialog>

    <Dialog v-model:visible="eventsDialog" modal :header="eventsMode === 'add' ? t('webhooks.bulk.addEventsHeader', { n: selection.length }, selection.length) : t('webhooks.bulk.removeEventsHeader', { n: selection.length }, selection.length)" :style="{ width: 'min(480px, 96vw)' }">
      <label for="wh-bulk-events" class="small" style="font-weight: 600">{{ t('webhooks.events') }}</label>
      <MultiSelect v-model="eventsPick" input-id="wh-bulk-events" :options="eventOptions" option-label="label" option-value="value" filter display="chip" fluid />
      <p v-if="eventsMode === 'remove'" class="small muted">{{ t('webhooks.bulk.keepOne') }}</p>
      <template #footer>
        <Button :label="t('common.cancel')" severity="secondary" text @click="eventsDialog = false" />
        <Button :label="t('common.apply')" icon="pi pi-check" :disabled="!eventsPick.length" @click="applyEvents" />
      </template>
    </Dialog>

    <Dialog :visible="!!secret" modal :header="t('webhooks.secretTitle')" :style="{ width: 'min(520px, 96vw)' }" @update:visible="(v: boolean) => !v && (secret = '')">
      <p class="small">{{ t('webhooks.secretOnce') }}</p>
      <div class="row">
        <InputText :model-value="secret" readonly fluid class="mono" :aria-label="t('webhooks.secretTitle')" />
        <Button icon="pi pi-copy" :aria-label="t('webhooks.copy')" @click="copySecret" />
      </div>
      <template #footer><Button :label="t('common.close')" @click="secret = ''" /></template>
    </Dialog>
  </section>
</template>

<style scoped>
.webhooks {
  margin-top: 16px;
}
.head {
  gap: 8px;
}
.url {
  overflow-wrap: anywhere;
}
.actions {
  gap: 0;
}
.form {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.form label {
  font-weight: 600;
  font-size: 13px;
  margin-top: 6px;
}
.toggle {
  gap: 8px;
  margin-top: 8px;
}
.toggle label {
  margin: 0;
}
.results {
  list-style: none;
  margin: 4px 0 0;
  padding: 0;
}
.results i {
  margin-right: 4px;
}
.error-text {
  color: var(--p-red-700, #b91c1c);
}
:global(.app-dark) .error-text {
  color: var(--p-red-300, #fca5a5);
}
</style>
