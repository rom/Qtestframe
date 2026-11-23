import { describe, expect, it } from 'vitest'
import type { User } from '@/lib/types'
import { atLimit, blankPolicy, blankUserFilter, deleteConfirmed, filterUsers, lastSuperAdmin, methodOptions, parseDomains, quotaText, storageAtLimit, userProviders } from './tenantAdmin'

function user(p: Partial<User>): User {
  return { id: p.email ?? 'u', email: 'u@example.com', display_name: '', super_admin: false, auth_provider: 'local', disabled: false, created_at: '', ...p }
}

describe('organisation administration', () => {
  it('shows quotas and their limits', () => {
    expect(quotaText(3, 0)).toBe('3')
    expect(quotaText(3, 10)).toBe('3 / 10')
    expect(atLimit(10, 10)).toBe(true)
    expect(atLimit(9, 10)).toBe(false)
    expect(atLimit(100, 0)).toBe(false)
    expect(storageAtLimit(5 * 1024 * 1024, 5)).toBe(true)
    expect(storageAtLimit(5 * 1024 * 1024 - 1, 5)).toBe(false)
    expect(storageAtLimit(1e12, 0)).toBe(false)
    expect(blankPolicy()).toEqual({ max_members: 0, max_projects: 0, max_storage_mb: 0, ai_disabled: false, allowed_providers: [], email_domains: [], note: '' })
  })
  it('parses e-mail domains and offers the sign-in methods', () => {
    expect(parseDomains(' @Example.com, b.example.org\nexample.com ;c.se ')).toEqual(['example.com', 'b.example.org', 'c.se'])
    expect(parseDomains('')).toEqual([])
    expect(methodOptions({ oidc: [{ name: 'corp', display_name: 'Corporate' }], saml: [{ name: 'adfs', display_name: '' }] }, 'Password')).toEqual([
      { value: 'local', label: 'Password' },
      { value: 'oidc:corp', label: 'Corporate (OIDC)' },
      { value: 'saml:adfs', label: 'adfs (SAML)' },
    ])
  })
  it('needs the slug typed to delete', () => {
    expect(deleteConfirmed({ slug: 'acme' }, ' ACME ')).toBe(true)
    expect(deleteConfirmed({ slug: 'acme' }, 'acm')).toBe(false)
    expect(deleteConfirmed({ slug: 'acme' }, '')).toBe(false)
  })
})

describe('user administration', () => {
  const users = [
    user({ id: '1', email: 'root@example.com', super_admin: true }),
    user({ id: '2', email: 'ann@example.com', display_name: 'Ann Tester', mfa_enabled: true }),
    user({ id: '3', email: 'bob@corp.example', auth_provider: 'oidc:corp', disabled: true }),
    user({ id: '4', email: 'old@example.com', super_admin: true, disabled: true }),
  ]
  it('filters by text, status, super admin, sign-in method and MFA', () => {
    const f = blankUserFilter()
    expect(filterUsers(users, f).length).toBe(4)
    expect(filterUsers(users, { ...f, text: 'ann tes' }).map((u) => u.id)).toEqual(['2'])
    expect(filterUsers(users, { ...f, status: 'disabled' }).map((u) => u.id)).toEqual(['3', '4'])
    expect(filterUsers(users, { ...f, status: 'active' }).map((u) => u.id)).toEqual(['1', '2'])
    expect(filterUsers(users, { ...f, superAdmin: true }).map((u) => u.id)).toEqual(['1', '4'])
    expect(filterUsers(users, { ...f, provider: 'oidc:corp' }).map((u) => u.id)).toEqual(['3'])
    expect(filterUsers(users, { ...f, mfa: 'on' }).map((u) => u.id)).toEqual(['2'])
    expect(filterUsers(users, { ...f, mfa: 'off' }).length).toBe(3)
    expect(userProviders(users)).toEqual(['local', 'oidc:corp'])
  })
  it('protects the last enabled super administrator', () => {
    expect(lastSuperAdmin(users, users[0])).toBe(true)
    expect(lastSuperAdmin(users, users[1])).toBe(false)
    expect(lastSuperAdmin(users, users[3])).toBe(false)
    const two = [...users, user({ id: '5', super_admin: true })]
    expect(lastSuperAdmin(two, two[0])).toBe(false)
  })
})
