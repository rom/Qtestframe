<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Menu from 'primevue/menu'
import UserAvatar from '@/components/UserAvatar.vue'
import ProjectIcon from '@/components/ProjectIcon.vue'
import Dialog from 'primevue/dialog'
import ThemePicker from '@/components/ThemePicker.vue'
import HelpMenu from '@/guide/HelpMenu.vue'
import UndoControls from '@/components/UndoControls.vue'
import NotificationBell from '@/components/NotificationBell.vue'
import MfaGraceBanner from '@/components/MfaGraceBanner.vue'
import { useBreakpoint } from '@/lib/breakpoint'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { useProjectStore } from '@/stores/project'
import { isSoftware } from '@/lib/projectTypes'
import { useMetaStore } from '@/stores/meta'
import { usePrefsStore } from '@/stores/prefs'
import { useNotify } from '@/composables/notify'
import { api, asItems } from '@/lib/api'
import { P } from '@/lib/permissions'
import { useI18n } from 'vue-i18n'
import { chooseLocal, currentLang, LANGUAGES, LANGUAGE_NAMES, type Lang } from '@/i18n'
import { enumLabel } from '@/i18n/labels'
import type { Tenant } from '@/lib/types'
import { useRolesStore } from '@/stores/roles'

const auth = useAuthStore()
const ui = useUiStore()
const proj = useProjectStore()
const meta = useMetaStore()
const prefs = usePrefsStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()
const { t } = useI18n()

const searchText = ref('')
const userMenu = ref<InstanceType<typeof Menu>>()
const tenantMenu = ref<InstanceType<typeof Menu>>()
const searchOpen = ref(false)
/** below 1024 px the sidebar is an off-canvas drawer */
const overlayNav = useBreakpoint('tablet')
/** below 768 px the search field collapses to a button */
const compactSearch = useBreakpoint(767)
const sidebarEl = ref<HTMLElement>()
const menuButton = ref<{ $el: HTMLElement }>()
const allTenants = ref<Tenant[]>([])

onMounted(async () => {
  meta.load()
  if (auth.isSuperAdmin) {
    try {
      allTenants.value = asItems(await api.get<{ items: Tenant[] } | Tenant[]>('/tenants'))
    } catch {
      /* ignore */
    }
  }
})

// roles of the organisation: custom role labels everywhere, role selects
const rolesStore = useRolesStore()
watch(
  () => auth.tenant?.id,
  () => void rolesStore.ensure(true),
  { immediate: true },
)

watch(
  () => route.fullPath,
  () => {
    ui.mobileNav = false
    searchOpen.value = false
    if (route.name === 'search') searchText.value = (route.query.q as string) || ''
  },
  { immediate: true },
)

// ---------- off-canvas navigation: focus moves in, Escape closes, focus returns ----------
const navOpen = computed(() => overlayNav.value && ui.mobileNav)
watch(navOpen, async (open, was) => {
  await nextTick()
  if (open) sidebarEl.value?.querySelector<HTMLElement>('a, button')?.focus()
  else if (was) menuButton.value?.$el?.focus()
})
watch(overlayNav, (v) => {
  if (!v) ui.mobileNav = false
})
function onKeydown(e: KeyboardEvent) {
  if (e.key === 'Escape' && navOpen.value) {
    e.preventDefault()
    ui.mobileNav = false
  }
}
onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
/** keep Tab inside the open drawer */
function trapTab(e: KeyboardEvent) {
  if (!navOpen.value || e.key !== 'Tab' || !sidebarEl.value) return
  const items = Array.from(sidebarEl.value.querySelectorAll<HTMLElement>('a[href], button:not([disabled])')).filter((el) => el.offsetParent !== null)
  if (!items.length) return
  const first = items[0]
  const last = items[items.length - 1]
  if (e.shiftKey && document.activeElement === first) {
    e.preventDefault()
    last.focus()
  } else if (!e.shiftKey && document.activeElement === last) {
    e.preventDefault()
    first.focus()
  }
}

const tenantOptions = computed(() => {
  const map = new Map<string, { label: string; value: string; role?: string }>()
  for (const m of auth.me?.memberships ?? []) {
    map.set(m.tenant_id, { label: m.tenant_name || m.tenant_slug || m.tenant_id, value: m.tenant_id, role: m.role })
  }
  for (const t of allTenants.value) if (!map.has(t.id)) map.set(t.id, { label: t.name, value: t.id, role: 'super_admin' })
  return [...map.values()]
})

const currentTenant = computed({
  get: () => auth.tenant?.id ?? null,
  set: async (id: string | null) => {
    if (!id || id === auth.tenant?.id) return
    try {
      await auth.switchTenant(id)
      proj.reset()
      notify.ok(t('nav.switchedTenant'), auth.tenant?.name)
      await router.push({ name: 'projects' })
    } catch (e) {
      notify.fail(e, t('nav.switchTenantFailed'))
    }
  },
})

const inProject = computed(() => !!route.params.projectId && !!proj.project)
const pid = computed(() => route.params.projectId as string)

const projectNav = computed(() => [
  { to: 'dashboard', icon: 'pi pi-chart-bar', label: t('nav.dashboard') },
  { to: 'requirements', icon: 'pi pi-list-check', label: t('nav.requirements') },
  { to: 'tests', icon: 'pi pi-check-square', label: t('nav.tests') },
  { to: 'runs', icon: 'pi pi-play-circle', label: t('nav.operations') },
  { to: 'traceability', icon: 'pi pi-th-large', label: t('nav.traceability') },
  { to: 'analytics', icon: 'pi pi-chart-bar', label: t('nav.analytics') },
  { to: 'graph', icon: 'pi pi-sitemap', label: t('nav.graph') },
  { to: 'flow', icon: 'pi pi-share-alt', label: t('nav.flow') },
  ...(isSoftware(proj.project) ? [{ to: 'prompts', icon: 'pi pi-sparkles', label: t('nav.prompts') }] : []),
  { to: 'catalog', icon: 'pi pi-book', label: t('nav.catalog') },
  { to: 'tags', icon: 'pi pi-hashtag', label: t('nav.tags') },
  { to: 'baselines', icon: 'pi pi-flag', label: t('nav.baselines') },
  { to: 'documents', icon: 'pi pi-folder', label: t('nav.documents') },
  { to: 'reports', icon: 'pi pi-file-pdf', label: t('nav.reports') },
  { to: 'settings', icon: 'pi pi-tags', label: t('nav.settings') },
  { to: 'export', icon: 'pi pi-download', label: t('nav.export') },
])

const currentTenantLabel = computed(() => tenantOptions.value.find((o) => o.value === currentTenant.value)?.label ?? t('nav.selectTenant'))
const tenantItems = computed(() =>
  tenantOptions.value.map((o) => ({
    label: o.label,
    icon: o.value === currentTenant.value ? 'pi pi-check' : 'pi pi-building',
    command: () => (currentTenant.value = o.value),
  })),
)

const showAdmin = computed(() => auth.can(P.memberRead) || auth.can(P.tokenManage) || auth.can(P.auditRead) || auth.can(P.settingsManage) || auth.isSuperAdmin)

const userItems = computed(() => [
  { label: auth.user?.email ?? '', disabled: true, icon: 'pi pi-user' },
  { separator: true },
  { label: t('nav.profileAndPassword'), icon: 'pi pi-id-card', command: () => router.push({ name: 'profile' }) },
  { label: ui.dark ? t('nav.lightMode') : t('nav.darkMode'), icon: ui.dark ? 'pi pi-sun' : 'pi pi-moon', command: () => prefs.toggleTheme() },
  { separator: true },
  {
    label: t('language.label'),
    items: LANGUAGES.map((l) => ({
      // each language is named in its own language
      label: LANGUAGE_NAMES[l],
      lang: l,
      icon: lang.value === l ? 'pi pi-check' : 'pi pi-globe',
      class: lang.value === l ? 'lang-current' : undefined,
      command: () => switchLanguage(l),
    })),
  },
  { separator: true },
  { label: t('nav.signOut'), icon: 'pi pi-sign-out', command: logout },
])

const { locale } = useI18n()
const lang = computed(() => (locale.value as Lang) || currentLang())
/** Top-bar language switch: applies now, remembers it on this device and stores it as preference. */
async function switchLanguage(l: Lang) {
  chooseLocal(l)
  if (!auth.me) return
  try {
    await prefs.save({ language: l })
  } catch (e) {
    console.warn('saving language preference failed', e)
  }
}

async function logout() {
  await auth.logout().catch(() => undefined)
  proj.reset()
  router.push({ name: 'login' })
}

function doSearch() {
  const q = searchText.value.trim()
  if (q) router.push({ name: 'search', query: { q } })
  searchOpen.value = false
}
const userName = computed(() => auth.user?.display_name || auth.user?.email || '')
const userRole = computed(() => (auth.isSuperAdmin ? t('nav.superAdmin') : enumLabel('role', auth.me?.role)) + (auth.tenant ? ' · ' + auth.tenant.name : ''))
</script>

<template>
  <div class="app-shell" :class="{ 'nav-overlay': overlayNav }">
    <aside
      id="app-sidebar"
      ref="sidebarEl"
      class="sidebar"
      :class="{ collapsed: ui.sidebarCollapsed && !overlayNav, open: navOpen }"
      :aria-label="t('nav.sidebar')"
      :inert="overlayNav && !navOpen ? true : undefined"
      :role="navOpen ? 'dialog' : undefined"
      :aria-modal="navOpen ? 'true' : undefined"
      @keydown="trapTab"
    >
      <div class="brand">
        <div class="logo" aria-hidden="true"><i class="pi pi-check" style="color: #fff; font-weight: 800" /></div>
        <span class="label">Qtestframe</span>
        <Button
          v-if="overlayNav"
          class="nav-close"
          icon="pi pi-times"
          text
          rounded
          severity="secondary"
          :aria-label="t('nav.closeMenu')"
          style="color: var(--qtf-sidebar-text)"
          @click="ui.mobileNav = false"
        />
      </div>
      <nav :aria-label="t('nav.mainNav')">
        <div class="section" aria-hidden="true">{{ t('nav.workspace') }}</div>
        <RouterLink class="nav-item" :to="{ name: 'projects' }" v-tooltip.right="ui.sidebarCollapsed && !overlayNav ? t('nav.projects') : ''">
          <i class="pi pi-folder-open" aria-hidden="true" /><span class="label">{{ t('nav.projects') }}</span>
        </RouterLink>
        <RouterLink class="nav-item" :to="{ name: 'search' }" v-tooltip.right="ui.sidebarCollapsed && !overlayNav ? t('nav.search') : ''">
          <i class="pi pi-search" aria-hidden="true" /><span class="label">{{ t('nav.search') }}</span>
        </RouterLink>
        <RouterLink class="nav-item" :to="{ name: 'reviews' }" v-tooltip.right="ui.sidebarCollapsed && !overlayNav ? t('nav.reviews') : ''">
          <i class="pi pi-eye" aria-hidden="true" /><span class="label">{{ t('nav.reviews') }}</span>
        </RouterLink>
        <template v-if="inProject">
          <div class="section project-section" :title="proj.project?.name"><ProjectIcon :icon="proj.project?.icon" :project-key="proj.project?.key" :color="proj.project?.color" :size="18" /><span class="ellipsis">{{ proj.project?.key }} · {{ proj.project?.name }}</span></div>
          <RouterLink
            v-for="n in projectNav"
            :key="n.to"
            class="nav-item"
            :to="{ name: n.to, params: { projectId: pid } }"
            v-tooltip.right="ui.sidebarCollapsed && !overlayNav ? n.label : ''"
          >
            <i :class="n.icon" aria-hidden="true" /><span class="label" :title="n.label">{{ n.label }}</span>
          </RouterLink>
        </template>
        <template v-if="showAdmin">
          <div class="section" aria-hidden="true">{{ t('nav.administration') }}</div>
          <RouterLink class="nav-item" :to="{ name: 'admin' }" v-tooltip.right="ui.sidebarCollapsed && !overlayNav ? t('nav.administration') : ''">
            <i class="pi pi-cog" aria-hidden="true" /><span class="label">{{ t('nav.administration') }}</span>
          </RouterLink>
        </template>
      </nav>
      <div v-if="!overlayNav" class="foot">
        <Button
          text
          :icon="ui.sidebarCollapsed ? 'pi pi-angle-double-right' : 'pi pi-angle-double-left'"
          severity="secondary"
          class="w-full"
          :label="ui.sidebarCollapsed ? '' : t('nav.collapse')"
          :aria-label="ui.sidebarCollapsed ? t('nav.expand') : t('nav.collapse')"
          :aria-expanded="!ui.sidebarCollapsed"
          style="color: var(--qtf-sidebar-text)"
          @click="ui.toggleSidebar()"
        />
      </div>
    </aside>
    <div class="backdrop" :class="{ show: navOpen }" aria-hidden="true" @click="ui.mobileNav = false" />
    <div class="main" :inert="navOpen ? true : undefined">
      <header class="topbar">
        <Button
          v-if="overlayNav"
          ref="menuButton"
          icon="pi pi-bars"
          text
          rounded
          severity="secondary"
          :aria-label="t('nav.menu')"
          aria-controls="app-sidebar"
          :aria-expanded="navOpen"
          @click="ui.mobileNav = true"
        />
        <template v-if="tenantOptions.length">
          <Select
            v-if="!overlayNav"
            v-model="currentTenant"
            :options="tenantOptions"
            option-label="label"
            option-value="value"
            :placeholder="t('nav.selectTenant')"
            class="tenant-select"
            :aria-label="t('nav.tenant')"
          >
            <template #value="{ value }">
              <span class="row nowrap" style="min-width: 0"><i class="pi pi-building muted" aria-hidden="true" /><span class="ellipsis">{{ tenantOptions.find((o) => o.value === value)?.label ?? t('nav.selectTenant') }}</span></span>
            </template>
            <template #option="{ option }">
              <div class="row nowrap" style="justify-content: space-between; width: 100%">
                <span>{{ option.label }}</span><span class="muted small">{{ enumLabel('role', option.role) }}</span>
              </div>
            </template>
          </Select>
          <template v-else>
            <Button
              icon="pi pi-building"
              text
              rounded
              severity="secondary"
              :aria-label="t('nav.switchTenantCurrent', { tenant: currentTenantLabel })"
              aria-haspopup="menu"
              v-tooltip.bottom="currentTenantLabel"
              @click="tenantMenu?.toggle($event)"
            />
            <Menu ref="tenantMenu" :model="tenantItems" popup :aria-label="t('nav.tenant')" />
          </template>
        </template>
        <form v-if="!compactSearch" class="search" role="search" @submit.prevent="doSearch">
          <IconField>
            <InputIcon class="pi pi-search" aria-hidden="true" />
            <InputText v-model="searchText" type="search" :placeholder="t('nav.searchPlaceholder')" class="w-full" :aria-label="t('nav.globalSearch')" />
          </IconField>
        </form>
        <div class="spacer" />
        <Button
          v-if="compactSearch"
          icon="pi pi-search"
          text
          rounded
          severity="secondary"
          :aria-label="t('nav.globalSearch')"
          aria-haspopup="dialog"
          @click="searchOpen = true"
        />
        <UndoControls />
        <NotificationBell />
        <HelpMenu />
        <ThemePicker compact :t="t" />
        <Button
          :icon="ui.dark ? 'pi pi-sun' : 'pi pi-moon'"
          text
          rounded
          severity="secondary"
          :aria-label="ui.dark ? t('nav.lightMode') : t('nav.darkMode')"
          v-tooltip.bottom="ui.dark ? t('nav.lightMode') : t('nav.darkMode')"
          @click="prefs.toggleTheme()"
        />
        <button type="button" class="user-button" aria-haspopup="menu" @click="userMenu?.toggle($event)">
          <UserAvatar :avatar="auth.user?.avatar ?? ''" :name="userName" :size="32" />
          <span class="user-text">
            <span style="font-weight: 600" class="ellipsis">{{ userName }}</span>
            <span class="muted small ellipsis">{{ userRole }}</span>
          </span>
          <span class="sr-only">{{ t('nav.userMenu') }}</span>
          <i class="pi pi-angle-down muted user-text" aria-hidden="true" />
        </button>
        <Menu ref="userMenu" :model="userItems" popup :aria-label="t('nav.userMenu')">
          <template #item="{ item, props: p }">
            <a v-bind="p.action" :aria-disabled="item.disabled === true ? 'true' : undefined">
              <span v-if="item.icon" :class="item.icon" v-bind="p.icon" />
              <span v-bind="p.label" :lang="(item as { lang?: string }).lang">{{ item.label }}</span>
            </a>
          </template>
        </Menu>
      </header>
      <main id="main-content" class="content" tabindex="-1">
        <div v-if="!auth.tenant && route.name !== 'profile' && !route.path.startsWith('/admin')" class="card" style="margin-bottom: 16px">
          <i class="pi pi-info-circle" aria-hidden="true" /> {{ auth.isSuperAdmin ? t('nav.noTenantSuperAdmin') : t('nav.noTenant') }}
          <Select
            v-if="tenantOptions.length && overlayNav"
            v-model="currentTenant"
            :options="tenantOptions"
            option-label="label"
            option-value="value"
            :placeholder="t('nav.selectTenant')"
            :aria-label="t('nav.tenant')"
            style="margin-top: 8px; display: flex; max-width: 100%"
          />
        </div>
        <MfaGraceBanner />
        <RouterView />
      </main>
    </div>
    <Dialog v-model:visible="searchOpen" :header="t('nav.globalSearch')" modal position="top" dismissable-mask class="search-dialog" :style="{ width: 'min(560px, calc(100vw - 16px))' }">
      <form role="search" @submit.prevent="doSearch">
        <div class="row nowrap">
          <IconField class="grow">
            <InputIcon class="pi pi-search" aria-hidden="true" />
            <InputText v-model="searchText" type="search" :placeholder="t('nav.searchPlaceholderShort')" class="w-full" :aria-label="t('nav.globalSearch')" autofocus />
          </IconField>
          <Button type="submit" icon="pi pi-arrow-right" :aria-label="t('common.search')" />
        </div>
      </form>
    </Dialog>
  </div>
</template>

<style scoped>
/* longer (e.g. Swedish) labels must not run out of the sidebar */
.nav-item .label {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.tenant-select {
  min-width: 160px;
  max-width: 240px;
}
.user-button {
  all: unset;
  cursor: pointer;
  display: flex;
  gap: 8px;
  align-items: center;
  min-width: 0;
  max-width: 260px;
  border-radius: 999px;
  padding: 2px;
}
.user-text {
  display: flex;
  flex-direction: column;
  line-height: 1.15;
  min-width: 0;
}
.nav-close {
  margin-left: auto;
}
.project-section {
  display: flex;
  align-items: center;
  gap: 6px;
}
@media (max-width: 1279px) {
  .user-text {
    display: none !important;
  }
  .tenant-select {
    max-width: 200px;
  }
}
</style>
