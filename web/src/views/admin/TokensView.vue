<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Select from 'primevue/select'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime, humanize, options, relativeTime } from '@/lib/format'
import { grantableRoles } from '@/lib/permissions'
import type { APIToken } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'

const auth = useAuthStore()
const notify = useNotify()
const tokens = ref<APIToken[]>([])
const loading = ref(false)
const roles = computed(() => options(grantableRoles(auth.me?.role ?? '', auth.isSuperAdmin)))

async function load() {
  loading.value = true
  try {
    tokens.value = asItems(await api.get<{ items: APIToken[] }>('/tokens'))
  } catch (e) {
    notify.fail(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

const dialog = ref(false)
const form = ref({ name: '', role: 'tester', expires_days: 90 as number | null })
const formError = ref('')
const busy = ref(false)
const created = ref<string | null>(null)
function openCreate() {
  form.value = { name: '', role: roles.value.some((r) => r.value === 'tester') ? 'tester' : roles.value[roles.value.length - 1]?.value ?? 'viewer', expires_days: 90 }
  formError.value = ''
  created.value = null
  dialog.value = true
}
async function create() {
  busy.value = true
  formError.value = ''
  try {
    const res = await api.post<{ token: string; item: APIToken }>('/tokens', {
      name: form.value.name.trim(),
      role: form.value.role,
      ...(form.value.expires_days ? { expires_days: form.value.expires_days } : {}),
    })
    created.value = res.token
    load()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
async function copy() {
  if (!created.value) return
  try {
    await navigator.clipboard.writeText(created.value)
    notify.ok('Token copied')
  } catch {
    notify.info('Copy failed', 'Select the token and copy it manually.')
  }
}
function revoke(t: APIToken) {
  notify.confirmDanger(`Revoke token "${t.name}"? Clients using it lose access immediately.`, async () => {
    try {
      await api.del(`/tokens/${t.id}`)
      load()
    } catch (e) {
      notify.fail(e)
    }
  })
}
const expired = (t: APIToken) => !!t.expires_at && new Date(t.expires_at).getTime() < Date.now()
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <span class="muted small">Tokens authenticate CI pipelines and scripts with <code>Authorization: Bearer qtf_…</code>. They are bound to this tenant.</span>
      <span class="grow" />
      <Button label="New token" icon="pi pi-plus" @click="openCreate" />
    </div>
    <DataTable :value="tokens" data-key="id" :loading="loading" sort-field="created_at" :sort-order="-1">
      <Column field="name" header="Name" sortable />
      <Column field="prefix" header="Token" style="width: 160px"><template #body="{ data }"><span class="mono">{{ data.prefix }}…</span></template></Column>
      <Column field="role" header="Role" sortable style="width: 120px"><template #body="{ data }"><Tag :value="humanize(data.role)" severity="secondary" /></template></Column>
      <Column field="created_at" header="Created" sortable style="width: 130px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column field="expires_at" header="Expires" sortable style="width: 140px">
        <template #body="{ data }">
          <Tag v-if="expired(data)" value="Expired" severity="danger" />
          <span v-else-if="data.expires_at">{{ formatDate(data.expires_at) }}</span>
          <span v-else class="muted">Never</span>
        </template>
      </Column>
      <Column field="last_used" header="Last used" sortable style="width: 150px">
        <template #body="{ data }"><span :title="formatDateTime(data.last_used)">{{ relativeTime(data.last_used) || 'never' }}</span></template>
      </Column>
      <Column style="width: 60px"><template #body="{ data }"><Button icon="pi pi-trash" text rounded severity="danger" aria-label="Revoke" @click="revoke(data)" /></template></Column>
      <template #empty><div class="empty"><i class="pi pi-key" />No tokens.</div></template>
    </DataTable>
  </div>

  <Dialog v-model:visible="dialog" :header="created ? 'Token created' : 'New API token'" modal style="width: min(520px, 95vw)" :closable="true">
    <div v-if="created" class="grid" style="gap: 12px">
      <Message severity="warn" :closable="false">Copy the token now. It will not be shown again.</Message>
      <div class="row nowrap">
        <InputText :model-value="created" readonly class="grow mono" @focus="(e: FocusEvent) => (e.target as HTMLInputElement).select()" />
        <Button icon="pi pi-copy" aria-label="Copy" @click="copy" />
      </div>
      <div class="row" style="justify-content: flex-end"><Button label="Done" @click="dialog = false" /></div>
    </div>
    <form v-else class="grid" style="gap: 12px" @submit.prevent="create">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div class="field"><label>Name</label><InputText v-model="form.name" placeholder="e.g. GitLab CI" autofocus required /></div>
      <div class="field">
        <label>Role</label>
        <Select v-model="form.role" :options="roles" option-label="label" option-value="value" />
        <small class="muted">A token cannot exceed your own role.</small>
      </div>
      <div class="field">
        <label>Expires after (days, empty = never)</label>
        <InputNumber v-model="form.expires_days" :min="1" :max="3650" show-buttons fluid />
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button label="Cancel" text severity="secondary" @click="dialog = false" />
        <Button type="submit" label="Create" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>
