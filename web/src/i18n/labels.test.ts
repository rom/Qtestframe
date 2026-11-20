import { afterEach, describe, expect, it } from 'vitest'
import { setLanguage } from './index'
import { customRoleLabels, enumLabel, enumOptions, obligationHint, obligationKeyword, relationText } from './labels'

afterEach(() => setLanguage('en'))

describe('enum labels', () => {
  it('translate API values without changing them', () => {
    expect(enumLabel('result', 'not_run')).toBe('Not run')
    expect(enumLabel('status', 'in_progress')).toBe('In progress')
    setLanguage('sv')
    expect(enumLabel('result', 'passed')).toBe('Godkänd')
    expect(enumLabel('result', 'failed')).toBe('Underkänd')
    expect(enumLabel('result', 'blocked')).toBe('Blockerad')
    expect(enumLabel('result', 'skipped')).toBe('Överhoppad')
    expect(enumLabel('result', 'not_run')).toBe('Ej körd')
    expect(enumLabel('priority', 'high')).toBe('Hög')
    expect(enumLabel('role', 'viewer')).toBe('Läsare')
    expect(enumOptions('executionType', ['manual', 'automated'])).toEqual([
      { label: 'Manuellt', value: 'manual' },
      { label: 'Automatiserat', value: 'automated' },
    ])
  })
  it('humanize unknown values', () => {
    expect(enumLabel('status', 'on_hold')).toBe('On hold')
    expect(enumLabel('status', '')).toBe('')
    expect(enumLabel('status', null)).toBe('')
    expect(enumOptions('priority', null)).toEqual([])
  })
})

describe('obligations', () => {
  it('keep the RFC 2119 keyword and explain it', () => {
    expect(obligationKeyword('must_not')).toBe('MUST NOT')
    expect(obligationKeyword(null)).toBe('')
    expect(obligationHint('must')).toBe('Absolute requirement')
    setLanguage('sv')
    expect(obligationHint('must')).toBe('SKA (krav)')
    expect(obligationHint('should')).toContain('BÖR')
    expect(obligationHint('may')).toContain('FÅR')
    expect(obligationHint('nonsense')).toBe('')
    expect(obligationHint(undefined)).toBe('')
  })
})

describe('relations', () => {
  it('label both directions', () => {
    expect(relationText('depends_on', 'outgoing')).toBe('depends on')
    expect(relationText('depends_on', 'incoming')).toBe('required by')
    setLanguage('sv')
    expect(relationText('depends_on', 'outgoing')).toBe('beror på')
    expect(relationText('refines', 'incoming')).toBe('förfinas av (underordnat)')
    expect(relationText('blocks', 'incoming')).toBe('incoming blocks')
  })
})

describe('role labels', () => {
  it('uses translations for built-in roles and stored labels for custom roles', () => {
    setLanguage('en')
    expect(enumLabel('role', 'test_lead')).toBe('Test lead')
    customRoleLabels.set('auditor', 'External auditor')
    expect(enumLabel('role', 'auditor')).toBe('External auditor')
    customRoleLabels.clear()
    expect(enumLabel('role', 'auditor')).toBe('Auditor')
  })
})
