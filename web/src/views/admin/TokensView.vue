<script setup lang="ts">
import HelpTip from '@/guide/HelpTip.vue'
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
import { formatDate, formatDateTime, relativeTime } from '@/lib/format'
import { grantableRoles } from '@/lib/permissions'
import type { APIToken } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel, enumOptions } from '@/i18n/labels'

const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()
const tokens = ref<APIToken[]>([])
const loading = ref(false)
const roles = computed(() => enumOptions('role', grantableRoles(auth.me?.role ?? '', auth.isSuperAdmin)))

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
    notify.ok(t('admin.tokenCopied'))
  } catch {
    notify.info(t('errors.copyFailed'), t('admin.copyTokenManually'))
  }
}
function revoke(tok: APIToken) {
  const accept = async () => {
    try {
      await api.del(`/tokens/${tok.id}`)
      load()
    } catch (e) {
      notify.fail(e)
    }
  }
  notify.confirmDanger(t('admin.confirmRevoke', { name: tok.name }), accept, undefined, t('admin.revoke'))
}
const expired = (t: APIToken) => !!t.expires_at && new Date(t.expires_at).getTime() < Date.now()
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <i18n-t keypath="admin.tokensHint" tag="span" class="muted small"><template #header><code>Authorization: Bearer qtf_…</code></template></i18n-t>
      <span class="grow" />
      <Button :label="t('admin.newToken')" icon="pi pi-plus" @click="openCreate" />
    </div>
    <DataTable v-stack :value="tokens" data-key="id" :loading="loading" :aria-busy="loading" sort-field="created_at" :sort-order="-1">
      <Column field="name" :header="t('common.name')" sortable body-class="stack-main" />
      <Column field="prefix" :header="t('admin.token')" style="width: 160px"><template #body="{ data }"><span class="mono">{{ data.prefix }}…</span></template></Column>
      <Column field="role" :header="t('common.role')" sortable style="width: 140px"><template #body="{ data }"><Tag :value="enumLabel('role', data.role)" severity="secondary" /></template></Column>
      <Column field="created_at" :header="t('common.created')" sortable style="width: 130px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column field="expires_at" :header="t('admin.expires')" sortable style="width: 140px">
        <template #body="{ data }">
          <Tag v-if="expired(data)" :value="t('admin.expired')" severity="danger" />
          <span v-else-if="data.expires_at">{{ formatDate(data.expires_at) }}</span>
          <span v-else class="muted">{{ t('admin.neverExpires') }}</span>
        </template>
      </Column>
      <Column field="last_used" :header="t('admin.lastUsed')" sortable style="width: 170px">
        <template #body="{ data }"><span :title="formatDateTime(data.last_used)">{{ relativeTime(data.last_used) || t('common.never') }}</span></template>
      </Column>
      <Column style="width: 60px"><template #body="{ data }"><Button icon="pi pi-trash" text rounded severity="danger" :aria-label="t('admin.revokeNamed', { name: data.name })" v-tooltip="t('admin.revoke')" @click="revoke(data)" /></template></Column>
      <template #empty><div class="empty"><i class="pi pi-key" />{{ t('admin.noTokens') }}</div></template>
    </DataTable>
  </div>

  <Dialog v-model:visible="dialog" :header="created ? t('admin.tokenCreated') : t('admin.newApiToken')" modal style="width: min(520px, 95vw)" :closable="true">
    <div v-if="created" class="grid" style="gap: 12px">
      <Message severity="warn" :closable="false">{{ t('admin.copyNow') }}</Message>
      <div class="row nowrap">
        <InputText :model-value="created" readonly class="grow mono" :aria-label="t('admin.token')" @focus="(e: FocusEvent) => (e.target as HTMLInputElement).select()" />
        <Button icon="pi pi-copy" :aria-label="t('common.copy')" v-tooltip="t('common.copy')" @click="copy" />
      </div>
      <div class="row" style="justify-content: flex-end"><Button :label="t('admin.done')" @click="dialog = false" /></div>
    </div>
    <form v-else class="grid" style="gap: 12px" @submit.prevent="create">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div v-field class="field"><label>{{ t('common.name') }}</label><InputText v-model="form.name" :placeholder="t('admin.tokenNamePlaceholder')" autofocus required /></div>
      <div v-field class="field">
        <label>{{ t('common.role') }}</label>
        <HelpTip id="token.role" :label="t('common.role')" />
        <Select v-model="form.role" :options="roles" option-label="label" option-value="value" />
        <small class="muted">{{ t('admin.tokenRoleHint') }}</small>
      </div>
      <div v-field class="field">
        <label>{{ t('admin.expiresAfter') }}</label>
        <HelpTip id="token.expires" :label="t('admin.expiresAfter')" />
        <InputNumber v-model="form.expires_days" :min="1" :max="3650" show-buttons fluid />
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>
