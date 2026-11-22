import { describe, expect, it } from 'vitest'
import { canSeeAdminGuide } from './access'
import type { Me } from '@/lib/types'

const me = (superAdmin: boolean, permissions: string[]) => ({ user: { super_admin: superAdmin }, permissions }) as unknown as Me

describe('canSeeAdminGuide', () => {
  it('offers the administrator guide to super admins and organisation admins only', () => {
    expect(canSeeAdminGuide(me(true, []))).toBe(true)
    expect(canSeeAdminGuide(me(false, ['tenant.manage', 'project.read']))).toBe(true)
    expect(canSeeAdminGuide(me(false, ['project.read', 'member.manage', 'audit.read']))).toBe(false)
    expect(canSeeAdminGuide(null)).toBe(false)
  })
})
