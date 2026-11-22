import { describe, expect, it } from 'vitest'
import { projectCreator } from './projectCreator'

describe('projectCreator', () => {
  it('uses the display name, then the e-mail', () => {
    expect(projectCreator({ created_by: 'u1', created_by_name: 'Ada', created_by_email: 'ada@example.com' })).toEqual({ name: 'Ada', email: 'ada@example.com' })
    expect(projectCreator({ created_by: 'u1', created_by_name: '', created_by_email: 'ada@example.com' })).toEqual({ name: 'ada@example.com', email: 'ada@example.com' })
  })
  it('is null when unknown or the account is gone', () => {
    expect(projectCreator({})).toBeNull()
    expect(projectCreator({ created_by: '' })).toBeNull()
    expect(projectCreator({ created_by: 'gone', created_by_name: '', created_by_email: '' })).toBeNull()
  })
})
