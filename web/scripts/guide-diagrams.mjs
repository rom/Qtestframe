#!/usr/bin/env node
// Diagrams for the user guide (docs/user-guide/images/<lang>-<name>.png),
// drawn from the SVG below with the labels of each language. Unlike the
// screenshots they need no server: run `npm run guide:diagrams` after
// changing a label or the layout.
//
//   QTF_CHROME        Chrome/Chromium executable (default: Playwright's, else a system Chrome)
//   QTF_SHOTS_DIR     output directory (default ../docs/user-guide/images)
//   QTF_SHOTS_QUANTIZE   0 = keep the full-colour PNGs (default: 256 colours with ImageMagick `convert` when installed)
import {
  existsSync,
  mkdirSync,
  renameSync,
  statSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { spawnSync } from "node:child_process";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { chromium } from "playwright-core";

const HERE = dirname(fileURLToPath(import.meta.url));
const QUANTIZE =
  process.env.QTF_SHOTS_QUANTIZE !== "0" &&
  spawnSync("convert", ["-version"]).status === 0;
const OUT = resolve(
  process.env.QTF_SHOTS_DIR || join(HERE, "../../docs/user-guide/images"),
);

function chromePath() {
  if (process.env.QTF_CHROME) return process.env.QTF_CHROME;
  try {
    const p = chromium.executablePath();
    if (p && existsSync(p)) return undefined;
  } catch {
    /* no bundled browser */
  }
  for (const p of [
    "/opt/pw-browsers/chromium",
    "/usr/bin/google-chrome",
    "/usr/bin/chromium",
    "/usr/bin/chromium-browser",
  ])
    if (existsSync(p)) return p;
  return undefined;
}

// ---------------------------------------------------------------- the item model

const MODEL_TEXT = {
  en: {
    frame: "One project (projects belong to an organisation)",
    risk: ["Risk", "likelihood × impact"],
    req: ["Requirement", "obligation, status, version"],
    crit: ["Evaluation criterion", "method, threshold"],
    rec: ["Verification record", "inspection, analysis …"],
    release: ["Release", "scope, exit criteria"],
    test: ["Test", "steps or script, version"],
    baseline: ["Baseline", "frozen versions"],
    cycle: ["Test cycle", "plan, testers, estimates"],
    run: ["Run", "environment, build"],
    exec: ["Execution", "result, evidence"],
    defect: ["Defect", "severity, priority"],
    session: ["Exploratory session", "charter, notes, debrief"],
    e: {
      addresses: "addresses",
      has: "has",
      evidence: "evidence for",
      relations: "refines, derived from, depends on …",
      verifies: "verifies (trace link)",
      scope: "scope",
      includes: "includes",
      freezes: "freezes versions of",
      creates: "plans and creates",
      contains: "contains",
      executes: "executes",
      found: "found in",
      notes: "defects from notes",
      linked: "also linked to requirements and tests",
    },
  },
  sv: {
    frame: "Ett projekt (projekt hör till en organisation)",
    risk: ["Risk", "sannolikhet × konsekvens"],
    req: ["Krav", "kravnivå, status, version"],
    crit: ["Utvärderingskriterium", "metod, tröskelvärde"],
    rec: ["Verifieringspost", "inspektion, analys …"],
    release: ["Release", "omfattning, utgångskriterier"],
    test: ["Testfall", "steg eller skript, version"],
    baseline: ["Baslinje", "frysta versioner"],
    cycle: ["Testcykel", "plan, testare, uppskattningar"],
    run: ["Körning", "miljö, bygge"],
    exec: ["Exekvering", "utfall, bevis"],
    defect: ["Defekt", "allvarlighetsgrad, prioritet"],
    session: ["Utforskande session", "uppdrag, anteckningar"],
    e: {
      addresses: "hanterar",
      has: "har",
      evidence: "bevis för",
      relations: "förfinar, härlett från, beror på …",
      verifies: "verifierar (spårlänk)",
      scope: "omfattning",
      includes: "omfattar",
      freezes: "fryser versioner av",
      creates: "planerar och skapar",
      contains: "innehåller",
      executes: "exekverar",
      found: "hittad i",
      notes: "defekter från anteckningar",
      linked: "kopplas även till krav och testfall",
    },
  },
};

const W = 250;
const H = 64;
const COL = [40, 440, 840, 1240];
const ROW = [120, 320, 520, 720];
const WIDTH = 1530;
const HEIGHT = 830;
const esc = (s) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;");

function box(x, y, [title, sub], accent) {
  return `<g><rect x="${x}" y="${y}" width="${W}" height="${H}" rx="10" fill="#ffffff" stroke="${accent}" stroke-width="2.5"/>
<text x="${x + W / 2}" y="${y + 27}" text-anchor="middle" class="t">${esc(title)}</text>
<text x="${x + W / 2}" y="${y + 49}" text-anchor="middle" class="s">${esc(sub)}</text></g>`;
}

/** An arrow from (x1,y1) to (x2,y2); the label sits at its middle, moved by dx/dy. */
function edge(x1, y1, x2, y2, label = "", dx = 0, dy = -8, anchor = "middle") {
  const lx = (x1 + x2) / 2 + dx;
  const ly = (y1 + y2) / 2 + dy;
  return (
    `<line x1="${x1}" y1="${y1}" x2="${x2}" y2="${y2}" class="l" marker-end="url(#arrow)"/>` +
    (label
      ? `\n<text x="${lx}" y="${ly}" text-anchor="${anchor}" class="e">${esc(label)}</text>`
      : "")
  );
}

function modelSvg(t) {
  const [c0, c1, c2, c3] = COL;
  const [r0, r1, r2, r3] = ROW;
  const mid = (c) => c + W / 2;
  const cy = (r) => r + H / 2;
  const req = "#1d4ed8";
  const test = "#047857";
  const run = "#b45309";
  const other = "#6b21a8";
  const e = t.e;
  return `<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="${HEIGHT}" viewBox="0 0 ${WIDTH} ${HEIGHT}">
<style>
.t{font:600 17px "DejaVu Sans",Arial,sans-serif;fill:#111827}
.s{font:13px "DejaVu Sans",Arial,sans-serif;fill:#374151}
.e{font:14px "DejaVu Sans",Arial,sans-serif;fill:#111827}
.f{font:600 16px "DejaVu Sans",Arial,sans-serif;fill:#374151}
.l{stroke:#374151;stroke-width:1.8;fill:none}
</style>
<defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="8" markerHeight="8" orient="auto-start-reverse"><path d="M0,0 L10,5 L0,10 z" fill="#374151"/></marker></defs>
<rect width="${WIDTH}" height="${HEIGHT}" fill="#ffffff"/>
<rect x="16" y="16" width="${WIDTH - 32}" height="${HEIGHT - 32}" rx="14" fill="#f8fafc" stroke="#94a3b8" stroke-width="1.5" stroke-dasharray="6 4"/>
<text x="36" y="46" class="f">${esc(t.frame)}</text>
${box(c0, r0, t.risk, other)}
${box(c1, r0, t.req, req)}
${box(c2, r0, t.crit, req)}
${box(c3, r0, t.rec, req)}
${box(c0, r1, t.release, other)}
${box(c1, r1, t.test, test)}
${box(c2, r1, t.baseline, other)}
${box(c0, r2, t.cycle, run)}
${box(c1, r2, t.run, run)}
${box(c2, r2, t.exec, run)}
${box(c3, r2, t.defect, other)}
${box(c3, r3, t.session, run)}
<path d="M ${mid(c1) - 50} ${r0} C ${mid(c1) - 50} ${r0 - 48}, ${mid(c1) + 50} ${r0 - 48}, ${mid(c1) + 50} ${r0}" class="l" marker-end="url(#arrow)"/>
<text x="${mid(c1)}" y="${r0 - 46}" text-anchor="middle" class="e">${esc(e.relations)}</text>
${edge(c1, cy(r0), c0 + W + 2, cy(r0), e.addresses)}
${edge(c1 + W, cy(r0), c2 - 2, cy(r0), e.has)}
${edge(c3, cy(r0), c2 + W + 2, cy(r0), e.evidence)}
${edge(mid(c1), r1, mid(c1), r0 + H + 2, e.verifies, 10, 5, "start")}
${edge(c0 + W, r1 + 12, c1 + 40, r0 + H + 2, e.scope, -12, 0, "end")}
${edge(c0 + W - 40, r1 + H, c1 + 20, r2 - 2, e.includes, -12, 8, "end")}
${edge(c2, cy(r1), c1 + W + 2, cy(r1), e.freezes)}
${edge(c2 + 40, r1, c1 + W - 30, r0 + H + 2)}
${edge(c0 + W, cy(r2), c1 - 2, cy(r2), e.creates)}
${edge(c1 + W, cy(r2), c2 - 2, cy(r2), e.contains)}
${edge(c2 + 40, r2, c1 + W - 20, r1 + H + 2, e.executes, 14, 6, "start")}
${edge(c3, cy(r2), c2 + W + 2, cy(r2), e.found)}
<text x="${c3 + W}" y="${r2 - 12}" text-anchor="end" class="e">${esc(e.linked)}</text>
${edge(mid(c3), r3, mid(c3), r2 + H + 2, e.notes, -10, 5, "end")}
</svg>`;
}

const DIAGRAMS = [
  {
    name: "item-model",
    svg: modelSvg,
    text: MODEL_TEXT,
    size: { width: WIDTH, height: HEIGHT },
  },
];

const browser = await chromium.launch({
  executablePath: chromePath(),
  args: ["--font-render-hinting=none"],
});
try {
  mkdirSync(OUT, { recursive: true });
  for (const d of DIAGRAMS) {
    for (const [lang, t] of Object.entries(d.text)) {
      const page = await browser.newPage({ viewport: d.size });
      await page.setContent(
        `<!doctype html><html lang="${lang}"><body style="margin:0">${d.svg(t)}</body></html>`,
      );
      const file = join(OUT, `${lang}-${d.name}.png`);
      writeFileSync(
        file,
        await page.locator("svg").screenshot({ type: "png" }),
      );
      await page.close();
      if (QUANTIZE) {
        const q = `${file}.q.png`;
        const r = spawnSync("convert", [
          file,
          "-strip",
          "-dither",
          "None",
          "-colors",
          "256",
          `PNG8:${q}`,
        ]);
        if (
          r.status === 0 &&
          existsSync(q) &&
          statSync(q).size < statSync(file).size
        )
          renameSync(q, file);
        else if (existsSync(q)) unlinkSync(q);
      }
      console.log(
        `  ✓ ${lang}-${d.name}.png (${Math.round(statSync(file).size / 1024)} KB)`,
      );
    }
  }
} finally {
  await browser.close();
}
