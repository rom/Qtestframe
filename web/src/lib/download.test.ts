import { describe, expect, it, vi } from 'vitest'
import { filenameFromDisposition, sanitizeFilename, saveBlob } from './download'

describe('filenameFromDisposition', () => {
  it('handles missing header', () => {
    expect(filenameFromDisposition(null)).toBe('download')
    expect(filenameFromDisposition(undefined, 'x.bin')).toBe('x.bin')
    expect(filenameFromDisposition('attachment')).toBe('download')
  })
  it('parses quoted, bare and RFC 5987 names', () => {
    expect(filenameFromDisposition('attachment; filename="report 1.xlsx"')).toBe('report 1.xlsx')
    expect(filenameFromDisposition('attachment; filename="a\\"b.csv"')).toBe('a"b.csv')
    expect(filenameFromDisposition('attachment; filename=plain.pdf; size=3')).toBe('plain.pdf')
    expect(filenameFromDisposition("attachment; filename*=UTF-8''Pr%C3%BCfung.docx; filename=\"x.docx\"")).toBe(
      'Prüfung.docx',
    )
    expect(filenameFromDisposition('attachment; filename*="UTF-8\'\'a%20b.zip"')).toBe('a b.zip')
    expect(filenameFromDisposition('attachment; filename*=raw.txt')).toBe('raw.txt')
  })
  it('falls back when the encoded name is broken', () => {
    expect(filenameFromDisposition("attachment; filename*=UTF-8''%E0%A4%A; filename=\"ok.csv\"")).toBe('ok.csv')
  })
  it('strips directories', () => {
    expect(filenameFromDisposition('attachment; filename="../../etc/passwd"')).toBe('passwd')
    expect(sanitizeFilename('..')).toBe('download')
    expect(sanitizeFilename('a\\b\\c.txt')).toBe('c.txt')
    expect(sanitizeFilename('', 'f')).toBe('f')
  })
})

describe('saveBlob', () => {
  it('no-ops without object URL support', () => {
    const orig = URL.createObjectURL
    // @ts-expect-error simulate missing API
    URL.createObjectURL = undefined
    expect(() => saveBlob(new Blob(['x']), 'x')).not.toThrow()
    URL.createObjectURL = orig
  })
  it('clicks an anchor and revokes the URL', () => {
    vi.useFakeTimers()
    URL.createObjectURL = vi.fn(() => 'blob:x') as unknown as typeof URL.createObjectURL
    const revoke = vi.fn()
    URL.revokeObjectURL = revoke
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    saveBlob(new Blob(['x']), 'f.txt')
    expect(click).toHaveBeenCalled()
    vi.runAllTimers()
    expect(revoke).toHaveBeenCalledWith('blob:x')
    vi.useRealTimers()
  })
})
