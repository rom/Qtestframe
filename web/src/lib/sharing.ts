// Pure helpers for project sharing (per-project access, internal/api/access.go).
import { can, rolesOf } from './permissions'
import type { Me, MemberCandidate, Project, ProjectMember } from './types'

/** Whether the user holds perm in a project of a list (its my_access), e.g. on the projects page. */
export function canInProject(me: Me | null | undefined, project: Pick<Project, 'my_access'> | null | undefined, perm: string | string[]): boolean {
  return can(me, perm, project?.my_access ?? null)
}

/** Whether the project is restricted to its members. */
export function isRestricted(project: Pick<Project, 'access'> | null | undefined): boolean {
  return project?.access === 'restricted'
}

/** Whether the user may change the project's sharing. */
export function canShare(me: Me | null | undefined, project: Pick<Project, 'my_access'> | null | undefined): boolean {
  return !!me?.user?.super_admin || !!project?.my_access?.can_share
}

/** Display name of a member or candidate: the name, else the e-mail. */
export function memberName(m: Pick<ProjectMember | MemberCandidate, 'display_name' | 'email' | 'id'>): string {
  return m.display_name || m.email || m.id
}

/** Members matching a filter text (e-mail, name or one of the roles; label gives a role's display name). */
export function filterMembers<T extends Pick<ProjectMember, 'display_name' | 'email' | 'role' | 'roles'>>(list: T[], text: string, label: (role: string) => string = (r) => r): T[] {
  const f = text.trim().toLowerCase()
  if (!f) return list
  return list.filter((m) => {
    const roles = rolesOf(m)
    return `${m.email ?? ''} ${m.display_name ?? ''} ${roles.join(' ')} ${roles.map(label).join(' ')}`.toLowerCase().includes(f)
  })
}

/**
 * Whether the caller may change or remove a project member: every role the
 * member holds is one the caller may give (internal/api/sharing.go
 * checkProjectGrant).
 */
export function mayManageMember(m: Pick<ProjectMember, 'role' | 'roles'>, grantable: readonly string[]): boolean {
  const roles = rolesOf(m)
  return roles.length > 0 && roles.every((r) => grantable.includes(r))
}

/** Roles of a member the caller may not give (shown locked in the role picker). */
export function lockedRoles(m: Pick<ProjectMember, 'role' | 'roles'>, grantable: readonly string[]): string[] {
  return rolesOf(m).filter((r) => !grantable.includes(r))
}

/** Sort order: users before groups, then by name. */
export function sortMembers<T extends Pick<ProjectMember, 'type' | 'display_name' | 'email' | 'id'>>(list: T[]): T[] {
  return [...list].sort((a, b) => (a.type === b.type ? memberName(a).localeCompare(memberName(b)) : a.type === 'user' ? -1 : 1))
}

/** Whether removing these members would also remove the user themselves. */
export function includesSelf(ids: string[], userId: string | undefined): boolean {
  return !!userId && ids.includes(userId)
}
