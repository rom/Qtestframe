import { describe, expect, it } from 'vitest'
import { can, canAny, diffRoles, grantableRoles, normalizeRoles, P, permissionsOfRoles, roleRank, rolesOf, sameRoles } from './permissions'
import type { Me } from './types'

function me(perms: string[], superAdmin = false, role = 'tester'): Me {
  return {
    user: { id: 'u', email: 'a@b', display_name: 'A', super_admin: superAdmin, auth_provider: 'local', disabled: false, created_at: '' },
    tenant: null,
    role,
    permissions: perms,
    memberships: [],
    csrf_token: '',
  }
}

describe('can', () => {
  it('returns false without user', () => {
    expect(can(null, P.projectRead)).toBe(false)
    expect(can(undefined, P.projectRead)).toBe(false)
  })
  it('checks single and multiple permissions', () => {
    const m = me([P.projectRead, P.testWrite])
    expect(can(m, P.projectRead)).toBe(true)
    expect(can(m, P.projectDelete)).toBe(false)
    expect(can(m, [P.projectRead, P.testWrite])).toBe(true)
    expect(can(m, [P.projectRead, P.runWrite])).toBe(false)
  })
  it('super admins hold everything', () => {
    expect(can(me([], true), P.tenantManage)).toBe(true)
  })
  it('tolerates a missing permissions list', () => {
    const m = me([])
    // @ts-expect-error simulate null from API
    m.permissions = null
    expect(can(m, P.projectRead)).toBe(false)
  })
  it('canAny', () => {
    expect(canAny(me([P.auditRead]), [P.memberManage, P.auditRead])).toBe(true)
    expect(canAny(me([]), [P.memberManage])).toBe(false)
  })
})

describe('roles', () => {
  it('ranks roles', () => {
    expect(roleRank('admin')).toBe(7)
    expect(roleRank('test_lead')).toBe(5)
    expect(roleRank('sme')).toBe(3)
    expect(roleRank('viewer')).toBe(1)
    expect(roleRank('nobody')).toBe(0)
  })
  it('grantable roles never exceed own role', () => {
    expect(grantableRoles('tester')).toEqual(['tester', 'sme', 'reviewer', 'viewer'])
    expect(grantableRoles('viewer', true)).toEqual(['admin', 'manager', 'test_lead', 'tester', 'sme', 'reviewer', 'viewer'])
    expect(grantableRoles('')).toEqual([])
    // several roles: the strongest counts
    expect(grantableRoles(['viewer', 'sme'])).toEqual(['sme', 'reviewer', 'viewer'])
    expect(grantableRoles([])).toEqual([])
  })
  it('normalises role sets like rbac.NormalizeRoles', () => {
    expect(normalizeRoles(['tester', ' auditor', 'sme', '', 'admin', 'tester', 'aaa', null])).toEqual(['admin', 'tester', 'sme', 'aaa', 'auditor'])
    expect(normalizeRoles(undefined)).toEqual([])
  })
  it('reads the roles of members of old and new servers', () => {
    expect(rolesOf({ role: 'tester' })).toEqual(['tester'])
    expect(rolesOf({ role: 'tester', roles: ['sme', 'tester'] })).toEqual(['tester', 'sme'])
    expect(rolesOf({ role: 'tester', roles: [] })).toEqual(['tester'])
    expect(rolesOf({ role: '' })).toEqual([])
    expect(rolesOf(null)).toEqual([])
  })
  it('compares and diffs role sets', () => {
    expect(sameRoles(['sme', 'tester'], ['tester', 'sme'])).toBe(true)
    expect(sameRoles(['sme'], ['sme', 'viewer'])).toBe(false)
    expect(diffRoles(['viewer', 'tester'], ['tester', 'sme'])).toEqual({ added: ['sme'], removed: ['viewer'] })
  })
  it('unions the permissions of several roles', () => {
    const matrix = { sme: ['requirement.write', 'project.read'], tester: ['run.execute', 'project.read'] }
    expect(permissionsOfRoles(matrix, ['sme', 'tester'])).toEqual(['project.read', 'requirement.write', 'run.execute'])
    expect(permissionsOfRoles(matrix, ['nobody'])).toEqual([])
    expect(permissionsOfRoles(null, ['sme'])).toEqual([])
  })
  it('lists roles within the own permissions', async () => {
    const { rolesWithin } = await import('./permissions')
    const roles = [
      { name: 'viewer', permissions: ['project.read'] },
      { name: 'auditor', permissions: ['project.read', 'audit.read'] },
      { name: 'empty', permissions: null },
    ]
    expect(rolesWithin(roles, ['project.read'])).toEqual(['viewer', 'empty'])
    expect(rolesWithin(roles, ['project.read', 'audit.read'])).toEqual(['viewer', 'auditor', 'empty'])
    expect(rolesWithin(roles, [], true)).toEqual(['viewer', 'auditor', 'empty'])
  })
})

describe('role matrix helpers', () => {
  it('turns permissions into message keys', async () => {
    const { permissionKey } = await import('./permissions')
    expect(permissionKey('member.read')).toBe('member_read')
    expect(permissionKey('export')).toBe('export')
  })
  it('orders known permissions first', async () => {
    const { matrixPermissions } = await import('./permissions')
    expect(matrixPermissions({ admin: ['zzz.custom', 'export', 'member.read'], viewer: null as unknown as string[] })).toEqual(['member.read', 'export', 'zzz.custom'])
    expect(matrixPermissions(null)).toEqual([])
  })
})
