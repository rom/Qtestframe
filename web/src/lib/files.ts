// Files, attachments and the document archive: pure helpers (sizes, types,
// previews, upload checks, link targets). See docs/api.md "Files".
import { API_BASE } from './api'
import type { FileInfo, FileLink } from './types'

/** Server default for files.max_upload_mb. */
export const DEFAULT_MAX_UPLOAD_MB = 25
export const MB = 1024 * 1024

/** Entities that take attachments, with their API collection. */
export const ATTACHABLE = { requirement: 'requirements', test: 'tests', run: 'runs', execution: 'executions', step: 'step-results', exploratory_session: 'exploratory-sessions' } as const
export type AttachableType = keyof typeof ATTACHABLE

/** Archive kinds a user can filter by. */
export const ARCHIVE_KINDS = ['attachment', 'document', 'report'] as const

/** Image types the server serves inline and accepts for avatars and icons. */
export const IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'] as const
/** `accept` attribute for avatar / icon pickers. */
export const IMAGE_ACCEPT = IMAGE_TYPES.join(',')

const EXT_TYPES: Record<string, string> = {
  png: 'image/png',
  jpg: 'image/jpeg',
  jpeg: 'image/jpeg',
  gif: 'image/gif',
  webp: 'image/webp',
  svg: 'image/svg+xml',
  pdf: 'application/pdf',
  txt: 'text/plain',
  log: 'text/plain',
  md: 'text/markdown',
  csv: 'text/csv',
  json: 'application/json',
  xml: 'application/xml',
  zip: 'application/zip',
  doc: 'application/msword',
  docx: 'application/vnd.openxmlformats-officedocument.wordprocessingml.document',
  xls: 'application/vnd.ms-excel',
  xlsx: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet',
  ppt: 'application/vnd.ms-powerpoint',
  pptx: 'application/vnd.openxmlformats-officedocument.presentationml.presentation',
}

/** Lower-case extension without the dot ("" when none). */
export function extension(name: string | null | undefined): string {
  const m = /\.([a-z0-9]{1,8})$/i.exec(name ?? '')
  return m ? m[1].toLowerCase() : ''
}

/** Content type without parameters, guessed from the name when missing or generic. */
export function effectiveType(contentType: string | null | undefined, name?: string | null): string {
  const t = (contentType ?? '').split(';')[0].trim().toLowerCase()
  if (t && t !== 'application/octet-stream') return t
  return EXT_TYPES[extension(name)] ?? (t || 'application/octet-stream')
}

/**
 * "0 B", "512 B", "1.5 KB", "2.3 MB", "1.1 GB" (binary units, locale-aware
 * decimals, e.g. sv "2,3 MB").
 */
export function formatFileSize(bytes: number | null | undefined, locale = 'en'): string {
  if (bytes === null || bytes === undefined || !Number.isFinite(bytes) || bytes < 0) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = bytes
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  const digits = i === 0 || v >= 100 ? 0 : 1
  let text: string
  try {
    text = new Intl.NumberFormat(locale, { maximumFractionDigits: digits, minimumFractionDigits: 0 }).format(v)
  } catch {
    text = v.toFixed(digits)
  }
  return `${text} ${units[i]}`
}

export type PreviewKind = 'image' | 'pdf' | 'text' | 'download'

/**
 * How a file can be shown: images (png, jpeg, gif, webp) in an <img>, PDF and
 * plain text in a sandboxed iframe (the types `?inline=1` serves inline);
 * everything else (SVG included) is downloaded.
 */
export function previewKind(file: Pick<FileInfo, 'content_type' | 'name'> | null | undefined): PreviewKind {
  if (!file) return 'download'
  const t = effectiveType(file.content_type, file.name)
  if ((IMAGE_TYPES as readonly string[]).includes(t)) return 'image'
  if (t === 'application/pdf') return 'pdf'
  if (t === 'text/plain') return 'text'
  return 'download'
}

/** PrimeIcons class for a file. */
export function fileIcon(contentType: string | null | undefined, name?: string | null): string {
  const t = effectiveType(contentType, name)
  const ext = extension(name)
  if (t.startsWith('image/')) return 'pi pi-image'
  if (t === 'application/pdf') return 'pi pi-file-pdf'
  if (t.includes('spreadsheet') || t.includes('excel') || t === 'text/csv' || ext === 'xlsx' || ext === 'xls' || ext === 'csv') return 'pi pi-file-excel'
  if (t.includes('wordprocessing') || t.includes('msword') || ext === 'docx' || ext === 'doc') return 'pi pi-file-word'
  if (t.includes('zip') || t.includes('compressed') || ext === 'zip' || ext === 'gz' || ext === 'tgz') return 'pi pi-box'
  if (t.startsWith('video/')) return 'pi pi-video'
  if (t.startsWith('audio/')) return 'pi pi-volume-up'
  if (t.startsWith('text/') || t === 'application/json' || t === 'application/xml') return 'pi pi-file-edit'
  return 'pi pi-file'
}

export type UploadProblem = 'empty' | 'too_large' | 'type'

/** Why a file cannot be uploaded (null when it can). */
export function uploadProblem(file: { size: number; type?: string; name?: string }, maxBytes: number, accept?: readonly string[]): UploadProblem | null {
  if (!file.size) return 'empty'
  if (maxBytes > 0 && file.size > maxBytes) return 'too_large'
  if (accept?.length && !accept.includes(effectiveType(file.type, file.name))) return 'type'
  return null
}

/** Download / inline URL of a file's content. */
export function contentUrl(id: string, inline = false): string {
  return `${API_BASE}/files/${encodeURIComponent(id)}/content${inline ? '?inline=1' : ''}`
}

/** API path of an item's attachments ("/requirements/<id>/attachments"). */
export function attachmentsPath(type: AttachableType, id: string): string {
  return `/${ATTACHABLE[type]}/${encodeURIComponent(id)}/attachments`
}

/** Comma separated tags (upload field) from a list, and back. */
export function tagsField(tags: readonly string[] | null | undefined): string {
  return (tags ?? []).map((t) => t.trim()).filter(Boolean).join(',')
}

export interface LinkTarget {
  name: string
  params: Record<string, string>
  query?: Record<string, string>
}

/**
 * Route of a linked item for the "linked to" chips. Execution links open
 * the execution runner using the link's `run_id` (or `runId` when given),
 * otherwise the operations list.
 */
export function linkTarget(link: FileLink, projectId: string, runId?: string): LinkTarget | null {
  const params = { projectId }
  switch (link.entity_type) {
    case 'requirement':
      return { name: 'requirements', params, query: { item: link.entity_id } }
    case 'test':
      return { name: 'tests', params, query: { item: link.entity_id } }
    case 'run':
      return { name: 'run', params: { ...params, runId: link.entity_id } }
    case 'execution':
      runId = runId || link.run_id
      return runId ? { name: 'execute', params: { ...params, runId, executionId: link.entity_id } } : { name: 'runs', params }
    case 'step':
      // evidence of a step: the runner of its execution, at that step
      runId = runId || link.run_id
      return runId && link.execution_id
        ? { name: 'execute', params: { ...params, runId, executionId: link.execution_id }, query: { step: link.entity_id } }
        : { name: 'runs', params }
    case 'exploratory_session':
      return { name: 'exploratorySession', params: { ...params, sessionId: link.entity_id } }
    default:
      return null
  }
}

/** PrimeIcons class of a linked item's type. */
export function linkIcon(type: string): string {
  switch (type) {
    case 'requirement':
      return 'pi pi-list-check'
    case 'test':
      return 'pi pi-check-square'
    case 'run':
      return 'pi pi-play-circle'
    case 'execution':
      return 'pi pi-play'
    case 'step':
      return 'pi pi-camera'
    case 'exploratory_session':
      return 'pi pi-compass'
    default:
      return 'pi pi-link'
  }
}

/** Sort files newest first, then by name. */
export function sortFiles<T extends Pick<FileInfo, 'created_at' | 'name'>>(files: readonly T[]): T[] {
  return [...files].sort((a, b) => (b.created_at ?? '').localeCompare(a.created_at ?? '') || a.name.localeCompare(b.name))
}

/** Query parameters of GET /projects/{pid}/documents. */
export function archiveParams(kind: string | null | undefined, q: string | null | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  if (kind && (ARCHIVE_KINDS as readonly string[]).includes(kind)) out.kind = kind
  const query = (q ?? '').trim()
  if (query) out.q = query
  return out
}

/** Archive query fields (docs/api.md). */
export const ARCHIVE_FIELDS = ['name', 'description', 'tag', 'kind', 'type', 'uploaded_by', 'created_at', 'size', 'linked'] as const

/** Upload percentage 0–100 from a progress event. */
export function progressPercent(loaded: number, total: number): number {
  if (!total || total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((loaded / total) * 100)))
}
