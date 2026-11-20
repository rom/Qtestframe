import { describe, expect, it } from 'vitest'
import { hueFor, imageUrl, parseImageRef, presetMap, presetName, presetValue } from './avatars'

describe('parseImageRef', () => {
  it('parses all forms', () => {
    expect(parseImageRef('')).toEqual({ kind: 'initials' })
    expect(parseImageRef(null)).toEqual({ kind: 'initials' })
    expect(parseImageRef('preset:fox')).toEqual({ kind: 'preset', name: 'fox' })
    expect(parseImageRef('file:abc-123')).toEqual({ kind: 'file', id: 'abc-123' })
  })
  it('treats invalid values as initials', () => {
    expect(parseImageRef('preset:Fox!')).toEqual({ kind: 'initials' })
    expect(parseImageRef('file:../x')).toEqual({ kind: 'initials' })
    expect(parseImageRef('http://x')).toEqual({ kind: 'initials' })
  })
})

describe('presets', () => {
  it('maps glob results to names', () => {
    expect(presetName('./avatars/fox.svg')).toBe('fox')
    expect(presetMap({ './a/fox.svg': '/u/fox', './a/Bad Name.svg': '/u/bad' })).toEqual({ fox: '/u/fox' })
    expect(presetValue('fox')).toBe('preset:fox')
  })
})

describe('imageUrl', () => {
  const presets = { fox: '/fox.svg' }
  it('resolves presets and files', () => {
    expect(imageUrl('preset:fox', 'avatar', presets)).toBe('/fox.svg')
    expect(imageUrl('preset:cat', 'avatar', presets)).toBeNull()
    expect(imageUrl('file:1', 'avatar', presets)).toBe('/api/v1/avatars/1')
    expect(imageUrl('file:1', 'icon', presets)).toBe('/api/v1/files/1/content?inline=1')
    expect(imageUrl('', 'icon', presets)).toBeNull()
  })
  it('hashes a stable hue', () => {
    expect(hueFor('a')).toBe(hueFor('a'))
    expect(hueFor(null)).toBe(0)
    expect(hueFor('abc')).toBeGreaterThanOrEqual(0)
  })
})

describe('shipped presets', () => {
  it('has about 24 avatars and 16 project icons with valid names', async () => {
    const { AVATAR_PRESETS, ICON_PRESETS } = await import('@/assets/presets')
    expect(Object.keys(AVATAR_PRESETS).length).toBeGreaterThanOrEqual(24)
    expect(Object.keys(ICON_PRESETS).length).toBeGreaterThanOrEqual(16)
    for (const n of ['fox', 'robot']) expect(AVATAR_PRESETS[n]).toBeTruthy()
    for (const n of ['rocket', 'shield', 'gear', 'flask']) expect(ICON_PRESETS[n]).toBeTruthy()
  })
})
