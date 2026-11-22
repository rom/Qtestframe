import type { Project } from './types'

/** Who started a project, or null when unknown (no creator recorded, or the account no longer exists). */
export function projectCreator(p: Pick<Project, 'created_by' | 'created_by_name' | 'created_by_email'>): { name: string; email: string } | null {
  const name = (p.created_by_name || p.created_by_email || '').trim()
  if (!name) return null
  return { name, email: (p.created_by_email || '').trim() }
}
