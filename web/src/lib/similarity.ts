// Similar requirements and tests (internal/similarity): the suggestions
// while editing (POST /projects/{pid}/similar), the review of suspected
// duplicates and potential conflicts (GET /projects/{pid}/consistency/
// review) and its actions, performed with the ordinary endpoints.
import { api } from './api'
import { DUPLICATE_LINK_LABEL } from './consistency'

export type SimilarKind = 'requirement' | 'test'

export interface SimilarItem {
  id: string
  kind: SimilarKind
  key: string
  title: string
  status: string
  /** trace links of the item */
  links: number
  created_at: string
}

export interface SimilarHit extends SimilarItem {
  score: number
}

export interface SimilarResponse {
  engine: 'tfidf' | 'embeddings'
  threshold: number
  items: SimilarHit[]
}

export interface EvidencePart {
  text: string
  mark?: boolean
}

export interface Evidence {
  field: 'title' | 'description' | 'details'
  parts: EvidencePart[]
}

export interface DuplicatePair {
  a: SimilarItem
  b: SimilarItem
  score: number
  dismissed: boolean
}

export type ConflictKind = 'obligation' | 'threshold' | 'exclusive'

export interface ConflictPair {
  a: SimilarItem
  b: SimilarItem
  kind: ConflictKind
  reason: string
  confidence: number
  similarity: number
  note: string
  evidence_a: Evidence
  evidence_b: Evidence
  dismissed: boolean
}

export interface ReviewResponse {
  kind: SimilarKind
  engine: 'tfidf' | 'embeddings'
  threshold: number
  conflict_confidence: number
  duplicates: DuplicatePair[]
  conflicts: ConflictPair[]
  dismissed: number
  truncated?: boolean
}

export interface MergeResponse {
  survivor: SimilarItem
  duplicate: SimilarItem
  moved: { links: number; relations: number; dependencies: number; item_links: number; dropped: number }
}

/** A score 0–1 as a whole percentage ("82 %" in the caller's format). */
export function percent(score: number): number {
  return Math.floor(score * 100 + 1e-9)
}

/** The text fields of an editor form for the similarity request. */
export interface SimilarText {
  title: string
  description: string
  details: string[]
}

/** The request body; null when there is nothing to compare yet. */
export function similarBody(kind: SimilarKind, text: SimilarText, excludeId?: string | null, limit = 5): Record<string, unknown> | null {
  const details = text.details.map((d) => (d ?? '').trim()).filter(Boolean)
  const title = (text.title ?? '').trim()
  const description = (text.description ?? '').trim()
  // a few words are needed before a comparison means anything
  if ((title + ' ' + description + ' ' + details.join(' ')).trim().length < 8) return null
  return { kind, title, description, details, exclude_id: excludeId ?? '', limit }
}

/** A key that changes when the compared text changes. */
export function similarKey(body: Record<string, unknown> | null): string {
  return body ? JSON.stringify(body) : ''
}

/** The dismissable check of a pair. */
export function dismissCheck(kind: SimilarKind, conflict: boolean): string {
  if (conflict) return 'potential_conflict'
  return kind === 'test' ? 'test_duplicate_suspect' : 'duplicate_suspect'
}

/** The item proposed to survive a merge: the one with more trace links, else the older one (a). */
export function suggestedSurvivor(p: Pick<DuplicatePair, 'a' | 'b'>): SimilarItem {
  return p.b.links > p.a.links ? p.b : p.a
}

/** Marks newer as a duplicate of older (a relation, an item link between tests). */
export async function markDuplicate(kind: SimilarKind, newer: SimilarItem, older: SimilarItem): Promise<void> {
  if (kind === 'test') {
    await api.post(`/tests/${newer.id}/item-links`, { target_type: 'test', target_id: older.id, label: DUPLICATE_LINK_LABEL })
    return
  }
  await api.post(`/requirements/${newer.id}/relations`, { target_id: older.id, type: 'duplicates' })
}

/** Records that two requirements conflict (relation conflicts_with). */
export async function markConflict(newer: SimilarItem, older: SimilarItem): Promise<void> {
  await api.post(`/requirements/${newer.id}/relations`, { target_id: older.id, type: 'conflicts_with' })
}

/** Dismisses a finding about a pair (dismiss=false restores it). */
export async function dismissPair(projectId: string, check: string, a: SimilarItem, b: SimilarItem, dismiss = true): Promise<void> {
  const body = { check, item_id: b.id, other_id: a.id }
  if (dismiss) await api.post(`/projects/${projectId}/consistency/dismissals`, body)
  else await api.del(`/projects/${projectId}/consistency/dismissals`, body)
}

/** Merges duplicate into survivor. */
export function mergePair(projectId: string, kind: SimilarKind, survivor: SimilarItem, duplicate: SimilarItem): Promise<MergeResponse> {
  return api.post<MergeResponse>(`/projects/${projectId}/consistency/merge`, { kind, survivor_id: survivor.id, duplicate_id: duplicate.id })
}

/** Severity of a similarity score for a tag: at or above the threshold a likely duplicate. */
export function scoreSeverity(score: number, threshold: number): 'warn' | 'secondary' {
  return score >= threshold ? 'warn' : 'secondary'
}
