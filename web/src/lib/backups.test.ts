import { describe, expect, it } from 'vitest'
import { hasFile, isRunning, retentionId, runSeconds, scheduleParts, shortHash, statusSeverity, verifyState, type BackupRun } from './backups'

function run(p: Partial<BackupRun> = {}): BackupRun {
  return {
    id: 'r1',
    kind: 'schedule',
    status: 'succeeded',
    started_by: 'system',
    started_at: '2026-11-24T02:00:00Z',
    finished_at: '2026-11-24T02:00:42Z',
    file_name: 'qtestframe-backup-20261124-020000.zip',
    size: 1024,
    sha256: 'abcdef0123456789abcdef',
    encrypted: false,
    remote_key: '',
    rows: 10,
    schema_version: 65,
    error: '',
    verify_error: '',
    ...p,
  }
}

describe('backups', () => {
  it('reads schedules', () => {
    expect(scheduleParts('daily 02:00')).toEqual({ kind: 'daily', time: '02:00' })
    expect(scheduleParts(' Daily 2:30 ')).toEqual({ kind: 'daily', time: '02:30' })
    expect(scheduleParts('weekly mon 03:15')).toEqual({ kind: 'weekly', day: 1, time: '03:15' })
    expect(scheduleParts('weekly sun 00:00')).toEqual({ kind: 'weekly', day: 0, time: '00:00' })
    expect(scheduleParts('every 6h')).toEqual({ kind: 'every', every: '6h' })
    expect(scheduleParts('every 1h30m')).toEqual({ kind: 'every', every: '1h30m' })
    for (const bad of ['', 'daily', 'weekly xyz 02:00', 'every', 'every 5s', 'hourly', 'daily 2']) expect(scheduleParts(bad).kind).toBe('invalid')
  })
  it('describes runs', () => {
    expect(statusSeverity('running')).toBe('info')
    expect(statusSeverity('succeeded')).toBe('success')
    expect(statusSeverity('failed')).toBe('danger')
    expect(hasFile(run())).toBe(true)
    expect(hasFile(run({ status: 'running' }))).toBe(false)
    expect(hasFile(run({ file_name: '' }))).toBe(false)
    expect(hasFile(run({ pruned_at: '2026-11-25T02:00:00Z' }))).toBe(false)
    // a failed upload keeps the file
    expect(hasFile(run({ status: 'failed' }))).toBe(true)
    expect(runSeconds(run())).toBe(42)
    expect(runSeconds(run({ finished_at: undefined }))).toBeNull()
    expect(runSeconds(run({ finished_at: 'nope' }))).toBeNull()
    expect(shortHash('abcdef0123456789abcdef')).toBe('abcdef012345…')
    expect(shortHash('abc')).toBe('abc')
    expect(verifyState(run())).toBe('none')
    expect(verifyState(run({ verified_at: '2026-11-24T03:00:00Z' }))).toBe('ok')
    expect(verifyState(run({ verified_at: '2026-11-24T03:00:00Z', verify_error: 'damaged' }))).toBe('failed')
  })
  it('names the retention rules and polls while a backup runs', () => {
    expect(retentionId(0, 0)).toBe('none')
    expect(retentionId(7, 0)).toBe('last')
    expect(retentionId(0, 30)).toBe('days')
    expect(retentionId(7, 30)).toBe('both')
    const settings = { enabled: true, schedule: 'daily 02:00', directory: '/b', keep_last: 7, keep_days: 0, encrypted: false, s3_enabled: false }
    expect(isRunning(null)).toBe(false)
    expect(isRunning({ settings, running: false, runs: [run()] })).toBe(false)
    expect(isRunning({ settings, running: true, runs: [] })).toBe(true)
    expect(isRunning({ settings, running: false, runs: [run({ status: 'running' })] })).toBe(true)
  })
})
