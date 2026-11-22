// MITRE ATT&CK and ATLAS techniques (the catalogue's list, GET
// /catalog/attack/techniques) for the risk dialog, the catalogue filters
// and the coverage view (GET /catalog/attack).
import { api } from '@/lib/api'

export interface AttackTactic {
  id: string
  name: string
  matrix?: string
  url?: string
}

export interface AttackTechnique {
  id: string
  /** with the parent's name for a sub-technique ("Brute Force: Credential Stuffing") */
  name: string
  /** enterprise | ics | mobile | atlas */
  matrix: string
  tactics: AttackTactic[]
  url: string
}

export interface AttackMatrix {
  id: string
  name: string
  version: string
  url: string
}

export interface TechniqueList {
  matrices: AttackMatrix[]
  tactics: AttackTactic[]
  items: AttackTechnique[]
}

export interface CoverageTechnique {
  id: string
  name: string
  url: string
  risks: number
  requirements: number
}

export interface CoverageTactic {
  id: string
  name: string
  url: string
  risks: number
  requirements: number
  techniques: CoverageTechnique[]
}

export interface CoverageMatrix extends AttackMatrix {
  risks: number
  tactics: CoverageTactic[]
}

let cached: Promise<TechniqueList> | null = null

/** The technique list, fetched once per page load. */
export function loadTechniques(): Promise<TechniqueList> {
  if (!cached) {
    cached = api.get<TechniqueList>('/catalog/attack/techniques').catch((e) => {
      cached = null
      throw e
    })
  }
  return cached
}

/** Forgets the cached list (tests). */
export function resetTechniques() {
  cached = null
}

/** The canonical form of a technique id typed by a user (" aml.t0051 " → "AML.T0051"). */
export function normalizeTechnique(id: string): string {
  return id.trim().toUpperCase()
}

/**
 * Splits text into technique ids (separated by commas, semicolons or
 * white space), normalised and without duplicates, and the ones the list
 * does not know.
 */
export function parseTechniques(text: string, known: ReadonlyMap<string, AttackTechnique>): { ids: string[]; unknown: string[] } {
  const ids: string[] = []
  const unknown: string[] = []
  for (const raw of text.split(/[\s,;]+/)) {
    const id = normalizeTechnique(raw)
    if (!id || ids.includes(id) || unknown.includes(id)) continue
    if (known.has(id)) ids.push(id)
    else unknown.push(id)
  }
  return { ids, unknown }
}

/** The ids of the techniques whose id or name contains q (at most limit). */
export function searchTechniques(list: readonly AttackTechnique[], q: string, limit = 20): AttackTechnique[] {
  const s = q.trim().toLowerCase()
  if (!s) return list.slice(0, limit)
  const exact = list.filter((t) => t.id.toLowerCase() === s)
  const rest = list.filter((t) => t.id.toLowerCase() !== s && (t.id.toLowerCase().startsWith(s) || t.name.toLowerCase().includes(s)))
  return [...exact, ...rest].slice(0, limit)
}

/** "T1110.004 Brute Force: Credential Stuffing" (the id alone when unknown). */
export function techniqueLabel(id: string, known?: ReadonlyMap<string, AttackTechnique>): string {
  const t = known?.get(id)
  return t ? `${t.id} ${t.name}` : id
}

/** Whether one of ids is want or a sub-technique of it. */
export function hasTechnique(ids: readonly string[], want: string): boolean {
  const w = normalizeTechnique(want)
  return ids.some((id) => id === w || id.startsWith(w + '.'))
}

/** The link to a technique's page on attack.mitre.org or atlas.mitre.org. */
export function techniqueUrl(id: string): string {
  if (id.startsWith('AML.')) return `https://atlas.mitre.org/techniques/${id}`
  const [base, sub] = id.split('.')
  return sub ? `https://attack.mitre.org/techniques/${base}/${sub}/` : `https://attack.mitre.org/techniques/${base}/`
}
