import { describe, expect, it } from 'vitest'
import { activeMention, insertMention, rankPeople, splitMentions, type Person } from './mentions'

const people: Person[] = [
  { handle: 'anna', name: 'Anna Andersson', kind: 'user', email: 'anna@example.com', uses: 3 },
  { handle: 'ben', name: 'Ben', kind: 'user', email: 'ben@example.com' },
  { handle: 'carl.carlsson', name: 'Carl Carlsson', kind: 'person', uses: 1 },
  { handle: 'åsa', name: 'Åsa Öberg', kind: 'user', email: 'asa@example.com' },
  { handle: 'annika', name: 'Annika', kind: 'person', uses: 9 },
]

describe('activeMention', () => {
  it('finds the mention being typed', () => {
    expect(activeMention('Ask @an', 7)).toEqual({ start: 4, query: 'an' })
    expect(activeMention('@', 1)).toEqual({ start: 0, query: '' })
    expect(activeMention('x (@carl.c', 10)).toEqual({ start: 3, query: 'carl.c' })
    expect(activeMention('Ask @anna later', 9)).toEqual({ start: 4, query: 'anna' })
  })
  it('ignores e-mail addresses, finished words and long runs', () => {
    expect(activeMention('mail x@example', 14)).toBeNull()
    expect(activeMention('@anna is here', 13)).toBeNull()
    expect(activeMention('plain', 5)).toBeNull()
    expect(activeMention('@' + 'x'.repeat(65), 66)).toBeNull()
  })
})

describe('insertMention', () => {
  it('replaces the typed part with the handle and a space', () => {
    expect(insertMention('Ask @an', 4, 7, 'anna')).toEqual({ text: 'Ask @anna ', caret: 10 })
    expect(insertMention('Ask @an today', 4, 7, 'anna')).toEqual({ text: 'Ask @anna today', caret: 9 })
    expect(insertMention('@annXX!', 0, 4, 'anna')).toEqual({ text: '@anna !', caret: 6 })
  })
})

describe('rankPeople', () => {
  it('ranks prefixes, then words, then substrings; users first', () => {
    expect(rankPeople(people, 'an').map((p) => p.handle)).toEqual(['anna', 'annika'])
    expect(rankPeople(people, 'carlsson').map((p) => p.handle)).toEqual(['carl.carlsson'])
    expect(rankPeople(people, 'asa').map((p) => p.handle)).toEqual(['åsa'])
    expect(rankPeople(people, 'oberg').map((p) => p.handle)).toEqual(['åsa'])
    expect(rankPeople(people, 'nik').map((p) => p.handle)).toEqual(['annika'])
    expect(rankPeople(people, 'ben@').map((p) => p.handle)).toEqual(['ben'])
    expect(rankPeople(people, 'zz')).toEqual([])
  })
  it('lists everyone for an empty query, users first, within the limit', () => {
    expect(rankPeople(people, '').map((p) => p.kind)).toEqual(['user', 'user', 'user', 'person', 'person'])
    expect(rankPeople(people, '', 2)).toHaveLength(2)
  })
})

describe('splitMentions', () => {
  it('splits text into plain parts and mentions', () => {
    const s = splitMentions('Ask @anna and @nobody. Mail a@b.c, @carl.carlsson.', people)
    expect(s.map((x) => x.text)).toEqual(['Ask ', '@anna', ' and ', '@nobody', '. Mail a@b.c, ', '@carl.carlsson', '.'])
    expect(s[1].person?.name).toBe('Anna Andersson')
    expect(s[3].person).toBeUndefined()
    expect(s[3].handle).toBe('nobody')
    expect(splitMentions('', people)).toEqual([])
    expect(splitMentions('@-', people)).toEqual([{ text: '@-' }])
  })
})
