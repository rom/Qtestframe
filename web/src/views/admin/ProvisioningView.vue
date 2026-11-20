<script setup lang="ts">
// SCIM provisioning (GET /scim, tenant.manage): the base URL for the
// identity provider, SCIM tokens (shown once), the default role, the
// group → role mapping and the recent provisioning events.
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
import HelpLink from '@/guide/HelpLink.vue'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime, relativeTime } from '@/lib/format'
import { scimActor, scimEvent, scimEventSeverity, sortGroups, tokenExpired, withNone, type ScimGroup, type ScimOverview, type ScimToken } from '@/lib/scim'
import type { AuditEntry } from '@/lib/types'
import { useRolesStore } from '@/stores/roles'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'
import { enumOptions } from '@/i18n/labels'

const notify = useNotify()
const { t } = useI18n()
const meta = useMetaStore()
const rolesStore = useRolesStore()
void rolesStore.ensure()
const roleOptions = computed(() => enumOptions('role', rolesStore.roles.length ? rolesStore.roles.map((r) => r.name) : meta.meta.roles))
const defaultOptions = computed(() => withNone(roleOptions.value, t('provisioning.noAccess')))
const groupOptions = computed(() => withNone(roleOptions.value, t('provisioning.notMapped')))

const overview = ref<ScimOverview | null>(null)
const events = ref<AuditEntry[]>([])
const loading = ref(false)
const eventsLoading = ref(false)
const error = ref('')
const groups = computed(() => sortGroups(overview.value?.groups ?? []))

async function load() {
  loading.value = true
  error.value = ''
  try {
    overview.value = await api.get<ScimOverview>('/scim')
  } catch (e) {
    error.value = errorMessage(e)
  } finally {
    loading.value = false
  }
}
async function loadEvents() {
  eventsLoading.value = true
  try {
    events.value = asItems(await api.get<{ items: AuditEntry[] }>('/scim/events?limit=50'))
  } catch (e) {
    notify.fail(e)
  } finally {
    eventsLoading.value = false
  }
}
onMounted(() => {
  void load()
  void loadEvents()
})

async function copyText(text: string, done: string, manual: string) {
  try {
    await navigator.clipboard.writeText(text)
    notify.ok(done)
  } catch {
    notify.info(t('errors.copyFailed'), manual)
  }
}

// ---------- roles ----------
async function setDefaultRole(role: string) {
  try {
    await api.patch('/scim/settings', { default_role: role })
    notify.ok(t('provisioning.roleSaved'))
    await Promise.all([load(), loadEvents()])
  } catch (e) {
    notify.fail(e)
    await load()
  }
}
async function setGroupRole(g: ScimGroup, role: string) {
  try {
    await api.patch(`/scim/groups/${g.id}`, { role })
    notify.ok(t('provisioning.roleSaved'), g.display_name)
    await Promise.all([load(), loadEvents()])
  } catch (e) {
    notify.fail(e)
    await load()
  }
}

// ---------- tokens ----------
const dialog = ref(false)
const form = ref({ name: '', expires_days: 365 as number | null })
const formError = ref('')
const busy = ref(false)
const created = ref<string | null>(null)
function openCreate() {
  form.value = { name: '', expires_days: 365 }
  formError.value = ''
  created.value = null
  dialog.value = true
}
async function create() {
  busy.value = true
  formError.value = ''
  try {
    const res = await api.post<{ token: string; item: ScimToken }>('/scim/tokens', {
      name: form.value.name.trim(),
      ...(form.value.expires_days ? { expires_days: form.value.expires_days } : {}),
    })
    created.value = res.token
    void load()
    void loadEvents()
  } catch (e) {
    formError.value = errorMessage(e)
  } finally {
    busy.value = false
  }
}
function revoke(tok: ScimToken) {
  notify.confirmDanger(
    t('provisioning.confirmRevoke', { name: tok.name }),
    async () => {
      try {
        await api.del(`/scim/tokens/${tok.id}`)
        await Promise.all([load(), loadEvents()])
      } catch (e) {
        notify.fail(e)
      }
    },
    undefined,
    t('admin.revoke'),
  )
}
</script>

<template>
  <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
  <div class="grid provisioning" style="gap: 16px">
    <section class="card" aria-labelledby="h-scim-connection">
      <div class="row" style="justify-content: space-between">
        <h2 id="h-scim-connection" class="card-title">{{ t('provisioning.connection') }}</h2>
        <span class="row nowrap small muted">{{ t('provisioning.howTo') }} <HelpLink page="administration" :anchor="{ en: 'provisioning-with-scim', sv: 'provisionering-med-scim' }" /></span>
      </div>
      <p class="muted">{{ t('provisioning.intro') }}</p>
      <div v-field class="field">
        <label>{{ t('provisioning.baseUrl') }}</label>
        <div class="row nowrap">
          <InputText :model-value="overview?.base_url ?? ''" readonly class="grow mono" @focus="(e: FocusEvent) => (e.target as HTMLInputElement).select()" />
          <Button icon="pi pi-copy" :aria-label="t('provisioning.copyUrl')" v-tooltip="t('provisioning.copyUrl')" :disabled="!overview" @click="copyText(overview?.base_url ?? '', t('provisioning.urlCopied'), t('admin.copyTokenManually'))" />
        </div>
        <small class="muted">{{ t('provisioning.baseUrlHint') }}</small>
      </div>
      <p v-if="overview" class="small scim-stats">{{ t('provisioning.stats', { n: overview.users, active: overview.active_users }, overview.users) }}</p>
    </section>

    <section class="card" aria-labelledby="h-scim-tokens">
      <div class="filterbar">
        <h2 id="h-scim-tokens" class="card-title">{{ t('provisioning.tokens') }}</h2>
        <span class="grow" />
        <Button :label="t('provisioning.newToken')" icon="pi pi-plus" @click="openCreate" />
      </div>
      <p class="muted small">{{ t('provisioning.tokensHint') }}</p>
      <DataTable v-stack :value="overview?.tokens ?? []" data-key="id" :loading="loading" :aria-busy="loading" sort-field="created_at" :sort-order="-1">
        <Column field="name" :header="t('common.name')" sortable body-class="stack-main" />
        <Column field="prefix" :header="t('admin.token')" style="width: 170px"><template #body="{ data }"><span class="mono">{{ data.prefix }}…</span></template></Column>
        <Column field="created_by" :header="t('provisioning.createdBy')" sortable />
        <Column field="created_at" :header="t('common.created')" sortable style="width: 130px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
        <Column field="expires_at" :header="t('admin.expires')" sortable style="width: 140px">
          <template #body="{ data }">
            <Tag v-if="tokenExpired(data)" :value="t('admin.expired')" severity="danger" />
            <span v-else-if="data.expires_at">{{ formatDate(data.expires_at) }}</span>
            <span v-else class="muted">{{ t('admin.neverExpires') }}</span>
          </template>
        </Column>
        <Column field="last_used" :header="t('admin.lastUsed')" sortable style="width: 170px">
          <template #body="{ data }"><span :title="formatDateTime(data.last_used)">{{ relativeTime(data.last_used) || t('common.never') }}</span></template>
        </Column>
        <Column style="width: 60px"><template #body="{ data }"><Button icon="pi pi-trash" text rounded severity="danger" :aria-label="t('admin.revokeNamed', { name: data.name })" v-tooltip="t('admin.revoke')" @click="revoke(data)" /></template></Column>
        <template #empty><div class="empty"><i class="pi pi-key" />{{ t('provisioning.noTokens') }}</div></template>
      </DataTable>
    </section>

    <section class="card" aria-labelledby="h-scim-roles">
      <h2 id="h-scim-roles" class="card-title">{{ t('provisioning.roles') }}</h2>
      <p class="muted small">{{ t('provisioning.rolesHint') }}</p>
      <div v-field class="field" style="max-width: 320px">
        <label>{{ t('provisioning.defaultRole') }}</label>
        <Select :model-value="overview?.default_role ?? 'viewer'" :options="defaultOptions" option-label="label" option-value="value" :disabled="!overview" @update:model-value="setDefaultRole" />
      </div>
      <h3 class="small-title">{{ t('provisioning.groups') }}</h3>
      <DataTable v-stack :value="groups" data-key="id" :loading="loading" :aria-busy="loading">
        <Column field="display_name" :header="t('provisioning.group')" body-class="stack-main" />
        <Column field="members" :header="t('provisioning.members')" style="width: 120px" />
        <Column field="role" :header="t('common.role')" style="width: 240px">
          <template #body="{ data }">
            <Select :model-value="data.role" :options="groupOptions" option-label="label" option-value="value" size="small" class="w-full" :aria-label="t('provisioning.groupRoleOf', { group: data.display_name })" @update:model-value="(r: string) => setGroupRole(data, r)" />
          </template>
        </Column>
        <template #empty><div class="empty"><i class="pi pi-users" />{{ t('provisioning.noGroups') }}</div></template>
      </DataTable>
    </section>

    <section class="card" aria-labelledby="h-scim-events">
      <div class="filterbar">
        <h2 id="h-scim-events" class="card-title">{{ t('provisioning.events.title') }}</h2>
        <span class="grow" />
        <Button icon="pi pi-refresh" text rounded severity="secondary" :aria-label="t('common.refresh')" v-tooltip="t('common.refresh')" :loading="eventsLoading" @click="loadEvents" />
      </div>
      <DataTable v-stack :value="events" data-key="id" :loading="eventsLoading" :aria-busy="eventsLoading" paginator :rows="10">
        <Column field="created_at" :header="t('common.time')" style="width: 170px"><template #body="{ data }"><span :title="formatDateTime(data.created_at)">{{ relativeTime(data.created_at) }}</span></template></Column>
        <Column field="action" :header="t('admin.action')" style="width: 200px">
          <template #body="{ data }"><Tag :value="t(`provisioning.events.${scimEvent(data.action)}`)" :severity="scimEventSeverity(data.action)" /></template>
        </Column>
        <Column field="details" :header="t('admin.detailsCol')" body-class="stack-main" />
        <Column field="user_email" :header="t('provisioning.events.actor')">
          <template #body="{ data }">
            <span v-if="scimActor(data.user_email).scim" class="row nowrap" style="gap: 6px"><i class="pi pi-sync" aria-hidden="true" />{{ scimActor(data.user_email).name }}</span>
            <span v-else>{{ data.user_email }}</span>
          </template>
        </Column>
        <template #empty><div class="empty"><i class="pi pi-history" />{{ t('provisioning.events.noEvents') }}</div></template>
      </DataTable>
    </section>
  </div>

  <Dialog v-model:visible="dialog" :header="created ? t('provisioning.tokenCreated') : t('provisioning.newToken')" modal style="width: min(520px, 95vw)">
    <div v-if="created" class="grid" style="gap: 12px">
      <Message severity="warn" :closable="false">{{ t('admin.copyNow') }}</Message>
      <div class="row nowrap">
        <InputText :model-value="created" readonly class="grow mono" :aria-label="t('admin.token')" @focus="(e: FocusEvent) => (e.target as HTMLInputElement).select()" />
        <Button icon="pi pi-copy" :aria-label="t('common.copy')" v-tooltip="t('common.copy')" @click="copyText(created ?? '', t('admin.tokenCopied'), t('admin.copyTokenManually'))" />
      </div>
      <div class="row" style="justify-content: flex-end"><Button :label="t('admin.done')" @click="dialog = false" /></div>
    </div>
    <form v-else class="grid" style="gap: 12px" @submit.prevent="create">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div v-field class="field"><label>{{ t('common.name') }}</label><InputText v-model="form.name" :placeholder="t('provisioning.tokenNamePlaceholder')" autofocus required maxlength="100" /></div>
      <div v-field class="field">
        <label>{{ t('admin.expiresAfter') }}</label>
        <InputNumber v-model="form.expires_days" :min="1" :max="3650" show-buttons fluid />
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.create')" icon="pi pi-check" :loading="busy" :disabled="!form.name.trim()" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.card-title {
  font-size: 1.05rem;
  margin: 0 0 8px;
}
.small-title {
  font-size: 0.95rem;
  margin: 16px 0 8px;
}
.scim-stats {
  margin: 12px 0 0;
}
</style>
