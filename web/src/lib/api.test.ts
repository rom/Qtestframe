import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  api,
  ApiError,
  asItems,
  buildUrl,
  errorMessage,
  getCsrfToken,
  isPasswordChangeRequired,
  setPasswordChangeHandler,
  request,
  setCsrfToken,
  setUnauthorizedHandler,
  statusCode,
} from './api'

function res(status: number, body?: unknown, headers: Record<string, string> = {}): Response {
  const text = body === undefined ? '' : typeof body === 'string' ? body : JSON.stringify(body)
  return new Response(status === 204 ? null : text, { status, headers })
}

const fetchMock = vi.fn()

beforeEach(() => {
  fetchMock.mockReset()
  vi.stubGlobal('fetch', fetchMock)
  setCsrfToken('')
  setUnauthorizedHandler(null)
})
afterEach(() => vi.unstubAllGlobals())

describe('buildUrl', () => {
  it('prefixes the API base', () => {
    expect(buildUrl('/projects')).toBe('/api/v1/projects')
    expect(buildUrl('projects')).toBe('/api/v1/projects')
    expect(buildUrl('/api/v1/x')).toBe('/api/v1/x')
  })
  it('encodes params and skips empty ones', () => {
    expect(buildUrl('/x', { q: 'a b', n: 0, t: true, e: '', u: undefined, z: null, arr: ['a', 'b'], none: [] })).toBe(
      '/api/v1/x?q=a+b&n=0&t=true&arr=a%2Cb',
    )
    expect(buildUrl('/x', {})).toBe('/api/v1/x')
  })
})

describe('request', () => {
  it('parses JSON and sends credentials', async () => {
    fetchMock.mockResolvedValue(res(200, { a: 1 }, { 'Content-Type': 'application/json' }))
    expect(await api.get('/x', { q: 'v' })).toEqual({ a: 1 })
    const [url, init] = fetchMock.mock.calls[0]
    expect(url).toBe('/api/v1/x?q=v')
    expect(init.credentials).toBe('same-origin')
    expect(init.method).toBe('GET')
    expect(init.headers['X-CSRF-Token']).toBeUndefined()
  })

  it('sends csrf header and JSON body on mutations', async () => {
    setCsrfToken('tok')
    expect(getCsrfToken()).toBe('tok')
    fetchMock.mockResolvedValue(res(201, { id: '1' }, { 'Content-Type': 'application/json' }))
    await api.post('/x', { name: 'n' })
    const init = fetchMock.mock.calls[0][1]
    expect(init.headers['X-CSRF-Token']).toBe('tok')
    expect(init.headers['Content-Type']).toBe('application/json')
    expect(init.body).toBe('{"name":"n"}')
  })

  it('sends If-Match for the version a change is based on', async () => {
    fetchMock.mockImplementation(() => Promise.resolve(res(200, { id: '1' }, { 'Content-Type': 'application/json' })))
    await api.patch('/x', { a: 1 }, { ifMatch: 4 })
    expect(fetchMock.mock.calls[0][1].headers['If-Match']).toBe('"4"')
    await api.patch('/x', { a: 1 }, { ifMatch: '' })
    expect(fetchMock.mock.calls[1][1].headers['If-Match']).toBeUndefined()
  })

  it('supports patch, put and delete with body', async () => {
    setCsrfToken('t')
    fetchMock.mockImplementation(() => Promise.resolve(res(204)))
    expect(await api.patch('/x', { a: 1 })).toBeUndefined()
    expect(await api.put('/x', { a: 1 })).toBeUndefined()
    expect(await api.del('/x', { ids: [1] })).toBeUndefined()
    expect(fetchMock.mock.calls.map((c) => c[1].method)).toEqual(['PATCH', 'PUT', 'DELETE'])
    expect(fetchMock.mock.calls[2][1].body).toBe('{"ids":[1]}')
  })

  it('handles empty 200 and text bodies', async () => {
    fetchMock.mockResolvedValueOnce(res(200, ''))
    expect(await request('GET', '/x')).toBeUndefined()
    fetchMock.mockResolvedValueOnce(res(200, 'hello', { 'Content-Type': 'text/plain' }))
    expect(await request('GET', '/x')).toBe('hello')
    fetchMock.mockResolvedValueOnce(res(200, '[1,2]', { 'Content-Type': 'text/plain' }))
    expect(await request('GET', '/x')).toEqual([1, 2])
  })

  it('throws ApiError with code and position', async () => {
    fetchMock.mockResolvedValue(res(400, { error: 'unexpected token', code: 'invalid_query', position: 7 }))
    const err = (await api.get('/x').catch((e) => e)) as ApiError
    expect(err).toBeInstanceOf(ApiError)
    expect(err.status).toBe(400)
    expect(err.code).toBe('invalid_query')
    expect(err.position).toBe(7)
    expect(err.message).toBe('unexpected token')
    expect(errorMessage(err)).toBe('unexpected token')
  })

  it('falls back to status code, message field and text bodies', async () => {
    fetchMock.mockResolvedValueOnce(res(404, { message: 'nope' }))
    let err = (await api.get('/x').catch((e) => e)) as ApiError
    expect(err.code).toBe('not_found')
    expect(err.message).toBe('nope')
    fetchMock.mockResolvedValueOnce(res(500, 'x'.repeat(400)))
    err = (await api.get('/x').catch((e) => e)) as ApiError
    expect(err.code).toBe('internal')
    expect(err.message.length).toBeLessThan(310)
    fetchMock.mockResolvedValueOnce(new Response('', { status: 409, statusText: 'Conflict' }))
    err = (await api.get('/x').catch((e) => e)) as ApiError
    expect(err.code).toBe('conflict')
    expect(err.message).toBe('Conflict')
    fetchMock.mockResolvedValueOnce(res(418, {}))
    err = (await api.get('/x').catch((e) => e)) as ApiError
    expect(err.message).toBe('HTTP 418')
  })

  it('calls the unauthorized handler on 401 unless suppressed', async () => {
    const h = vi.fn()
    setUnauthorizedHandler(h)
    fetchMock.mockImplementation(() => Promise.resolve(res(401, { error: 'login', code: 'unauthorized' })))
    await expect(api.get('/x')).rejects.toBeInstanceOf(ApiError)
    expect(h).toHaveBeenCalledTimes(1)
    await expect(api.get('/x', undefined, { noAuthRedirect: true })).rejects.toBeInstanceOf(ApiError)
    expect(h).toHaveBeenCalledTimes(1)
  })

  it('default unauthorized handler redirects to /login', async () => {
    const assign = vi.fn()
    const loc = { pathname: '/projects', search: '?a=1', assign }
    vi.stubGlobal('location', loc)
    Object.defineProperty(window, 'location', { value: loc, configurable: true, writable: true })
    fetchMock.mockImplementation(() => Promise.resolve(res(401, {})))
    await api.get('/x').catch(() => undefined)
    expect(assign).toHaveBeenCalledWith('/login?redirect=%2Fprojects%3Fa%3D1')
    loc.pathname = '/login'
    await api.get('/x').catch(() => undefined)
    expect(assign).toHaveBeenCalledTimes(1)
  })
})

describe('download', () => {
  it('saves the blob with the server filename', async () => {
    const create = vi.fn(() => 'blob:1')
    const revoke = vi.fn()
    URL.createObjectURL = create as unknown as typeof URL.createObjectURL
    URL.revokeObjectURL = revoke
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    fetchMock.mockResolvedValue(res(200, 'a,b', { 'Content-Disposition': 'attachment; filename="P.csv"' }))
    expect(await api.download('/projects/1/export', { format: 'csv' })).toBe('P.csv')
    expect(fetchMock.mock.calls[0][0]).toBe('/api/v1/projects/1/export?format=csv')
    expect(create).toHaveBeenCalled()
    expect(click).toHaveBeenCalled()
  })
  it('uses the fallback name and rejects errors', async () => {
    fetchMock.mockResolvedValueOnce(res(200, 'x'))
    expect(await api.download('/x', undefined, 'export.bin')).toBe('export.bin')
    fetchMock.mockResolvedValueOnce(res(403, { error: 'denied', code: 'forbidden' }))
    await expect(api.download('/x')).rejects.toMatchObject({ code: 'forbidden' })
  })
})

describe('password change required', () => {
  afterEach(() => setPasswordChangeHandler(null))
  it('calls the handler on 403 password_change_required only', async () => {
    const h = vi.fn()
    setPasswordChangeHandler(h)
    fetchMock.mockResolvedValueOnce(res(403, { error: 'change', code: 'password_change_required' }))
    const err = await api.get('/projects').catch((e) => e)
    expect(isPasswordChangeRequired(err)).toBe(true)
    expect(h).toHaveBeenCalledTimes(1)
    fetchMock.mockResolvedValueOnce(res(403, { error: 'no', code: 'forbidden' }))
    const other = await api.get('/projects').catch((e) => e)
    expect(isPasswordChangeRequired(other)).toBe(false)
    expect(isPasswordChangeRequired(new Error('x'))).toBe(false)
    expect(h).toHaveBeenCalledTimes(1)
  })
})

describe('postDownload / getBlob', () => {
  it('posts JSON and saves the response', async () => {
    URL.createObjectURL = vi.fn(() => 'blob:2') as unknown as typeof URL.createObjectURL
    URL.revokeObjectURL = vi.fn()
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => undefined)
    setCsrfToken('c')
    fetchMock.mockResolvedValueOnce(res(200, 'pdf', { 'Content-Disposition': 'attachment; filename="R.pdf"' }))
    expect(await api.postDownload('/projects/1/reports/generate', { format: 'pdf' }, 'fallback.pdf')).toBe('R.pdf')
    const [, init] = fetchMock.mock.calls[0]
    expect(init.method).toBe('POST')
    expect(init.body).toBe('{"format":"pdf"}')
    expect(init.headers['X-CSRF-Token']).toBe('c')
    fetchMock.mockResolvedValueOnce(res(200, 'img'))
    expect(await (await api.getBlob('/files/1/content', { inline: 1 })).text()).toBe('img')
  })
})

describe('helpers', () => {
  it('asItems normalises lists', () => {
    expect(asItems(null)).toEqual([])
    expect(asItems([1, 2])).toEqual([1, 2])
    expect(asItems({ items: [3] })).toEqual([3])
    expect(asItems({ items: null })).toEqual([])
  })
  it('statusCode maps statuses', () => {
    expect(statusCode(400)).toBe('bad_request')
    expect(statusCode(401)).toBe('unauthorized')
    expect(statusCode(403)).toBe('forbidden')
    expect(statusCode(502)).toBe('internal')
    expect(statusCode(422)).toBe('error')
  })
  it('errorMessage handles any value', () => {
    expect(errorMessage(new Error('e'))).toBe('e')
    expect(errorMessage('s')).toBe('s')
  })
})
