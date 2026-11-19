import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { P } from './lib/permissions'
import { api, asItems } from './lib/api'
import { startRoute } from './lib/prefs'
import { usePrefsStore } from './stores/prefs'
import type { Project } from './lib/types'

const routes: RouteRecordRaw[] = [
  { path: '/login', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
  {
    path: '/',
    component: () => import('./layouts/AppLayout.vue'),
    children: [
      // '/' resolves to the user's start page (see beforeEach)
      { path: '', name: 'home', component: () => import('./views/ProjectsView.vue') },
      { path: 'projects', name: 'projects', component: () => import('./views/ProjectsView.vue') },
      {
        path: 'p/:projectId',
        component: () => import('./views/ProjectLayout.vue'),
        props: true,
        children: [
          { path: '', redirect: (to) => ({ name: 'dashboard', params: to.params }) },
          { path: 'dashboard', name: 'dashboard', component: () => import('./views/DashboardView.vue') },
          { path: 'requirements', name: 'requirements', component: () => import('./views/RequirementsView.vue') },
          { path: 'tests', name: 'tests', component: () => import('./views/TestsView.vue') },
          { path: 'runs', name: 'runs', component: () => import('./views/RunsView.vue') },
          { path: 'runs/:runId', name: 'run', component: () => import('./views/RunDetailView.vue'), props: true },
          {
            path: 'runs/:runId/exec/:executionId',
            name: 'execute',
            component: () => import('./views/ExecutionRunnerView.vue'),
            props: true,
          },
          { path: 'traceability', name: 'traceability', component: () => import('./views/TraceabilityView.vue') },
          { path: 'settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
          { path: 'export', name: 'export', component: () => import('./views/ExportView.vue') },
          { path: 'baselines', name: 'baselines', component: () => import('./views/BaselinesView.vue'), meta: { perm: P.requirementRead } },
        ],
      },
      { path: 'search', name: 'search', component: () => import('./views/SearchView.vue') },
      {
        path: 'admin',
        component: () => import('./views/admin/AdminLayout.vue'),
        children: [
          { path: '', name: 'admin', redirect: { name: 'members' } },
          { path: 'members', name: 'members', component: () => import('./views/admin/MembersView.vue'), meta: { perm: P.memberRead } },
          { path: 'tokens', name: 'tokens', component: () => import('./views/admin/TokensView.vue'), meta: { perm: P.tokenManage } },
          { path: 'audit', name: 'audit', component: () => import('./views/admin/AuditView.vue'), meta: { perm: P.auditRead } },
          { path: 'tenants', name: 'tenants', component: () => import('./views/admin/TenantsView.vue'), meta: { superAdmin: true } },
          { path: 'users', name: 'users', component: () => import('./views/admin/UsersView.vue'), meta: { superAdmin: true } },
        ],
      },
      { path: 'profile', name: 'profile', component: () => import('./views/ProfileView.vue') },
      { path: ':pathMatch(.*)*', name: 'notfound', component: () => import('./views/NotFoundView.vue') },
    ],
  },
]

export const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.loaded) await auth.fetchMe()
  if (to.meta.public) {
    if (to.name === 'login' && auth.me) return { path: (to.query.redirect as string) || '/' }
    return true
  }
  if (!auth.me) return { name: 'login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : {} }
  if (to.name === 'home') return await homeRoute()
  if (to.meta.superAdmin && !auth.me.user.super_admin) return { name: 'projects' }
  if (typeof to.meta.perm === 'string' && !auth.can(to.meta.perm)) return { name: 'projects' }
  return true
})

/** Start page from the user's preferences (start_page). */
async function homeRoute() {
  const prefs = usePrefsStore()
  const p = prefs.prefs
  const needsProject = p.start_page === 'last_project' || p.start_page === 'default_project'
  let projectIds: string[] | null = null
  if (needsProject) {
    try {
      projectIds = asItems(await api.get<{ items: Project[] }>('/projects')).map((x) => x.id)
    } catch {
      projectIds = null
    }
  }
  return startRoute(p, { lastProjectId: prefs.lastProject(), projectIds })
}
