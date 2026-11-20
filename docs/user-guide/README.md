# User guide sources

The end-user guide of Qtestframe, in English (`en/`) and Swedish (`sv/`).
Both folders contain the same file names; screenshots are in `images/`,
named `<lang>-<name>.png`.

The web client bundles these files at build time and shows them under
**Help → User guide** (`/help/:page`), in the user's language with English as
the fallback (`web/src/guide/`). Links between pages are written as relative
Markdown links (`[FAQ](faq.md#anchor)`); the viewer turns them into app links.
Anchors are GitHub-style slugs of the headings.

## Writing rules

- Use the labels of the user interface exactly as `web/src/i18n/locales/en.ts`
  and `sv.ts` define them, in **bold**.
- Every image needs a meaningful alt text (at least a short sentence that
  says what the screenshot shows). Swedish pages reference `sv-*.png`, English
  pages `en-*.png`, and both languages use the same set of screenshots.
- Pages and their order are listed in `web/src/guide/pages.ts` (`GUIDE_PAGES`).
- `npm run test` in `web/` checks that both languages have the same pages, that
  all links and anchors resolve, that images have alt texts, and that the FAQ
  has at least 25 questions (`web/src/guide/content.test.ts`).

## PDF and single-file Markdown

`make guide` exports the guide as one document per language into
`dist/user-guide/`:

- `qtestframe-user-guide-<lang>.md`: all pages joined in reading order, with
  links between pages turned into anchors of the joined file and the
  screenshots it uses copied to `dist/user-guide/images/`;
- `qtestframe-user-guide-<lang>.pdf`: the same content printed on A4, a new
  page for every chapter, page numbers in the footer and bookmarks for the
  headings.

```bash
make guide
```

`make guide-md` writes only the Markdown files. `GUIDE_LANGS=sv` limits the
languages, `GUIDE_OUT=…` changes the folder. The export uses the guide code of
the web client (`web/src/guide/compile.ts`, through
`web/scripts/guide-export.mjs`), so page order, language fallback and anchors
match the in-app guide. The PDF needs Chrome or Chromium: Playwright's
(`cd web && npx playwright-core install chromium`), a system Chrome, or the
executable in `QTF_CHROME`.

## Screenshots

`web/scripts/guide-screenshots.mjs` (npm script `guide:screenshots`) captures
every screenshot the guide references, in both languages, with the light
theme at 1440×900 (phone shots at 390×844). It needs a running server with the
demo data and a super admin account:

```bash
cd web && npm run build
cd .. && CGO_ENABLED=0 go build -o bin/qtestframe ./cmd/qtestframe
./bin/qtestframe seed-demo --config examples/qtestframe.yaml
./bin/qtestframe serve --config examples/qtestframe.yaml -p 8092
```

```bash
cd web && QTF_URL=http://127.0.0.1:8092 npm run guide:screenshots
```

The script signs in through the API, creates (or recreates) the organisations
`guide-en` and `guide-sv` with the tutorial project `SHOP` in the matching
language, and then drives the UI to each state: editors, dialogs, the runner,
reports, the document archive, the administration pages, the intro tour and a
few phone layouts. Pages that do not exist are skipped with a warning. The
admin's own preferences and organisation are restored at the end.

| Variable | Default | Meaning |
|---|---|---|
| `QTF_URL` | `http://127.0.0.1:8080` | server |
| `QTF_EMAIL`, `QTF_PASSWORD` | `admin@example.com`, `Change-me-1234` | super admin login |
| `QTF_SHOTS_LANGS` | `en,sv` | languages |
| `QTF_SHOTS_ONLY` | all | comma-separated shot names, e.g. `runner,dashboard` |
| `QTF_SHOTS_DIR` | `docs/user-guide/images` | output directory |
| `QTF_SHOTS_QUANTIZE` | on when ImageMagick `convert` exists | `0` keeps full-colour PNGs; otherwise they are reduced to 256 colours to keep the repository small |
| `QTF_CHROME` | Playwright's Chromium, else a system Chrome | browser executable |

Run it against a throwaway database: it deletes and recreates the `SHOP`
project in the two guide organisations.
