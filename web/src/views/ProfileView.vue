<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import Password from 'primevue/password'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Tag from 'primevue/tag'
import InputText from 'primevue/inputtext'
import Select from 'primevue/select'
import ToggleSwitch from 'primevue/toggleswitch'
import DataTable from 'primevue/datatable'
import Column from 'primevue/column'
import Avatar from 'primevue/avatar'
import { useConfirm } from 'primevue/useconfirm'
import { api, asItems, errorMessage } from '@/lib/api'
import { useAuthStore } from '@/stores/auth'
import { usePrefsStore } from '@/stores/prefs'
import { useProjectStore } from '@/stores/project'
import { useNotify } from '@/composables/notify'
import { formatDate, formatDateTime, formatDateTimePref, formatFullDateTime, getFormatLocale, initials, relativeTime } from '@/lib/format'
import { browserTimeZone, normalizePrefs, preferencesPatch, timeZones } from '@/lib/prefs'
import { deviceIcon, parseUserAgent, type UserAgentInfo } from '@/lib/useragent'
import { P } from '@/lib/permissions'
import type { Me, Preferences, Project, RbacInfo, SessionInfo } from '@/lib/types'
import { useI18n } from 'vue-i18n'
import { chooseLocal, LANGUAGES, LANGUAGE_NAMES, type Lang } from '@/i18n'
import { normalizeLang } from '@/i18n/select'
import { enumLabel } from '@/i18n/labels'

const auth = useAuthStore()
const prefsStore = usePrefsStore()
const proj = useProjectStore()
const notify = useNotify()
const confirm = useConfirm()
const router = useRouter()
const { t } = useI18n()

const isLocal = computed(() => !auth.user?.auth_provider || auth.user.auth_provider === 'local')

// ---------- account ----------
const displayName = ref(auth.user?.display_name ?? '')
watch(
  () => auth.user?.display_name,
  (v) => (displayName.value = v ?? ''),
)
const savingName = ref(false)
const nameError = ref('')
async function saveName() {
  savingName.value = true
  nameError.value = ''
  try {
    const me = await api.patch<Me>('/auth/me', { display_name: displayName.value.trim() })
    if (me?.user) auth.set(me)
    notify.ok(t('profile.nameSaved'))
  } catch (e) {
    nameError.value = errorMessage(e)
  } finally {
    savingName.value = false
  }
}

// ---------- preferences ----------
function toForm(p: Preferences) {
  return {
    theme: p.theme ?? '',
    start_page: p.start_page ?? '',
    default_project_id: p.default_project_id ?? '',
    default_tenant_id: p.default_tenant_id ?? '',
    page_size: p.page_size ?? 0,
    date_format: p.date_format ?? '',
    timezone: p.timezone ?? '',
    compact_tables: !!p.compact_tables,
    hide_analysis: !!p.hide_analysis,
    create_linked_test: typeof p.create_linked_test === 'boolean' ? p.create_linked_test : null,
  }
}
const pform = ref(toForm(prefsStore.prefs))
// language is kept apart from the shared preference helpers: '' = browser default
const storedLang = computed(() => normalizeLang(auth.user?.preferences?.language) ?? '')
const langForm = ref<string>(storedLang.value)
const stored = computed(() => JSON.stringify(preferencesPatch(toForm(prefsStore.prefs))))
const prefsDirty = computed(() => JSON.stringify(preferencesPatch(pform.value)) !== stored.value || langForm.value !== storedLang.value)
watch(
  () => prefsStore.prefs,
  (p) => {
    if (!prefsDirty.value) pform.value = toForm(p)
  },
)
watch(storedLang, (l, old) => {
  if (langForm.value === old) langForm.value = l
})
const languageOptions = computed(() => [{ label: t('language.browserDefault'), value: '' }, ...LANGUAGES.map((l) => ({ label: LANGUAGE_NAMES[l], value: l as string }))])
const savingPrefs = ref(false)
const prefsError = ref('')
async function savePrefs() {
  savingPrefs.value = true
  prefsError.value = ''
  try {
    const lang = (normalizeLang(langForm.value) as Lang | null) ?? null
    await prefsStore.save({ ...preferencesPatch(pform.value), language: lang })
    // keep this device in line (login page, before /auth/me answers)
    chooseLocal(lang)
    pform.value = toForm(normalizePrefs(auth.user?.preferences))
    langForm.value = storedLang.value
    notify.ok(t('profile.preferencesSaved'))
  } catch (e) {
    prefsError.value = errorMessage(e)
  } finally {
    savingPrefs.value = false
  }
}
function resetPrefs() {
  pform.value = toForm(prefsStore.prefs)
  langForm.value = storedLang.value
}

const themeOptions = computed(() => [
  { label: t('profile.themeNotSet'), value: '' },
  { label: t('profile.themeLight'), value: 'light' },
  { label: t('profile.themeDark'), value: 'dark' },
  { label: t('profile.themeSystem'), value: 'system' },
])
const startOptions = computed(() => [
  { label: t('profile.startDefault'), value: '' },
  { label: t('profile.startProjects'), value: 'projects' },
  { label: t('profile.startLastProject'), value: 'last_project' },
  { label: t('profile.startDefaultProject'), value: 'default_project' },
  { label: t('profile.startSearch'), value: 'search' },
])
const pageSizeOptions = computed(() => [{ label: t('profile.rowsDefault', { n: 25 }), value: 0 }, ...[10, 25, 50, 100].map((n) => ({ label: String(n), value: n }))])
const sampleDate = new Date(Date.now() - 5 * 60 * 1000)
const dateOptions = computed(() =>
  [
    { label: t('profile.dateDefault'), value: '' },
    { label: t('profile.dateLocale'), value: 'locale' },
    { label: t('profile.dateIso'), value: 'iso' },
    { label: t('profile.dateRelative'), value: 'relative' },
  ].map((o) => ({ ...o, example: formatDateTimePref(sampleDate, { date_format: o.value, timezone: pform.value.timezone }, new Date(), getFormatLocale()) })),
)
const browserTz = browserTimeZone()
const tzOptions = computed(() => [
  { label: browserTz ? t('profile.browserZoneWith', { zone: browserTz }) : t('profile.browserZone'), value: '' },
  ...timeZones(pform.value.timezone).map((z) => ({ label: z.replace(/_/g, ' '), value: z })),
])
const linkedTestOptions = computed(() => [
  { label: t('profile.serverDefault'), value: null as boolean | null },
  { label: t('profile.linkedYes'), value: true as boolean | null },
  { label: t('profile.linkedNo'), value: false as boolean | null },
])
const tenantOptions = computed(() => [
  { label: t('profile.defaultTenantNone'), value: '' },
  ...(auth.me?.memberships ?? []).map((m) => ({ label: m.tenant_name || m.tenant_slug || m.tenant_id, value: m.tenant_id })),
])
const projects = ref<Project[]>([])
const projectsError = ref('')
const projectOptions = computed(() => {
  const opts = [{ label: t('common.none'), value: '' }, ...projects.value.map((p) => ({ label: `${p.key} · ${p.name}`, value: p.id }))]
  if (pform.value.default_project_id && !projects.value.some((p) => p.id === pform.value.default_project_id)) {
    opts.push({ label: t('profile.projectElsewhere'), value: pform.value.default_project_id })
  }
  return opts
})
async function loadProjects() {
  if (!auth.tenant) return
  try {
    projects.value = asItems(await api.get<{ items: Project[] }>('/projects'))
  } catch (e) {
    projectsError.value = errorMessage(e)
  }
}

// ---------- memberships ----------
const rbac = ref<RbacInfo | null>(null)
async function loadRbac() {
  try {
    rbac.value = await api.get<RbacInfo>('/rbac')
  } catch {
    rbac.value = null
  }
}
function permsFor(tenantId: string, role: string): string[] {
  if (tenantId === auth.tenant?.id && auth.me?.permissions?.length) return auth.me.permissions
  return rbac.value?.matrix?.[role] ?? []
}

// ---------- password ----------
const current = ref('')
const next = ref('')
const repeat = ref('')
const pwError = ref('')
const pwBusy = ref(false)
async function changePassword() {
  pwError.value = ''
  if (next.value !== repeat.value) {
    pwError.value = t('profile.passwordsDiffer')
    return
  }
  pwBusy.value = true
  try {
    await api.post('/auth/password', { current_password: current.value, new_password: next.value })
    current.value = next.value = repeat.value = ''
    notify.ok(t('profile.passwordChanged'))
  } catch (e) {
    pwError.value = errorMessage(e)
  } finally {
    pwBusy.value = false
  }
}

// ---------- sessions ----------
const sessions = ref<SessionInfo[]>([])
const sessionsLoading = ref(false)
const sessionsError = ref('')
async function loadSessions() {
  sessionsLoading.value = true
  sessionsError.value = ''
  try {
    sessions.value = asItems(await api.get<SessionInfo[]>('/auth/sessions'))
  } catch (e) {
    sessionsError.value = errorMessage(e)
  } finally {
    sessionsLoading.value = false
  }
}
const sessionRows = computed(() => sessions.value.map((s) => ({ ...s, ua: parseUserAgent(s.user_agent) })))
/** Device label in the active language ("Firefox 128 on Windows" / "Firefox 128 på Windows"). */
function uaLabel(ua: UserAgentInfo): string {
  if (ua.device === 'unknown' && !ua.browser && !ua.os && ua.label === 'Unknown device') return t('profile.unknownDevice')
  if (ua.browser && ua.os && ua.device !== 'cli' && ua.device !== 'bot') return t('profile.uaOn', { browser: `${ua.browser}${ua.version ? ' ' + ua.version : ''}`, os: ua.os })
  return ua.label
}
const others = computed(() => sessions.value.filter((s) => !s.current).length)
function tenantName(id: string) {
  const m = auth.me?.memberships?.find((x) => x.tenant_id === id)
  return m ? m.tenant_name || m.tenant_slug || '' : ''
}
function revoke(s: SessionInfo) {
  confirm.require({
    header: s.current ? t('profile.signOutDeviceQ') : t('profile.revokeSessionQ'),
    message: s.current ? t('profile.currentSessionMsg') : t('profile.otherSessionMsg'),
    icon: 'pi pi-sign-out',
    rejectProps: { label: t('common.cancel'), severity: 'secondary', outlined: true },
    acceptProps: { label: s.current ? t('profile.signOut') : t('profile.revoke'), severity: 'danger' },
    accept: async () => {
      try {
        await api.del(`/auth/sessions/${s.id}`)
        if (s.current) {
          auth.clear()
          proj.reset()
          router.push({ name: 'login' })
          return
        }
        sessions.value = sessions.value.filter((x) => x.id !== s.id)
        notify.ok(t('profile.sessionRevoked'))
      } catch (e) {
        notify.fail(e, t('profile.revokeFailed'))
      }
    },
  })
}
const revokingOthers = ref(false)
function revokeOthers() {
  confirm.require({
    header: t('profile.signOutOthersQ'),
    message: t('profile.signOutOthersMsg'),
    icon: 'pi pi-sign-out',
    rejectProps: { label: t('common.cancel'), severity: 'secondary', outlined: true },
    acceptProps: { label: t('profile.signOutOthersBtn'), severity: 'danger' },
    accept: async () => {
      revokingOthers.value = true
      try {
        const res = await api.post<{ revoked: number }>('/auth/sessions/revoke-others')
        notify.ok(t('profile.othersSignedOut'), t('profile.revokedCount', { count: res?.revoked ?? 0 }))
        await loadSessions()
      } catch (e) {
        notify.fail(e)
      } finally {
        revokingOthers.value = false
      }
    },
  })
}

onMounted(() => {
  loadProjects()
  loadRbac()
  loadSessions()
})
</script>

<template>
  <div class="page-header">
    <Avatar :label="initials(auth.user?.display_name || auth.user?.email)" shape="circle" size="large" style="background: var(--p-primary-color); color: var(--p-primary-contrast-color)" />
    <div>
      <h1>{{ auth.user?.display_name || auth.user?.email }}</h1>
      <div class="sub">{{ t('profile.subtitle') }}</div>
    </div>
  </div>

  <div class="profile-grid">
    <!-- account -->
    <section class="card" aria-labelledby="h-account">
      <h3 id="h-account"><i class="pi pi-user" /> {{ t('profile.account') }}</h3>
      <Message v-if="nameError" severity="error" :closable="false" style="margin-bottom: 10px">{{ nameError }}</Message>
      <div class="kv">
        <label for="dn" class="small muted">{{ t('profile.displayName') }}</label>
        <form class="row nowrap" @submit.prevent="saveName">
          <InputText id="dn" v-model="displayName" class="grow" :placeholder="t('profile.yourName')" />
          <Button type="submit" :label="t('common.save')" icon="pi pi-check" size="small" :loading="savingName" :disabled="displayName.trim() === (auth.user?.display_name ?? '')" />
        </form>
        <span class="small muted">{{ t('common.email') }}</span><span>{{ auth.user?.email }}</span>
        <span class="small muted">{{ t('profile.signInProvider') }}</span><span>{{ isLocal ? t('profile.localAccount') : auth.user?.auth_provider }}</span>
        <span class="small muted">{{ t('profile.memberSince') }}</span><span>{{ formatDate(auth.user?.created_at) }}</span>
        <template v-if="auth.isSuperAdmin">
          <span class="small muted">{{ t('common.role') }}</span><span><Tag severity="danger" :value="t('profile.superAdministrator')" icon="pi pi-shield" /></span>
        </template>
      </div>
    </section>

    <!-- memberships -->
    <section class="card" aria-labelledby="h-members">
      <h3 id="h-members"><i class="pi pi-building" /> {{ t('profile.memberships') }}</h3>
      <div v-if="!(auth.me?.memberships ?? []).length" class="muted">{{ t('profile.noMemberships') }}</div>
      <div v-for="m in auth.me?.memberships ?? []" :key="m.tenant_id" style="padding: 8px 0; border-bottom: 1px solid var(--qtf-border)">
        <div class="row">
          <b>{{ m.tenant_name || m.tenant_slug }}</b>
          <span v-if="m.tenant_slug" class="mono small muted">{{ m.tenant_slug }}</span>
          <Tag v-if="m.tenant_id === auth.tenant?.id" :value="t('profile.current')" severity="info" />
          <span class="grow" />
          <Tag :value="enumLabel('role', m.role)" severity="secondary" />
        </div>
        <div style="margin-top: 4px">
          <span v-for="p in permsFor(m.tenant_id, m.role)" :key="p" class="perm-chip">{{ p }}</span>
        </div>
      </div>
    </section>

    <!-- preferences -->
    <section class="card" aria-labelledby="h-prefs" style="grid-column: 1 / -1">
      <div class="row" style="margin-bottom: 12px">
        <h3 id="h-prefs" style="margin: 0"><i class="pi pi-sliders-h" /> {{ t('profile.preferences') }}</h3>
        <span class="small muted">{{ t('profile.preferencesHint') }}</span>
      </div>
      <Message v-if="prefsError" severity="error" :closable="false" style="margin-bottom: 10px">{{ prefsError }}</Message>
      <form class="form-grid" @submit.prevent="savePrefs">
        <div class="field">
          <label for="pf-theme">{{ t('profile.theme') }}</label>
          <Select v-model="pform.theme" input-id="pf-theme" :placeholder="t('profile.themeNotSet')" :options="themeOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label for="pf-lang">{{ t('language.label') }}</label>
          <Select v-model="langForm" input-id="pf-lang" :placeholder="t('language.browserDefault')" :options="languageOptions" option-label="label" option-value="value">
            <template #option="{ option }"><span :lang="option.value || undefined">{{ option.label }}</span></template>
          </Select>
          <span class="small muted">{{ t('profile.languageHint') }}</span>
        </div>
        <div class="field">
          <label for="pf-start">{{ t('profile.startPage') }}</label>
          <Select v-model="pform.start_page" input-id="pf-start" :placeholder="t('profile.startDefault')" :options="startOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label for="pf-project">{{ t('profile.defaultProject') }}</label>
          <Select v-model="pform.default_project_id" input-id="pf-project" :placeholder="t('common.none')" :options="projectOptions" option-label="label" option-value="value" filter :disabled="!auth.tenant" />
          <span v-if="projectsError" class="small" style="color: var(--p-red-500)">{{ projectsError }}</span>
          <span v-else class="small muted">{{ t('profile.defaultProjectHint') }}</span>
        </div>
        <div class="field">
          <label for="pf-tenant">{{ t('profile.defaultTenant') }}</label>
          <Select v-model="pform.default_tenant_id" input-id="pf-tenant" :placeholder="t('profile.defaultTenantNone')" :options="tenantOptions" option-label="label" option-value="value" />
          <span class="small muted">{{ t('profile.defaultTenantHint') }}</span>
        </div>
        <div class="field">
          <label for="pf-rows">{{ t('profile.rowsPerPage') }}</label>
          <Select v-model="pform.page_size" input-id="pf-rows" :options="pageSizeOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label for="pf-date">{{ t('profile.dateFormat') }}</label>
          <Select v-model="pform.date_format" input-id="pf-date" :placeholder="t('profile.dateDefault')" :options="dateOptions" option-label="label" option-value="value">
            <template #option="{ option }">
              <div class="row nowrap" style="justify-content: space-between; width: 100%">
                <span>{{ option.label }}</span><span class="muted small mono">{{ option.example }}</span>
              </div>
            </template>
          </Select>
          <span class="small muted">{{ t('common.example', { value: formatDateTimePref(sampleDate, pform, new Date(), getFormatLocale()) }) }}</span>
        </div>
        <div class="field">
          <label for="pf-tz">{{ t('profile.timeZone') }}</label>
          <Select v-model="pform.timezone" input-id="pf-tz" :placeholder="tzOptions[0].label" :options="tzOptions" option-label="label" option-value="value" filter :filter-placeholder="t('profile.searchZones')" :virtual-scroller-options="{ itemSize: 38 }" />
        </div>
        <div class="field">
          <label for="pf-linked">{{ t('profile.linkedTest') }}</label>
          <Select v-model="pform.create_linked_test" input-id="pf-linked" :placeholder="t('profile.serverDefault')" :options="linkedTestOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label>{{ t('profile.tables') }}</label>
          <label class="row nowrap small" for="pf-compact"><ToggleSwitch v-model="pform.compact_tables" input-id="pf-compact" /> {{ t('profile.compactTables') }}</label>
        </div>
        <div class="field">
          <label>{{ t('profile.requirementEditor') }}</label>
          <label class="row nowrap small" for="pf-analysis"><ToggleSwitch v-model="pform.hide_analysis" input-id="pf-analysis" /> {{ t('profile.hideAnalysis') }}</label>
        </div>
        <div class="full row" style="justify-content: flex-end">
          <span v-if="prefsDirty" class="small muted" style="margin-right: auto"><i class="pi pi-circle-fill" style="font-size: 8px; color: var(--p-amber-500)" /> {{ t('profile.unsaved') }}</span>
          <Button :label="t('common.discard')" text severity="secondary" :disabled="!prefsDirty" @click="resetPrefs" />
          <Button type="submit" :label="t('profile.savePreferences')" icon="pi pi-check" :loading="savingPrefs" :disabled="!prefsDirty" />
        </div>
      </form>
    </section>

    <!-- security -->
    <section class="card" aria-labelledby="h-security" style="grid-column: 1 / -1">
      <h3 id="h-security"><i class="pi pi-lock" /> {{ t('profile.security') }}</h3>
      <div class="charts" style="grid-template-columns: minmax(260px, 360px) minmax(0, 1fr); align-items: start">
        <div>
          <div class="section-title" style="margin-top: 0">{{ t('profile.password') }}</div>
          <Message v-if="!isLocal" severity="info" :closable="false">{{ t('profile.managedByIdp', { provider: auth.user?.auth_provider ?? '' }) }}</Message>
          <template v-else>
            <Message v-if="pwError" severity="error" :closable="false" style="margin-bottom: 12px">{{ pwError }}</Message>
            <form class="grid" style="gap: 12px" @submit.prevent="changePassword">
              <div class="field"><label for="pw-cur">{{ t('profile.currentPassword') }}</label><Password v-model="current" input-id="pw-cur" :feedback="false" toggle-mask fluid autocomplete="current-password" /></div>
              <div class="field"><label for="pw-new">{{ t('profile.newPassword') }}</label><Password v-model="next" input-id="pw-new" toggle-mask fluid autocomplete="new-password" /></div>
              <div class="field"><label for="pw-rep">{{ t('profile.repeatPassword') }}</label><Password v-model="repeat" input-id="pw-rep" :feedback="false" toggle-mask fluid autocomplete="new-password" /></div>
              <div><Button type="submit" :label="t('profile.changePassword')" icon="pi pi-lock" :loading="pwBusy" :disabled="!current || !next" /></div>
            </form>
          </template>
          <div class="section-title">{{ t('profile.apiTokens') }}</div>
          <div class="small">
            <i18n-t v-if="auth.can(P.tokenManage)" keypath="profile.tokensManaged" tag="span">
              <template #link><RouterLink :to="{ name: 'tokens' }">{{ t('profile.tokensLink') }}</RouterLink></template>
            </i18n-t>
            <span v-else class="muted">{{ t('profile.noTokenRole') }}</span>
          </div>
        </div>
        <div style="min-width: 0">
          <div class="section-title" style="margin-top: 0">
            {{ t('profile.activeSessions') }}
            <span class="grow" />
            <Button icon="pi pi-refresh" text rounded size="small" severity="secondary" :aria-label="t('profile.refreshSessions')" v-tooltip="t('profile.refreshSessions')" @click="loadSessions" />
            <Button :label="t('profile.signOutOthers')" icon="pi pi-sign-out" size="small" severity="danger" outlined :disabled="!others" :loading="revokingOthers" @click="revokeOthers" />
          </div>
          <Message v-if="sessionsError" severity="error" :closable="false">{{ t('profile.sessionsUnavailable', { error: sessionsError }) }}</Message>
          <DataTable v-else :value="sessionRows" data-key="id" :loading="sessionsLoading" size="small" scrollable>
            <Column :header="t('profile.device')" style="min-width: 220px">
              <template #body="{ data }">
                <div class="row nowrap">
                  <i :class="deviceIcon(data.ua.device)" class="muted" style="font-size: 18px" />
                  <div style="min-width: 0">
                    <div class="row nowrap" style="gap: 6px">
                      <span style="font-weight: 600" :title="data.user_agent">{{ uaLabel(data.ua) }}</span>
                      <Tag v-if="data.current" :value="t('profile.thisDevice')" severity="success" style="white-space: nowrap; flex: none" />
                    </div>
                    <div v-if="tenantName(data.tenant_id)" class="small muted">{{ tenantName(data.tenant_id) }}</div>
                  </div>
                </div>
              </template>
            </Column>
            <Column field="ip" :header="t('profile.ipAddress')" style="width: 140px"><template #body="{ data }"><span class="mono">{{ data.ip || '—' }}</span></template></Column>
            <Column :header="t('profile.signedIn')" style="width: 140px">
              <template #body="{ data }"><span class="small" :title="formatFullDateTime(data.created_at)">{{ formatDateTime(data.created_at) }}</span></template>
            </Column>
            <Column :header="t('profile.lastSeen')" style="width: 150px">
              <template #body="{ data }"><span class="small muted" :title="formatFullDateTime(data.last_seen)">{{ data.last_seen ? relativeTime(data.last_seen, new Date(), true) : '—' }}</span></template>
            </Column>
            <Column style="width: 60px">
              <template #body="{ data }">
                <Button icon="pi pi-times" text rounded size="small" severity="danger" :aria-label="data.current ? t('profile.signOut') : t('profile.revoke')" v-tooltip="data.current ? t('profile.signOutDevice') : t('profile.revoke')" @click="revoke(data)" />
              </template>
            </Column>
            <template #empty><div class="muted small">{{ t('profile.noSessions') }}</div></template>
          </DataTable>
        </div>
      </div>
    </section>
  </div>
</template>
