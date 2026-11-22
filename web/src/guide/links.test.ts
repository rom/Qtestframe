import { describe, expect, it } from 'vitest'
import { assetMap, classifyHref, helpPath, imageName, resolveImage } from './links'

describe('classifyHref', () => {
  it('turns relative Markdown links into guide pages', () => {
    expect(classifyHref('faq.md')).toEqual({ kind: 'page', page: 'faq', hash: '' })
    expect(classifyHref('./tests.md#shared-steps')).toEqual({ kind: 'page', page: 'tests', hash: 'shared-steps' })
    expect(classifyHref('../sv/runs.md#ci')).toEqual({ kind: 'page', page: 'runs', hash: 'ci' })
    expect(classifyHref('Tutorial')).toEqual({ kind: 'page', page: 'tutorial', hash: '' })
  })
  it('keeps anchors on the same page', () => {
    expect(classifyHref('#t%C3%A4ckning')).toEqual({ kind: 'anchor', hash: 'täckning' })
    expect(classifyHref('./#x')).toEqual({ kind: 'anchor', hash: 'x' })
    expect(classifyHref('#%E0%A4%A')).toEqual({ kind: 'anchor', hash: '%E0%A4%A' })
  })
  it('allows web and mail links only', () => {
    expect(classifyHref('https://example.com/a')).toEqual({ kind: 'external', href: 'https://example.com/a' })
    expect(classifyHref('mailto:x@example.com').kind).toBe('external')
    expect(classifyHref('//cdn.example.com/x')).toEqual({ kind: 'external', href: 'https://cdn.example.com/x' })
    expect(classifyHref('javascript:alert(1)').kind).toBe('invalid')
    expect(classifyHref('data:text/html,x').kind).toBe('invalid')
  })
  it('passes app paths through', () => {
    expect(classifyHref('/projects')).toEqual({ kind: 'app', path: '/projects' })
  })
  it('rejects nonsense', () => {
    expect(classifyHref('').kind).toBe('invalid')
    expect(classifyHref('#').kind).toBe('invalid')
    expect(classifyHref('./').kind).toBe('invalid')
    expect(classifyHref('../../secret/file.txt').kind).toBe('invalid')
  })
  it('recognises links into the other guide', () => {
    expect(classifyHref('../../user-guide/en/administration.md#members')).toEqual({ kind: 'page', page: 'administration', hash: 'members', book: 'user' })
    expect(classifyHref('../../admin-guide/sv/security.md')).toEqual({ kind: 'page', page: 'security', hash: '', book: 'admin' })
    expect(classifyHref('../../other-guide/en/x.md').kind).toBe('invalid')
  })
})

describe('helpPath', () => {
  it('builds router paths', () => {
    expect(helpPath('index')).toBe('/help')
    expect(helpPath('faq')).toBe('/help/faq')
    expect(helpPath('faq', 'why-not')).toBe('/help/faq#why-not')
    expect(helpPath('index', 'x')).toBe('/help#x')
  })
  it('builds paths of the administrator guide', () => {
    expect(helpPath('index', '', 'admin')).toBe('/admin-guide')
    expect(helpPath('security', 'key-providers', 'admin')).toBe('/admin-guide/security#key-providers')
    expect(helpPath('faq', '', 'user')).toBe('/help/faq')
  })
})

describe('images', () => {
  const assets = assetMap({
    '../../../docs/user-guide/images/en-editor.png': '/assets/en-editor-abc.png',
    '../../../docs/user-guide/images/sv-editor.png': '/assets/sv-editor-def.png',
    '../../../docs/user-guide/images/en-runner.png': '/assets/en-runner-1.png',
    '../../../docs/user-guide/images/logo.svg': 'data:image/svg+xml,x',
  })
  it('extracts safe file names', () => {
    expect(imageName('../images/en-editor.png')).toBe('en-editor.png')
    expect(imageName('images/a.PNG?x=1')).toBe('a.PNG')
    expect(imageName('https://x/y.png')).toBeNull()
    expect(imageName('//x/y.png')).toBeNull()
    expect(imageName('../images/notes.txt')).toBeNull()
    expect(imageName('')).toBeNull()
  })
  it('uses the language’s own screenshot', () => {
    expect(resolveImage('../images/en-editor.png', 'sv', assets)).toBe('/assets/sv-editor-def.png')
    expect(resolveImage('../images/sv-editor.png', 'sv', assets)).toBe('/assets/sv-editor-def.png')
  })
  it('falls back to English, then the name as written', () => {
    expect(resolveImage('../images/sv-runner.png', 'sv', assets)).toBe('/assets/en-runner-1.png')
    expect(resolveImage('../images/logo.svg', 'sv', assets)).toBe('data:image/svg+xml,x')
    expect(resolveImage('../images/sv-missing.png', 'sv', assets)).toBeNull()
    expect(resolveImage('javascript:x', 'en', assets)).toBeNull()
  })
})
