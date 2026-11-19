// Defect references on executions (IDs such as JIRA-123 or URLs).

const URL_RE = /^https?:\/\/\S+$/i

/** Whether the value is already an absolute http(s) URL. */
export function isUrl(value: string | null | undefined): boolean {
  return !!value && URL_RE.test(value.trim())
}

/**
 * Link target for a defect: URLs are returned unchanged; IDs are inserted
 * into the project's template at every `{id}` (appended when the template
 * has no placeholder). Returns null when no link can be built.
 */
export function defectUrl(id: string | null | undefined, template?: string | null): string | null {
  const v = (id ?? '').trim()
  if (!v) return null
  if (isUrl(v)) return v
  const t = (template ?? '').trim()
  if (!t || !isUrl(t.replace(/\{id\}/g, 'x'))) return null
  const enc = encodeURIComponent(v)
  return t.includes('{id}') ? t.replace(/\{id\}/g, enc) : t + enc
}

/** Short label for display: IDs as is, URLs as their last meaningful path part. */
export function defectLabel(value: string | null | undefined): string {
  const v = (value ?? '').trim()
  if (!isUrl(v)) return v
  try {
    const u = new URL(v)
    const params = ['id', 'issue', 'key', 'selectedIssue']
    for (const p of params) {
      const q = u.searchParams.get(p)
      if (q) return q
    }
    const parts = u.pathname.split('/').filter(Boolean)
    if (parts.length) return decodeURIComponent(parts[parts.length - 1])
    return u.host
  } catch {
    return v
  }
}

/** Trim, drop empties and duplicates (case-insensitive), keep order. */
export function normalizeDefects(list: readonly (string | null | undefined)[] | null | undefined): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const raw of list ?? []) {
    const v = (raw ?? '').trim()
    if (!v) continue
    const k = v.toLowerCase()
    if (seen.has(k)) continue
    seen.add(k)
    out.push(v)
  }
  return out
}

export interface DefectLink {
  value: string
  label: string
  url: string | null
}

/** Defects prepared for rendering. */
export function defectLinks(list: readonly string[] | null | undefined, template?: string | null): DefectLink[] {
  return normalizeDefects(list).map((value) => ({ value, label: defectLabel(value), url: defectUrl(value, template) }))
}

/** Problem with a template: `not_url` (invalid), `no_placeholder` (works, ID appended) or null. */
export function templateProblem(template: string | null | undefined): 'not_url' | 'no_placeholder' | null {
  const t = (template ?? '').trim()
  if (!t) return null
  if (!isUrl(t.replace(/\{id\}/g, 'x'))) return 'not_url'
  if (!t.includes('{id}')) return 'no_placeholder'
  return null
}

/** Whether a template is usable: empty, or an http(s) URL (ideally containing {id}); English message. */
export function validateTemplate(template: string | null | undefined): string | null {
  switch (templateProblem(template)) {
    case 'not_url':
      return 'The template must be an http(s) URL, e.g. https://jira.example.com/browse/{id}'
    case 'no_placeholder':
      return 'Without {id} the defect ID is appended to the URL.'
    default:
      return null
  }
}
