import { describe, expect, it } from 'vitest'
import {
  bookable,
  bookingSeverity,
  classificationSeverity,
  cleanComponents,
  componentsText,
  defaultBookingWindow,
  duplicateComponents,
  environmentIcon,
  environmentSeverity,
  expiryState,
  fits,
  fromLocalInput,
  maxOverlap,
  parseTable,
  startOfWeek,
  timelineBars,
  timelineDays,
  toLocalInput,
  validWindow,
  type Booking,
} from './environments'

const bk = (id: string, from: string, to: string, status = 'approved'): Booking => ({
  id, environment_id: 'e', project_id: 'p', user_id: 'u', booked_by: 'u@example.com', starts_at: from, ends_at: to, purpose: '',
  run_id: null, cycle_id: null, session_id: null, status, decided_by: '', decided_at: null, decision_note: '', created_at: '', updated_at: '',
})
const h = (n: number) => `2030-01-01T${String(n).padStart(2, '0')}:00:00Z`

describe('environments', () => {
  it('colours and icons', () => {
    expect(environmentSeverity('available')).toBe('success')
    expect(environmentSeverity('in_use')).toBe('info')
    expect(environmentSeverity('maintenance')).toBe('warn')
    expect(environmentSeverity('broken')).toBe('danger')
    expect(environmentSeverity('retired')).toBe('secondary')
    expect(bookingSeverity('pending')).toBe('warn')
    expect(bookingSeverity('approved')).toBe('success')
    expect(bookingSeverity('rejected')).toBe('danger')
    expect(bookingSeverity('cancelled')).toBe('secondary')
    expect(classificationSeverity('production_copy')).toBe('danger')
    expect(classificationSeverity('anonymised')).toBe('warn')
    expect(classificationSeverity('synthetic')).toBe('success')
    expect(environmentIcon('hil_rig')).toBe('pi pi-microchip')
    expect(environmentIcon('device')).toBe('pi pi-mobile')
    expect(environmentIcon('dev')).toBe('pi pi-code')
    expect(environmentIcon('pre_prod')).toBe('pi pi-globe')
    expect(environmentIcon('staging')).toBe('pi pi-server')
    expect(bookable({ status: 'retired' })).toBe(false)
    expect(bookable({ status: 'broken' })).toBe(true)
  })
  it('components', () => {
    expect(componentsText([{ name: 'app', version: '2.3.1' }, { name: 'DB', version: '' }])).toBe('app 2.3.1, DB')
    expect(componentsText(null)).toBe('')
    expect(cleanComponents([{ name: ' app ', version: ' 1 ' }, { name: '', version: '' }])).toEqual([{ name: 'app', version: '1' }])
    expect(duplicateComponents([{ name: 'App', version: '1' }, { name: 'app', version: '2' }, { name: '', version: '' }])).toEqual(['app'])
  })
  it('booking times', () => {
    expect(toLocalInput(null)).toBe('')
    expect(toLocalInput('nonsense')).toBe('')
    const local = toLocalInput('2030-01-01T08:30:00Z')
    expect(local).toMatch(/^2030-01-0\dT\d\d:30$/)
    expect(fromLocalInput(local)).toBe('2030-01-01T08:30:00.000Z')
    expect(fromLocalInput('')).toBe('')
    expect(fromLocalInput('x')).toBe('')
    const w = defaultBookingWindow(new Date(2030, 0, 1, 9, 20))
    expect(w.from).toBe('2030-01-01T10:00')
    expect(w.to).toBe('2030-01-01T14:00')
    expect(validWindow(w.from, w.to)).toBe(true)
    expect(validWindow(w.to, w.from)).toBe(false)
    expect(validWindow('', w.to)).toBe(false)
  })
  it('capacity', () => {
    const list = [bk('a', h(8), h(10)), bk('b', h(9), h(11)), bk('c', h(10), h(12)), bk('d', h(9), h(12), 'cancelled')]
    expect(maxOverlap(list.slice(0, 3), h(0), h(23))).toBe(2)
    expect(maxOverlap(list.slice(0, 3), h(12), h(13))).toBe(0)
    expect(fits(2, list, h(12), h(13))).toBe(true)
    expect(fits(2, list, h(9), h(10))).toBe(false)
    expect(fits(3, list, h(9), h(10))).toBe(true)
    // the booking being moved does not count against itself
    expect(fits(2, list, h(9), h(10), 'b')).toBe(true)
    expect(fits(0, [], h(1), h(2))).toBe(true)
  })
  it('timeline', () => {
    const mon = startOfWeek(new Date(2030, 0, 3, 15)) // a Thursday
    expect(mon.getDay()).toBe(1)
    expect(mon.getDate()).toBe(31) // 31 December 2029
    const days = timelineDays(mon)
    expect(days).toHaveLength(7)
    expect(days[6].getDay()).toBe(0)
    const from = new Date('2030-01-01T00:00:00Z')
    const to = new Date('2030-01-02T00:00:00Z')
    const { bars, lanes } = timelineBars([bk('a', h(6), h(12)), bk('b', h(10), h(14)), bk('c', h(12), h(18)), bk('x', '2030-02-01T00:00:00Z', '2030-02-02T00:00:00Z')], from, to)
    expect(bars.map((b) => b.booking.id)).toEqual(['a', 'b', 'c'])
    expect(bars[0].left).toBe(25)
    expect(bars[0].width).toBe(25)
    expect(bars.map((b) => b.lane)).toEqual([0, 1, 0])
    expect(lanes).toBe(2)
    // a bar crossing the end is clipped
    const clip = timelineBars([bk('a', '2029-12-31T12:00:00Z', h(6))], from, to)
    expect(clip.bars[0].left).toBe(0)
    expect(timelineBars([], from, to).lanes).toBe(1)
  })
  it('data set expiry', () => {
    expect(expiryState('', '2030-01-01')).toBe('')
    expect(expiryState('2029-12-31', '2030-01-01')).toBe('expired')
    expect(expiryState('2030-01-01', '2030-01-01')).toBe('expiring')
    expect(expiryState('2030-01-15', '2030-01-01')).toBe('expiring')
    expect(expiryState('2030-01-16', '2030-01-01')).toBe('')
    expect(expiryState('2030-01-16', 'bad')).toBe('')
  })
  it('parses pasted tables', () => {
    expect(parseTable('')).toEqual({ parameters: [], rows: [] })
    expect(parseTable('name,user,role\nAdmin,ann,"admin, all"\n,bob,viewer\n')).toEqual({
      parameters: ['user', 'role'],
      rows: [
        { name: 'Admin', values: { user: 'ann', role: 'admin, all' } },
        { name: '', values: { user: 'bob', role: 'viewer' } },
      ],
    })
    expect(parseTable('user\trole\nann\tadmin')).toEqual({ parameters: ['user', 'role'], rows: [{ name: '', values: { user: 'ann', role: 'admin' } }] })
    expect(parseTable('user;role\nann;"a ""b"""').rows[0].values.role).toBe('a "b"')
    expect(parseTable('user,role\nann').rows[0].values.role).toBe('')
  })
})
