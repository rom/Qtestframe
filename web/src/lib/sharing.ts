// Pure helpers for project sharing (per-project access, internal/api/access.go).
import { can } from './permissions'
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

/** Members matching a filter text (e-mail, name or role). */
export function filterMembers<T extends Pick<ProjectMember, 'display_name' | 'email' | 'role'>>(list: T[], text: string): T[] {
  const f = text.trim().toLowerCase()
  if (!f) return list
  return list.filter((m) => `${m.email ?? ''} ${m.display_name ?? ''} ${m.role}`.toLowerCase().includes(f))
}

/** Sort order: users before groups, then by name. */
export function sortMembers<T extends Pick<ProjectMember, 'type' | 'display_name' | 'email' | 'id'>>(list: T[]): T[] {
  return [...list].sort((a, b) => (a.type === b.type ? memberName(a).localeCompare(memberName(b)) : a.type === 'user' ? -1 : 1))
}

/** Whether removing these members would also remove the user themselves. */
export function includesSelf(ids: string[], userId: string | undefined): boolean {
  return !!userId && ids.includes(userId)
}
