<script setup lang="ts">
// Administration → System settings (super admins): runtime-editable server
// configuration (GET/PUT /admin/settings). A side list picks the section;
// each section is saved on its own. Values show where they come from
// (default, configuration file, environment, changed here); settings set by
// an environment variable or locked by the operator are read-only.
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useI18n } from 'vue-i18n'
import HelpTip from '@/guide/HelpTip.vue'
import SystemSettingField from '@/components/SystemSettingField.vue'
import OidcProviderDialog from '@/components/OidcProviderDialog.vue'
import { api, errorMessage } from '@/lib/api'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { formatDateTime } from '@/lib/format'
import {
  changedValues,
  fieldId,
  isSection,
  SECTIONS,
  sectionForm,
  sourceSeverity,
  TESTABLE,
  testValues,
  type FormValue,
  type OidcProvider,
  type OidcProviderView,
  type SectionId,
  type SystemSettings,
  type TestResult,
} from '@/lib/systemSettings'

const { t } = useI18n()
const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const notify = useNotify()

const data = ref<SystemSettings | null>(null)
const error = ref('')
const forms = ref<Record<string, Record<string, FormValue>>>({})
const removeSecret = ref<Record<string, boolean>>({})
const saving = ref(false)
const testing = ref(false)
const results = ref<Partial<Record<SectionId, TestResult>>>({})
const announce = ref('')

/** The MFA page is built separately; its link only shows once the route exists. */
const hasMfa = computed(() => router.hasRoute('mfaPolicy'))
const sections = computed(() => SECTIONS.filter((s) => s !== 'mfa' || hasMfa.value))
const current = computed<SectionId>(() => {
  const q = route.query.section
  return isSection(q) && sections.value.includes(q) ? q : 'general'
})
const settingsOf = computed(() => (data.value?.settings ?? []).filter((s) => s.section === current.value))
const editable = computed(() => settingsOf.value.filter((s) => !s.locked))
const readonly = computed(() => settingsOf.value.filter((s) => s.locked))
const pending = computed(() => (data.value ? changedValues(data.value.settings, current.value, forms.value[current.value] ?? {}, removeSecret.value) : {}))
const dirty = computed(() => Object.keys(pending.value).length > 0)
const sectionLabel = (s: SectionId) => t(`sysSettings.section.${s}`)

function initForms(only?: string) {
  if (!data.value) return
  for (const s of data.value.sections) {
    if (only && s !== only) continue
    forms.value[s] = sectionForm(data.value.settings, s)
    for (const x of data.value.settings) if (x.section === s) delete removeSecret.value[x.key]
  }
}

async function load() {
  error.value = ''
  try {
    data.value = await api.get<SystemSettings>('/admin/settings')
    initForms()
  } catch (e) {
    error.value = errorMessage(e)
  }
}
onMounted(() => {
  if (auth.isSuperAdmin) void load()
})

function pick(s: SectionId) {
  void router.replace({ query: { ...route.query, section: s } })
}
watch(current, () => (announce.value = ''))

async function save() {
  if (!dirty.value) return
  saving.value = true
  const section = current.value
  try {
    data.value = await api.put<SystemSettings>('/admin/settings', { values: pending.value })
    initForms(section)
    notify.ok(t('sysSettings.saved'))
    announce.value = t('sysSettings.saved')
  } catch (e) {
    notify.fail(e)
    announce.value = errorMessage(e)
  } finally {
    saving.value = false
  }
}

function discard() {
  initForms(current.value)
}

function reset(key: string) {
  const name = t(`sysSettings.fields.${fieldId(key)}.label`)
  notify.confirmDanger(
    t('sysSettings.confirmReset', { name }),
    async () => {
      try {
        data.value = await api.del<SystemSettings>(`/admin/settings/${encodeURIComponent(key)}`)
        initForms(current.value)
        notify.ok(t('sysSettings.resetDone', { name }))
        announce.value = t('sysSettings.resetDone', { name })
      } catch (e) {
        notify.fail(e)
      }
    },
    t('sysSettings.reset'),
    t('sysSettings.reset'),
  )
}

async function runTest() {
  const kind = TESTABLE[current.value]
  if (!kind || !data.value) return
  const section = current.value
  testing.value = true
  results.value[section] = undefined
  announce.value = t('sysSettings.testing')
  try {
    const r = await api.post<TestResult>(`/admin/settings/test/${kind}`, { values: testValues(data.value.settings, section, forms.value[section] ?? {}) })
    results.value[section] = r
  } catch (e) {
    results.value[section] = { ok: false, error: errorMessage(e) }
  } finally {
    testing.value = false
    const r = results.value[section]
    announce.value = r ? `${r.ok ? t('sysSettings.testOk') : t('sysSettings.testFailed')}: ${r.ok ? (r.message ?? '') : (r.error ?? '')}` : ''
  }
}

async function exportConfig() {
  try {
    await api.download('/admin/settings/export', undefined, 'qtestframe-config.yaml')
    notify.ok(t('sysSettings.exported'))
  } catch (e) {
    notify.fail(e)
  }
}

// ---------------------------------------------------------------- OIDC

const dialog = ref(false)
const editingProvider = ref<OidcProviderView | null>(null)
const providerError = ref('')
const providerBusy = ref(false)
const oidc = computed(() => data.value?.oidc)

function openProvider(p: OidcProviderView | null) {
  editingProvider.value = p
  providerError.value = ''
  dialog.value = true
}

async function saveProvider(p: OidcProvider & { clear_secret?: boolean }) {
  providerBusy.value = true
  providerError.value = ''
  try {
    data.value = editingProvider.value
      ? await api.put<SystemSettings>(`/admin/settings/oidc/${encodeURIComponent(editingProvider.value.name)}`, p)
      : await api.post<SystemSettings>('/admin/settings/oidc', p)
    dialog.value = false
    notify.ok(t('sysSettings.oidc.saved'))
    announce.value = t('sysSettings.oidc.saved')
  } catch (e) {
    providerError.value = errorMessage(e)
  } finally {
    providerBusy.value = false
  }
}

async function toggleProvider(p: OidcProviderView, enabled: boolean) {
  try {
    data.value = await api.put<SystemSettings>(`/admin/settings/oidc/${encodeURIComponent(p.name)}`, { ...p, enabled })
    announce.value = `${p.display_name || p.name}: ${enabled ? t('sysSettings.oidc.status.on') : t('sysSettings.oidc.status.off')}`
  } catch (e) {
    notify.fail(e)
  }
}

function deleteProvider(p: OidcProviderView) {
  notify.confirmDanger(t('sysSettings.oidc.confirmDelete', { name: p.display_name || p.name }), async () => {
    try {
      data.value = await api.del<SystemSettings>(`/admin/settings/oidc/${encodeURIComponent(p.name)}`)
      notify.ok(t('sysSettings.oidc.deleted'))
      announce.value = t('sysSettings.oidc.deleted')
    } catch (e) {
      notify.fail(e)
    }
  })
}

function resetProviders() {
  notify.confirmDanger(
    t('sysSettings.oidc.confirmResetList'),
    async () => {
      try {
        data.value = await api.del<SystemSettings>('/admin/settings/auth.oidc')
        announce.value = t('sysSettings.oidc.resetList')
      } catch (e) {
        notify.fail(e)
      }
    },
    t('sysSettings.oidc.resetList'),
    t('sysSettings.reset'),
  )
}
</script>

<template>
  <div class="sys-page">
    <div class="card sys-intro">
      <div class="row" style="gap: 6px; align-items: center">
        <h2 class="plain-heading" style="margin: 0">{{ t('sysSettings.title') }}</h2>
        <HelpTip id="system.sources" :label="t('sysSettings.title')" />
        <span class="grow" />
        <Button :label="t('sysSettings.export')" icon="pi pi-download" severity="secondary" outlined size="small" :disabled="!data" v-tooltip.bottom="t('sysSettings.exportHint')" @click="exportConfig" />
      </div>
      <p class="small muted" style="margin: 6px 0 0">{{ t('sysSettings.intro') }}</p>
      <template v-if="data">
        <p class="small muted" style="margin: 4px 0 0">{{ data.config_file ? t('sysSettings.configFile', { file: data.config_file }) : t('sysSettings.noConfigFile') }}</p>
        <p v-if="data.locked_patterns.length" class="small muted" style="margin: 4px 0 0"><i class="pi pi-lock" aria-hidden="true" /> {{ t('sysSettings.lockedPatterns', { list: data.locked_patterns.join(', ') }) }}</p>
        <Message v-if="data.load_error" severity="warn" :closable="false" style="margin-top: 8px">{{ t('sysSettings.loadError', { error: data.load_error }) }}</Message>
        <Message v-if="data.restart_pending" severity="info" :closable="false" style="margin-top: 8px">{{ t('sysSettings.restartPending') }}</Message>
      </template>
    </div>

    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="!data" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else class="sys-layout">
      <nav class="sys-nav card" :aria-label="t('sysSettings.sections')">
        <ul>
          <li v-for="s in sections" :key="s">
            <button type="button" class="sys-nav-item" :class="{ active: s === current }" :aria-current="s === current ? 'page' : undefined" @click="pick(s)">
              {{ sectionLabel(s) }}
            </button>
          </li>
        </ul>
      </nav>

      <section class="card sys-section" :aria-labelledby="`h-sys-${current}`">
        <div class="row" style="gap: 6px; align-items: center">
          <h2 :id="`h-sys-${current}`" class="plain-heading" style="margin: 0">{{ sectionLabel(current) }}</h2>
          <HelpTip v-if="current === 'general'" id="system.general" :label="sectionLabel(current)" />
          <HelpTip v-else-if="current === 'security'" id="system.security" :label="sectionLabel(current)" />
          <HelpTip v-else-if="current === 'oidc'" id="system.oidc" :label="sectionLabel(current)" />
          <HelpTip v-else-if="current === 'logging'" id="system.logging" :label="sectionLabel(current)" />
          <HelpTip v-else-if="current === 'email'" id="system.email" :label="sectionLabel(current)" />
          <HelpTip v-else-if="current === 'runner'" id="system.runner" :label="sectionLabel(current)" />
        </div>
        <p class="small muted" style="margin: 4px 0 12px">{{ t(`sysSettings.sectionIntro.${current}`) }}</p>

        <!-- single sign-on -->
        <template v-if="current === 'oidc' && oidc">
          <div class="row" style="gap: 8px; flex-wrap: wrap; align-items: center">
            <Tag :value="t(`sysSettings.source.${oidc.source}`)" :severity="sourceSeverity(oidc.source)" />
            <span class="small muted">{{ oidc.stored ? t('sysSettings.oidc.listFromDb') : t('sysSettings.oidc.listFromFile') }}</span>
            <span class="grow" />
            <Button v-if="oidc.stored && !oidc.locked" :label="t('sysSettings.oidc.resetList')" icon="pi pi-undo" text size="small" @click="resetProviders" />
            <Button v-if="!oidc.locked" :label="t('sysSettings.oidc.add')" icon="pi pi-plus" size="small" @click="openProvider(null)" />
          </div>
          <p v-if="oidc.locked" class="small sys-lock"><i class="pi pi-lock" aria-hidden="true" /> {{ t(oidc.lock_reason === 'config' ? 'sysSettings.lock.config' : 'sysSettings.lock.readonly') }}</p>
          <p v-if="oidc.stored && oidc.updated_by" class="small muted">{{ t('sysSettings.changedBy', { who: oidc.updated_by, date: formatDateTime(oidc.updated_at) }) }}</p>
          <ul v-if="oidc.providers.length" class="provider-list">
            <li v-for="p in oidc.providers" :key="p.name" class="provider">
              <div class="provider-main">
                <div class="row" style="gap: 8px; flex-wrap: wrap; align-items: center">
                  <b>{{ p.display_name || p.name }}</b>
                  <span class="mono small muted">{{ p.name }}</span>
                  <Tag :value="p.enabled ? t('sysSettings.oidc.status.on') : t('sysSettings.oidc.status.off')" :severity="p.enabled ? 'success' : 'secondary'" />
                  <Tag :value="p.has_secret ? t('sysSettings.oidc.secretBadge.yes') : t('sysSettings.oidc.secretBadge.no')" :severity="p.has_secret ? 'info' : 'warn'" icon="pi pi-key" />
                  <span v-if="p.in_file" class="small muted">{{ t('sysSettings.oidc.fromFile') }}</span>
                </div>
                <div class="small mono provider-issuer">{{ p.issuer }}</div>
              </div>
              <div class="row nowrap provider-actions">
                <ToggleSwitch :model-value="p.enabled" :disabled="oidc.locked" :aria-label="t('sysSettings.oidc.enableNamed', { name: p.display_name || p.name })" @update:model-value="(v: boolean) => toggleProvider(p, v)" />
                <Button icon="pi pi-pencil" text rounded :disabled="oidc.locked" :aria-label="t('sysSettings.oidc.editNamed', { name: p.display_name || p.name })" v-tooltip="t('common.edit')" @click="openProvider(p)" />
                <Button icon="pi pi-trash" text rounded severity="danger" :disabled="oidc.locked" :aria-label="t('sysSettings.oidc.deleteNamed', { name: p.display_name || p.name })" v-tooltip="t('common.delete')" @click="deleteProvider(p)" />
              </div>
            </li>
          </ul>
          <div v-else class="empty"><i class="pi pi-sign-in" aria-hidden="true" />{{ t('sysSettings.oidc.none') }}</div>
        </template>

        <!-- retention: fixed values and the backup page -->
        <template v-else-if="current === 'retention'">
          <ul class="info-list">
            <li>{{ t('sysSettings.retention.trash', { days: data.trash_retention_days }) }}</li>
            <li>{{ t('sysSettings.retention.sessions') }}</li>
            <li>{{ t('sysSettings.retention.backups') }}</li>
          </ul>
          <RouterLink :to="{ name: 'backup' }" class="p-button p-button-outlined p-button-sm"><i class="pi pi-database" aria-hidden="true" />&nbsp;{{ t('sysSettings.retention.openBackup') }}</RouterLink>
        </template>

        <!-- multi-factor authentication: its own page -->
        <template v-else-if="current === 'mfa'">
          <p>{{ t('sysSettings.mfa.text') }}</p>
          <RouterLink v-if="hasMfa" :to="{ name: 'mfaPolicy' }" class="p-button p-button-sm"><i class="pi pi-shield" aria-hidden="true" />&nbsp;{{ t('sysSettings.mfa.open') }}</RouterLink>
        </template>

        <!-- the settings of a section -->
        <form v-else class="sys-form" @submit.prevent="save">
          <Message v-if="current === 'runner'" severity="warn" :closable="false">{{ t('sysSettings.runnerWarning') }}</Message>
          <p v-if="current === 'runner'" class="small"><i class="pi pi-refresh" aria-hidden="true" /> {{ t('sysSettings.restartNote') }}</p>
          <p v-if="current === 'email'" class="small muted">{{ t('sysSettings.emailTestTo', { email: auth.me?.user?.email ?? '' }) }}</p>
          <div class="sys-fields">
            <SystemSettingField
              v-for="s in [...editable, ...readonly]"
              :key="s.key"
              v-model="forms[current][s.key]"
              v-model:remove-secret="removeSecret[s.key]"
              :setting="s"
              :busy="saving"
              @reset="reset(s.key)"
            />
          </div>
          <div class="sys-test" role="status" aria-live="polite">
            <span v-if="testing" class="small muted">{{ t('sysSettings.testing') }}</span>
            <Message v-else-if="results[current]" :severity="results[current]!.ok ? 'success' : 'error'" :closable="false">
              <div>
                <b>{{ results[current]!.ok ? t('sysSettings.testOk') : t('sysSettings.testFailed') }}</b>
                {{ results[current]!.ok ? results[current]!.message : results[current]!.error }}
              </div>
            </Message>
          </div>
          <div v-if="editable.length" class="row sys-buttons">
            <Button v-if="TESTABLE[current] === 'syslog'" type="button" :label="t('sysSettings.syslogTest')" icon="pi pi-send" severity="secondary" outlined :loading="testing" @click="runTest" />
            <Button v-if="TESTABLE[current] === 'email'" type="button" :label="t('sysSettings.emailTest')" icon="pi pi-envelope" severity="secondary" outlined :loading="testing" @click="runTest" />
            <span class="grow" />
            <span v-if="dirty" class="small muted">{{ t('sysSettings.unsaved') }}</span>
            <Button type="button" :label="t('sysSettings.discard')" text severity="secondary" :disabled="!dirty || saving" @click="discard" />
            <Button type="submit" :label="t('sysSettings.save')" icon="pi pi-check" :loading="saving" :disabled="!dirty" />
          </div>
        </form>
      </section>
    </div>
    <div class="sr-only" role="status" aria-live="polite">{{ announce }}</div>
  </div>

  <OidcProviderDialog v-model:visible="dialog" :provider="editingProvider" :busy="providerBusy" :error="providerError" @save="saveProvider" />
</template>

<style scoped>
.sys-page {
  display: grid;
  gap: 16px;
}
.sys-layout {
  display: grid;
  grid-template-columns: minmax(180px, 230px) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
@media (max-width: 760px) {
  .sys-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}
.sys-nav {
  padding: 8px;
}
.sys-nav ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 2px;
}
@media (max-width: 760px) {
  .sys-nav ul {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }
}
.sys-nav-item {
  width: 100%;
  text-align: left;
  background: none;
  border: 0;
  border-radius: 6px;
  padding: 8px 10px;
  font: inherit;
  color: inherit;
  cursor: pointer;
  border-left: 3px solid transparent;
}
.sys-nav-item:hover {
  background: var(--qtf-hover, rgba(127, 127, 127, 0.12));
}
.sys-nav-item.active {
  font-weight: 700;
  border-left-color: var(--p-primary-color);
  background: var(--qtf-hover, rgba(127, 127, 127, 0.12));
}
.sys-nav-item:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 1px;
}
.sys-section {
  min-width: 0;
}
.sys-form {
  display: grid;
  gap: 8px;
}
.sys-buttons {
  gap: 8px;
  flex-wrap: wrap;
  align-items: center;
}
.sys-test:empty {
  display: none;
}
.provider-list {
  list-style: none;
  padding: 0;
  margin: 12px 0 0;
  display: grid;
  gap: 8px;
}
.provider {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  align-items: center;
  justify-content: space-between;
  border: 1px solid var(--qtf-border);
  border-radius: 8px;
  padding: 10px 12px;
}
.provider-main {
  min-width: 0;
  flex: 1 1 260px;
}
.provider-issuer {
  word-break: break-all;
  margin-top: 2px;
}
.provider-actions {
  gap: 4px;
  align-items: center;
}
.info-list {
  margin: 0 0 12px;
  padding-left: 18px;
  display: grid;
  gap: 6px;
}
</style>
