<script setup lang="ts">
// Administration → Organisations (super admins): create, rename, suspend
// and reactivate, delete (after a backup and a typed confirmation), the
// usage of each organisation and its policy (quotas, AI assistance, the
// allowed sign-in methods, e-mail domains of new single sign-on users).
// Super administrators cannot sign in as another user; they switch into an
// organisation under their own name.
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import InputNumber from 'primevue/inputnumber'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import MultiSelect from 'primevue/multiselect'
import Tag from 'primevue/tag'
import Textarea from 'primevue/textarea'
import ToggleSwitch from 'primevue/toggleswitch'
import { api, asItems, errorMessage } from '@/lib/api'
import { formatDate, formatDateTime } from '@/lib/format'
import { formatFileSize } from '@/lib/files'
import type { Providers } from '@/lib/types'
import { atLimit, blankPolicy, deleteConfirmed, methodOptions, parseDomains, quotaText, storageAtLimit, type AdminTenant, type TenantPolicy } from '@/lib/tenantAdmin'
import { overrideForm, overrideValue, RETENTION_OVERRIDE_KEYS, MAX_RETENTION_DAYS, type OverrideForm } from '@/lib/privacy'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { useI18n } from 'vue-i18n'

const auth = useAuthStore()
const notify = useNotify()
const router = useRouter()
const { t } = useI18n()
const tenants = ref<AdminTenant[]>([])
const loading = ref(false)
const announce = ref('')

async function load() {
  loading.value = true
  try {
    tenants.value = asItems(await api.get<{ items: AdminTenant[] }>('/admin/tenants'))
  } catch (e) {
    notify.fail(e)
  } finally {
    loading.value = false
  }
}
onMounted(load)

function replace(x: AdminTenant) {
  const i = tenants.value.findIndex((y) => y.id === x.id)
  if (i >= 0) tenants.value[i] = x
}

// ---------------------------------------------------------------- create and rename

const dialog = ref(false)
const editing = ref<AdminTenant | null>(null)
const form = ref({ slug: '', name: '' })
const formError = ref('')
function openCreate() {
  editing.value = null
  form.value = { slug: '', name: '' }
  formError.value = ''
  dialog.value = true
}
function openEdit(x: AdminTenant) {
  editing.value = x
  form.value = { slug: x.slug, name: x.name }
  formError.value = ''
  dialog.value = true
}
async function save() {
  formError.value = ''
  try {
    if (editing.value) await api.patch(`/tenants/${editing.value.id}`, { name: form.value.name })
    else await api.post('/tenants', form.value)
    dialog.value = false
    notify.ok(t('admin.tenant.saved'))
    void load()
  } catch (e) {
    formError.value = errorMessage(e)
  }
}

// ---------------------------------------------------------------- suspend, switch, audit

function suspend(x: AdminTenant) {
  notify.confirmDanger(
    t('admin.tenant.confirmSuspend', { name: x.name }),
    async () => {
      try {
        replace(await api.post<AdminTenant>(`/admin/tenants/${x.id}/suspend`))
        notify.ok(t('admin.tenant.suspended', { name: x.name }))
        announce.value = t('admin.tenant.suspended', { name: x.name })
      } catch (e) {
        notify.fail(e)
      }
    },
    t('admin.tenant.suspend'),
    t('admin.tenant.suspend'),
  )
}
async function reactivate(x: AdminTenant) {
  try {
    replace(await api.post<AdminTenant>(`/admin/tenants/${x.id}/reactivate`))
    notify.ok(t('admin.tenant.reactivated', { name: x.name }))
    announce.value = t('admin.tenant.reactivated', { name: x.name })
  } catch (e) {
    notify.fail(e)
  }
}
async function switchTo(x: AdminTenant) {
  try {
    await auth.switchTenant(x.id)
    notify.ok(t('admin.switchedTenant'), x.name)
  } catch (e) {
    notify.fail(e)
  }
}
function openAudit(x: AdminTenant) {
  void router.push({ name: 'audit', query: { tenant: x.id } })
}

// ---------------------------------------------------------------- details and policy

const detail = ref<AdminTenant | null>(null)
const policy = ref<TenantPolicy>(blankPolicy())
const domains = ref('')
const policyError = ref('')
const policyBusy = ref(false)
const methods = ref<{ value: string; label: string }[]>([])
const retention = ref<OverrideForm>(overrideForm(null))

async function openDetail(x: AdminTenant) {
  detail.value = x
  policy.value = { ...blankPolicy(), ...x.policy, allowed_providers: [...(x.policy.allowed_providers ?? [])] }
  domains.value = (x.policy.email_domains ?? []).join('\n')
  retention.value = overrideForm(x.policy.retention)
  policyError.value = ''
  try {
    const p = await api.get<Providers>('/auth/providers')
    methods.value = methodOptions({ oidc: p.oidc ?? [], saml: p.saml ?? [] }, t('admin.tenant.methodLocal'))
  } catch {
    methods.value = methodOptions({ oidc: [], saml: [] }, t('admin.tenant.methodLocal'))
  }
  // methods of the policy that are no longer offered stay selectable
  for (const m of policy.value.allowed_providers) if (!methods.value.some((o) => o.value === m)) methods.value.push({ value: m, label: m })
}

async function savePolicy() {
  if (!detail.value) return
  policyBusy.value = true
  policyError.value = ''
  try {
    const next = await api.put<AdminTenant>(`/admin/tenants/${detail.value.id}/policy`, { ...policy.value, email_domains: parseDomains(domains.value), retention: overrideValue(retention.value) })
    replace(next)
    detail.value = null
    notify.ok(t('admin.tenant.policySaved'))
    announce.value = t('admin.tenant.policySaved')
  } catch (e) {
    policyError.value = errorMessage(e)
  } finally {
    policyBusy.value = false
  }
}

// ---------------------------------------------------------------- delete

const deleting = ref<AdminTenant | null>(null)
const typed = ref('')
const deleteBusy = ref(false)
const backupBusy = ref(false)
const canDelete = computed(() => !!deleting.value && deleteConfirmed(deleting.value, typed.value) && deleting.value.id !== auth.tenant?.id)

function openDelete(x: AdminTenant) {
  deleting.value = x
  typed.value = ''
}
async function downloadBackup() {
  backupBusy.value = true
  try {
    await api.download('/admin/backup', undefined, 'qtestframe-backup.zip')
    notify.ok(t('backup.downloaded'))
  } catch (e) {
    notify.fail(e)
  } finally {
    backupBusy.value = false
  }
}
async function remove() {
  if (!deleting.value || !canDelete.value) return
  deleteBusy.value = true
  const x = deleting.value
  try {
    await api.del(`/tenants/${x.id}`)
    deleting.value = null
    notify.ok(t('admin.tenant.deleted', { name: x.name }))
    announce.value = t('admin.tenant.deleted', { name: x.name })
    void load()
  } catch (e) {
    notify.fail(e)
  } finally {
    deleteBusy.value = false
  }
}
</script>

<template>
  <div class="card">
    <div class="filterbar">
      <span class="muted small">{{ t('admin.tenantsHint') }}</span>
      <span class="grow" />
      <Button :label="t('admin.newTenant')" icon="pi pi-plus" @click="openCreate" />
    </div>
    <p class="small muted" style="margin: 0 0 8px"><i class="pi pi-info-circle" aria-hidden="true" /> {{ t('admin.tenant.noImpersonation') }}</p>
    <DataTable v-stack :value="tenants" data-key="id" :loading="loading" :aria-busy="loading" sort-field="slug" :sort-order="1">
      <Column field="slug" :header="t('admin.slug')" sortable><template #body="{ data }"><span class="mono">{{ data.slug }}</span></template></Column>
      <Column field="name" :header="t('common.name')" sortable body-class="stack-main">
        <template #body="{ data }">
          <span class="row" style="gap: 6px; flex-wrap: wrap">
            {{ data.name }}
            <Tag v-if="data.suspended_at" :value="t('admin.tenant.suspendedTag')" severity="danger" icon="pi pi-pause" />
            <Tag v-if="auth.tenant?.id === data.id" :value="t('admin.tenant.current')" severity="info" />
          </span>
        </template>
      </Column>
      <Column :header="t('admin.tenant.members')" style="width: 110px">
        <template #body="{ data }"><span :class="{ 'warn-text': atLimit(data.usage.members, data.policy.max_members) }">{{ quotaText(data.usage.members, data.policy.max_members) }}</span></template>
      </Column>
      <Column :header="t('admin.tenant.projects')" style="width: 110px">
        <template #body="{ data }"><span :class="{ 'warn-text': atLimit(data.usage.projects, data.policy.max_projects) }">{{ quotaText(data.usage.projects, data.policy.max_projects) }}</span></template>
      </Column>
      <Column :header="t('admin.tenant.storage')" style="width: 140px">
        <template #body="{ data }">
          <span :class="{ 'warn-text': storageAtLimit(data.usage.storage_bytes, data.policy.max_storage_mb) }">{{ formatFileSize(data.usage.storage_bytes) }}<template v-if="data.policy.max_storage_mb"> / {{ data.policy.max_storage_mb }} MB</template></span>
        </template>
      </Column>
      <Column field="created_at" :header="t('common.created')" sortable style="width: 120px"><template #body="{ data }">{{ formatDate(data.created_at) }}</template></Column>
      <Column style="width: 290px">
        <template #body="{ data }">
          <div class="row nowrap" style="gap: 0">
            <Button v-if="auth.tenant?.id !== data.id" icon="pi pi-sign-in" text rounded v-tooltip="t('admin.switchToTenant')" :aria-label="t('admin.switchToNamed', { name: data.name })" @click="switchTo(data)" />
            <Button icon="pi pi-sliders-h" text rounded severity="secondary" :aria-label="t('admin.tenant.detailsNamed', { name: data.name })" v-tooltip="t('admin.tenant.details')" @click="openDetail(data)" />
            <Button icon="pi pi-history" text rounded severity="secondary" :aria-label="t('admin.tenant.auditNamed', { name: data.name })" v-tooltip="t('admin.tenant.audit')" @click="openAudit(data)" />
            <Button icon="pi pi-pencil" text rounded severity="secondary" :aria-label="t('admin.editNamed', { name: data.name })" v-tooltip="t('admin.renameTenant')" @click="openEdit(data)" />
            <Button v-if="data.suspended_at" icon="pi pi-play" text rounded severity="success" :aria-label="t('admin.tenant.reactivateNamed', { name: data.name })" v-tooltip="t('admin.tenant.reactivate')" @click="reactivate(data)" />
            <Button v-else icon="pi pi-pause" text rounded severity="warn" :aria-label="t('admin.tenant.suspendNamed', { name: data.name })" v-tooltip="t('admin.tenant.suspend')" @click="suspend(data)" />
            <Button icon="pi pi-trash" text rounded severity="danger" :aria-label="t('admin.deleteNamed', { name: data.name })" v-tooltip="t('common.delete')" @click="openDelete(data)" />
          </div>
        </template>
      </Column>
    </DataTable>
    <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>
  </div>

  <Dialog v-model:visible="dialog" :header="editing ? t('admin.renameTenant') : t('admin.newTenant')" modal style="width: min(440px, 95vw)">
    <form class="grid" style="gap: 12px" @submit.prevent="save">
      <Message v-if="formError" severity="error" :closable="false">{{ formError }}</Message>
      <div v-field class="field"><label>{{ t('admin.slug') }}</label><InputText v-model="form.slug" :disabled="!!editing" placeholder="acme" required /></div>
      <div v-field class="field"><label>{{ t('common.name') }}</label><InputText v-model="form.name" :placeholder="t('admin.tenantNamePlaceholder')" required /></div>
      <div class="row" style="justify-content: flex-end">
        <Button :label="t('common.cancel')" text severity="secondary" @click="dialog = false" />
        <Button type="submit" :label="t('common.save')" icon="pi pi-check" :disabled="!form.slug || !form.name" />
      </div>
    </form>
  </Dialog>

  <Dialog :visible="!!detail" :header="detail ? t('admin.tenant.detailsNamed', { name: detail.name }) : ''" modal style="width: min(680px, 95vw)" @update:visible="(v: boolean) => !v && (detail = null)">
    <template v-if="detail">
      <h3 class="plain-heading sub">{{ t('admin.tenant.usage') }}</h3>
      <dl class="usage">
        <div><dt>{{ t('admin.tenant.members') }}</dt><dd>{{ quotaText(detail.usage.members, detail.policy.max_members) }}</dd></div>
        <div><dt>{{ t('admin.tenant.projects') }}</dt><dd>{{ quotaText(detail.usage.projects, detail.policy.max_projects) }}</dd></div>
        <div><dt>{{ t('admin.tenant.requirements') }}</dt><dd>{{ detail.usage.requirements }}</dd></div>
        <div><dt>{{ t('admin.tenant.tests') }}</dt><dd>{{ detail.usage.tests }}</dd></div>
        <div><dt>{{ t('admin.tenant.runs') }}</dt><dd>{{ detail.usage.runs }}</dd></div>
        <div><dt>{{ t('admin.tenant.files') }}</dt><dd>{{ detail.usage.files }} · {{ formatFileSize(detail.usage.storage_bytes) }}</dd></div>
        <div><dt>{{ t('admin.tenant.lastActivity') }}</dt><dd>{{ detail.usage.last_activity ? formatDateTime(detail.usage.last_activity) : '—' }}</dd></div>
        <div><dt>{{ t('admin.tenant.status') }}</dt><dd>{{ detail.suspended_at ? t('admin.tenant.suspendedSince', { date: formatDateTime(detail.suspended_at) }) : t('admin.tenant.active') }}</dd></div>
      </dl>
      <h3 class="plain-heading sub">{{ t('admin.tenant.orgSettings') }}</h3>
      <ul class="org-list small">
        <li><i :class="detail.ai_enabled ? 'pi pi-check' : 'pi pi-minus'" aria-hidden="true" /> {{ t('admin.tenant.aiState', { state: detail.ai_enabled ? t('sysSettings.yes') : t('sysSettings.no') }) }}</li>
        <li><i :class="detail.erecords_enabled ? 'pi pi-check' : 'pi pi-minus'" aria-hidden="true" /> {{ t('admin.tenant.erecordsState', { state: detail.erecords_enabled ? t('sysSettings.yes') : t('sysSettings.no') }) }}</li>
        <li><i :class="detail.mfa_required ? 'pi pi-check' : 'pi pi-minus'" aria-hidden="true" /> {{ t('admin.tenant.mfaState', { state: detail.mfa_required ? t('sysSettings.yes') : t('sysSettings.no') }) }}</li>
      </ul>
      <p class="small muted">{{ t('admin.tenant.orgSettingsHint') }}</p>

      <form class="grid" style="gap: 12px" @submit.prevent="savePolicy">
        <h3 class="plain-heading sub">{{ t('admin.tenant.policy') }}</h3>
        <p class="small muted" style="margin: 0">{{ t('admin.tenant.policyHint') }}</p>
        <Message v-if="policyError" severity="error" :closable="false">{{ policyError }}</Message>
        <div class="quota-grid">
          <div v-field class="field"><label>{{ t('admin.tenant.maxMembers') }}</label><InputNumber v-model="policy.max_members" :min="0" :max="1000000" :use-grouping="false" fluid /></div>
          <div v-field class="field"><label>{{ t('admin.tenant.maxProjects') }}</label><InputNumber v-model="policy.max_projects" :min="0" :max="1000000" :use-grouping="false" fluid /></div>
          <div v-field class="field"><label>{{ t('admin.tenant.maxStorage') }}</label><InputNumber v-model="policy.max_storage_mb" :min="0" :max="100000000" :use-grouping="false" fluid /></div>
        </div>
        <small class="muted">{{ t('admin.tenant.quotaHint') }}</small>
        <div class="row nowrap" style="gap: 10px">
          <ToggleSwitch v-model="policy.ai_disabled" input-id="tp-ai" aria-describedby="tp-ai-hint" />
          <div>
            <label for="tp-ai"><b>{{ t('admin.tenant.aiDisabled') }}</b></label>
            <p id="tp-ai-hint" class="small muted" style="margin: 2px 0 0">{{ t('admin.tenant.aiDisabledHint') }}</p>
          </div>
        </div>
        <div v-field class="field">
          <label>{{ t('admin.tenant.methods') }}</label>
          <MultiSelect v-model="policy.allowed_providers" :options="methods" option-label="label" option-value="value" display="chip" :placeholder="t('admin.tenant.methodsAny')" aria-describedby="tp-methods-hint" fluid />
          <small id="tp-methods-hint" class="muted">{{ t('admin.tenant.methodsHint') }}</small>
        </div>
        <div v-field class="field">
          <label>{{ t('admin.tenant.domains') }}</label>
          <Textarea v-model="domains" rows="2" auto-resize class="mono" spellcheck="false" placeholder="example.com" aria-describedby="tp-domains-hint" />
          <small id="tp-domains-hint" class="muted">{{ t('admin.tenant.domainsHint') }}</small>
        </div>
        <fieldset class="tp-retention">
          <legend>{{ t('privacy.override.title') }}</legend>
          <p class="small muted" style="margin: 0 0 8px">{{ t('privacy.override.intro') }}</p>
          <div class="tp-retention-grid">
            <div v-for="k in RETENTION_OVERRIDE_KEYS" :key="k" v-field class="field">
              <label>{{ t(`privacy.override.key.${k}`) }}</label>
              <InputNumber v-model="retention[k]" :min="0" :max="MAX_RETENTION_DAYS" :use-grouping="false" :placeholder="t('privacy.override.inherit')" fluid />
            </div>
          </div>
        </fieldset>
        <div v-field class="field">
          <label>{{ t('admin.tenant.note') }}</label>
          <Textarea v-model="policy.note" rows="2" auto-resize maxlength="2000" />
        </div>
        <div class="row" style="justify-content: flex-end">
          <Button :label="t('common.cancel')" text severity="secondary" @click="detail = null" />
          <Button type="submit" :label="t('admin.tenant.savePolicy')" icon="pi pi-check" :loading="policyBusy" />
        </div>
      </form>
    </template>
  </Dialog>

  <Dialog :visible="!!deleting" :header="deleting ? t('admin.tenant.deleteTitle', { name: deleting.name }) : ''" modal style="width: min(520px, 95vw)" @update:visible="(v: boolean) => !v && (deleting = null)">
    <form v-if="deleting" class="grid" style="gap: 12px" @submit.prevent="remove">
      <Message severity="error" :closable="false">{{ t('admin.tenant.deleteWarning', { projects: deleting.usage.projects, members: deleting.usage.members, requirements: deleting.usage.requirements, tests: deleting.usage.tests }) }}</Message>
      <div class="row" style="gap: 8px; flex-wrap: wrap; align-items: center">
        <span class="small">{{ t('admin.tenant.exportFirst') }}</span>
        <Button type="button" :label="t('admin.tenant.downloadBackup')" icon="pi pi-download" severity="secondary" outlined size="small" :loading="backupBusy" @click="downloadBackup" />
      </div>
      <Message v-if="deleting.id === auth.tenant?.id" severity="warn" :closable="false">{{ t('admin.tenant.deleteCurrent') }}</Message>
      <div v-field class="field">
        <label>{{ t('admin.tenant.typeSlug', { slug: deleting.slug }) }}</label>
        <InputText v-model="typed" class="mono" autocomplete="off" spellcheck="false" />
      </div>
      <div class="row" style="justify-content: flex-end">
        <Button type="button" :label="t('common.cancel')" text severity="secondary" @click="deleting = null" />
        <Button type="submit" :label="t('admin.tenant.deleteForGood')" icon="pi pi-trash" severity="danger" :loading="deleteBusy" :disabled="!canDelete" />
      </div>
    </form>
  </Dialog>
</template>

<style scoped>
.tp-retention {
  border: 1px solid var(--p-content-border-color);
  border-radius: 6px;
  padding: 8px 12px 12px;
  margin: 0;
}
.tp-retention legend {
  font-weight: 600;
  padding: 0 4px;
}
.tp-retention-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(150px, 1fr));
  gap: 8px;
}
.sub {
  font-size: 1rem;
  margin: 12px 0 6px;
}
.sub:first-child {
  margin-top: 0;
}
.usage {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 140px), 1fr));
  gap: 8px;
  margin: 0;
}
.usage div {
  border: 1px solid var(--qtf-border);
  border-radius: 6px;
  padding: 6px 8px;
}
.usage dt {
  font-size: 12px;
  color: var(--p-text-muted-color);
}
.usage dd {
  margin: 2px 0 0;
  font-weight: 600;
}
.org-list {
  list-style: none;
  padding: 0;
  margin: 0 0 4px;
  display: grid;
  gap: 4px;
}
.quota-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 160px), 1fr));
  gap: 12px;
}
</style>
