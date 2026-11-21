import { describe, expect, it } from 'vitest'
import { can, PROJECT_PERMISSIONS } from './permissions'
import { canInProject, canShare, filterMembers, includesSelf, isRestricted, memberName, sortMembers } from './sharing'
import type { Me, ProjectAccess, ProjectMember } from './types'

const me = (permissions: string[], superAdmin = false) => ({ user: { id: 'u1', super_admin: superAdmin }, permissions }) as unknown as Me
const access = (permissions: string[], canShare = false): ProjectAccess => ({ source: 'member', role: 'viewer', permissions, can_share: canShare })
const member = (x: Partial<ProjectMember>): ProjectMember => ({ project_id: 'p', type: 'user', id: 'x', role: 'viewer', created_at: '', updated_at: '', ...x })

describe('project permissions', () => {
  it('uses the project access for project permissions', () => {
    const m = me(['project.read', 'requirement.read', 'requirement.write', 'audit.read'])
    expect(can(m, 'requirement.write')).toBe(true)
    // a viewer in the project: no write there
    expect(can(m, 'requirement.write', access(['project.read', 'requirement.read']))).toBe(false)
    // organisation permissions stay those of the organisation role
    expect(can(m, 'audit.read', access(['project.read']))).toBe(true)
    // raised in the project
    expect(can(me(['project.read']), ['test.write', 'project.read'], access(['project.read', 'test.write']))).toBe(true)
    expect(can(me([], true), 'project.delete', access([]))).toBe(true)
    expect(can(null, 'project.read')).toBe(false)
  })
  it('knows the project-scoped permissions', () => {
    expect(PROJECT_PERMISSIONS.has('run.execute')).toBe(true)
    expect(PROJECT_PERMISSIONS.has('member.manage')).toBe(false)
    expect(PROJECT_PERMISSIONS.has('project.create')).toBe(false)
  })
  it('checks a project of a list', () => {
    const m = me(['project.read', 'project.delete'])
    expect(canInProject(m, { my_access: access(['project.read']) }, 'project.delete')).toBe(false)
    expect(canInProject(m, {}, 'project.delete')).toBe(true)
    expect(canInProject(m, null, 'project.delete')).toBe(true)
  })
  it('tells restricted projects and who may share', () => {
    expect(isRestricted({ access: 'restricted' })).toBe(true)
    expect(isRestricted({ access: 'organisation' })).toBe(false)
    expect(isRestricted(null)).toBe(false)
    expect(canShare(me([]), { my_access: access([], true) })).toBe(true)
    expect(canShare(me([]), { my_access: access([]) })).toBe(false)
    expect(canShare(me([], true), {})).toBe(true)
  })
})

describe('members', () => {
  const list = [member({ id: 'g', type: 'group', display_name: 'QA' }), member({ id: 'b', email: 'bo@x.se', role: 'tester' }), member({ id: 'a', display_name: 'Anna', email: 'anna@x.se' })]
  it('names, filters and sorts', () => {
    expect(memberName(list[1])).toBe('bo@x.se')
    expect(memberName(list[2])).toBe('Anna')
    expect(memberName({ id: 'z' })).toBe('z')
    expect(filterMembers(list, 'tester').map((m) => m.id)).toEqual(['b'])
    expect(filterMembers(list, ' ')).toHaveLength(3)
    expect(sortMembers(list).map((m) => m.id)).toEqual(['a', 'b', 'g'])
    expect(sortMembers([list[0], list[0]]).map((m) => m.id)).toEqual(['g', 'g'])
  })
  it('notices oneself in a selection', () => {
    expect(includesSelf(['a', 'u1'], 'u1')).toBe(true)
    expect(includesSelf(['a'], 'u1')).toBe(false)
    expect(includesSelf(['a'], undefined)).toBe(false)
  })
})
