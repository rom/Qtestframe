import { mkdtempSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { TestCase, TestResult } from '@playwright/test/reporter';
import QtestframeReporter, { displayTitle, findKey, junitXML, mapStatus, outputOf } from '../src/index';

// ---------------------------------------------------------------- fakes

interface FakeTest {
  title: string;
  describe?: string;
  project?: string;
  tags?: string[];
  annotations?: { type: string; description?: string }[];
  outcome?: 'expected' | 'unexpected' | 'flaky' | 'skipped';
}

let ids = 0;
function fakeTest(f: FakeTest): TestCase {
  const root: any = { type: 'root', title: '' };
  const project: any = { type: 'project', title: f.project ?? 'chromium', parent: root };
  const file: any = { type: 'file', title: 'shop.spec.ts', parent: project };
  const parent: any = f.describe ? { type: 'describe', title: f.describe, parent: file } : file;
  return {
    id: `t${ids++}`,
    title: f.title,
    parent,
    tags: f.tags ?? [],
    annotations: f.annotations ?? [],
    titlePath: () => ['', f.project ?? 'chromium', 'shop.spec.ts', ...(f.describe ? [f.describe] : []), f.title],
    outcome: () => f.outcome ?? 'expected',
  } as unknown as TestCase;
}

function fakeResult(r: Partial<TestResult> & { status: TestResult['status'] }): TestResult {
  return { duration: 100, retry: 0, errors: [], steps: [], attachments: [], ...r } as unknown as TestResult;
}

interface Call {
  method: string;
  path: string;
  body: any;
}

/** Stubs fetch with a fake Qtestframe API and records the calls. */
function fakeServer(opts: { runs?: any[]; fail?: string } = {}) {
  const calls: Call[] = [];
  const fetchMock = vi.fn(async (url: string, init: RequestInit) => {
    const u = new URL(url);
    const path = u.pathname.replace('/api/v1', '') + u.search;
    const method = init.method ?? 'GET';
    let body: any = init.body;
    if (typeof body === 'string') body = JSON.parse(body);
    calls.push({ method, path, body });
    expect((init.headers as Record<string, string>).Authorization).toBe('Bearer qtf_tok');
    const json = (status: number, v: unknown) => new Response(JSON.stringify(v), { status });
    if (opts.fail && path.startsWith(opts.fail)) return json(500, { error: 'boom' });
    if (path === '/projects') return json(200, { items: [{ id: 'p1', key: 'SHOP' }] });
    if (path === '/projects/p1/runs' && method === 'GET') return json(200, { items: opts.runs ?? [] });
    if (path === '/projects/p1/runs') return json(201, { id: 'run1' });
    const route = u.pathname.replace('/api/v1', '');
    if (route.endsWith('/results')) return json(200, { updated: body.results.length, unknown: ['SHOP-TC-99'], rejected: [{ test_key: 'SHOP-TC-2', reason: 'draft' }] });
    if (path.includes('/junit')) return json(200, { updated: 1, unknown: [], unmapped: [], rejected: [], created: [{ key: 'SHOP-TC-50', title: 'cart › empty' }] });
    if (path.endsWith('/executions'))
      return json(200, {
        items: [
          { id: 'e1', test_key: 'SHOP-TC-1', test_title: 'Pay' },
          { id: 'e50', test_key: 'SHOP-TC-50', test_title: 'cart › empty' },
          { id: 'e1ff', test_key: 'SHOP-TC-1', test_title: 'Pay', configuration_id: 'cff', configuration_name: 'Firefox' },
        ],
      });
    if (path.endsWith('/attachments')) return json(201, { id: 'f1' });
    return new Response('not found', { status: 404 });
  });
  vi.stubGlobal('fetch', fetchMock);
  return calls;
}

afterEach(() => {
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
});

// ---------------------------------------------------------------- helpers

describe('findKey', () => {
  it('prefers the annotation, then tags, then the title', () => {
    expect(findKey(fakeTest({ title: 'SHOP-TC-3', annotations: [{ type: 'Qtestframe', description: ' shop-tc-1 ' }], tags: ['@SHOP-TC-2'] }))).toBe('SHOP-TC-1');
    expect(findKey(fakeTest({ title: 'SHOP-TC-3', annotations: [{ type: 'issue', description: 'X-1' }, { type: 'qtestframe' }], tags: ['@smoke', '@SHOP-TC-2'] }))).toBe('SHOP-TC-2');
    expect(findKey(fakeTest({ title: 'pays SHOP-TC-3' }))).toBe('SHOP-TC-3');
    expect(findKey(fakeTest({ title: 'pays', describe: 'SHOP-TC-4 checkout' }))).toBe('SHOP-TC-4');
    expect(findKey(fakeTest({ title: 'no key' }))).toBeUndefined();
    expect(findKey(fakeTest({ title: 'JIRA-12 works' }), /JIRA-\d+/)).toBe('JIRA-12');
  });
});

describe('mapStatus', () => {
  it('maps Playwright statuses and outcomes', () => {
    const t = (outcome: FakeTest['outcome']) => fakeTest({ title: 'x', outcome });
    expect(mapStatus(t('expected'), fakeResult({ status: 'passed' }))).toBe('passed');
    expect(mapStatus(t('unexpected'), fakeResult({ status: 'failed' }))).toBe('failed');
    expect(mapStatus(t('unexpected'), fakeResult({ status: 'timedOut' }))).toBe('failed');
    expect(mapStatus(t('expected'), fakeResult({ status: 'interrupted' }))).toBe('failed');
    expect(mapStatus(t('skipped'), fakeResult({ status: 'skipped' }))).toBe('skipped');
    // test.fail(): an expected failure passes
    expect(mapStatus(t('expected'), fakeResult({ status: 'failed' }))).toBe('passed');
    expect(mapStatus(t('flaky'), fakeResult({ status: 'passed', retry: 1 }))).toBe('passed');
    const noOutcome = { ...fakeTest({ title: 'x' }), outcome: undefined } as unknown as TestCase;
    expect(mapStatus(noOutcome, fakeResult({ status: 'passed' }))).toBe('passed');
    expect(mapStatus(noOutcome, fakeResult({ status: 'timedOut' }))).toBe('failed');
  });
});

describe('outputOf', () => {
  it('lists errors and test.step() titles with their status', () => {
    const test = fakeTest({ title: 'Pay', describe: 'checkout', project: 'firefox' });
    expect(displayTitle(test)).toBe('checkout › Pay');
    const out = outputOf(
      test,
      fakeResult({
        status: 'failed',
        retry: 1,
        errors: [{ message: 'x', stack: '\u001b[31mError: expected 1\u001b[39m\n    at shop.spec.ts:3' }, { value: 'thrown' }, {}],
        steps: [
          { category: 'hook', title: 'beforeEach', duration: 1, steps: [{ category: 'test.step', title: 'login', duration: 5.4, steps: [] }] },
          {
            category: 'test.step',
            title: 'pay',
            duration: 20,
            error: { message: '\u001b[2mtimeout\u001b[22m\nmore' },
            steps: [{ category: 'pw:api', title: 'click', duration: 1 }, { category: 'test.step', title: 'confirm', duration: 2, steps: [] }],
          },
        ] as any,
      }),
    );
    expect(out).toBe(
      [
        '[project firefox, retry #1, status failed]',
        'Error: expected 1\n    at shop.spec.ts:3',
        'thrown',
        'Steps:',
        '  ✓ login (5 ms)',
        '  ✗ pay (20 ms): timeout',
        '    ✓ confirm (2 ms)',
      ].join('\n'),
    );
  });

  it('writes JUnit XML for tests without a key', () => {
    const xml = junitXML([
      { key: '', name: 'a <b> & "c"\u0001', status: 'failed', durationMs: 1500, outputs: ['out'], evidence: [] },
      { key: '', name: 'skip', status: 'skipped', durationMs: 0, outputs: [], evidence: [] },
      { key: '', name: "it's ok", status: 'passed', durationMs: 1, outputs: [], evidence: [] },
    ]);
    expect(xml).toContain('<testcase name="a &lt;b&gt; &amp; &quot;c&quot;" classname="playwright" time="1.500"><failure message="failed"/><system-out>out</system-out></testcase>');
    expect(xml).toContain('<testcase name="skip" classname="playwright" time="0.000"><skipped/></testcase>');
    expect(xml).toContain('name="it&apos;s ok"');
  });
});

// ---------------------------------------------------------------- reporter

describe('QtestframeReporter', () => {
  it('creates the run, reports results and uploads evidence of failures', async () => {
    const calls = fakeServer();
    const dir = mkdtempSync(join(tmpdir(), 'qtf-'));
    const shot = join(dir, 'test-failed-1.png');
    writeFileSync(shot, 'png');
    const big = join(dir, 'video.webm');
    writeFileSync(big, Buffer.alloc(64));
    const logs: string[] = [];
    const r = new QtestframeReporter({
      url: 'https://qtf.example/api/v1/',
      token: 'qtf_tok',
      project: 'shop',
      run: 'Nightly',
      createRun: true,
      build: '7',
      environment: 'staging',
      createMissing: true,
      maxAttachmentBytes: 32,
      logger: (m) => logs.push(m),
    });
    expect(r.printsToStdio()).toBe(false);
    r.onBegin({} as any, {} as any);

    const pay = fakeTest({ title: 'Pay @SHOP-TC-1', outcome: 'unexpected' });
    const payFF = fakeTest({ title: 'Pay @SHOP-TC-1', project: 'firefox' });
    const refund = fakeTest({ title: 'Refund', annotations: [{ type: 'qtestframe', description: 'SHOP-TC-2' }] });
    const empty = fakeTest({ title: 'empty', describe: 'cart', outcome: 'unexpected' });
    const flaky = fakeTest({ title: 'flaky SHOP-TC-3', outcome: 'flaky' });
    r.onTestEnd(pay, fakeResult({
      status: 'failed',
      duration: 1200,
      errors: [{ message: 'boom' }],
      attachments: [
        { name: 'screenshot', contentType: 'image/png', path: shot },
        { name: 'video', contentType: 'video/webm', path: big },
        { name: 'trace', contentType: 'application/zip', path: join(dir, 'missing.zip') },
        { name: 'stdout', contentType: 'text/plain' },
      ],
    }));
    r.onTestEnd(payFF, fakeResult({ status: 'passed', duration: 800 }));
    r.onTestEnd(refund, fakeResult({ status: 'passed', duration: 10 }));
    r.onTestEnd(empty, fakeResult({ status: 'failed', attachments: [{ name: 'error-context', contentType: 'text/markdown', body: Buffer.from('ctx') }, { name: 'huge', contentType: 'text/plain', body: Buffer.alloc(40) }] }));
    r.onTestEnd(flaky, fakeResult({ status: 'failed', duration: 5 }));
    r.onTestEnd(flaky, fakeResult({ status: 'passed', duration: 6, retry: 1 }));
    await r.onEnd({ status: 'failed' } as any);

    const paths = calls.map((c) => `${c.method} ${c.path}`);
    expect(paths).toEqual([
      'GET /projects',
      'GET /projects/p1/runs',
      'POST /projects/p1/runs',
      'POST /runs/run1/results',
      'POST /runs/run1/junit?create_missing=true',
      'GET /runs/run1/executions',
      'POST /executions/e1/attachments',
      'POST /executions/e50/attachments',
    ]);
    expect(calls[2].body).toEqual({ name: 'Nightly', build: '7', environment: 'staging' });
    const results = calls[3].body.results;
    expect(results.map((x: any) => [x.test_key, x.status, x.duration_ms])).toEqual([
      ['SHOP-TC-1', 'failed', 2000],
      ['SHOP-TC-2', 'passed', 10],
      ['SHOP-TC-3', 'passed', 6],
    ]);
    expect(results[0].output).toContain('== Pay @SHOP-TC-1\n[project chromium, status failed]\nboom');
    expect(results[0].output).toContain('[project firefox, status passed]');
    const junit = calls[4].body as FormData;
    const xml = await (junit.get('file') as Blob).text();
    expect(xml).toContain('<testcase name="cart › empty"');
    const shotForm = calls[6].body as FormData;
    expect((shotForm.get('file') as File).name).toBe('test-failed-1.png');
    expect(shotForm.get('description')).toBe('Playwright screenshot of Pay @SHOP-TC-1');
    expect(((calls[7].body as FormData).get('file') as File).name).toBe('error-context');
    expect(logs).toEqual(
      expect.arrayContaining([
        'created run "Nightly"',
        'results: 3 recorded; unknown keys: SHOP-TC-99',
        'rejected SHOP-TC-2: draft',
        'tests without a key: 1 recorded; created SHOP-TC-50',
        'uploaded 2 attachment(s) of failed tests',
      ]),
    );
    expect(logs.some((l) => l.startsWith(`skipped ${big}`))).toBe(true);
    expect(logs.some((l) => l.startsWith('skipped attachment huge'))).toBe(true);
    expect(logs.some((l) => l.includes('could not upload') && l.includes('missing.zip'))).toBe(true);
  });

  it('reuses an existing run and reads options from the environment', async () => {
    vi.stubEnv('QTF_URL', 'https://qtf.example');
    vi.stubEnv('QTF_TOKEN', 'qtf_tok');
    vi.stubEnv('QTF_PROJECT', 'SHOP');
    vi.stubEnv('QTF_RUN', 'Nightly');
    const calls = fakeServer({
      runs: [
        { id: 'old', name: 'Nightly', status: 'completed', created_at: '2026-01-01T00:00:00Z' },
        { id: 'aborted', name: 'Nightly', status: 'aborted', created_at: '2026-03-01T00:00:00Z' },
        { id: 'new', name: 'Nightly', status: 'completed', created_at: '2026-02-01T00:00:00Z' },
      ],
    });
    const logs: string[] = [];
    const r = new QtestframeReporter({ logger: (m) => logs.push(m), attachEvidence: false });
    r.onBegin({} as any, {} as any);
    r.onTestEnd(fakeTest({ title: 'x SHOP-TC-1', outcome: 'unexpected' }), fakeResult({ status: 'failed', attachments: [{ name: 's', contentType: 'image/png', body: Buffer.from('x') }] }));
    r.onTestEnd(fakeTest({ title: 'no key' }), fakeResult({ status: 'passed' }));
    await r.onEnd({ status: 'passed' } as any);
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual(['GET /projects', 'GET /projects/p1/runs', 'POST /runs/new/results']);
    expect(logs).toContain('1 test(s) without a test key were not reported: no key');
  });

  it('names the configuration of the results and attaches evidence to its executions', async () => {
    const calls = fakeServer();
    vi.stubEnv('QTF_CONFIGURATION', 'firefox');
    const r = new QtestframeReporter({ url: 'https://qtf.example', token: 'qtf_tok', runId: 'r1', createMissing: true, logger: () => {} });
    r.onBegin({} as any, {} as any);
    r.onTestEnd(fakeTest({ title: 'Pay SHOP-TC-1', outcome: 'unexpected' }), fakeResult({ status: 'failed', attachments: [{ name: 's', contentType: 'image/png', body: Buffer.from('x') }] }));
    r.onTestEnd(fakeTest({ title: 'no key' }), fakeResult({ status: 'passed' }));
    await r.onEnd({} as any);
    expect(calls.map((c) => `${c.method} ${c.path}`)).toEqual([
      'POST /runs/r1/results?configuration=firefox',
      'POST /runs/r1/junit?create_missing=true&configuration=firefox',
      'GET /runs/r1/executions',
      'POST /executions/e1ff/attachments',
    ]);
  });

  it('uses runId directly and creates a dated run when no name is given', async () => {
    let calls = fakeServer();
    let r = new QtestframeReporter({ url: 'https://qtf.example', token: 'qtf_tok', runId: 'r9', logger: () => {} });
    r.onBegin({} as any, {} as any);
    r.onTestEnd(fakeTest({ title: 'SHOP-TC-1' }), fakeResult({ status: 'skipped' }));
    await r.onEnd({} as any);
    expect(calls.map((c) => c.path)).toEqual(['/runs/r9/results']);

    calls = fakeServer();
    r = new QtestframeReporter({ url: 'https://qtf.example', token: 'qtf_tok', project: 'SHOP', logger: () => {} });
    r.onBegin({} as any, {} as any);
    await r.onEnd({} as any);
    expect(calls[2].body.name).toMatch(/^Playwright \d{4}-\d\d-\d\d \d\d:\d\d$/);
    expect(calls).toHaveLength(3);
  });

  it('never fails the test run', async () => {
    // no URL: disabled
    const logs: string[] = [];
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    let r = new QtestframeReporter({ logger: (m) => logs.push(m) });
    r.onBegin({} as any, {} as any);
    await r.onEnd({} as any);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(logs[0]).toContain('url or token missing');

    const run = (opts: object) => {
      const lg: string[] = [];
      const rep = new QtestframeReporter({ url: 'https://qtf.example', token: 'qtf_tok', logger: (m) => lg.push(m), ...opts });
      rep.onBegin({} as any, {} as any);
      rep.onTestEnd(fakeTest({ title: 'SHOP-TC-1', outcome: 'unexpected' }), fakeResult({ status: 'failed', attachments: [{ name: 's', contentType: '', body: Buffer.from('x') }] }));
      return rep.onEnd({} as any).then(() => lg);
    };
    fakeServer();
    expect((await run({}))[0]).toContain('option project (or QTF_PROJECT) is required');
    expect((await run({ project: 'NOPE' }))[0]).toContain('project NOPE not found');
    expect((await run({ project: 'SHOP', run: 'x' }))[0]).toContain('run "x" not found in project SHOP');
    fakeServer({ fail: '/runs/r1/results' });
    expect((await run({ runId: 'r1' }))[0]).toBe('reporting failed: POST /runs/r1/results: 500 boom');
    fakeServer({ fail: '/executions/e1/attachments' });
    expect((await run({ runId: 'r1' })).join('\n')).toContain('could not upload s for SHOP-TC-1: POST /executions/e1/attachments: 500 boom');
    vi.stubGlobal('fetch', vi.fn(async () => new Response('<html>gateway</html>', { status: 502 })));
    expect((await run({ runId: 'r1' }))[0]).toBe('reporting failed: POST /runs/r1/results: 502 <html>gateway</html>');
    vi.stubGlobal('fetch', vi.fn(async () => new Response('', { status: 200 })));
    expect((await run({ runId: 'r1' }))[0]).toContain('reporting failed');
    vi.stubGlobal('fetch', vi.fn(async () => { throw new Error('ECONNREFUSED'); }));
    expect((await run({ project: 'SHOP' }))[0]).toBe('cannot find or create the run: ECONNREFUSED');
    r = new QtestframeReporter({ url: 'https://qtf.example', token: 'qtf_tok', runId: 'r1', keyPattern: 'X-\\d+', logger: () => {} });
    r.onTestEnd(fakeTest({ title: 'X-1' }), fakeResult({ status: 'passed' }));
    await r.onEnd({} as any); // onBegin never ran: nothing to report into
  });
});
