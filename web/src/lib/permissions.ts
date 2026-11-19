// Permission helpers mirroring internal/rbac.
import type { Me } from './types'

export const P = {
  tenantManage: 'tenant.manage',
  memberRead: 'member.read',
  memberManage: 'member.manage',
  projectRead: 'project.read',
  projectCreate: 'project.create',
  projectUpdate: 'project.update',
  projectDelete: 'project.delete',
  requirementRead: 'requirement.read',
  requirementWrite: 'requirement.write',
  testRead: 'test.read',
  testWrite: 'test.write',
  runRead: 'run.read',
  runWrite: 'run.write',
  runExecute: 'run.execute',
  scriptExecute: 'script.execute',
  export: 'export',
  auditRead: 'audit.read',
  tokenManage: 'token.manage',
} as const

export type Permission = (typeof P)[keyof typeof P]

/** Roles from most to least privileged (rbac.Roles). */
export const ROLES = ['admin', 'manager', 'tester', 'viewer'] as const

/**
 * Whether the current user holds a permission. Super admins hold every
 * permission. Pass an array to require all of them.
 */
export function can(me: Me | null | undefined, perm: string | string[]): boolean {
  if (!me) return false
  if (me.user?.super_admin) return true
  const perms = me.permissions ?? []
  const want = Array.isArray(perm) ? perm : [perm]
  return want.every((p) => perms.includes(p))
}

/** Whether any of the permissions is held. */
export function canAny(me: Me | null | undefined, perms: string[]): boolean {
  return perms.some((p) => can(me, p))
}

/** Privilege rank of a role (higher = more privileged, unknown = 0). */
export function roleRank(role: string): number {
  const i = (ROLES as readonly string[]).indexOf(role)
  return i < 0 ? 0 : ROLES.length - i
}

/** Roles that may be granted by someone holding `role` (e.g. for API tokens). */
export function grantableRoles(role: string, superAdmin = false): string[] {
  if (superAdmin) return [...ROLES]
  const r = roleRank(role)
  return ROLES.filter((x) => roleRank(x) <= r)
}
