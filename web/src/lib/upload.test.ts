import { afterEach, describe, expect, it, vi } from 'vitest'
import { upload, uploadError, uploadForm } from './upload'
import { ApiError, setCsrfToken, setPasswordChangeHandler, setUnauthorizedHandler } from './api'

class FakeXHR {
  static last: FakeXHR
  method = ''
  url = ''
  headers: Record<string, string> = {}
  withCredentials = false
  status = 0
  responseText = ''
  body: FormData | null = null
  upload: { onprogress: ((e: ProgressEvent) => void) | null } = { onprogress: null }
  onload: (() => void) | null = null
  onerror: (() => void) | null = null
  onabort: (() => void) | null = null
  constructor() {
    FakeXHR.last = this
  }
  open(m: string, u: string) {
    this.method = m
    this.url = u
  }
  setRequestHeader(k: string, v: string) {
    this.headers[k] = v
  }
  send(b: FormData) {
    this.body = b
  }
  abort() {
    this.onabort?.()
  }
  respond(status: number, text: string) {
    this.status = status
    this.responseText = text
    this.onload?.()
  }
}
const xhr = () => new FakeXHR() as unknown as XMLHttpRequest

afterEach(() => {
  setCsrfToken('')
  setPasswordChangeHandler(null)
  setUnauthorizedHandler(null)
})

describe('upload', () => {
  it('posts a multipart form with CSRF and reports progress', async () => {
    setCsrfToken('tok')
    const progress: number[] = []
    const p = upload<{ id: string }>('/projects/p/documents', new File(['abc'], 'a.txt'), { xhr, fields: { description: 'd', tags: '' }, onProgress: (n) => progress.push(n) })
    const x = FakeXHR.last
    expect(x.method).toBe('POST')
    expect(x.url).toBe('/api/v1/projects/p/documents')
    expect(x.headers['X-CSRF-Token']).toBe('tok')
    expect(x.withCredentials).toBe(true)
    expect((x.body!.get('file') as File).name).toBe('a.txt')
    expect(x.body!.get('description')).toBe('d')
    expect(x.body!.has('tags')).toBe(false)
    x.upload.onprogress?.({ lengthComputable: true, loaded: 1, total: 2 } as ProgressEvent)
    x.upload.onprogress?.({ lengthComputable: false, loaded: 1, total: 0 } as ProgressEvent)
    x.respond(201, '{"id":"f1"}')
    await expect(p).resolves.toEqual({ id: 'f1' })
    expect(progress).toEqual([50, 100])
  })
  it('uses PUT, another field and handles empty / non-JSON bodies', async () => {
    const p = upload('/auth/me/avatar', new Blob(['x']), { xhr, method: 'PUT', field: 'backup' })
    expect(FakeXHR.last.method).toBe('PUT')
    expect(FakeXHR.last.body!.has('backup')).toBe(true)
    FakeXHR.last.respond(204, '')
    await expect(p).resolves.toBeUndefined()
    const q = upload('/x', new Blob(['x']), { xhr })
    FakeXHR.last.respond(200, 'plain')
    await expect(q).resolves.toBe('plain')
  })
  it('rejects with ApiError and calls the global handlers', async () => {
    const pw = vi.fn()
    const unauth = vi.fn()
    setPasswordChangeHandler(pw)
    setUnauthorizedHandler(unauth)
    const p = upload('/x', new Blob(['x']), { xhr })
    FakeXHR.last.respond(403, '{"error":"change it","code":"password_change_required"}')
    await expect(p).rejects.toMatchObject({ status: 403, code: 'password_change_required', message: 'change it' })
    expect(pw).toHaveBeenCalled()
    const q = upload('/x', new Blob(['x']), { xhr })
    FakeXHR.last.respond(401, '')
    await expect(q).rejects.toBeInstanceOf(ApiError)
    expect(unauth).toHaveBeenCalled()
  })
  it('handles network errors and aborts', async () => {
    const p = upload('/x', new Blob(['x']), { xhr })
    FakeXHR.last.onerror?.()
    await expect(p).rejects.toMatchObject({ code: 'network' })
    const ctl = new AbortController()
    const q = upload('/x', new Blob(['x']), { xhr, signal: ctl.signal })
    ctl.abort()
    await expect(q).rejects.toMatchObject({ code: 'aborted' })
  })
})

describe('uploadError / uploadForm', () => {
  it('parses error bodies', () => {
    expect(uploadError(400, '{"error":"too big","code":"bad_request"}')).toMatchObject({ message: 'too big', code: 'bad_request' })
    expect(uploadError(500, 'x'.repeat(400)).message).toHaveLength(301)
    expect(uploadError(413, '')).toMatchObject({ message: 'HTTP 413', code: 'error' })
    expect(uploadError(409, '{}')).toMatchObject({ code: 'conflict' })
  })
  it('appends fields before the file', () => {
    const fd = uploadForm(new Blob(['x']), undefined, 'file', { a: '1', b: null, c: undefined })
    expect([...fd.keys()]).toEqual(['a', 'file'])
  })
})
