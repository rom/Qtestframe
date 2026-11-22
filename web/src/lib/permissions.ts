// Permission helpers mirroring internal/rbac.
import type { Me, ProjectAccess } from './types'

export const P = {
  tenantManage: 'tenant.manage',
  memberRead: 'member.read',
  memberManage: 'member.manage',
  roleManage: 'role.manage',
  settingsManage: 'settings.manage',
  projectRead: 'project.read',
  projectCreate: 'project.create',
  projectUpdate: 'project.update',
  projectDelete: 'project.delete',
  requirementRead: 'requirement.read',
  requirementWrite: 'requirement.write',
  requirementReview: 'requirement.review',
  waiverApprove: 'waiver.approve',
  changeCreate: 'change.create',
  changeApprove: 'change.approve',
  testRead: 'test.read',
  testWrite: 'test.write',
  testReview: 'test.review',
  runRead: 'run.read',
  runWrite: 'run.write',
  runExecute: 'run.execute',
  commentCreate: 'comment.create',
  cyclePlan: 'cycle.plan',
  scriptExecute: 'script.execute',
  export: 'export',
  auditRead: 'audit.read',
  tokenManage: 'token.manage',
} as const

export type Permission = (typeof P)[keyof typeof P]

/** Roles from most to least privileged (rbac.Roles). */
export const ROLES = ['admin', 'manager', 'test_lead', 'tester', 'sme', 'reviewer', 'viewer'] as const

/**
 * Normalises a role set like rbac.NormalizeRoles: drops empty names and
 * duplicates, puts admin and the other built-in roles first (most
 * privileged first), then custom roles alphabetically. The first role is
 * the primary role.
 */
export function normalizeRoles(roles: readonly (string | null | undefined)[] | null | undefined): string[] {
  const seen = new Set<string>()
  const builtin: string[] = []
  const custom: string[] = []
  for (const raw of roles ?? []) {
    const r = (raw ?? '').trim()
    if (!r || seen.has(r)) continue
    seen.add(r)
    if ((ROLES as readonly string[]).includes(r)) builtin.push(r)
    else custom.push(r)
  }
  builtin.sort((a, b) => roleRank(b) - roleRank(a))
  custom.sort()
  return [...builtin, ...custom]
}

/** The roles of a member, membership, token or invitation: `roles`, or the single `role` of older servers. */
export function rolesOf(x: { role?: string | null; roles?: readonly string[] | null } | null | undefined): string[] {
  if (!x) return []
  if (x.roles && x.roles.length) return normalizeRoles(x.roles)
  return x.role ? [x.role] : []
}

/** Whether two role sets hold the same roles (in any order). */
export function sameRoles(a: readonly string[], b: readonly string[]): boolean {
  const x = normalizeRoles(a)
  const y = normalizeRoles(b)
  return x.length === y.length && x.every((r, i) => r === y[i])
}

/** Roles added to and removed from a set. */
export function diffRoles(before: readonly string[], after: readonly string[]): { added: string[]; removed: string[] } {
  return {
    added: normalizeRoles(after).filter((r) => !before.includes(r)),
    removed: normalizeRoles(before).filter((r) => !after.includes(r)),
  }
}

/** Union of the permissions of several roles of a role matrix (sorted). */
export function permissionsOfRoles(matrix: Record<string, readonly string[] | null> | null | undefined, roles: readonly string[]): string[] {
  const out = new Set<string>()
  for (const r of roles) for (const p of matrix?.[r] ?? []) out.add(p)
  return [...out].sort()
}

/**
 * Whether the current user holds a permission. Super admins hold every
 * permission. Pass an array to require all of them.
 */
export function can(me: Me | null | undefined, perm: string | string[], access?: ProjectAccess | null): boolean {
  if (!me) return false
  if (me.user?.super_admin) return true
  const perms = me.permissions ?? []
  const want = Array.isArray(perm) ? perm : [perm]
  // inside a project its effective permissions replace the organisation
  // role's project permissions (project sharing, internal/api/access.go)
  return want.every((p) => (access && PROJECT_PERMISSIONS.has(p) ? access.permissions : perms).includes(p))
}

/** Permissions a project role governs (internal/api/access.go projectScoped); the others always come from the organisation role. */
export const PROJECT_PERMISSIONS: ReadonlySet<string> = new Set<string>([
  P.projectRead,
  P.projectUpdate,
  P.projectDelete,
  P.requirementRead,
  P.requirementWrite,
  P.requirementReview,
  P.waiverApprove,
  P.changeCreate,
  P.changeApprove,
  P.testRead,
  P.testWrite,
  P.testReview,
  P.runRead,
  P.runWrite,
  P.runExecute,
  P.cyclePlan,
  P.scriptExecute,
  P.export,
  P.commentCreate,
])

/** Whether any of the permissions is held. */
export function canAny(me: Me | null | undefined, perms: string[]): boolean {
  return perms.some((p) => can(me, p))
}

/** Privilege rank of a role (higher = more privileged, unknown = 0). */
export function roleRank(role: string): number {
  const i = (ROLES as readonly string[]).indexOf(role)
  return i < 0 ? 0 : ROLES.length - i
}

/**
 * Roles whose permissions are all held by the caller (who may then give
 * them to an API token). `roles` are the organisation's roles with their
 * permissions; super admins may grant any role.
 */
export function rolesWithin(roles: readonly { name: string; permissions: readonly string[] | null }[], mine: readonly string[], superAdmin = false): string[] {
  const have = new Set(mine)
  return roles.filter((r) => superAdmin || (r.permissions ?? []).every((p) => have.has(p))).map((r) => r.name)
}

/** Roles that may be granted by someone holding `role` (or several roles: the strongest counts; built-in roles only; see rolesWithin). */
export function grantableRoles(role: string | readonly string[], superAdmin = false): string[] {
  if (superAdmin) return [...ROLES]
  const held = typeof role === 'string' ? [role] : role
  const r = Math.max(0, ...held.map(roleRank))
  return ROLES.filter((x) => roleRank(x) <= r)
}

/** Message key suffix of a permission ("member.read" → "member_read"). */
export function permissionKey(perm: string): string {
  return perm.replace(/[^a-z0-9]+/gi, '_')
}

/** Every permission of a role matrix: the known ones in their order, then unknown ones sorted. */
export function matrixPermissions(matrix: Record<string, readonly string[] | null> | null | undefined): string[] {
  const known = Object.values(P) as string[]
  const seen = new Set(Object.values(matrix ?? {}).flatMap((x) => x ?? []))
  return [...known.filter((p) => seen.has(p)), ...[...seen].filter((p) => !known.includes(p)).sort()]
}
