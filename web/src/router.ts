import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useAuthStore } from './stores/auth'
import { P } from './lib/permissions'
import { api, asItems } from './lib/api'
import { startRoute } from './lib/prefs'
import { usePrefsStore } from './stores/prefs'
import type { Project } from './lib/types'
import { watch } from 'vue'
import { i18n, t } from './i18n'

const routes: RouteRecordRaw[] = [
  { path: '/login', name: 'login', component: () => import('./views/LoginView.vue'), meta: { public: true } },
  // e-mail invitation: choose name and password, or sign in with single sign-on
  { path: '/invite/:token', name: 'invite', component: () => import('./views/InviteView.vue'), meta: { public: true } },
  // forced password change after an administrator reset (see beforeEach)
  { path: '/change-password', name: 'changePassword', component: () => import('./views/ChangePasswordView.vue'), meta: { passwordChange: true } },
  // forced set-up of a second factor required by the MFA policy (see beforeEach)
  { path: '/mfa-setup', name: 'mfaSetup', component: () => import('./views/MfaSetupView.vue'), meta: { mfaEnrol: true } },
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
          { path: 'releases', name: 'releases', component: () => import('./views/ReleasesView.vue'), meta: { perm: P.projectRead } },
          { path: 'defects', name: 'defects', component: () => import('./views/DefectsView.vue'), meta: { perm: P.projectRead } },
          { path: 'risks', name: 'risks', component: () => import('./views/RisksView.vue'), meta: { perm: P.requirementRead } },
          { path: 'traceability', name: 'traceability', component: () => import('./views/TraceabilityView.vue') },
          { path: 'graph', name: 'graph', component: () => import('./views/GraphView.vue') },
          { path: 'analytics', name: 'analytics', component: () => import('./views/AnalyticsView.vue'), meta: { perm: P.requirementRead } },
          { path: 'prompts', name: 'prompts', component: () => import('./views/PromptsView.vue') },
          { path: 'flow', name: 'flow', component: () => import('./views/FlowView.vue'), meta: { perm: P.testRead } },
          { path: 'catalog', name: 'catalog', component: () => import('./views/CatalogView.vue') },
          { path: 'tags', name: 'tags', component: () => import('./views/TagsView.vue') },
          { path: 'settings', name: 'settings', component: () => import('./views/SettingsView.vue') },
          { path: 'export', name: 'export', component: () => import('./views/ExportView.vue') },
          { path: 'baselines', name: 'baselines', component: () => import('./views/BaselinesView.vue'), meta: { perm: P.requirementRead } },
          { path: 'documents', name: 'documents', component: () => import('./views/DocumentsView.vue') },
          { path: 'reports', name: 'reports', component: () => import('./views/ReportsView.vue') },
        ],
      },
      { path: 'notifications', name: 'notifications', component: () => import('./views/NotificationsView.vue') },
      { path: 'reviews', name: 'reviews', component: () => import('./views/ReviewsView.vue') },
      { path: 'search', name: 'search', component: () => import('./views/SearchView.vue') },
      { path: 'go/:key', name: 'go', component: () => import('./views/GoView.vue') },
      {
        path: 'admin',
        component: () => import('./views/admin/AdminLayout.vue'),
        children: [
          { path: '', name: 'admin', redirect: { name: 'members' } },
          { path: 'members', name: 'members', component: () => import('./views/admin/MembersView.vue'), meta: { perm: P.memberRead } },
          { path: 'tokens', name: 'tokens', component: () => import('./views/admin/TokensView.vue'), meta: { perm: P.tokenManage } },
          { path: 'provisioning', name: 'provisioning', component: () => import('./views/admin/ProvisioningView.vue'), meta: { perm: P.tenantManage } },
          { path: 'audit', name: 'audit', component: () => import('./views/admin/AuditView.vue'), meta: { perm: P.auditRead } },
          { path: 'tenants', name: 'tenants', component: () => import('./views/admin/TenantsView.vue'), meta: { superAdmin: true } },
          { path: 'users', name: 'users', component: () => import('./views/admin/UsersView.vue'), meta: { superAdmin: true } },
          { path: 'roles', name: 'rbac', component: () => import('./views/admin/RbacView.vue') },
          { path: 'execution', name: 'executionSettings', component: () => import('./views/admin/ExecutionSettingsView.vue'), meta: { perm: P.settingsManage } },
          { path: 'mfa', name: 'mfaPolicy', component: () => import('./views/admin/MfaPolicyView.vue'), meta: { perm: P.tenantManage } },
          { path: 'backup', name: 'backup', component: () => import('./views/admin/BackupView.vue'), meta: { superAdmin: true } },
          // runtime server configuration (OIDC, syslog, SMTP, security, runner); links the MFA page (route mfaPolicy)
          { path: 'system', name: 'systemSettings', component: () => import('./views/admin/SystemSettingsView.vue'), meta: { superAdmin: true } },
        ],
      },
      { path: 'profile', name: 'profile', component: () => import('./views/ProfileView.vue') },
      // user guide (src/guide); its page title titles.help comes from i18n/locales/guide.*.ts
      { path: 'help/:page?', component: () => import('./guide/HelpView.vue'), name: 'help' },
      { path: ':pathMatch(.*)*', name: 'notfound', component: () => import('./views/NotFoundView.vue') },
    ],
  },
]

export const router = createRouter({ history: createWebHistory(), routes })

/** Page title from the route name (titles.<name>), in the active language. */
export function pageTitle(name: unknown): string {
  const key = `titles.${String(name ?? '')}`
  const app = t('app.name')
  return name && i18n.global.te(key) ? `${t(key)} · ${app}` : app
}

function updateTitle() {
  if (typeof document !== 'undefined') document.title = pageTitle(router.currentRoute.value.name)
}
router.afterEach(updateTitle)
watch(i18n.global.locale, updateTitle)

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (!auth.loaded) await auth.fetchMe()
  if (to.meta.public) {
    if (to.name === 'login' && auth.me) return { path: (to.query.redirect as string) || '/' }
    return true
  }
  if (!auth.me) return { name: 'login', query: to.fullPath !== '/' ? { redirect: to.fullPath } : {} }
  // a reset password must be changed before anything else
  if (auth.me.user?.must_change_password) return to.meta.passwordChange ? true : { name: 'changePassword' }
  if (to.meta.passwordChange) return { path: '/' }
  // then a second factor, when the MFA policy requires one now
  if (auth.me.mfa?.enrolment_required) return to.meta.mfaEnrol ? true : { name: 'mfaSetup' }
  if (to.meta.mfaEnrol) return { path: '/' }
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
