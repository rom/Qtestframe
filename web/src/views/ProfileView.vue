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
import { formatDate, formatDateTime, formatDateTimePref, formatFullDateTime, humanize, initials, relativeTime } from '@/lib/format'
import { browserTimeZone, normalizePrefs, preferencesPatch, timeZones } from '@/lib/prefs'
import { deviceIcon, parseUserAgent } from '@/lib/useragent'
import { P } from '@/lib/permissions'
import type { Me, Preferences, Project, RbacInfo, SessionInfo } from '@/lib/types'

const auth = useAuthStore()
const prefsStore = usePrefsStore()
const proj = useProjectStore()
const notify = useNotify()
const confirm = useConfirm()
const router = useRouter()

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
    notify.ok('Display name saved')
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
const stored = computed(() => JSON.stringify(preferencesPatch(toForm(prefsStore.prefs))))
const prefsDirty = computed(() => JSON.stringify(preferencesPatch(pform.value)) !== stored.value)
watch(
  () => prefsStore.prefs,
  (p) => {
    if (!prefsDirty.value) pform.value = toForm(p)
  },
)
const savingPrefs = ref(false)
const prefsError = ref('')
async function savePrefs() {
  savingPrefs.value = true
  prefsError.value = ''
  try {
    await prefsStore.save(preferencesPatch(pform.value))
    pform.value = toForm(normalizePrefs(auth.user?.preferences))
    notify.ok('Preferences saved')
  } catch (e) {
    prefsError.value = errorMessage(e)
  } finally {
    savingPrefs.value = false
  }
}
function resetPrefs() {
  pform.value = toForm(prefsStore.prefs)
}

const themeOptions = [
  { label: 'Not set (this device)', value: '' },
  { label: 'Light', value: 'light' },
  { label: 'Dark', value: 'dark' },
  { label: 'Follow the operating system', value: 'system' },
]
const startOptions = [
  { label: 'Default (project list)', value: '' },
  { label: 'Project list', value: 'projects' },
  { label: 'Last opened project', value: 'last_project' },
  { label: 'Default project', value: 'default_project' },
  { label: 'Search', value: 'search' },
]
const pageSizeOptions = [{ label: 'Default (25)', value: 0 }, ...[10, 25, 50, 100].map((n) => ({ label: String(n), value: n }))]
const sampleDate = new Date(Date.now() - 5 * 60 * 1000)
const dateOptions = computed(() =>
  [
    { label: 'Default', value: '' },
    { label: 'Locale', value: 'locale' },
    { label: 'ISO', value: 'iso' },
    { label: 'Relative', value: 'relative' },
  ].map((o) => ({ ...o, example: formatDateTimePref(sampleDate, { date_format: o.value, timezone: pform.value.timezone }) })),
)
const browserTz = browserTimeZone()
const tzOptions = computed(() => [
  { label: `Browser${browserTz ? ' (' + browserTz + ')' : ''}`, value: '' },
  ...timeZones(pform.value.timezone).map((z) => ({ label: z.replace(/_/g, ' '), value: z })),
])
const linkedTestOptions = [
  { label: 'Server default', value: null as boolean | null },
  { label: 'Yes, create a linked test', value: true as boolean | null },
  { label: 'No', value: false as boolean | null },
]
const tenantOptions = computed(() => [
  { label: 'None (last used / first membership)', value: '' },
  ...(auth.me?.memberships ?? []).map((m) => ({ label: m.tenant_name || m.tenant_slug || m.tenant_id, value: m.tenant_id })),
])
const projects = ref<Project[]>([])
const projectsError = ref('')
const projectOptions = computed(() => {
  const opts = [{ label: 'None', value: '' }, ...projects.value.map((p) => ({ label: `${p.key} · ${p.name}`, value: p.id }))]
  if (pform.value.default_project_id && !projects.value.some((p) => p.id === pform.value.default_project_id)) {
    opts.push({ label: 'Project of another tenant or deleted', value: pform.value.default_project_id })
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
    pwError.value = 'The new passwords do not match.'
    return
  }
  pwBusy.value = true
  try {
    await api.post('/auth/password', { current_password: current.value, new_password: next.value })
    current.value = next.value = repeat.value = ''
    notify.ok('Password changed')
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
const others = computed(() => sessions.value.filter((s) => !s.current).length)
function tenantName(id: string) {
  const m = auth.me?.memberships?.find((x) => x.tenant_id === id)
  return m ? m.tenant_name || m.tenant_slug || '' : ''
}
function revoke(s: SessionInfo) {
  confirm.require({
    header: s.current ? 'Sign out this device?' : 'Revoke session?',
    message: s.current ? 'This is the session you are using; you will be signed out.' : 'The device using this session is signed out immediately.',
    icon: 'pi pi-sign-out',
    rejectProps: { label: 'Cancel', severity: 'secondary', outlined: true },
    acceptProps: { label: s.current ? 'Sign out' : 'Revoke', severity: 'danger' },
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
        notify.ok('Session revoked')
      } catch (e) {
        notify.fail(e, 'Could not revoke session')
      }
    },
  })
}
const revokingOthers = ref(false)
function revokeOthers() {
  confirm.require({
    header: 'Sign out all other sessions?',
    message: 'Every other browser and device signed in with your account is signed out. This device stays signed in.',
    icon: 'pi pi-sign-out',
    rejectProps: { label: 'Cancel', severity: 'secondary', outlined: true },
    acceptProps: { label: 'Sign out others', severity: 'danger' },
    accept: async () => {
      revokingOthers.value = true
      try {
        const res = await api.post<{ revoked: number }>('/auth/sessions/revoke-others')
        notify.ok('Other sessions signed out', `${res?.revoked ?? 0} revoked`)
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
      <div class="sub">Your account, personal preferences and sign-in security.</div>
    </div>
  </div>

  <div class="profile-grid">
    <!-- account -->
    <section class="card" aria-labelledby="h-account">
      <h3 id="h-account"><i class="pi pi-user" /> Account</h3>
      <Message v-if="nameError" severity="error" :closable="false" style="margin-bottom: 10px">{{ nameError }}</Message>
      <div class="kv">
        <label for="dn" class="small muted">Display name</label>
        <form class="row nowrap" @submit.prevent="saveName">
          <InputText id="dn" v-model="displayName" class="grow" placeholder="Your name" />
          <Button type="submit" label="Save" icon="pi pi-check" size="small" :loading="savingName" :disabled="displayName.trim() === (auth.user?.display_name ?? '')" />
        </form>
        <span class="small muted">E-mail</span><span>{{ auth.user?.email }}</span>
        <span class="small muted">Sign-in provider</span><span>{{ isLocal ? 'Local account' : auth.user?.auth_provider }}</span>
        <span class="small muted">Member since</span><span>{{ formatDate(auth.user?.created_at) }}</span>
        <template v-if="auth.isSuperAdmin">
          <span class="small muted">Role</span><span><Tag severity="danger" value="Super administrator" icon="pi pi-shield" /></span>
        </template>
      </div>
    </section>

    <!-- memberships -->
    <section class="card" aria-labelledby="h-members">
      <h3 id="h-members"><i class="pi pi-building" /> Memberships</h3>
      <div v-if="!(auth.me?.memberships ?? []).length" class="muted">No tenant memberships.</div>
      <div v-for="m in auth.me?.memberships ?? []" :key="m.tenant_id" style="padding: 8px 0; border-bottom: 1px solid var(--qtf-border)">
        <div class="row">
          <b>{{ m.tenant_name || m.tenant_slug }}</b>
          <span v-if="m.tenant_slug" class="mono small muted">{{ m.tenant_slug }}</span>
          <Tag v-if="m.tenant_id === auth.tenant?.id" value="current" severity="info" />
          <span class="grow" />
          <Tag :value="humanize(m.role)" severity="secondary" />
        </div>
        <div style="margin-top: 4px">
          <span v-for="p in permsFor(m.tenant_id, m.role)" :key="p" class="perm-chip">{{ p }}</span>
        </div>
      </div>
    </section>

    <!-- preferences -->
    <section class="card" aria-labelledby="h-prefs" style="grid-column: 1 / -1">
      <div class="row" style="margin-bottom: 12px">
        <h3 id="h-prefs" style="margin: 0"><i class="pi pi-sliders-h" /> Preferences</h3>
        <span class="small muted">Stored with your account and applied on every device.</span>
      </div>
      <Message v-if="prefsError" severity="error" :closable="false" style="margin-bottom: 10px">{{ prefsError }}</Message>
      <form class="form-grid" @submit.prevent="savePrefs">
        <div class="field">
          <label for="pf-theme">Theme</label>
          <Select v-model="pform.theme" input-id="pf-theme" placeholder="Not set (this device)" :options="themeOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label for="pf-start">Start page</label>
          <Select v-model="pform.start_page" input-id="pf-start" placeholder="Default (project list)" :options="startOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label for="pf-project">Default project</label>
          <Select v-model="pform.default_project_id" input-id="pf-project" placeholder="None" :options="projectOptions" option-label="label" option-value="value" filter :disabled="!auth.tenant" />
          <span v-if="projectsError" class="small" style="color: var(--p-red-500)">{{ projectsError }}</span>
          <span v-else class="small muted">Opened on sign-in when the start page is “Default project”.</span>
        </div>
        <div class="field">
          <label for="pf-tenant">Default tenant</label>
          <Select v-model="pform.default_tenant_id" input-id="pf-tenant" placeholder="None (last used / first membership)" :options="tenantOptions" option-label="label" option-value="value" />
          <span class="small muted">Selected at sign-in when no tenant is given.</span>
        </div>
        <div class="field">
          <label for="pf-rows">Rows per table page</label>
          <Select v-model="pform.page_size" input-id="pf-rows" :options="pageSizeOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label for="pf-date">Date format</label>
          <Select v-model="pform.date_format" input-id="pf-date" placeholder="Default" :options="dateOptions" option-label="label" option-value="value">
            <template #option="{ option }">
              <div class="row nowrap" style="justify-content: space-between; width: 100%">
                <span>{{ option.label }}</span><span class="muted small mono">{{ option.example }}</span>
              </div>
            </template>
          </Select>
          <span class="small muted">Example: {{ formatDateTimePref(sampleDate, pform) }}</span>
        </div>
        <div class="field">
          <label for="pf-tz">Time zone</label>
          <Select v-model="pform.timezone" input-id="pf-tz" :placeholder="tzOptions[0].label" :options="tzOptions" option-label="label" option-value="value" filter filter-placeholder="Search time zones" :virtual-scroller-options="{ itemSize: 38 }" />
        </div>
        <div class="field">
          <label for="pf-linked">Also create linked test (new requirements)</label>
          <Select v-model="pform.create_linked_test" input-id="pf-linked" placeholder="Server default" :options="linkedTestOptions" option-label="label" option-value="value" />
        </div>
        <div class="field">
          <label>Tables</label>
          <label class="row nowrap small" for="pf-compact"><ToggleSwitch v-model="pform.compact_tables" input-id="pf-compact" /> Compact tables (denser rows)</label>
        </div>
        <div class="field">
          <label>Requirement editor</label>
          <label class="row nowrap small" for="pf-analysis"><ToggleSwitch v-model="pform.hide_analysis" input-id="pf-analysis" /> Hide the live requirement-language analysis</label>
        </div>
        <div class="full row" style="justify-content: flex-end">
          <span v-if="prefsDirty" class="small muted" style="margin-right: auto"><i class="pi pi-circle-fill" style="font-size: 8px; color: var(--p-amber-500)" /> Unsaved changes</span>
          <Button label="Discard" text severity="secondary" :disabled="!prefsDirty" @click="resetPrefs" />
          <Button type="submit" label="Save preferences" icon="pi pi-check" :loading="savingPrefs" :disabled="!prefsDirty" />
        </div>
      </form>
    </section>

    <!-- security -->
    <section class="card" aria-labelledby="h-security" style="grid-column: 1 / -1">
      <h3 id="h-security"><i class="pi pi-lock" /> Security</h3>
      <div class="charts" style="grid-template-columns: minmax(260px, 360px) minmax(0, 1fr); align-items: start">
        <div>
          <div class="section-title" style="margin-top: 0">Password</div>
          <Message v-if="!isLocal" severity="info" :closable="false">
            You sign in via {{ auth.user?.auth_provider }}; your password is managed by your identity provider.
          </Message>
          <template v-else>
            <Message v-if="pwError" severity="error" :closable="false" style="margin-bottom: 12px">{{ pwError }}</Message>
            <form class="grid" style="gap: 12px" @submit.prevent="changePassword">
              <div class="field"><label for="pw-cur">Current password</label><Password v-model="current" input-id="pw-cur" :feedback="false" toggle-mask fluid autocomplete="current-password" /></div>
              <div class="field"><label for="pw-new">New password</label><Password v-model="next" input-id="pw-new" toggle-mask fluid autocomplete="new-password" /></div>
              <div class="field"><label for="pw-rep">Repeat new password</label><Password v-model="repeat" input-id="pw-rep" :feedback="false" toggle-mask fluid autocomplete="new-password" /></div>
              <div><Button type="submit" label="Change password" icon="pi pi-lock" :loading="pwBusy" :disabled="!current || !next" /></div>
            </form>
          </template>
          <div class="section-title">API tokens</div>
          <div class="small">
            <template v-if="auth.can(P.tokenManage)">
              Tokens for CI and scripts are managed under <RouterLink :to="{ name: 'tokens' }">Administration → API tokens</RouterLink>.
            </template>
            <span v-else class="muted">Your role in this tenant cannot create API tokens.</span>
          </div>
        </div>
        <div style="min-width: 0">
          <div class="section-title" style="margin-top: 0">
            Active sessions
            <span class="grow" />
            <Button icon="pi pi-refresh" text rounded size="small" severity="secondary" aria-label="Refresh sessions" @click="loadSessions" />
            <Button label="Sign out all other sessions" icon="pi pi-sign-out" size="small" severity="danger" outlined :disabled="!others" :loading="revokingOthers" @click="revokeOthers" />
          </div>
          <Message v-if="sessionsError" severity="error" :closable="false">Sessions are unavailable: {{ sessionsError }}</Message>
          <DataTable v-else :value="sessionRows" data-key="id" :loading="sessionsLoading" size="small" scrollable>
            <Column header="Device" style="min-width: 220px">
              <template #body="{ data }">
                <div class="row nowrap">
                  <i :class="deviceIcon(data.ua.device)" class="muted" style="font-size: 18px" />
                  <div style="min-width: 0">
                    <div class="row nowrap" style="gap: 6px">
                      <span style="font-weight: 600" :title="data.user_agent">{{ data.ua.label }}</span>
                      <Tag v-if="data.current" value="This device" severity="success" style="white-space: nowrap; flex: none" />
                    </div>
                    <div v-if="tenantName(data.tenant_id)" class="small muted">{{ tenantName(data.tenant_id) }}</div>
                  </div>
                </div>
              </template>
            </Column>
            <Column field="ip" header="IP address" style="width: 140px"><template #body="{ data }"><span class="mono">{{ data.ip || '—' }}</span></template></Column>
            <Column header="Signed in" style="width: 140px">
              <template #body="{ data }"><span class="small" :title="formatFullDateTime(data.created_at)">{{ formatDateTime(data.created_at) }}</span></template>
            </Column>
            <Column header="Last seen" style="width: 130px">
              <template #body="{ data }"><span class="small muted" :title="formatFullDateTime(data.last_seen)">{{ data.last_seen ? relativeTime(data.last_seen, new Date(), true) : '—' }}</span></template>
            </Column>
            <Column style="width: 60px">
              <template #body="{ data }">
                <Button icon="pi pi-times" text rounded size="small" severity="danger" :aria-label="data.current ? 'Sign out' : 'Revoke'" v-tooltip="data.current ? 'Sign out this device' : 'Revoke'" @click="revoke(data)" />
              </template>
            </Column>
            <template #empty><div class="muted small">No active sessions.</div></template>
          </DataTable>
        </div>
      </div>
    </section>
  </div>
</template>
