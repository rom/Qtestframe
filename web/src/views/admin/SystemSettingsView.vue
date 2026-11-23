<script setup lang="ts">
// Administration → System settings (super admins): the server
// configuration (GET/PUT /admin/settings). A grouped side navigation picks
// the section and a search finds a setting in any section; each section is
// saved on its own after a confirmation that lists the changes. Values show
// where they come from (default, configuration file, environment, changed
// here); settings set by an environment variable, locked by the operator or
// only settable in the file are read-only. Restart settings are listed in a
// banner until the server restarts. The values changed here can be exported
// to and imported from a JSON file (without secrets).
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Dialog from 'primevue/dialog'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import InputText from 'primevue/inputtext'
import Message from 'primevue/message'
import ProgressSpinner from 'primevue/progressspinner'
import Tag from 'primevue/tag'
import ToggleSwitch from 'primevue/toggleswitch'
import { useI18n } from 'vue-i18n'
import HelpTip from '@/guide/HelpTip.vue'
import SystemSettingField from '@/components/SystemSettingField.vue'
import OidcProviderDialog from '@/components/OidcProviderDialog.vue'
import { api, errorMessage } from '@/lib/api'
import { saveBlob } from '@/lib/download'
import { useAuthStore } from '@/stores/auth'
import { useNotify } from '@/composables/notify'
import { formatDateTime } from '@/lib/format'
import {
  changedValues,
  exportSettings,
  fieldId,
  formatRoleMapping,
  importPlan,
  INFO_SECTIONS,
  isSection,
  pendingRestart,
  searchSettings,
  SECTION_GROUPS,
  secretsRows,
  sectionForm,
  settingsDiff,
  sourceSeverity,
  TESTABLE,
  testValues,
  type FormValue,
  type ImportPlan,
  type OidcProvider,
  type OidcProviderView,
  type SamlProviderView,
  type SectionId,
  type SettingChange,
  type SystemSettings,
  type TestResult,
} from '@/lib/systemSettings'

const { t, te } = useI18n()
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
const outboundURL = ref('')

/** The MFA page is built separately; its link only shows once the route exists. */
const hasMfa = computed(() => router.hasRoute('mfaPolicy'))
const groups = computed(() =>
  (Object.keys(SECTION_GROUPS) as (keyof typeof SECTION_GROUPS)[]).map((g) => ({
    id: g,
    sections: (SECTION_GROUPS[g] as readonly SectionId[]).filter((s) => s !== 'mfa' || hasMfa.value),
  })),
)
const sections = computed(() => groups.value.flatMap((g) => g.sections))
const current = computed<SectionId>(() => {
  const q = route.query.section
  return isSection(q) && sections.value.includes(q) ? q : 'general'
})
const settingsOf = computed(() => (data.value?.settings ?? []).filter((s) => s.section === current.value))
const editable = computed(() => settingsOf.value.filter((s) => !s.locked))
const readonly = computed(() => settingsOf.value.filter((s) => s.locked))
const pending = computed(() => (data.value ? changedValues(data.value.settings, current.value, forms.value[current.value] ?? {}, removeSecret.value) : {}))
const dirty = computed(() => Object.keys(pending.value).length > 0)
const restartList = computed(() => (data.value ? pendingRestart(data.value.settings) : []))
const sectionLabel = (s: SectionId | string) => t(`sysSettings.section.${s}`)
const fieldLabel = (key: string) => {
  const id = `sysSettings.fields.${fieldId(key)}.label`
  return te(id) ? t(id) : key
}

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

// ---------------------------------------------------------------- search

const query = ref('')
const hits = computed(() => {
  if (!data.value || !query.value.trim()) return []
  const text = (key: string) => {
    const id = fieldId(key)
    return `${fieldLabel(key)} ${te(`sysSettings.fields.${id}.hint`) ? t(`sysSettings.fields.${id}.hint`) : ''}`
  }
  return searchSettings(data.value.settings, query.value, text).filter((h) => sections.value.includes(h.section as SectionId))
})
const sectionHits = computed(() => {
  const q = query.value.trim().toLowerCase()
  return q ? sections.value.filter((s) => sectionLabel(s).toLowerCase().includes(q) && !hits.value.some((h) => h.section === s)) : []
})
watch(hits, (h) => (announce.value = query.value.trim() ? t('sysSettings.search.count', { n: h.length }) : ''))

async function goTo(section: string, key?: string) {
  pick(section as SectionId)
  query.value = ''
  if (!key) return
  await nextTick()
  await nextTick()
  const el = document.querySelector<HTMLElement>(`.sys-field[data-key="${CSS.escape(key)}"]`)
  if (!el) return
  el.scrollIntoView({ block: 'center' })
  el.classList.add('flash')
  setTimeout(() => el.classList.remove('flash'), 1600)
  el.querySelector<HTMLElement>('input:not([type=hidden]), textarea, [role=combobox], [role=switch]')?.focus()
}

// ---------------------------------------------------------------- save with confirmation

const confirmOpen = ref(false)
const confirmChanges = ref<SettingChange[]>([])
const confirmValues = ref<Record<string, unknown>>({})
const confirmImport = ref<ImportPlan | null>(null)

function save() {
  if (!dirty.value || !data.value) return
  openConfirm(pending.value, null)
}

function openConfirm(values: Record<string, unknown>, plan: ImportPlan | null) {
  if (!data.value) return
  confirmValues.value = values
  confirmChanges.value = settingsDiff(data.value.settings, values)
  confirmImport.value = plan
  confirmOpen.value = true
}

async function applyConfirmed() {
  saving.value = true
  const imported = !!confirmImport.value
  const section = current.value
  try {
    data.value = await api.put<SystemSettings>('/admin/settings', { values: confirmValues.value })
    if (imported) initForms()
    else initForms(section)
    confirmOpen.value = false
    const msg = imported ? t('sysSettings.importDone', { n: confirmChanges.value.length }) : t('sysSettings.saved')
    notify.ok(msg)
    announce.value = msg
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
  const name = fieldLabel(key)
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

// ---------------------------------------------------------------- tests

async function runTest() {
  const kind = TESTABLE[current.value]
  if (!kind || !data.value) return
  const section = current.value
  testing.value = true
  results.value[section] = undefined
  announce.value = t('sysSettings.testing')
  try {
    const body: Record<string, unknown> = { values: testValues(data.value.settings, section, forms.value[section] ?? {}) }
    if (kind === 'outbound') body.url = outboundURL.value.trim()
    results.value[section] = await api.post<TestResult>(`/admin/settings/test/${kind}`, body)
  } catch (e) {
    results.value[section] = { ok: false, error: errorMessage(e) }
  } finally {
    testing.value = false
    const r = results.value[section]
    announce.value = r ? `${r.ok ? t('sysSettings.testOk') : t('sysSettings.testFailed')}: ${r.ok ? (r.message ?? '') : (r.error ?? '')}` : ''
  }
}

const samlResults = ref<Record<string, TestResult | undefined>>({})
const samlTesting = ref('')
async function testSaml(p: SamlProviderView) {
  samlTesting.value = p.name
  samlResults.value[p.name] = undefined
  try {
    samlResults.value[p.name] = await api.post<TestResult>('/admin/settings/test/saml', { name: p.name })
  } catch (e) {
    samlResults.value[p.name] = { ok: false, error: errorMessage(e) }
  } finally {
    samlTesting.value = ''
    const r = samlResults.value[p.name]
    announce.value = r ? `${r.ok ? t('sysSettings.testOk') : t('sysSettings.testFailed')}: ${r.ok ? (r.message ?? '') : (r.error ?? '')}` : ''
  }
}

// ---------------------------------------------------------------- export and import

async function exportConfig() {
  try {
    await api.download('/admin/settings/export', undefined, 'qtestframe-config.yaml')
    notify.ok(t('sysSettings.exported'))
  } catch (e) {
    notify.fail(e)
  }
}

function exportJSON() {
  if (!data.value) return
  const file = exportSettings(data.value.settings)
  saveBlob(new Blob([JSON.stringify(file, null, 2) + '\n'], { type: 'application/json' }), 'qtestframe-settings.json')
  notify.ok(t('sysSettings.exportedJson', { n: Object.keys(file.values).length }))
}

const importInput = ref<HTMLInputElement | null>(null)
async function importFile(e: Event) {
  const input = e.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ''
  if (!file || !data.value) return
  let plan: ImportPlan
  try {
    plan = importPlan(data.value.settings, await file.text())
  } catch {
    notify.fail(new Error(t('sysSettings.importBad')))
    return
  }
  if (!Object.keys(plan.values).length) {
    notify.ok(t('sysSettings.importNothing'))
    announce.value = t('sysSettings.importNothing')
    return
  }
  openConfirm(plan.values, plan)
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
      <div class="row" style="gap: 6px; align-items: center; flex-wrap: wrap">
        <h2 class="plain-heading" style="margin: 0">{{ t('sysSettings.title') }}</h2>
        <HelpTip id="system.sources" :label="t('sysSettings.title')" />
        <span class="grow" />
        <Button :label="t('sysSettings.exportJson')" icon="pi pi-file-export" severity="secondary" outlined size="small" :disabled="!data" v-tooltip.bottom="t('sysSettings.exportJsonHint')" @click="exportJSON" />
        <Button :label="t('sysSettings.importJson')" icon="pi pi-file-import" severity="secondary" outlined size="small" :disabled="!data" v-tooltip.bottom="t('sysSettings.importJsonHint')" @click="importInput?.click()" />
        <input ref="importInput" type="file" accept="application/json,.json" class="sr-only" tabindex="-1" aria-hidden="true" @change="importFile" />
        <Button :label="t('sysSettings.export')" icon="pi pi-download" severity="secondary" outlined size="small" :disabled="!data" v-tooltip.bottom="t('sysSettings.exportHint')" @click="exportConfig" />
      </div>
      <p class="small muted" style="margin: 6px 0 0">{{ t('sysSettings.intro') }}</p>
      <template v-if="data">
        <p class="small muted" style="margin: 4px 0 0">{{ data.config_file ? t('sysSettings.configFile', { file: data.config_file }) : t('sysSettings.noConfigFile') }}</p>
        <p v-if="data.locked_patterns.length" class="small muted" style="margin: 4px 0 0"><i class="pi pi-lock" aria-hidden="true" /> {{ t('sysSettings.lockedPatterns', { list: data.locked_patterns.join(', ') }) }}</p>
        <Message v-if="data.load_error" severity="warn" :closable="false" style="margin-top: 8px">{{ t('sysSettings.loadError', { error: data.load_error }) }}</Message>
        <Message v-if="restartList.length" severity="warn" :closable="false" style="margin-top: 8px" class="restart-banner">
          <div><i class="pi pi-refresh" aria-hidden="true" /> <b>{{ t('sysSettings.restartPending') }}</b></div>
          <ul class="restart-list">
            <li v-for="s in restartList" :key="s.key">
              <button type="button" class="link-button" @click="goTo(s.section, s.key)">{{ sectionLabel(s.section) }} › {{ fieldLabel(s.key) }}</button>
            </li>
          </ul>
        </Message>
      </template>
    </div>

    <Message v-if="error" severity="error" :closable="false">{{ error }}</Message>
    <div v-else-if="!data" class="empty"><ProgressSpinner style="width: 30px; height: 30px" :aria-label="t('common.loading')" /></div>
    <div v-else class="sys-layout">
      <div class="sys-side">
        <div class="card sys-search" role="search">
          <IconField>
            <InputIcon class="pi pi-search" />
            <InputText v-model="query" type="search" :placeholder="t('sysSettings.search.placeholder')" :aria-label="t('sysSettings.search.label')" fluid />
          </IconField>
        </div>
        <nav class="sys-nav card" :aria-label="t('sysSettings.sections')">
          <div v-for="g in groups" :key="g.id" class="sys-nav-group">
            <h3 :id="`sys-group-${g.id}`" class="sys-nav-heading">{{ t(`sysSettings.group.${g.id}`) }}</h3>
            <ul :aria-labelledby="`sys-group-${g.id}`">
              <li v-for="s in g.sections" :key="s">
                <button type="button" class="sys-nav-item" :class="{ active: s === current && !query.trim() }" :aria-current="s === current ? 'page' : undefined" @click="pick(s); query = ''">
                  {{ sectionLabel(s) }}
                </button>
              </li>
            </ul>
          </div>
        </nav>
      </div>

      <!-- search results across all sections -->
      <section v-if="query.trim()" class="card sys-section" aria-labelledby="h-sys-search">
        <h2 id="h-sys-search" class="plain-heading" style="margin: 0 0 8px">{{ t('sysSettings.search.title', { q: query.trim() }) }}</h2>
        <p v-if="!hits.length && !sectionHits.length" class="muted">{{ t('sysSettings.search.none') }}</p>
        <ul v-else class="hit-list">
          <li v-for="s in sectionHits" :key="`s-${s}`">
            <button type="button" class="link-button hit" @click="goTo(s)"><i class="pi pi-folder" aria-hidden="true" /> {{ sectionLabel(s) }}</button>
          </li>
          <li v-for="h in hits" :key="h.key">
            <button type="button" class="link-button hit" @click="goTo(h.section, h.key)">
              <span class="muted small">{{ sectionLabel(h.section) }} ›</span> {{ fieldLabel(h.key) }} <span class="mono small muted">{{ h.key }}</span>
            </button>
          </li>
        </ul>
      </section>

      <section v-else class="card sys-section" :aria-labelledby="`h-sys-${current}`">
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
          <h3 class="plain-heading sys-sub">{{ t('sysSettings.oidc.title') }}</h3>
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
                  <i :class="`pi pi-${p.icon || 'sign-in'}`" aria-hidden="true" />
                  <b>{{ p.display_name || p.name }}</b>
                  <span class="mono small muted">{{ p.name }}</span>
                  <Tag :value="p.enabled ? t('sysSettings.oidc.status.on') : t('sysSettings.oidc.status.off')" :severity="p.enabled ? 'success' : 'secondary'" />
                  <Tag :value="p.has_secret ? t('sysSettings.oidc.secretBadge.yes') : t('sysSettings.oidc.secretBadge.no')" :severity="p.has_secret ? 'info' : 'warn'" icon="pi pi-key" />
                  <Tag v-if="p.auto_provision" :value="t('sysSettings.oidc.jit')" severity="secondary" />
                  <span v-if="p.in_file" class="small muted">{{ t('sysSettings.oidc.fromFile') }}</span>
                </div>
                <div class="small mono provider-issuer">{{ p.issuer }}</div>
                <div v-if="p.allowed_domains?.length" class="small muted">{{ t('sysSettings.oidc.domainsList', { list: p.allowed_domains.join(', ') }) }}</div>
              </div>
              <div class="row nowrap provider-actions">
                <ToggleSwitch :model-value="p.enabled" :disabled="oidc.locked" :aria-label="t('sysSettings.oidc.enableNamed', { name: p.display_name || p.name })" @update:model-value="(v: boolean) => toggleProvider(p, v)" />
                <Button icon="pi pi-pencil" text rounded :disabled="oidc.locked" :aria-label="t('sysSettings.oidc.editNamed', { name: p.display_name || p.name })" v-tooltip="t('common.edit')" @click="openProvider(p)" />
                <Button icon="pi pi-trash" text rounded severity="danger" :disabled="oidc.locked" :aria-label="t('sysSettings.oidc.deleteNamed', { name: p.display_name || p.name })" v-tooltip="t('common.delete')" @click="deleteProvider(p)" />
              </div>
            </li>
          </ul>
          <div v-else class="empty"><i class="pi pi-sign-in" aria-hidden="true" />{{ t('sysSettings.oidc.none') }}</div>

          <h3 class="plain-heading sys-sub">{{ t('sysSettings.saml.title') }}</h3>
          <p class="small muted">{{ t('sysSettings.saml.intro') }}</p>
          <ul v-if="data.saml.length" class="provider-list">
            <li v-for="p in data.saml" :key="p.name" class="provider saml">
              <div class="provider-main">
                <div class="row" style="gap: 8px; flex-wrap: wrap; align-items: center">
                  <b>{{ p.display_name || p.name }}</b>
                  <span class="mono small muted">{{ p.name }}</span>
                  <Tag :value="t('sysSettings.source.file')" severity="info" />
                  <Tag v-if="p.auto_provision" :value="t('sysSettings.oidc.jit')" severity="secondary" />
                </div>
                <dl class="saml-list small">
                  <dt>{{ t('sysSettings.saml.entityId') }}</dt>
                  <dd class="mono">{{ p.entity_id }}</dd>
                  <dt>{{ t('sysSettings.saml.metadata') }}</dt>
                  <dd class="mono">{{ p.metadata_url || p.metadata_file }}</dd>
                  <dt>{{ t('sysSettings.saml.acs') }}</dt>
                  <dd class="mono">{{ p.acs_url }}</dd>
                  <dt>{{ t('sysSettings.saml.attributes') }}</dt>
                  <dd class="mono">{{ p.email_attr || 'email' }} · {{ p.name_attr || 'displayName' }} · {{ p.groups_attr || 'groups' }}</dd>
                  <template v-if="p.role_mapping && Object.keys(p.role_mapping).length">
                    <dt>{{ t('sysSettings.oidc.roleMapping') }}</dt>
                    <dd class="mono pre">{{ formatRoleMapping(p.role_mapping) }}</dd>
                  </template>
                  <template v-if="p.allowed_domains.length">
                    <dt>{{ t('sysSettings.oidc.allowedDomains') }}</dt>
                    <dd class="mono">{{ p.allowed_domains.join(', ') }}</dd>
                  </template>
                </dl>
                <div role="status" aria-live="polite">
                  <Message v-if="samlResults[p.name]" :severity="samlResults[p.name]!.ok ? 'success' : 'error'" :closable="false">
                    <b>{{ samlResults[p.name]!.ok ? t('sysSettings.testOk') : t('sysSettings.testFailed') }}</b>
                    {{ samlResults[p.name]!.ok ? samlResults[p.name]!.message : samlResults[p.name]!.error }}
                  </Message>
                </div>
              </div>
              <div class="row nowrap provider-actions">
                <Button :label="t('sysSettings.saml.test')" icon="pi pi-bolt" severity="secondary" outlined size="small" :loading="samlTesting === p.name" :aria-label="t('sysSettings.saml.testNamed', { name: p.display_name || p.name })" @click="testSaml(p)" />
              </div>
            </li>
          </ul>
          <div v-else class="empty"><i class="pi pi-id-card" aria-hidden="true" />{{ t('sysSettings.saml.none') }}</div>
        </template>

        <!-- authorisation: where roles and rights are managed -->
        <template v-else-if="current === 'authorisation'">
          <ul class="info-list">
            <li>{{ t('sysSettings.authz.organisations') }}</li>
            <li>{{ t('sysSettings.authz.projects') }}</li>
            <li>{{ t('sysSettings.authz.defaultRole') }}</li>
            <li>{{ t('sysSettings.authz.visibility') }}</li>
            <li>{{ t('sysSettings.authz.impersonation') }}</li>
          </ul>
          <div class="row" style="gap: 8px; flex-wrap: wrap">
            <RouterLink :to="{ name: 'rbac' }" class="p-button p-button-outlined p-button-sm"><i class="pi pi-table" aria-hidden="true" />&nbsp;{{ t('sysSettings.authz.openRoles') }}</RouterLink>
            <RouterLink :to="{ name: 'tenants' }" class="p-button p-button-outlined p-button-sm"><i class="pi pi-building" aria-hidden="true" />&nbsp;{{ t('sysSettings.authz.openTenants') }}</RouterLink>
            <RouterLink :to="{ name: 'users' }" class="p-button p-button-outlined p-button-sm"><i class="pi pi-id-card" aria-hidden="true" />&nbsp;{{ t('sysSettings.authz.openUsers') }}</RouterLink>
            <RouterLink :to="{ name: 'provisioning' }" class="p-button p-button-outlined p-button-sm"><i class="pi pi-sync" aria-hidden="true" />&nbsp;{{ t('sysSettings.authz.openProvisioning') }}</RouterLink>
          </div>
        </template>

        <!-- retention: fixed values and the backup page -->
        <template v-else-if="current === 'retention'">
          <ul class="info-list">
            <li>{{ t('sysSettings.retention.trash', { days: data.trash_retention_days }) }}</li>
            <li>{{ t('sysSettings.retention.sessions') }}</li>
            <li>{{ t('sysSettings.retention.backups') }}</li>
            <li>{{ t('sysSettings.retention.audit') }}</li>
          </ul>
          <RouterLink :to="{ name: 'backup' }" class="p-button p-button-outlined p-button-sm"><i class="pi pi-database" aria-hidden="true" />&nbsp;{{ t('sysSettings.retention.openBackup') }}</RouterLink>
        </template>

        <!-- multi-factor authentication: its own page -->
        <template v-else-if="current === 'mfa'">
          <p>{{ t('sysSettings.mfa.text') }}</p>
          <RouterLink v-if="hasMfa" :to="{ name: 'mfaPolicy' }" class="p-button p-button-sm"><i class="pi pi-shield" aria-hidden="true" />&nbsp;{{ t('sysSettings.mfa.open') }}</RouterLink>
        </template>

        <!-- the settings of a section -->
        <form v-else-if="!INFO_SECTIONS.includes(current)" class="sys-form" @submit.prevent="save">
          <Message v-if="current === 'runner'" severity="warn" :closable="false">{{ t('sysSettings.runnerWarning') }}</Message>
          <Message v-if="current === 'similarity'" severity="warn" :closable="false">{{ t('sysSettings.similarityWarning') }}</Message>
          <Message v-if="current === 'ai'" severity="warn" :closable="false">{{ t('sysSettings.aiWarning') }}</Message>
          <Message v-if="current === 'outbound'" severity="info" :closable="false">{{ t('sysSettings.outboundNote') }}</Message>
          <p v-if="current === 'runner' || current === 'live' || current === 'server'" class="small"><i class="pi pi-refresh" aria-hidden="true" /> {{ t(`sysSettings.restartNote.${current}`) }}</p>
          <p v-if="current === 'email'" class="small muted">{{ t('sysSettings.emailTestTo', { email: auth.me?.user?.email ?? '' }) }}</p>
          <p v-if="current === 'tokens'" class="small muted">{{ t('sysSettings.tokensNote') }}</p>
          <!-- encryption of stored secrets: read-only, set in the file -->
          <div v-if="current === 'server' && data.secrets" class="sys-secrets" role="group" aria-labelledby="h-sys-secrets">
            <h3 id="h-sys-secrets" class="plain-heading"><i class="pi pi-lock" aria-hidden="true" /> {{ t('sysSettings.secrets.title') }}</h3>
            <p class="small muted">{{ t('sysSettings.secrets.intro') }}</p>
            <dl class="sys-secrets-list">
              <dt>{{ t('sysSettings.secrets.providerLabel') }}</dt>
              <dd>{{ t(`sysSettings.secrets.provider.${data.secrets.provider}`) }}</dd>
              <template v-for="r in secretsRows(data.secrets)" :key="r.id">
                <dt>{{ t(`sysSettings.secrets.info.${r.id}`) }}</dt>
                <dd :class="{ mono: !r.message }">{{ r.message ? t(`sysSettings.secrets.source.${r.value}`) : r.value }}</dd>
              </template>
              <dt>{{ t('sysSettings.secrets.keyId') }}</dt>
              <dd class="mono">{{ data.secrets.key_id }}</dd>
              <dt>{{ t('sysSettings.secrets.activeKey') }}</dt>
              <dd class="mono">{{ data.secrets.active_key }} ({{ t('sysSettings.secrets.keys') }}: {{ data.secrets.keys }})</dd>
              <dt>{{ t('sysSettings.secrets.values') }}</dt>
              <dd>{{ t('sysSettings.secrets.valuesText', { encrypted: data.secrets.encrypted, plaintext: data.secrets.plaintext }) }}</dd>
            </dl>
            <Message v-if="data.secrets.plaintext > 0" severity="warn" :closable="false">{{ t('sysSettings.secrets.plaintextNote') }}</Message>
          </div>
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
          <div v-if="current === 'outbound'" v-field class="field sys-outbound-test">
            <label>{{ t('sysSettings.outboundTestUrl') }}</label>
            <InputText v-model="outboundURL" type="url" class="mono" placeholder="https://jira.example.com" aria-describedby="sys-outbound-hint" />
            <small id="sys-outbound-hint" class="muted">{{ t('sysSettings.outboundTestHint') }}</small>
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
            <Button v-if="TESTABLE[current] === 'embeddings'" type="button" :label="t('sysSettings.embeddingsTest')" icon="pi pi-bolt" severity="secondary" outlined :loading="testing" @click="runTest" />
            <Button v-if="TESTABLE[current] === 'ai'" type="button" :label="t('sysSettings.aiTest')" icon="pi pi-sparkles" severity="secondary" outlined :loading="testing" @click="runTest" />
            <Button v-if="TESTABLE[current] === 'outbound'" type="button" :label="t('sysSettings.outboundTest')" icon="pi pi-globe" severity="secondary" outlined :loading="testing" :disabled="!outboundURL.trim()" @click="runTest" />
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

  <OidcProviderDialog v-model:visible="dialog" :provider="editingProvider" :busy="providerBusy" :error="providerError" :icons="data?.oidc_icons" @save="saveProvider" />

  <Dialog v-model:visible="confirmOpen" :header="confirmImport ? t('sysSettings.confirm.importTitle') : t('sysSettings.confirm.title')" modal style="width: min(640px, 95vw)">
    <p class="small">{{ t('sysSettings.confirm.intro', { n: confirmChanges.length }) }}</p>
    <div class="table-scroll">
      <table class="diff-table">
        <caption class="sr-only">{{ t('sysSettings.confirm.caption') }}</caption>
        <thead>
          <tr>
            <th scope="col">{{ t('sysSettings.confirm.setting') }}</th>
            <th scope="col">{{ t('sysSettings.confirm.from') }}</th>
            <th scope="col">{{ t('sysSettings.confirm.to') }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="c in confirmChanges" :key="c.key">
            <th scope="row">
              {{ fieldLabel(c.key) }}
              <div class="mono small muted">{{ c.key }}</div>
              <Tag v-if="c.apply === 'restart'" :value="t('sysSettings.apply.restart')" severity="warn" icon="pi pi-refresh" class="sys-badge" />
            </th>
            <template v-if="c.secret">
              <td colspan="2"><i class="pi pi-key" aria-hidden="true" /> {{ confirmValues[c.key] === '' ? t('sysSettings.confirm.secretRemoved') : t('sysSettings.confirm.secretChanged') }}</td>
            </template>
            <template v-else>
              <td class="mono diff-old">{{ c.from || t('sysSettings.empty') }}</td>
              <td class="mono diff-new">{{ c.to || t('sysSettings.empty') }}</td>
            </template>
          </tr>
        </tbody>
      </table>
    </div>
    <template v-if="confirmImport">
      <p v-if="confirmImport.unchanged" class="small muted">{{ t('sysSettings.confirm.unchanged', { n: confirmImport.unchanged }) }}</p>
      <p v-if="confirmImport.locked.length" class="small warn-text">{{ t('sysSettings.confirm.skippedLocked', { list: confirmImport.locked.join(', ') }) }}</p>
      <p v-if="confirmImport.unknown.length" class="small warn-text">{{ t('sysSettings.confirm.skippedUnknown', { list: confirmImport.unknown.join(', ') }) }}</p>
      <p v-if="confirmImport.secrets.length" class="small warn-text">{{ t('sysSettings.confirm.skippedSecrets', { list: confirmImport.secrets.join(', ') }) }}</p>
    </template>
    <p class="small muted">{{ t('sysSettings.confirm.audit') }}</p>
    <template #footer>
      <Button :label="t('common.cancel')" text severity="secondary" @click="confirmOpen = false" />
      <Button :label="confirmImport ? t('sysSettings.confirm.import') : t('sysSettings.confirm.save')" icon="pi pi-check" :loading="saving" @click="applyConfirmed" />
    </template>
  </Dialog>
</template>

<style scoped>
.sys-page {
  display: grid;
  gap: 16px;
}
.sys-layout {
  display: grid;
  grid-template-columns: minmax(200px, 250px) minmax(0, 1fr);
  gap: 16px;
  align-items: start;
}
@media (max-width: 760px) {
  .sys-layout {
    grid-template-columns: minmax(0, 1fr);
  }
}
.sys-side {
  display: grid;
  gap: 8px;
}
.sys-search {
  padding: 8px;
}
.sys-nav {
  padding: 8px;
}
.sys-nav-heading {
  font-size: 11px;
  text-transform: uppercase;
  letter-spacing: 0.04em;
  margin: 8px 10px 4px;
  color: var(--p-text-muted-color);
}
.sys-nav-group:first-child .sys-nav-heading {
  margin-top: 2px;
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
.sys-nav-item:focus-visible,
.link-button:focus-visible {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 1px;
}
.sys-section {
  min-width: 0;
}
.sys-sub {
  font-size: 1rem;
  margin: 16px 0 6px;
}
.sys-sub:first-child {
  margin-top: 0;
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
.sys-outbound-test {
  max-width: 520px;
}
.link-button {
  background: none;
  border: 0;
  padding: 2px 0;
  font: inherit;
  color: var(--p-primary-color);
  cursor: pointer;
  text-align: left;
  text-decoration: underline;
}
.hit-list,
.restart-list {
  list-style: none;
  margin: 4px 0 0;
  padding: 0;
  display: grid;
  gap: 4px;
}
.hit {
  color: inherit;
  text-decoration: none;
}
.hit:hover {
  text-decoration: underline;
}
:deep(.sys-field.flash) {
  outline: 2px solid var(--p-primary-color);
  outline-offset: 4px;
  border-radius: 4px;
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
.provider.saml {
  align-items: flex-start;
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
.saml-list {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 2px 10px;
  margin: 6px 0;
}
.saml-list dt {
  font-weight: 600;
}
.saml-list dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.pre {
  white-space: pre-wrap;
}
.info-list {
  margin: 0 0 12px;
  padding-left: 18px;
  display: grid;
  gap: 6px;
}
.sys-secrets {
  border: 1px solid var(--p-content-border-color, #ddd);
  border-radius: 6px;
  padding: 10px 12px;
  margin-bottom: 12px;
}
.sys-secrets h3 {
  font-size: 1rem;
  margin: 0 0 4px;
}
.sys-secrets-list {
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: 4px 12px;
  margin: 8px 0;
}
.sys-secrets-list dt {
  font-weight: 600;
}
.sys-secrets-list dd {
  margin: 0;
  overflow-wrap: anywhere;
}
.table-scroll {
  overflow-x: auto;
}
.diff-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.diff-table th,
.diff-table td {
  text-align: left;
  vertical-align: top;
  padding: 6px 8px;
  border-bottom: 1px solid var(--qtf-border);
  overflow-wrap: anywhere;
}
.diff-table thead th {
  font-weight: 700;
}
.diff-table tbody th {
  font-weight: 600;
}
.diff-old {
  text-decoration: line-through;
  opacity: 0.85;
}
.sys-badge {
  font-size: 11px;
  margin-top: 2px;
}
</style>
