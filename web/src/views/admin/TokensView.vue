<script setup lang="ts">
import HelpTip from '@/guide/HelpTip.vue'
import { computed, onMounted, ref } from 'vue'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputText from 'primevue/inputtext'
import InputNumber from 'primevue/inputnumber'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime, relativeTime } from '@/lib/format'
import { grantableRoles, rolesOf, rolesWithin } from '@/lib/permissions'
import RolePicker from '@/components/RolePicker.vue'
import { useRolesStore } from '@/stores/roles'
import type { APIToken } from '@/lib/types'
import { useAuthStore } from '@/stores/auth'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumLabel } from '@/i18n/labels'

const auth = useAuthStore()
const notify = useNotify()
const { t } = useI18n()
const tokens = ref<APIToken[]>([])
const loading = ref(false)
const rolesStore = useRolesStore()
void rolesStore.ensure()
/** roles whose permissions the user holds (custom roles included); built-in ranks until loaded */
const roleNames = computed(() =>
  rolesStore.roles.length ? rolesWithin(rolesStore.roles, auth.me?.permissions ?? [], auth.isSuperAdmin) : grantableRoles(rolesOf(auth.me), auth.isSuperAdmin),
)

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
const form = ref({ name: '', roles: ['tester'], expires_days: 90 as number | null })
const metaStore = useMetaStore()
void metaStore.load()
// the server's token policy (System settings → API tokens)
const maxDays = computed(() => metaStore.meta.token_max_days || 3650)
const expiryRequired = computed(() => metaStore.meta.token_require_expiry === true || (metaStore.meta.token_max_days ?? 0) > 0)
const formError = ref('')
const busy = ref(false)
const created = ref<string | null>(null)
/** a new token gets the user's own roles (those it may have), else the weakest role offered */
function defaultRoles(): string[] {
  const mine = rolesOf(auth.me).filter((r) => roleNames.value.includes(r))
  if (mine.length) return mine
  return roleNames.value.includes('tester') ? ['tester'] : roleNames.value.slice(-1)
}
function openCreate() {
  form.value = { name: '', roles: defaultRoles(), expires_days: 90 }
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
      roles: form.value.roles,
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
      <Column field="role" :header="t('common.roles')" sortable style="width: 180px"
        ><template #body="{ data }"
          ><span class="row wrap" style="gap: 4px"><Tag v-for="r in rolesOf(data)" :key="r" :value="enumLabel('role', r)" severity="secondary" /></span></template
      ></Column>
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
      <div class="field">
        <label id="tok-roles-label" for="tok-roles">{{ t('common.roles') }}</label>
        <HelpTip id="token.role" :label="t('common.roles')" />
        <RolePicker v-model="form.roles" input-id="tok-roles" :options="roleNames" aria-labelledby="tok-roles-label" aria-describedby="tok-roles-hint" />
        <small id="tok-roles-hint" class="muted">{{ t('admin.tokenRoleHint') }}</small>
      </div>
      <div v-field class="field">
        <label>{{ t('admin.expiresAfter') }}</label>
        <HelpTip id="token.expires" :label="t('admin.expiresAfter')" />
        <InputNumber v-model="form.expires_days" :min="1" :max="maxDays" show-buttons fluid aria-describedby="tok-exp-hint" />
        <small id="tok-exp-hint" class="muted">{{ expiryRequired ? t('admin.tokenPolicy', { days: maxDays }) : t('admin.tokenNoExpiry') }}</small>
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim() || (expiryRequired && !form.expires_days)" />
      </div>
    </form>
  </Dialog>
</template>
