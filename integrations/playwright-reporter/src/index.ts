/**
 * Playwright reporter for Qtestframe.
 *
 * Records the result of every Playwright test in a Qtestframe test run:
 * the run is found (or created) when the test run begins, results are
 * collected per test key and sent in one request when it ends, and
 * screenshots, traces and videos of failed tests are uploaded as evidence.
 * Problems while reporting are logged and never fail the test run.
 */
import { readFile, stat } from 'node:fs/promises';
import { basename } from 'node:path';
import type { FullConfig, FullResult, Reporter, Suite, TestCase, TestResult, TestStep } from '@playwright/test/reporter';

export interface QtestframeReporterOptions {
  /** Server base URL, e.g. https://qtestframe.example.com (default: $QTF_URL). */
  url?: string;
  /** API token, sent as `Authorization: Bearer` (default: $QTF_TOKEN). */
  token?: string;
  /** Project key, e.g. SHOP (default: $QTF_PROJECT). Needed unless runId is given. */
  project?: string;
  /** Name of the test run (default: $QTF_RUN, else "Playwright <date and time>"). */
  run?: string;
  /** Id of an existing run; wins over project/run (default: $QTF_RUN_ID). */
  runId?: string;
  /** Create the run when the project has none with that name (default: true when no run name is given). */
  createRun?: boolean;
  /** Build of a created run (default: $QTF_BUILD). */
  build?: string;
  /** Environment of a created run (default: $QTF_ENVIRONMENT). */
  environment?: string;
  /**
   * Configuration (name or id) the results were obtained in, e.g. "Chrome / Windows"
   * (default: $QTF_CONFIGURATION). The run gets the configuration when it does not
   * target it yet. Without one, results go to the run's only configuration (or none).
   */
  configuration?: string;
  /** Create draft automated tests for Playwright tests without a key (needs test.write). */
  createMissing?: boolean;
  /** Pattern of a test key in tags and titles (default /[A-Z][A-Z0-9]{1,15}-TC-\d+/). */
  keyPattern?: string | RegExp;
  /** Upload attachments (screenshots, traces, videos) of failed tests (default true). */
  attachEvidence?: boolean;
  /** Largest attachment to upload in bytes (default 20 MB). */
  maxAttachmentBytes?: number;
  /** Timeout of one request in milliseconds (default 60 000). */
  timeoutMs?: number;
  /** Where messages go (default console.log with a "[qtestframe]" prefix). */
  logger?: (message: string) => void;
}

export const DEFAULT_KEY_PATTERN = /[A-Z][A-Z0-9]{1,15}-TC-\d+/;

export type QtfStatus = 'passed' | 'failed' | 'skipped';

/** One result as sent to POST /runs/{id}/results. */
export interface ImportResult {
  test_key: string;
  status: QtfStatus;
  duration_ms: number;
  output: string;
}

interface Attachment {
  name: string;
  contentType: string;
  path?: string;
  body?: Buffer;
}

interface Collected {
  key: string;
  name: string;
  status: QtfStatus;
  durationMs: number;
  outputs: string[];
  evidence: Attachment[];
}

interface ImportResponse {
  updated: number;
  unknown?: string[];
  unmapped?: string[];
  rejected?: { test_key: string; reason?: string }[];
  created?: { key: string; title: string }[];
}

const MAX_OUTPUT = 60_000;
const ANSI = /\u001b\[[0-9;]*[A-Za-z]/g;

/** Finds the test key: a `qtestframe` annotation, a tag, else the title. */
export function findKey(test: TestCase, pattern: RegExp = DEFAULT_KEY_PATTERN): string | undefined {
  for (const a of test.annotations ?? []) {
    if (a.type.toLowerCase() === 'qtestframe' && a.description?.trim()) {
      return a.description.trim().toUpperCase();
    }
  }
  for (const tag of test.tags ?? []) {
    const m = pattern.exec(tag.replace(/^@/, ''));
    if (m) return m[0];
  }
  for (const s of [test.title, ...test.titlePath()]) {
    const m = pattern.exec(s);
    if (m) return m[0];
  }
  return undefined;
}

/** Maps a Playwright result to a Qtestframe result. */
export function mapStatus(test: TestCase, result: TestResult): QtfStatus {
  if (result.status === 'skipped') return 'skipped';
  if (result.status === 'interrupted') return 'failed';
  const outcome = typeof test.outcome === 'function' ? test.outcome() : undefined;
  if (outcome === 'expected' || outcome === 'flaky') return 'passed';
  if (outcome === 'unexpected') return 'failed';
  return result.status === 'passed' ? 'passed' : 'failed';
}

/** The test title without project and file: "describe › test". */
export function displayTitle(test: TestCase): string {
  const parts: string[] = [];
  for (let s: Suite | undefined = test.parent; s; s = s.parent) {
    if (s.type === 'describe' && s.title) parts.unshift(s.title);
  }
  parts.push(test.title);
  return parts.join(' › ');
}

function projectName(test: TestCase): string {
  for (let s: Suite | undefined = test.parent; s; s = s.parent) {
    if (s.type === 'project') return s.title;
  }
  return '';
}

function stepLines(steps: TestStep[], depth: number, out: string[]): void {
  for (const s of steps) {
    if (s.category === 'test.step') {
      const mark = s.error ? '✗' : '✓';
      let line = `${'  '.repeat(depth + 1)}${mark} ${s.title} (${Math.round(s.duration)} ms)`;
      if (s.error?.message) line += ': ' + s.error.message.replace(ANSI, '').split('\n')[0];
      out.push(line);
      stepLines(s.steps ?? [], depth + 1, out);
    } else {
      stepLines(s.steps ?? [], depth, out);
    }
  }
}

/** Output text of one result: errors and test.step() titles with status. */
export function outputOf(test: TestCase, result: TestResult): string {
  const lines: string[] = [];
  const project = projectName(test);
  const head = [project && `project ${project}`, result.retry ? `retry #${result.retry}` : '', `status ${result.status}`].filter(Boolean);
  lines.push(`[${head.join(', ')}]`);
  for (const e of result.errors ?? []) {
    const text = (e.stack ?? e.message ?? e.value ?? '').replace(ANSI, '').trim();
    if (text) lines.push(text);
  }
  const steps: string[] = [];
  stepLines(result.steps ?? [], 0, steps);
  if (steps.length) lines.push('Steps:', ...steps);
  return lines.join('\n');
}

const rank: Record<QtfStatus, number> = { skipped: 0, passed: 1, failed: 2 };

function truncate(s: string, max = MAX_OUTPUT): string {
  return s.length <= max ? s : s.slice(0, max) + '\n… (truncated)';
}

function escapeXML(s: string): string {
  return s.replace(/[<>&"']/g, (c) => ({ '<': '&lt;', '>': '&gt;', '&': '&amp;', '"': '&quot;', "'": '&apos;' })[c]!)
    // characters XML 1.0 cannot carry
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f]/g, '');
}

/** JUnit XML for results without a key (used with createMissing). */
export function junitXML(items: Collected[]): string {
  const cases = items.map((c) => {
    const time = (c.durationMs / 1000).toFixed(3);
    const out = c.outputs.length ? `<system-out>${escapeXML(truncate(c.outputs.join('\n\n')))}</system-out>` : '';
    const st = c.status === 'failed' ? '<failure message="failed"/>' : c.status === 'skipped' ? '<skipped/>' : '';
    return `<testcase name="${escapeXML(c.name)}" classname="playwright" time="${time}">${st}${out}</testcase>`;
  });
  return `<?xml version="1.0" encoding="UTF-8"?>\n<testsuite name="playwright" tests="${items.length}">\n${cases.join('\n')}\n</testsuite>\n`;
}

class QtestframeReporter implements Reporter {
  private readonly o: QtestframeReporterOptions;
  private readonly pattern: RegExp;
  private readonly log: (m: string) => void;
  private readonly base: string;
  private readonly token: string;
  private readonly enabled: boolean;
  private runPromise?: Promise<string | undefined>;
  private readonly entries = new Map<string, { test: TestCase; result: TestResult }>();

  constructor(options: QtestframeReporterOptions = {}) {
    const env = process.env;
    this.o = {
      ...options,
      project: options.project ?? env.QTF_PROJECT,
      run: options.run ?? env.QTF_RUN,
      runId: options.runId ?? env.QTF_RUN_ID,
      build: options.build ?? env.QTF_BUILD,
      environment: options.environment ?? env.QTF_ENVIRONMENT,
      configuration: options.configuration ?? env.QTF_CONFIGURATION,
    };
    this.log = options.logger ?? ((m) => console.log(`[qtestframe] ${m}`));
    const p = options.keyPattern ?? DEFAULT_KEY_PATTERN;
    this.pattern = typeof p === 'string' ? new RegExp(p) : p;
    this.base = (options.url ?? env.QTF_URL ?? '').trim().replace(/\/+$/, '').replace(/\/api\/v1$/, '');
    this.token = (options.token ?? env.QTF_TOKEN ?? '').trim();
    this.enabled = this.base !== '' && this.token !== '';
    if (!this.enabled) this.log('url or token missing (options or QTF_URL/QTF_TOKEN): results are not reported');
  }

  printsToStdio(): boolean {
    return false;
  }

  onBegin(_config: FullConfig, _suite: Suite): void {
    if (!this.enabled) return;
    this.runPromise = this.resolveRun().catch((err) => {
      this.log(`cannot find or create the run: ${String(err?.message ?? err)}`);
      return undefined;
    });
  }

  onTestEnd(test: TestCase, result: TestResult): void {
    // the last attempt (retry) of a test counts
    this.entries.set(test.id, { test, result });
  }

  async onEnd(_result: FullResult): Promise<void> {
    if (!this.enabled) return;
    try {
      await this.report();
    } catch (err: any) {
      this.log(`reporting failed: ${String(err?.message ?? err)}`);
    }
  }

  // ------------------------------------------------------------ HTTP

  private async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { Authorization: `Bearer ${this.token}`, Accept: 'application/json' };
    let payload: BodyInit | undefined;
    if (body instanceof FormData) {
      payload = body;
    } else if (body !== undefined) {
      headers['Content-Type'] = 'application/json';
      payload = JSON.stringify(body);
    }
    const res = await fetch(this.base + '/api/v1' + path, {
      method,
      headers,
      body: payload,
      signal: AbortSignal.timeout(this.o.timeoutMs ?? 60_000),
    });
    const text = await res.text();
    if (!res.ok) {
      let msg = text.slice(0, 200);
      try {
        msg = JSON.parse(text).error ?? msg;
      } catch {
        /* not JSON */
      }
      throw new Error(`${method} ${path}: ${res.status} ${msg}`);
    }
    return (text ? JSON.parse(text) : undefined) as T;
  }

  private async resolveRun(): Promise<string> {
    if (this.o.runId) return this.o.runId;
    if (!this.o.project) throw new Error('option project (or QTF_PROJECT) is required unless runId is given');
    const projects = await this.request<{ items: { id: string; key: string }[] }>('GET', '/projects');
    const project = projects.items.find((p) => p.key.toUpperCase() === this.o.project!.toUpperCase());
    if (!project) throw new Error(`project ${this.o.project} not found`);
    const name = this.o.run || `Playwright ${new Date().toISOString().replace('T', ' ').slice(0, 16)}`;
    const createRun = this.o.createRun ?? !this.o.run;
    const runs = await this.request<{ items: { id: string; name: string; status: string; created_at: string }[] }>(
      'GET',
      `/projects/${encodeURIComponent(project.id)}/runs`,
    );
    const found = runs.items
      .filter((r) => r.name === name)
      .sort((a, b) => Number(a.status === 'aborted') - Number(b.status === 'aborted') || b.created_at.localeCompare(a.created_at))[0];
    if (found) return found.id;
    if (!createRun) throw new Error(`run "${name}" not found in project ${project.key} (set createRun: true to create it)`);
    const run = await this.request<{ id: string }>('POST', `/projects/${encodeURIComponent(project.id)}/runs`, {
      name,
      build: this.o.build ?? '',
      environment: this.o.environment ?? '',
    });
    this.log(`created run "${name}"`);
    return run.id;
  }

  /** "?configuration=…" (or "&…" after other parameters) when a configuration is set. */
  private configQuery(sep: '?' | '&' = '?'): string {
    const c = this.o.configuration?.trim();
    return c ? `${sep}configuration=${encodeURIComponent(c)}` : '';
  }

  // ------------------------------------------------------------ report

  private collect(): { keyed: Collected[]; unkeyed: Collected[] } {
    const byKey = new Map<string, Collected>();
    const byName = new Map<string, Collected>();
    for (const { test, result } of this.entries.values()) {
      const key = findKey(test, this.pattern);
      const name = displayTitle(test);
      const map = key ? byKey : byName;
      const id = key ?? name;
      const status = mapStatus(test, result);
      let c = map.get(id);
      if (!c) {
        c = { key: key ?? '', name, status, durationMs: 0, outputs: [], evidence: [] };
        map.set(id, c);
      } else if (rank[status] > rank[c.status]) {
        c.status = status;
      }
      c.durationMs += Math.max(0, Math.round(result.duration));
      c.outputs.push((map === byKey && name ? `== ${name}\n` : '') + outputOf(test, result));
      if (status === 'failed') {
        for (const a of result.attachments ?? []) {
          if (a.path || a.body) c.evidence.push(a);
        }
      }
    }
    return { keyed: [...byKey.values()], unkeyed: [...byName.values()] };
  }

  private async report(): Promise<void> {
    const runId = await this.runPromise;
    if (!runId) return;
    const { keyed, unkeyed } = this.collect();
    if (keyed.length) {
      const results: ImportResult[] = keyed.map((c) => ({
        test_key: c.key,
        status: c.status,
        duration_ms: c.durationMs,
        output: truncate(c.outputs.join('\n\n')),
      }));
      this.summarise('results', await this.request<ImportResponse>('POST', `/runs/${encodeURIComponent(runId)}/results${this.configQuery()}`, { results }));
    }
    if (unkeyed.length && this.o.createMissing) {
      const form = new FormData();
      form.append('file', new Blob([junitXML(unkeyed)], { type: 'application/xml' }), 'playwright.xml');
      const res = await this.request<ImportResponse>('POST', `/runs/${encodeURIComponent(runId)}/junit?create_missing=true${this.configQuery('&')}`, form);
      this.summarise('tests without a key', res);
    } else if (unkeyed.length) {
      this.log(`${unkeyed.length} test(s) without a test key were not reported: ${unkeyed.map((c) => c.name).join(', ')}`);
    }
    if (this.o.attachEvidence ?? true) {
      const items = [...keyed, ...(this.o.createMissing ? unkeyed : [])];
      await this.uploadEvidence(runId, items.filter((c) => c.evidence.length));
    }
  }

  private summarise(what: string, r: ImportResponse): void {
    const parts = [`${r.updated} recorded`];
    if (r.created?.length) parts.push(`created ${r.created.map((c) => c.key).join(', ')}`);
    if (r.unknown?.length) parts.push(`unknown keys: ${r.unknown.join(', ')}`);
    if (r.unmapped?.length) parts.push(`without key: ${r.unmapped.join(', ')}`);
    this.log(`${what}: ${parts.join('; ')}`);
    for (const x of r.rejected ?? []) this.log(`rejected ${x.test_key}: ${x.reason ?? 'the test may not be executed'}`);
  }

  private async uploadEvidence(runId: string, items: Collected[]): Promise<void> {
    if (!items.length) return;
    const all = await this.request<{
      items: { id: string; test_key: string; test_title: string; configuration_id?: string | null; configuration_name?: string }[];
    }>('GET', `/runs/${encodeURIComponent(runId)}/executions`);
    // with a configuration, evidence belongs to the executions in it
    const cfg = this.o.configuration?.trim().toLowerCase();
    const inConfig = cfg
      ? all.items.filter((e) => (e.configuration_name ?? '').toLowerCase() === cfg || (e.configuration_id ?? '').toLowerCase() === cfg)
      : all.items;
    const execs = { items: inConfig.length ? inConfig : all.items };
    // the first execution of a test wins (several without a configuration)
    const byKey = new Map<string, string>();
    const byTitle = new Map<string, string>();
    for (const e of execs.items) {
      if (!byKey.has(e.test_key.toUpperCase())) byKey.set(e.test_key.toUpperCase(), e.id);
      // tests created for results without a key carry the result's title
      if (!byTitle.has(e.test_title)) byTitle.set(e.test_title, e.id);
    }
    const max = this.o.maxAttachmentBytes ?? 20 << 20;
    let n = 0;
    for (const c of items) {
      const exec = c.key ? byKey.get(c.key.toUpperCase()) : byTitle.get(c.name.split(/\s+/).join(' '));
      if (!exec) continue;
      for (const a of c.evidence) {
        try {
          let data: Buffer;
          let name = a.name;
          if (a.body) {
            data = a.body;
          } else {
            const size = (await stat(a.path!)).size;
            if (size > max) {
              this.log(`skipped ${a.path}: larger than ${max} bytes`);
              continue;
            }
            data = await readFile(a.path!);
            name = basename(a.path!);
          }
          if (data.length > max) {
            this.log(`skipped attachment ${a.name}: larger than ${max} bytes`);
            continue;
          }
          const form = new FormData();
          form.append('file', new Blob([new Uint8Array(data)], { type: a.contentType || 'application/octet-stream' }), name);
          form.append('description', `Playwright ${a.name} of ${c.name}`);
          await this.request('POST', `/executions/${encodeURIComponent(exec)}/attachments`, form);
          n++;
        } catch (err: any) {
          this.log(`could not upload ${a.path ?? a.name} for ${c.key}: ${String(err?.message ?? err)}`);
        }
      }
    }
    if (n) this.log(`uploaded ${n} attachment(s) of failed tests`);
  }
}

export default QtestframeReporter;
export { QtestframeReporter };
