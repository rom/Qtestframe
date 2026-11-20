import { describe, expect, it } from 'vitest'
import { can, canAny, grantableRoles, P, roleRank } from './permissions'
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
    expect(roleRank('admin')).toBe(5)
    expect(roleRank('test_lead')).toBe(3)
    expect(roleRank('viewer')).toBe(1)
    expect(roleRank('nobody')).toBe(0)
  })
  it('grantable roles never exceed own role', () => {
    expect(grantableRoles('tester')).toEqual(['tester', 'viewer'])
    expect(grantableRoles('viewer', true)).toEqual(['admin', 'manager', 'test_lead', 'tester', 'viewer'])
    expect(grantableRoles('')).toEqual([])
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
