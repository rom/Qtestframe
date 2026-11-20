<script setup lang="ts">
// Outgoing webhooks of a project: URL, events, signing secret (shown once),
// test delivery and the outcome of the latest delivery.
import { computed, onMounted, ref } from 'vue'
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
    <DataTable :value="hooks" data-key="id" size="small">
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
.error-text {
  color: var(--p-red-600, #c62828);
}
</style>
