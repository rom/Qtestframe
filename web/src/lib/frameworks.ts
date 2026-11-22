// MITRE D3FEND and EMB3D (the catalogue's lists, GET /catalog/d3fend and
// GET /catalog/emb3d) for the pickers of requirements, tests and risks, the
// catalogue preview and filters, and the coverage views
// (GET /catalog/d3fend/coverage, GET /catalog/emb3d/coverage).
import { api } from '@/lib/api'
import type { AttackTechnique } from '@/lib/attack'

export const D3FEND_TACTICS = ['Model', 'Harden', 'Detect', 'Isolate', 'Deceive', 'Evict', 'Restore'] as const
export const EMB3D_CATEGORIES = ['hardware', 'system software', 'application software', 'networking'] as const

export interface D3fendTechnique {
  id: string
  name: string
  /** Model, Harden, Detect, Isolate, Deceive, Evict or Restore */
  tactic: string
  parent?: string
  parent_name?: string
  url: string
  /** the number of ATT&CK techniques it counters */
  attack?: number
}

export interface Emb3dItem {
  id: string
  /** threat (TID-nnn) or mitigation (MID-nnn); "" for an unknown id */
  kind: 'threat' | 'mitigation' | ''
  name: string
  category?: string
  maturity?: string
  url: string
}

export interface D3fendList {
  version: string
  release: string
  url: string
  tactics: { id: string; name: string; url: string }[]
  items: D3fendTechnique[]
}

export interface Emb3dList {
  version: string
  url: string
  categories: string[]
  properties: { id: string; name: string; category: string; parent?: string }[]
  items: Emb3dItem[]
}

/** A catalogue requirement whose D3FEND techniques counter an ATT&CK technique of a risk. */
export interface DefenseRequirement {
  id: string
  title: string
  d3fend: string[]
  /** the risk lists it in mitigated_by */
  mitigates: boolean
}

/** An ATT&CK technique of a catalogue risk with the defensive techniques and requirements that counter it. */
export interface RiskDefense {
  technique: AttackTechnique
  d3fend: D3fendTechnique[]
  requirements: DefenseRequirement[]
  /** a mitigating requirement counters it (else a gap) */
  covered: boolean
}

export interface D3fendCoverageTactic {
  id: string
  name: string
  url: string
  requirements: number
  tests: number
  used: number
  total: number
  techniques: { id: string; name: string; parent?: string; url: string; requirements: number; tests: number }[]
}

export interface D3fendCoverage {
  version: string
  url: string
  tactics: D3fendCoverageTactic[]
  risks: number
  covered: number
  gaps: number
}

export interface Emb3dCoverageCategory {
  id: string
  risks: number
  requirements: number
  tests: number
  addressed: number
  total: number
  threats: { id: string; name: string; url: string; risks: number; requirements: number; tests: number }[]
}

export interface Emb3dCoverage {
  version: string
  url: string
  categories: Emb3dCoverageCategory[]
  mitigations: number
}

function cachedGet<T>(path: string) {
  let cached: Promise<T> | null = null
  return {
    load(): Promise<T> {
      if (!cached) {
        cached = api.get<T>(path).catch((e) => {
          cached = null
          throw e
        })
      }
      return cached
    },
    reset() {
      cached = null
    },
  }
}

const d3fend = cachedGet<D3fendList>('/catalog/d3fend')
const emb3d = cachedGet<Emb3dList>('/catalog/emb3d')

/** The D3FEND list, fetched once per page load. */
export const loadD3fend = () => d3fend.load()
/** The EMB3D list, fetched once per page load. */
export const loadEmb3d = () => emb3d.load()
/** Forgets the cached lists (tests). */
export function resetFrameworks() {
  d3fend.reset()
  emb3d.reset()
}

/** The canonical form of an id typed by a user (" d3-mfa " → "D3-MFA"). */
export function normalizeFrameworkId(id: string): string {
  return id.trim().toUpperCase()
}

/** The kind of an EMB3D id by its form. */
export function emb3dKind(id: string): 'threat' | 'mitigation' | '' {
  return /^TID-\d{3}$/.test(id) ? 'threat' : /^MID-\d{3}$/.test(id) ? 'mitigation' : ''
}

/** "D3-MFA Multi-factor Authentication" (the id alone when unknown). */
export function frameworkLabel(id: string, known?: ReadonlyMap<string, { name: string }>): string {
  const x = known?.get(id)
  return x?.name ? `${id} ${x.name}` : id
}

/** The page of a D3FEND technique or an EMB3D threat or mitigation ("" for an unknown form). */
export function frameworkUrl(id: string, known?: ReadonlyMap<string, { url: string }>): string {
  const x = known?.get(id)
  if (x?.url) return x.url
  if (emb3dKind(id) === 'threat') return `https://emb3d.mitre.org/threats/${id}.html`
  if (emb3dKind(id) === 'mitigation') return `https://emb3d.mitre.org/mitigations/${id}.html`
  return ''
}

/** The ids of ids that known does not have (unknown, revoked or mistyped). */
export function unknownIds(ids: readonly string[], known: ReadonlyMap<string, unknown> | null): string[] {
  return known ? ids.filter((id) => !known.has(id)) : []
}

/** The options of a picker: every item of the list, with the ids no longer in it first (so that they can be removed). */
export function pickerOptions<T extends { id: string; name: string }>(list: readonly T[], value: readonly string[], loaded: boolean): { label: string; value: string }[] {
  const opts = list.map((x) => ({ label: `${x.id} ${x.name}`, value: x.id }))
  if (loaded) {
    const ids = new Set(list.map((x) => x.id))
    for (const id of value) if (!ids.has(id)) opts.unshift({ label: id, value: id })
  }
  return opts
}

/** The share as a whole percentage ("–" for none). */
export function share(n: number, of: number): string {
  return of > 0 ? `${Math.round((100 * n) / of)} %` : '–'
}
