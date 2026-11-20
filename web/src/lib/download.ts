// Helpers for file downloads (export endpoint).

/**
 * Extract the filename from a Content-Disposition header. Supports RFC 5987
 * `filename*=UTF-8''...` (preferred), quoted and bare `filename=` forms.
 */
export function filenameFromDisposition(header: string | null | undefined, fallback = 'download'): string {
  if (!header) return fallback
  const star = /filename\*\s*=\s*([^;]+)/i.exec(header)
  if (star) {
    let v = star[1].trim().replace(/^"(.*)"$/, '$1')
    const m = /^([\w-]*)'[^']*'(.*)$/.exec(v)
    if (m) v = m[2]
    try {
      const decoded = decodeURIComponent(v)
      if (decoded) return sanitizeFilename(decoded, fallback)
    } catch {
      /* fall through to plain filename */
    }
  }
  const quoted = /filename\s*=\s*"((?:\\.|[^"\\])*)"/i.exec(header)
  if (quoted) return sanitizeFilename(quoted[1].replace(/\\(.)/g, '$1'), fallback)
  const bare = /filename\s*=\s*([^;\s]+)/i.exec(header)
  if (bare) return sanitizeFilename(bare[1], fallback)
  return fallback
}

/** Strip directory components and control characters. */
export function sanitizeFilename(name: string, fallback = 'download'): string {
  // eslint-disable-next-line no-control-regex
  const base = name.replace(/[\u0000-\u001f]/g, '').split(/[\\/]/).pop()?.trim() ?? ''
  return base && base !== '.' && base !== '..' ? base : fallback
}

/** Trigger a browser download for a blob. */
export function saveBlob(blob: Blob, filename: string): void {
  if (typeof document === 'undefined' || typeof URL.createObjectURL !== 'function') return
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

/** Export formats that contain requirements only (ReqIF); aspects do not apply. */
export const REQIF_FORMATS: readonly string[] = ['reqif', 'reqifz']

/** Whether an export format is ReqIF (`.reqif` or `.reqifz`). */
export function isReqIFFormat(format: string): boolean {
  return REQIF_FORMATS.includes(format)
}

/** The file extension of an export: CSV with several aspects is a ZIP archive. */
export function exportExtension(format: string, aspectCount: number): string {
  return format === 'csv' && aspectCount > 1 ? 'zip' : format
}
