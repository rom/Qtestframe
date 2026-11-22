#!/usr/bin/env node
// Exports the user guide (docs/user-guide) or, with --guide admin, the
// administrator guide (docs/admin-guide) as one document per language:
//
//   <out>/qtestframe-<guide>-guide-<lang>.md    all pages joined, links between
//                                                pages turned into anchors
//   <out>/images/                               the screenshots it uses
//   <out>/qtestframe-<guide>-guide-<lang>.pdf   printed from the same content
//
// Links from one guide into the other become links to the page on GitHub.
//
// It uses the guide code of the web client (src/guide: page order, language
// fallback, heading anchors, link and image rules), loaded through Vite,
// and Chromium for the PDF. Run `make guide` (or guide-md / guide-pdf).
//
//   node scripts/guide-export.mjs [--guide user|admin] [--out DIR] [--lang en,sv] [--no-pdf]
//
//   QTF_CHROME   Chrome/Chromium executable (default: Playwright's, else a system Chrome)
import { copyFileSync, existsSync, mkdirSync, readdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'

const web = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const repo = resolve(web, '..')
// the screenshots of both guides live with the user guide
const imagesDir = join(repo, 'docs', 'user-guide', 'images')
const PAGE_OF = { en: 'Page', sv: 'Sida' }
// links from one guide into the other point at the page in the repository
const REPO_DOCS = 'https://github.com/rom/Qtestframe/blob/rom/docs'

function args(argv) {
  const o = { out: null, langs: null, pdf: true, guide: 'user' }
  for (let i = 0; i < argv.length; i++) {
    const a = argv[i]
    if (a === '--out') o.out = resolve(argv[++i] ?? '')
    else if (a === '--guide') o.guide = argv[++i] ?? ''
    else if (a === '--lang') o.langs = (argv[++i] ?? '').split(',').map((s) => s.trim()).filter(Boolean)
    else if (a === '--no-pdf') o.pdf = false
    else if (a === '-h' || a === '--help') {
      console.log('usage: guide-export.mjs [--guide user|admin] [--out DIR] [--lang en,sv] [--no-pdf]')
      process.exit(0)
    } else {
      console.error(`unknown argument ${a}`)
      process.exit(2)
    }
  }
  if (o.guide !== 'user' && o.guide !== 'admin') {
    console.error(`--guide must be user or admin, not "${o.guide}"`)
    process.exit(2)
  }
  o.out ??= join(repo, 'dist', `${o.guide}-guide`)
  return o
}

function chromePath(chromium) {
  if (process.env.QTF_CHROME) return process.env.QTF_CHROME
  try {
    const p = chromium.executablePath()
    if (p && existsSync(p)) return undefined
  } catch {
    /* no bundled browser */
  }
  for (const p of ['/opt/pw-browsers/chromium', '/usr/bin/google-chrome', '/usr/bin/chromium', '/usr/bin/chromium-browser', '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome']) {
    if (existsSync(p)) return p
  }
  return undefined
}

async function loadGuideCode() {
  const { createServer } = await import('vite')
  const server = await createServer({
    root: web,
    configFile: join(web, 'vite.config.ts'),
    logLevel: 'error',
    appType: 'custom',
    server: { middlewareMode: true, hmr: false, watch: null },
    optimizeDeps: { noDiscovery: true, include: [] },
  })
  try {
    return { ...(await server.ssrLoadModule('/src/guide/pages.ts')), ...(await server.ssrLoadModule('/src/guide/compile.ts')) }
  } finally {
    await server.close()
  }
}

const opts = args(process.argv.slice(2))
const g = await loadGuideCode()
const docs = join(repo, 'docs', `${opts.guide}-guide`)
const pageList = opts.guide === 'admin' ? g.ADMIN_GUIDE_PAGES : g.GUIDE_PAGES
const otherGuideUrl = (lang) => (book, page, hash) => `${REPO_DOCS}/${book}-guide/${lang}/${page}.md${hash ? '#' + encodeURIComponent(hash) : ''}`

// the Markdown sources, as the web client bundles them
const files = {}
const langs = readdirSync(docs, { withFileTypes: true }).filter((d) => d.isDirectory() && /^[a-z]{2}$/.test(d.name)).map((d) => d.name)
for (const lang of langs) {
  for (const f of readdirSync(join(docs, lang))) if (f.endsWith('.md')) files[`${lang}/${f}`] = readFileSync(join(docs, lang, f), 'utf8')
}
const sources = g.collectSources(files)
const images = readdirSync(imagesDir).filter((f) => /\.(png|jpe?g|gif|webp|svg)$/i.test(f))
const wanted = opts.langs ?? langs
for (const l of wanted) {
  if (!langs.includes(l)) {
    console.error(`no guide in language "${l}" (have ${langs.join(', ')})`)
    process.exit(2)
  }
}

mkdirSync(opts.out, { recursive: true })
const built = []
for (const lang of wanted) {
  const pages = g.pageOrder(sources, lang, 'en', pageList).map((page) => ({ page, markdown: g.resolvePage(sources, lang, page).markdown }))
  const guide = g.compileGuide(pages, { lang, images, book: opts.guide, otherGuideUrl: otherGuideUrl(lang) })
  const base = join(opts.out, `qtestframe-${opts.guide}-guide-${lang}`)
  writeFileSync(base + '.md', guide.markdown)
  // writing into docs/user-guide itself: the images are already there
  if (guide.images.length && resolve(imagesDir) !== resolve(join(opts.out, 'images'))) {
    mkdirSync(join(opts.out, 'images'), { recursive: true })
    for (const name of guide.images) copyFileSync(join(imagesDir, name), join(opts.out, 'images', name))
  }
  console.log(`${base}.md (${pages.length} pages, ${guide.images.length} images)`)
  built.push({ lang, guide, base })
}

if (opts.pdf) {
  const { chromium } = await import('playwright-core')
  let browser
  try {
    browser = await chromium.launch({ executablePath: chromePath(chromium) })
  } catch (e) {
    console.error(`cannot start Chrome/Chromium for the PDF: ${e.message.split('\n')[0]}`)
    console.error('install one (cd web && npx playwright-core install chromium), set QTF_CHROME, or use make guide-md')
    process.exit(1)
  }
  try {
    const assets = Object.fromEntries(images.map((n) => [n, pathToFileURL(join(imagesDir, n)).href]))
    for (const { lang, guide, base } of built) {
      const html = g.printableHtml(guide, { lang, assets })
      const htmlFile = base + '.print.html'
      writeFileSync(htmlFile, html)
      const page = await browser.newPage()
      await page.goto(pathToFileURL(htmlFile).href, { waitUntil: 'load' })
      const footer = `<div style="width:100%;font:8px system-ui,sans-serif;color:#64748b;padding:0 16mm;display:flex;justify-content:space-between"><span>${guide.title.replace(/[<&]/g, '')}</span><span>${PAGE_OF[lang] ?? PAGE_OF.en} <span class="pageNumber"></span> / <span class="totalPages"></span></span></div>`
      await page.pdf({ path: base + '.pdf', format: 'A4', printBackground: true, displayHeaderFooter: true, headerTemplate: '<span></span>', footerTemplate: footer, margin: { top: '18mm', bottom: '20mm', left: '16mm', right: '16mm' }, outline: true, tagged: true })
      await page.close()
      rmSync(htmlFile)
      console.log(`${base}.pdf`)
    }
  } finally {
    await browser.close()
  }
}
