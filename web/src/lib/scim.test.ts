import { describe, expect, it } from 'vitest'
import en from '@/i18n/locales/en'
import sv from '@/i18n/locales/sv'
import { SCIM_EVENTS, scimActor, scimEvent, scimEventSeverity, sortGroups, tokenExpired, withNone, type ScimGroup } from './scim'

describe('scim events', () => {
  it('maps audit actions to labels', () => {
    expect(scimEvent('scim.user.create')).toBe('user_create')
    expect(scimEvent('scim.group.role')).toBe('group_role')
    expect(scimEvent('scim.settings')).toBe('settings')
    expect(scimEvent('scim.user.explode')).toBe('other')
    expect(scimEvent('member.add')).toBe('other')
    expect(scimEvent('user.create')).toBe('other')
  })
  it('has a label for every event in both languages', () => {
    for (const e of [...SCIM_EVENTS, 'other']) {
      expect(en.provisioning.events[e as keyof typeof en.provisioning.events]).toBeTruthy()
      expect(sv.provisioning.events[e as keyof typeof sv.provisioning.events]).toBeTruthy()
    }
  })
  it('colours events', () => {
    expect(scimEventSeverity('scim.user.create')).toBe('success')
    expect(scimEventSeverity('scim.user.reactivate')).toBe('success')
    expect(scimEventSeverity('scim.group.delete')).toBe('danger')
    expect(scimEventSeverity('scim.user.deactivate')).toBe('warn')
    expect(scimEventSeverity('scim.group.role')).toBe('info')
    expect(scimEventSeverity('scim.settings')).toBe('info')
    expect(scimEventSeverity('scim.user.update')).toBe('secondary')
    expect(scimEventSeverity('other')).toBe('secondary')
  })
  it('names the actor', () => {
    expect(scimActor('SCIM: Entra ID')).toEqual({ scim: true, name: 'Entra ID' })
    expect(scimActor('admin@example.com')).toEqual({ scim: false, name: 'admin@example.com' })
  })
})

describe('scim tokens and groups', () => {
  it('detects expired tokens', () => {
    const now = Date.parse('2026-06-01T00:00:00Z')
    expect(tokenExpired({ expires_at: null }, now)).toBe(false)
    expect(tokenExpired({}, now)).toBe(false)
    expect(tokenExpired({ expires_at: '2026-05-31T00:00:00Z' }, now)).toBe(true)
    expect(tokenExpired({ expires_at: '2026-06-02T00:00:00Z' }, now)).toBe(false)
    expect(tokenExpired({ expires_at: '2999-01-01T00:00:00Z' })).toBe(false)
  })
  it('lists unmapped groups first', () => {
    const g = (display_name: string, role: string): ScimGroup => ({ id: display_name, display_name, external_id: '', role, members: 0 })
    expect(sortGroups([g('b', 'tester'), g('z', ''), g('a', 'admin'), g('c', '')]).map((x) => x.display_name)).toEqual(['c', 'z', 'a', 'b'])
  })
  it('adds a none option', () => {
    expect(withNone([{ label: 'Admin', value: 'admin' }, { label: 'x', value: '' }], 'None')).toEqual([
      { label: 'None', value: '' },
      { label: 'Admin', value: 'admin' },
    ])
  })
})
