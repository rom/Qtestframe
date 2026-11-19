<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import Button from 'primevue/button'
import Select from 'primevue/select'
import InputText from 'primevue/inputtext'
import IconField from 'primevue/iconfield'
import InputIcon from 'primevue/inputicon'
import Menu from 'primevue/menu'
import Avatar from 'primevue/avatar'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { useProjectStore } from '@/stores/project'
import { useMetaStore } from '@/stores/meta'
import { useNotify } from '@/composables/notify'
import { api, asItems } from '@/lib/api'
import { P } from '@/lib/permissions'
import { initials, humanize } from '@/lib/format'
import type { Tenant } from '@/lib/types'

const auth = useAuthStore()
const ui = useUiStore()
const proj = useProjectStore()
const meta = useMetaStore()
const route = useRoute()
const router = useRouter()
const notify = useNotify()

const searchText = ref('')
const userMenu = ref<InstanceType<typeof Menu>>()
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

watch(
  () => route.fullPath,
  () => {
    ui.mobileNav = false
    if (route.name === 'search') searchText.value = (route.query.q as string) || ''
  },
  { immediate: true },
)

const tenantOptions = computed(() => {
  const map = new Map<string, { label: string; value: string; role?: string }>()
  for (const m of auth.me?.memberships ?? []) {
    map.set(m.tenant_id, { label: m.tenant_name || m.tenant_slug || m.tenant_id, value: m.tenant_id, role: m.role })
  }
  for (const t of allTenants.value) if (!map.has(t.id)) map.set(t.id, { label: t.name, value: t.id, role: 'super admin' })
  return [...map.values()]
})

const currentTenant = computed({
  get: () => auth.tenant?.id ?? null,
  set: async (id: string | null) => {
    if (!id || id === auth.tenant?.id) return
    try {
      await auth.switchTenant(id)
      proj.reset()
      notify.ok('Switched tenant', auth.tenant?.name)
      await router.push({ name: 'projects' })
    } catch (e) {
      notify.fail(e, 'Could not switch tenant')
    }
  },
})

const inProject = computed(() => !!route.params.projectId && !!proj.project)
const pid = computed(() => route.params.projectId as string)

const projectNav = computed(() => [
  { to: 'dashboard', icon: 'pi pi-chart-bar', label: 'Dashboard' },
  { to: 'requirements', icon: 'pi pi-list-check', label: 'Requirements' },
  { to: 'tests', icon: 'pi pi-check-square', label: 'Tests' },
  { to: 'runs', icon: 'pi pi-play-circle', label: 'Operations' },
  { to: 'traceability', icon: 'pi pi-th-large', label: 'Traceability' },
  { to: 'settings', icon: 'pi pi-tags', label: 'Categories & settings' },
  { to: 'export', icon: 'pi pi-download', label: 'Export' },
])

const showAdmin = computed(() => auth.can(P.memberRead) || auth.can(P.tokenManage) || auth.can(P.auditRead) || auth.isSuperAdmin)

const userItems = computed(() => [
  { label: auth.user?.email ?? '', disabled: true, icon: 'pi pi-user' },
  { separator: true },
  { label: 'Profile & password', icon: 'pi pi-id-card', command: () => router.push({ name: 'profile' }) },
  { label: ui.dark ? 'Light mode' : 'Dark mode', icon: ui.dark ? 'pi pi-sun' : 'pi pi-moon', command: () => ui.toggleDark() },
  { separator: true },
  { label: 'Sign out', icon: 'pi pi-sign-out', command: logout },
])

async function logout() {
  await auth.logout().catch(() => undefined)
  proj.reset()
  router.push({ name: 'login' })
}

function doSearch() {
  const q = searchText.value.trim()
  if (q) router.push({ name: 'search', query: { q } })
}
</script>

<template>
  <div class="app-shell">
    <aside class="sidebar" :class="{ collapsed: ui.sidebarCollapsed, open: ui.mobileNav }">
      <div class="brand">
        <div class="logo"><i class="pi pi-check" style="color: #fff; font-weight: 800" /></div>
        <span class="label">Qtestframe</span>
      </div>
      <nav>
        <div class="section">Workspace</div>
        <RouterLink class="nav-item" :to="{ name: 'projects' }" v-tooltip.right="ui.sidebarCollapsed ? 'Projects' : ''">
          <i class="pi pi-folder-open" /><span class="label">Projects</span>
        </RouterLink>
        <RouterLink class="nav-item" :to="{ name: 'search' }" v-tooltip.right="ui.sidebarCollapsed ? 'Search' : ''">
          <i class="pi pi-search" /><span class="label">Search</span>
        </RouterLink>
        <template v-if="inProject">
          <div class="section" :title="proj.project?.name">{{ proj.project?.key }} · {{ proj.project?.name }}</div>
          <RouterLink
            v-for="n in projectNav"
            :key="n.to"
            class="nav-item"
            :to="{ name: n.to, params: { projectId: pid } }"
            v-tooltip.right="ui.sidebarCollapsed ? n.label : ''"
          >
            <i :class="n.icon" /><span class="label">{{ n.label }}</span>
          </RouterLink>
        </template>
        <template v-if="showAdmin">
          <div class="section">Administration</div>
          <RouterLink class="nav-item" :to="{ name: 'admin' }" v-tooltip.right="ui.sidebarCollapsed ? 'Administration' : ''">
            <i class="pi pi-cog" /><span class="label">Administration</span>
          </RouterLink>
        </template>
      </nav>
      <div class="foot desktop-only">
        <Button
          text
          :icon="ui.sidebarCollapsed ? 'pi pi-angle-double-right' : 'pi pi-angle-double-left'"
          severity="secondary"
          class="w-full"
          :label="ui.sidebarCollapsed ? '' : 'Collapse'"
          style="color: var(--qtf-sidebar-text)"
          @click="ui.toggleSidebar()"
        />
      </div>
    </aside>
    <div class="backdrop" :class="{ show: ui.mobileNav }" @click="ui.mobileNav = false" />
    <div class="main">
      <header class="topbar">
        <Button class="mobile-only" icon="pi pi-bars" text rounded severity="secondary" aria-label="Menu" @click="ui.mobileNav = true" />
        <Select
          v-if="tenantOptions.length"
          v-model="currentTenant"
          :options="tenantOptions"
          option-label="label"
          option-value="value"
          placeholder="Select tenant"
          class="desktop-only"
          style="min-width: 190px"
          aria-label="Tenant"
        >
          <template #value="{ value }">
            <span class="row nowrap"><i class="pi pi-building muted" />{{ tenantOptions.find((t) => t.value === value)?.label ?? 'Select tenant' }}</span>
          </template>
          <template #option="{ option }">
            <div class="row nowrap" style="justify-content: space-between; width: 100%">
              <span>{{ option.label }}</span><span class="muted small">{{ humanize(option.role) }}</span>
            </div>
          </template>
        </Select>
        <form class="search" @submit.prevent="doSearch">
          <IconField>
            <InputIcon class="pi pi-search" />
            <InputText v-model="searchText" placeholder="Search requirements, tests, runs…  (query language supported)" class="w-full" aria-label="Global search" />
          </IconField>
        </form>
        <div class="spacer" />
        <Button
          :icon="ui.dark ? 'pi pi-sun' : 'pi pi-moon'"
          text
          rounded
          severity="secondary"
          :aria-label="ui.dark ? 'Light mode' : 'Dark mode'"
          v-tooltip.bottom="ui.dark ? 'Light mode' : 'Dark mode'"
          @click="ui.toggleDark()"
        />
        <button class="row nowrap" style="all: unset; cursor: pointer; display: flex; gap: 8px; align-items: center" aria-label="User menu" @click="userMenu?.toggle($event)">
          <Avatar :label="initials(auth.user?.display_name || auth.user?.email)" shape="circle" style="background: var(--p-primary-color); color: var(--p-primary-contrast-color)" />
          <span class="desktop-only" style="display: flex; flex-direction: column; line-height: 1.15">
            <span style="font-weight: 600">{{ auth.user?.display_name || auth.user?.email }}</span>
            <span class="muted small">{{ auth.isSuperAdmin ? 'Super admin' : humanize(auth.me?.role) }}{{ auth.tenant ? ' · ' + auth.tenant.name : '' }}</span>
          </span>
          <i class="pi pi-angle-down muted desktop-only" />
        </button>
        <Menu ref="userMenu" :model="userItems" popup />
      </header>
      <main class="content">
        <div v-if="!auth.tenant && route.name !== 'profile' && !route.path.startsWith('/admin')" class="card" style="margin-bottom: 16px">
          <i class="pi pi-info-circle" /> You have no current tenant. Select one in the tenant switcher{{ auth.isSuperAdmin ? ' or create one under Administration → Tenants' : '' }}.
          <Select
            v-if="tenantOptions.length"
            v-model="currentTenant"
            :options="tenantOptions"
            option-label="label"
            option-value="value"
            placeholder="Select tenant"
            class="mobile-only"
            style="margin-top: 8px"
          />
        </div>
        <RouterView />
      </main>
    </div>
  </div>
</template>
