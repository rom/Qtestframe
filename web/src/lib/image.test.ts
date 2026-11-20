import { describe, expect, it } from 'vitest'
import { cropRect, imageFileName, nextQuality, normalizeCrop, outputSize, outputTypes } from './image'

describe('cropRect', () => {
  it('takes the centred square of a landscape image', () => {
    expect(cropRect(1000, 500)).toEqual({ sx: 250, sy: 0, sw: 500, sh: 500 })
  })
  it('takes the centred square of a portrait image', () => {
    expect(cropRect(400, 800)).toEqual({ sx: 0, sy: 200, sw: 400, sh: 400 })
  })
  it('moves the crop to the edges', () => {
    expect(cropRect(1000, 500, { zoom: 1, x: -1, y: 0 }).sx).toBe(0)
    expect(cropRect(1000, 500, { zoom: 1, x: 1, y: 0 }).sx).toBe(500)
  })
  it('zooms into the middle', () => {
    expect(cropRect(800, 800, { zoom: 2, x: 0, y: 0 })).toEqual({ sx: 200, sy: 200, sw: 400, sh: 400 })
    expect(cropRect(800, 800, { zoom: 2, x: 1, y: -1 })).toEqual({ sx: 400, sy: 0, sw: 400, sh: 400 })
  })
  it('handles empty images', () => {
    expect(cropRect(0, 10)).toEqual({ sx: 0, sy: 0, sw: 0, sh: 0 })
  })
})

describe('normalizeCrop', () => {
  it('clamps and defaults', () => {
    expect(normalizeCrop({ zoom: 10, x: -5, y: NaN })).toEqual({ zoom: 4, x: -1, y: 0 })
    expect(normalizeCrop(null)).toEqual({ zoom: 1, x: 0, y: 0 })
    expect(normalizeCrop({ zoom: 0.2 })).toEqual({ zoom: 1, x: 0, y: 0 })
  })
})

describe('output', () => {
  it('never upscales', () => {
    expect(outputSize({ sx: 0, sy: 0, sw: 1000, sh: 1000 })).toBe(256)
    expect(outputSize({ sx: 0, sy: 0, sw: 100, sh: 100 })).toBe(100)
    expect(outputSize({ sx: 0, sy: 0, sw: 0, sh: 0 })).toBe(1)
  })
  it('prefers webp when supported', () => {
    expect(outputTypes(true)).toEqual(['image/webp', 'image/png'])
    expect(outputTypes(false)).toEqual(['image/png'])
  })
  it('steps quality down', () => {
    expect(nextQuality(0.92)).toBe(0.85)
    expect(nextQuality(0.6)).toBe(0.45)
    expect(nextQuality(0.45)).toBeNull()
  })
  it('names files', () => {
    expect(imageFileName('My Photo.JPG', 'image/webp')).toBe('My-Photo.webp')
    expect(imageFileName('', 'image/png')).toBe('image.png')
    expect(imageFileName('x', 'image/jpeg')).toBe('x.jpg')
  })
})
