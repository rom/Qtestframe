// Thin fetch wrapper for the Qtestframe REST API (/api/v1).
import { filenameFromDisposition, saveBlob } from './download'

export const API_BASE = '/api/v1'

/** Error thrown for every non-2xx response. */
export class ApiError extends Error {
  readonly status: number
  readonly code: string
  readonly position?: number
  readonly body: unknown

  constructor(status: number, code: string, message: string, position?: number, body?: unknown) {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.code = code
    this.position = position
    this.body = body
  }
}

let csrfToken = ''

export function setCsrfToken(token: string | null | undefined): void {
  csrfToken = token ?? ''
}

export function getCsrfToken(): string {
  return csrfToken
}

export type UnauthorizedHandler = () => void

function defaultUnauthorized(): void {
  if (typeof window === 'undefined') return
  const { pathname, search } = window.location
  if (pathname.startsWith('/login')) return
  window.location.assign('/login?redirect=' + encodeURIComponent(pathname + search))
}

let onUnauthorized: UnauthorizedHandler = defaultUnauthorized

/** Replace the 401 handler (the app routes to /login via the router). */
export function setUnauthorizedHandler(fn: UnauthorizedHandler | null): void {
  onUnauthorized = fn ?? defaultUnauthorized
}

export type PasswordChangeHandler = () => void
let onPasswordChange: PasswordChangeHandler | null = null

/** Called when any request answers 403 `password_change_required`. */
export function setPasswordChangeHandler(fn: PasswordChangeHandler | null): void {
  onPasswordChange = fn
}

/** Code the server uses while the user must change a reset password. */
export const PASSWORD_CHANGE_REQUIRED = 'password_change_required'

/** Whether an error means the user has to change the password first. */
export function isPasswordChangeRequired(e: unknown): boolean {
  return e instanceof ApiError && e.status === 403 && e.code === PASSWORD_CHANGE_REQUIRED
}

/** Global reactions to an error response (used by fetch and upload requests). */
export function handleErrorResponse(err: ApiError, opts: { noAuthRedirect?: boolean } = {}): void {
  if (err.status === 401 && !opts.noAuthRedirect) onUnauthorized()
  else if (isPasswordChangeRequired(err)) onPasswordChange?.()
}

export type ParamValue = string | number | boolean | null | undefined | string[]

/** Build "/api/v1/<path>?a=b", skipping empty parameters; arrays are comma joined. */
export function buildUrl(path: string, params?: Record<string, ParamValue>): string {
  const base = path.startsWith('/api/') ? path : API_BASE + (path.startsWith('/') ? path : '/' + path)
  if (!params) return base
  const qs = new URLSearchParams()
  for (const [k, v] of Object.entries(params)) {
    if (v === undefined || v === null || v === '') continue
    if (Array.isArray(v)) {
      if (v.length) qs.set(k, v.join(','))
      continue
    }
    qs.set(k, String(v))
  }
  const s = qs.toString()
  return s ? `${base}?${s}` : base
}

export interface RequestOptions {
  params?: Record<string, ParamValue>
  body?: unknown
  /** Do not trigger the login redirect on 401 (used by /auth/me and /auth/login). */
  noAuthRedirect?: boolean
  signal?: AbortSignal
}

async function toError(res: Response): Promise<ApiError> {
  let body: unknown = undefined
  let message = res.statusText || `HTTP ${res.status}`
  let code = statusCode(res.status)
  let position: number | undefined
  const text = await res.text().catch(() => '')
  if (text) {
    try {
      body = JSON.parse(text)
      const b = body as { error?: unknown; code?: unknown; position?: unknown; message?: unknown }
      if (typeof b.error === 'string' && b.error) message = b.error
      else if (typeof b.message === 'string' && b.message) message = b.message
      if (typeof b.code === 'string' && b.code) code = b.code
      if (typeof b.position === 'number') position = b.position
    } catch {
      body = text
      message = text.length > 300 ? text.slice(0, 300) + '…' : text
    }
  }
  return new ApiError(res.status, code, message, position, body)
}

/** Default machine code for a status when the body carries none. */
export function statusCode(status: number): string {
  switch (status) {
    case 400:
      return 'bad_request'
    case 401:
      return 'unauthorized'
    case 403:
      return 'forbidden'
    case 404:
      return 'not_found'
    case 409:
      return 'conflict'
    default:
      return status >= 500 ? 'internal' : 'error'
  }
}

async function send(method: string, path: string, opts: RequestOptions = {}): Promise<Response> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  const init: RequestInit = { method, headers, credentials: 'same-origin', signal: opts.signal }
  if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    init.body = JSON.stringify(opts.body)
  }
  if (method !== 'GET' && method !== 'HEAD' && csrfToken) headers['X-CSRF-Token'] = csrfToken
  const res = await fetch(buildUrl(path, opts.params), init)
  if (!res.ok) {
    const err = await toError(res)
    handleErrorResponse(err, opts)
    throw err
  }
  return res
}

/** Perform a JSON request; resolves undefined for 204 / empty bodies. */
export async function request<T = unknown>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  const res = await send(method, path, opts)
  if (res.status === 204) return undefined as T
  const text = await res.text()
  if (!text) return undefined as T
  const type = res.headers.get('Content-Type') || ''
  if (type.includes('json') || /^[[{"]/.test(text.trim())) return JSON.parse(text) as T
  return text as unknown as T
}

/** Download a file response (export) and hand it to the browser. Returns the filename. */
export async function download(path: string, params?: Record<string, ParamValue>, fallbackName = 'download'): Promise<string> {
  const res = await send('GET', path, { params })
  const blob = await res.blob()
  const name = filenameFromDisposition(res.headers.get('Content-Disposition'), fallbackName)
  saveBlob(blob, name)
  return name
}

/** GET a binary response as a Blob (e.g. a preview). */
export async function getBlob(path: string, params?: Record<string, ParamValue>): Promise<Blob> {
  const res = await send('GET', path, { params })
  return res.blob()
}

/** POST JSON and save the binary response as a file (report generation). Returns the filename. */
export async function postDownload(path: string, body: unknown, fallbackName = 'download'): Promise<string> {
  const res = await send('POST', path, { body })
  const blob = await res.blob()
  const name = filenameFromDisposition(res.headers.get('Content-Disposition'), fallbackName)
  saveBlob(blob, name)
  return name
}

export const api = {
  get: <T = unknown>(path: string, params?: Record<string, ParamValue>, opts: RequestOptions = {}) =>
    request<T>('GET', path, { ...opts, params }),
  post: <T = unknown>(path: string, body?: unknown, opts: RequestOptions = {}) =>
    request<T>('POST', path, { ...opts, body }),
  put: <T = unknown>(path: string, body?: unknown, opts: RequestOptions = {}) => request<T>('PUT', path, { ...opts, body }),
  patch: <T = unknown>(path: string, body?: unknown, opts: RequestOptions = {}) =>
    request<T>('PATCH', path, { ...opts, body }),
  del: <T = unknown>(path: string, body?: unknown, opts: RequestOptions = {}) =>
    request<T>('DELETE', path, { ...opts, body }),
  download,
  postDownload,
  getBlob,
}

/** Normalise list responses: accepts `{items,total}`, a bare array or null. */
export function asItems<T>(res: { items?: T[] | null } | T[] | null | undefined): T[] {
  if (!res) return []
  if (Array.isArray(res)) return res
  return res.items ?? []
}

/** Human readable message for any thrown value. */
export function errorMessage(e: unknown): string {
  if (e instanceof ApiError) return e.message
  if (e instanceof Error) return e.message
  return String(e)
}
