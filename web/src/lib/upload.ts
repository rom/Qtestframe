// Multipart uploads with progress (XMLHttpRequest: fetch has no upload progress).
import { ApiError, buildUrl, getCsrfToken, handleErrorResponse, statusCode } from './api'
import { progressPercent } from './files'

export interface UploadOptions {
  /** HTTP method (POST by default; PUT for avatars and icons). */
  method?: 'POST' | 'PUT'
  /** multipart field of the file (default "file") */
  field?: string
  /** further form fields (description, tags, confirm, …) */
  fields?: Record<string, string | undefined | null>
  /** 0–100 */
  onProgress?: (percent: number) => void
  signal?: AbortSignal
  /** for tests */
  xhr?: () => XMLHttpRequest
}

/** Parse an error body like api.ts does. */
export function uploadError(status: number, text: string): ApiError {
  let message = `HTTP ${status}`
  let code = statusCode(status)
  let body: unknown = text
  if (text) {
    try {
      body = JSON.parse(text)
      const b = body as { error?: unknown; code?: unknown }
      if (typeof b.error === 'string' && b.error) message = b.error
      if (typeof b.code === 'string' && b.code) code = b.code
    } catch {
      message = text.length > 300 ? text.slice(0, 300) + '…' : text
    }
  }
  return new ApiError(status, code, message, undefined, body)
}

/** Build the multipart body. */
export function uploadForm(file: Blob, name: string | undefined, field: string, fields: UploadOptions['fields'] = {}): FormData {
  const fd = new FormData()
  for (const [k, v] of Object.entries(fields ?? {})) if (v !== undefined && v !== null && v !== '') fd.append(k, v)
  if (name) fd.append(field, file, name)
  else fd.append(field, file)
  return fd
}

/** Upload one file; resolves the parsed JSON response. */
export function upload<T = unknown>(path: string, file: Blob, opts: UploadOptions = {}): Promise<T> {
  const xhr = opts.xhr ? opts.xhr() : new XMLHttpRequest()
  const name = (file as File).name
  return new Promise<T>((resolve, reject) => {
    xhr.open(opts.method ?? 'POST', buildUrl(path))
    xhr.withCredentials = true
    xhr.setRequestHeader('Accept', 'application/json')
    const csrf = getCsrfToken()
    if (csrf) xhr.setRequestHeader('X-CSRF-Token', csrf)
    if (xhr.upload && opts.onProgress) {
      xhr.upload.onprogress = (e: ProgressEvent) => {
        if (e.lengthComputable) opts.onProgress?.(progressPercent(e.loaded, e.total))
      }
    }
    xhr.onload = () => {
      const text = xhr.responseText ?? ''
      if (xhr.status >= 200 && xhr.status < 300) {
        opts.onProgress?.(100)
        if (!text) return resolve(undefined as T)
        try {
          resolve(JSON.parse(text) as T)
        } catch {
          resolve(text as unknown as T)
        }
        return
      }
      const err = uploadError(xhr.status, text)
      handleErrorResponse(err)
      reject(err)
    }
    xhr.onerror = () => reject(new ApiError(0, 'network', 'Network error'))
    xhr.onabort = () => reject(new ApiError(0, 'aborted', 'Upload cancelled'))
    opts.signal?.addEventListener('abort', () => xhr.abort())
    xhr.send(uploadForm(file, name, opts.field ?? 'file', opts.fields))
  })
}
