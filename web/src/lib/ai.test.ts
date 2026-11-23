import { afterEach, describe, expect, it, vi } from 'vitest'
import { addCweTags, addIds, aiStatus, clearAiStatus, draftTestBody, featureState, formatCost, parseSse, runAssist, splitFixed, streamPost, type AiStatus, type PromptAnswer } from './ai'
import { ApiError } from './api'
import type { LanguageIssue } from './types'

const status = (over: Partial<AiStatus> = {}): AiStatus => ({
  available: true,
  features: { draft_tests: true, suggest_criteria: false },
  system_enabled: true,
  organisation_enabled: true,
  rate_limit: 30,
  remaining: 30,
  price_input: 0,
  price_output: 0,
  prompt_version: 'v',
  ...over,
})

function streamResponse(chunks: string[]): Response {
  const enc = new TextEncoder()
  const body = new ReadableStream<Uint8Array>({
    start(c) {
      for (const ch of chunks) c.enqueue(enc.encode(ch))
      c.close()
    },
  })
  return new Response(body, { status: 200, headers: { 'Content-Type': 'text/event-stream' } })
}

afterEach(() => {
  vi.restoreAllMocks()
  clearAiStatus()
})

describe('ai', () => {
  it('parses server-sent events across chunks', () => {
    const a = parseSse('event: start\ndata: {"a":1}\n\n: ping\n\nevent: progress\ndata: {"ch')
    expect(a.events).toEqual([{ event: 'start', data: '{"a":1}' }])
    const b = parseSse(a.rest + 'ars":5}\r\n\r\ndata: x\ndata: y\n\n')
    expect(b.events).toEqual([
      { event: 'progress', data: '{"chars":5}' },
      { event: 'message', data: 'x\ny' },
    ])
    expect(b.rest).toBe('')
  })

  it('runs a streamed request and reports progress', async () => {
    const fetch = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      streamResponse(['event: start\ndata: {}\n\nevent: progress\ndata: {"chars":12}\n\n', 'event: result\ndata: {"meta":{"model":"m"},"result":{"criteria":[]}}\n\n']),
    )
    const progress: number[] = []
    const r = await runAssist<{ criteria: unknown[] }>('p1', 'suggest_criteria', { requirement_id: 'r' }, { onProgress: (n) => progress.push(n) })
    expect(r.meta.model).toBe('m')
    expect(progress).toEqual([12])
    const [url, init] = fetch.mock.calls[0]
    expect(url).toBe('/api/v1/projects/p1/ai/suggest-criteria')
    expect((init?.headers as Record<string, string>).Accept).toBe('text/event-stream')
  })

  it('streams the deltas of a sent prompt', async () => {
    const fetch = vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      streamResponse(['event: start\ndata: {}\n\nevent: delta\ndata: {"text":"# Arch"}\n\n', 'event: delta\ndata: {"text":"itecture"}\n\nevent: result\ndata: {"answer":"# Architecture","truncated":false,"meta":{"model":"m"}}\n\n']),
    )
    let text = ''
    const r = await streamPost<PromptAnswer>('/projects/p1/prompts/send', { kind: 'architecture' }, { onDelta: (d) => (text += d) })
    expect(text).toBe('# Architecture')
    expect(r.answer).toBe('# Architecture')
    expect(fetch.mock.calls[0][0]).toBe('/api/v1/projects/p1/prompts/send')
    expect(JSON.parse(String(fetch.mock.calls[0][1]?.body)).kind).toBe('architecture')
  })

  it('turns error events and error responses into ApiErrors', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(streamResponse(['event: error\ndata: {"status":502,"code":"ai_invalid_answer","error":"bad"}\n\n']))
    await expect(runAssist('p', 'draft_tests', {})).rejects.toMatchObject({ status: 502, code: 'ai_invalid_answer' })
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(new Response(JSON.stringify({ error: 'slow', code: 'rate_limited' }), { status: 429 }))
    const e = await runAssist('p', 'draft_tests', {}).catch((x) => x)
    expect(e).toBeInstanceOf(ApiError)
    expect(e.code).toBe('rate_limited')
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(streamResponse(['event: start\ndata: {}\n\n']))
    await expect(runAssist('p', 'draft_tests', {})).rejects.toMatchObject({ code: 'ai_service' })
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(new Response('{"meta":{},"result":1}', { status: 200, headers: { 'Content-Type': 'application/json' } }))
    expect((await runAssist('p', 'draft_tests', {})).result).toBe(1)
  })

  it('caches the status and explains unavailable features', async () => {
    const f = vi.fn().mockResolvedValue(status())
    await aiStatus('p', f)
    await aiStatus('p', f)
    expect(f).toHaveBeenCalledTimes(1)
    await aiStatus('p', f, true)
    expect(f).toHaveBeenCalledTimes(2)
    expect(featureState(status(), 'draft_tests')).toEqual({ ok: true, reason: '' })
    expect(featureState(status(), 'suggest_criteria').reason).toBe('ai.reason.feature_off')
    expect(featureState(status({ available: false, reason: 'project' }), 'draft_tests').reason).toBe('ai.reason.project')
    expect(featureState(null, 'draft_tests').reason).toBe('ai.reason.loading')
  })

  it('splits fixed findings', () => {
    const i = (rule: string, offset: number): LanguageIssue => ({ rule, severity: 'warning', message: rule, offset, length: 1 }) as LanguageIssue
    const r = splitFixed([i('tbd', 1), i('ambiguous-term', 2), i('ambiguous-term', 3)], ['ambiguous-term', 'tbd'])
    expect(r.fixed.map((x) => x.offset)).toEqual([1, 3])
    expect(r.remaining.map((x) => x.offset)).toEqual([2])
  })

  it('builds test bodies, tags and costs', () => {
    const b = draftTestBody(
      { title: ' T ', description: 'D', preconditions: 'P', test_data: 'alice', expected_result: 'E', steps: [{ action: 'a', data: '', expected: 'e' }, { action: ' ', data: '', expected: '' }], criteria: [], criterion_ids: [] },
      'Test data',
    )
    expect(b).toMatchObject({ title: 'T', preconditions: 'P\n\nTest data: alice', status: 'draft', execution_type: 'manual' })
    expect((b.steps as unknown[]).length).toBe(1)
    expect(draftTestBody({ title: 'x', description: '', preconditions: '', test_data: 'd', expected_result: '', steps: [], criteria: [], criterion_ids: [] }, 'Data').preconditions).toBe('Data: d')
    expect(addCweTags(['cwe-79', 'auth'], ['CWE-79', 'CWE-434'])).toEqual(['cwe-79', 'auth', 'CWE-434'])
    expect(addIds(null, ['a', 'a', 'b'])).toEqual(['a', 'b'])
    expect(formatCost(0, 'en')).toBe('')
    expect(formatCost(0.0006, 'en')).toBe('0.0006')
    expect(formatCost(1.5, 'en')).toBe('1.50')
  })
})
