import { describe, expect, it } from 'vitest'
import { confirmImpact, impactGate, impactGateFor, settleImpact } from './impact'
import type { ChangeImpact } from '@/lib/correlations'

const imp = (total: number) => ({ kind: '', id: '', key: '', title: '', requirements: [], tests: [], issues: [], documents: [], results: [], total }) as ChangeImpact
const tick = () => new Promise((r) => setTimeout(r, 0))

describe('impact gate', () => {
  it('asks only when something is affected', async () => {
    expect(await confirmImpact(imp(0), 'p')).toBe('suspect')
    expect(await confirmImpact(null, 'p')).toBe('suspect')
    const asked = confirmImpact(imp(2), 'p', ['title'])
    expect(impactGate.open).toBe(true)
    expect(impactGate.fields).toEqual(['title'])
    settleImpact('notify')
    expect(await asked).toBe('notify')
    expect(impactGate.open).toBe(false)
  })
  it('runs the dry run and resolves the parameters', async () => {
    const p = impactGateFor('p', async (q) => {
      expect(q).toEqual({ dry_run: 'impact' })
      return { dry_run: 'impact', status: 200, impact: imp(1) }
    })
    await tick()
    expect(impactGate.open).toBe(true)
    settleImpact(null)
    expect(await p).toBeNull()
    expect(
      await impactGateFor('p', async () => {
        throw new Error('x')
      }),
    ).toEqual({ impact: 'suspect' })
    expect(await impactGateFor('p', async () => ({ dry_run: 'impact', status: 200, impact: imp(0) }))).toEqual({ impact: 'suspect' })
  })
})
