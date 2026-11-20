import { describe, expect, it } from 'vitest'
import { createSlugger, slugify } from './slug'

describe('slugify', () => {
  it('makes GitHub-style anchors', () => {
    expect(slugify('3. Write requirements')).toBe('3-write-requirements')
    expect(slugify('  Runs & executions  ')).toBe('runs-executions')
    expect(slugify('What does "not run" mean?')).toBe('what-does-not-run-mean')
    expect(slugify('snake_case and-dash')).toBe('snake_case-and-dash')
  })
  it('keeps letters of other scripts', () => {
    expect(slugify('Översikt och täckning')).toBe('översikt-och-täckning')
  })
  it('never returns an empty slug', () => {
    expect(slugify('!!!')).toBe('section')
    expect(slugify('')).toBe('section')
  })
})

describe('createSlugger', () => {
  it('numbers repeated headings', () => {
    const s = createSlugger()
    expect(s('Example')).toBe('example')
    expect(s('Example')).toBe('example-1')
    expect(s('Example')).toBe('example-2')
    expect(s('Example 1')).toBe('example-1-1')
  })
  it('is independent per document', () => {
    createSlugger()('A')
    expect(createSlugger()('A')).toBe('a')
  })
})
