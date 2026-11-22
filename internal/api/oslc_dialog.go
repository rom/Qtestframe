package api

import (
	"encoding/json"
	"html/template"
	"net/http"
	"strings"

	"github.com/rom/qtestframe/internal/model"
)

// OSLC delegated dialogs (OSLC Core 2.0 "Delegated User Interface"): small
// pages another tool shows in a frame or a window so that its user picks
// (selection dialog) or creates (creation dialog) a requirement or test
// case of a project. The page uses the browser's Qtestframe session (the
// user signs in in another tab when there is none) and the regular API;
// the result goes back by the windowName protocol (#oslc-core-windowName-1.0)
// or by postMessage (#oslc-core-postMessage-1.0, the default):
//
//	{"oslc:results": [{"oslc:label": "SHOP-REQ-1 Login", "rdf:resource": "<the OSLC resource URL>"}]}
//
// Pages: GET /api/v1/oslc/projects/{pid}/dialogs/{requirement|test}/{select|create},
// their script /api/v1/oslc/dialog.js. The pages may be framed by the
// origins of server.frame_ancestors.

func (s *Server) routesOSLCDialogs(pub func(string, http.HandlerFunc)) {
	pub("GET /api/v1/oslc/projects/{pid}/dialogs/{kind}/{mode}", s.handleOSLCDialog)
	pub("GET /api/v1/oslc/dialog.js", handleOSLCDialogJS)
}

type dialogText struct {
	Lang, Title, Intro, Search, SearchButton, Results, None, OK, Cancel, Create             string
	TitleLabel, Description, Obligation, Preconditions, Expected, SignIn, SignInLink, Retry string
	Loading, Created, Failed, Selected, Skip                                                string
}

var dialogTexts = map[string]map[string]dialogText{
	"en": {
		"select": {Search: "Search", SearchButton: "Search", Results: "Results", None: "Nothing found.", OK: "Select", Cancel: "Cancel",
			Intro: "Pick one or more items and choose Select. The other tool links to them.", Loading: "Loading…", Selected: "{n} selected"},
		"create": {Create: "Create", Cancel: "Cancel", TitleLabel: "Title", Description: "Description (Markdown)", Obligation: "Obligation",
			Preconditions: "Preconditions", Expected: "Expected result", Intro: "The new item is created in Qtestframe and linked by the other tool.",
			Created: "Created {key}.", Loading: "Loading…"},
	},
	"sv": {
		"select": {Search: "Sök", SearchButton: "Sök", Results: "Resultat", None: "Inget hittades.", OK: "Välj", Cancel: "Avbryt",
			Intro: "Markera en eller flera och välj Välj. Det andra verktyget länkar till dem.", Loading: "Läser in…", Selected: "{n} valda"},
		"create": {Create: "Skapa", Cancel: "Avbryt", TitleLabel: "Titel", Description: "Beskrivning (Markdown)", Obligation: "Kravnivå",
			Preconditions: "Förutsättningar", Expected: "Förväntat resultat", Intro: "Den nya posten skapas i Qtestframe och länkas av det andra verktyget.",
			Created: "Skapade {key}.", Loading: "Läser in…"},
	},
}

func dialogLang(r *http.Request) string {
	if l := r.URL.Query().Get("lang"); l == "sv" || l == "en" {
		return l
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag := strings.ToLower(strings.TrimSpace(strings.SplitN(part, ";", 2)[0]))
		switch {
		case strings.HasPrefix(tag, "sv"):
			return "sv"
		case strings.HasPrefix(tag, "en"):
			return "en"
		}
	}
	return "en"
}

func dialogTitle(lang, kind, mode string) string {
	t := map[string]string{
		"en/requirement/select": "Select requirements", "en/test/select": "Select test cases",
		"en/requirement/create": "New requirement", "en/test/create": "New test case",
		"sv/requirement/select": "Välj krav", "sv/test/select": "Välj testfall",
		"sv/requirement/create": "Nytt krav", "sv/test/create": "Nytt testfall",
	}
	return t[lang+"/"+kind+"/"+mode]
}

func dialogCommon(lang string) dialogText {
	if lang == "sv" {
		return dialogText{SignIn: "Du är inte inloggad i Qtestframe.", SignInLink: "Logga in (ny flik)", Retry: "Försök igen",
			Failed: "Det gick inte: {error}", Skip: "Hoppa till innehållet"}
	}
	return dialogText{SignIn: "You are not signed in to Qtestframe.", SignInLink: "Sign in (new tab)", Retry: "Try again",
		Failed: "That did not work: {error}", Skip: "Skip to content"}
}

var dialogPage = template.Must(template.New("dialog").Parse(`<!doctype html>
<html lang="{{.T.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.T.Title}} – Qtestframe</title>
<style>
:root { color-scheme: light dark; --bg: #ffffff; --fg: #1b1f24; --muted: #4b5563; --line: #c3c8cf; --accent: #1d4ed8; --accent-fg: #ffffff; --hover: #eef2ff; --err: #b91c1c; }
@media (prefers-color-scheme: dark) { :root { --bg: #16191d; --fg: #eceff3; --muted: #b5bcc6; --line: #4b5563; --accent: #93b4ff; --accent-fg: #0b1220; --hover: #232a36; --err: #fca5a5; } }
* { box-sizing: border-box; }
body { margin: 0; font: 15px/1.45 system-ui, -apple-system, "Segoe UI", Roboto, sans-serif; background: var(--bg); color: var(--fg); }
main { padding: 16px; max-width: 760px; }
h1 { font-size: 1.2rem; margin: 0 0 4px; }
p { margin: 4px 0 10px; }
.muted { color: var(--muted); }
label { display: block; font-weight: 600; margin: 10px 0 4px; }
input[type=search], input[type=text], textarea, select { width: 100%; padding: 7px 9px; font: inherit; color: inherit; background: var(--bg); border: 1px solid var(--line); border-radius: 6px; }
textarea { min-height: 110px; resize: vertical; }
.row { display: flex; gap: 8px; align-items: flex-end; }
.row > :first-child { flex: 1; }
button { font: inherit; padding: 7px 14px; border-radius: 6px; border: 1px solid var(--accent); background: var(--bg); color: var(--accent); cursor: pointer; }
button.primary { background: var(--accent); color: var(--accent-fg); }
button:disabled { opacity: .6; cursor: default; }
:focus-visible { outline: 3px solid var(--accent); outline-offset: 2px; }
fieldset { border: 1px solid var(--line); border-radius: 6px; margin: 12px 0; padding: 4px 8px 8px; }
legend { font-weight: 600; padding: 0 4px; }
ul.results { list-style: none; margin: 0; padding: 0; max-height: 46vh; overflow: auto; }
ul.results li label { display: flex; gap: 8px; font-weight: normal; margin: 0; padding: 6px 4px; border-radius: 4px; cursor: pointer; }
ul.results li label:hover { background: var(--hover); }
.key { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; color: var(--muted); }
.actions { display: flex; gap: 8px; justify-content: flex-end; margin-top: 14px; }
.error { color: var(--err); }
.skip { position: absolute; left: -9999px; }
.skip:focus { left: 8px; top: 8px; background: var(--bg); padding: 4px 8px; }
[hidden] { display: none !important; }
</style>
</head>
<body data-pid="{{.PID}}" data-kind="{{.Kind}}" data-mode="{{.Mode}}" data-resource-base="{{.ResourceBase}}" data-texts="{{.TextsJSON}}">
<a class="skip" href="#main">{{.T.Skip}}</a>
<main id="main">
<h1>{{.T.Title}}</h1>
<p class="muted">{{.T.Intro}}</p>
<p id="project"></p>
<div id="signin" hidden>
  <p>{{.T.SignIn}}</p>
  <p class="row"><a href="/login" target="_blank" rel="noopener">{{.T.SignInLink}}</a> <button type="button" id="retry">{{.T.Retry}}</button></p>
</div>
<p id="status" role="status" aria-live="polite"></p>
{{if eq .Mode "select"}}
<form id="search" role="search" hidden>
  <div class="row">
    <div><label for="q">{{.T.Search}}</label><input type="search" id="q" name="q" autocomplete="off"></div>
    <button type="submit">{{.T.SearchButton}}</button>
  </div>
</form>
<form id="pick" hidden>
  <fieldset><legend>{{.T.Results}}</legend><ul class="results" id="list"></ul></fieldset>
  <div class="actions"><button type="button" id="cancel">{{.T.Cancel}}</button><button type="submit" class="primary" id="ok" disabled>{{.T.OK}}</button></div>
</form>
{{else}}
<form id="create" hidden>
  <label for="title">{{.T.TitleLabel}}</label><input type="text" id="title" name="title" required maxlength="500">
  <label for="description">{{.T.Description}}</label><textarea id="description" name="description"></textarea>
  {{if eq .Kind "requirement"}}
  <label for="obligation">{{.T.Obligation}}</label>
  <select id="obligation" name="obligation">{{range .Obligations}}<option value="{{.}}">{{.}}</option>{{end}}</select>
  {{else}}
  <label for="preconditions">{{.T.Preconditions}}</label><textarea id="preconditions" name="preconditions"></textarea>
  <label for="expected_result">{{.T.Expected}}</label><textarea id="expected_result" name="expected_result"></textarea>
  {{end}}
  <div class="actions"><button type="button" id="cancel">{{.T.Cancel}}</button><button type="submit" class="primary" id="ok">{{.T.Create}}</button></div>
</form>
{{end}}
</main>
<script src="/api/v1/oslc/dialog.js"></script>
</body>
</html>
`))

// dialogHeaders let the page be framed by the configured origins and keep
// window.opener (postMessage to the tool that opened it).
func (s *Server) dialogHeaders(w http.ResponseWriter) {
	ancestors := "'self'"
	for _, o := range s.conf().Server.FrameAncestors {
		ancestors += " " + strings.TrimRight(o, "/")
	}
	h := w.Header()
	h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; form-action 'self'; base-uri 'none'; frame-ancestors "+ancestors)
	if len(s.conf().Server.FrameAncestors) > 0 {
		h.Del("X-Frame-Options")
	} else {
		h.Set("X-Frame-Options", "SAMEORIGIN")
	}
	h.Set("Cross-Origin-Opener-Policy", "unsafe-none")
	h.Set("Cache-Control", "no-store")
}

func (s *Server) handleOSLCDialog(w http.ResponseWriter, r *http.Request) {
	kind, mode := r.PathValue("kind"), r.PathValue("mode")
	if (kind != model.KindRequirement && kind != model.KindTest) || (mode != "select" && mode != "create") {
		writeError(w, http.StatusNotFound, "not_found", "no such dialog")
		return
	}
	lang := dialogLang(r)
	t := dialogTexts[lang][mode]
	c := dialogCommon(lang)
	t.Lang, t.Title, t.SignIn, t.SignInLink, t.Retry, t.Failed, t.Skip = lang, dialogTitle(lang, kind, mode), c.SignIn, c.SignInLink, c.Retry, c.Failed, c.Skip
	texts, _ := json.Marshal(map[string]string{"none": t.None, "loading": t.Loading, "created": t.Created, "failed": t.Failed, "selected": t.Selected})
	u := oslcURLs{base: s.oslcBase(r)}
	s.dialogHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = dialogPage.Execute(w, map[string]any{"T": t, "PID": r.PathValue("pid"), "Kind": kind, "Mode": mode,
		"ResourceBase": u.collection(r.PathValue("pid"), kind) + "/", "TextsJSON": string(texts), "Obligations": model.Obligations})
}

func handleOSLCDialogJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write([]byte(dialogJS))
}

// dialogJS runs the dialogs: it checks the session, searches or creates
// through the API and sends the result to the calling tool.
const dialogJS = `(function () {
  'use strict';
  var body = document.body;
  var pid = body.dataset.pid, kind = body.dataset.kind, mode = body.dataset.mode, resBase = body.dataset.resourceBase;
  var T = JSON.parse(body.dataset.texts || '{}');
  var statusEl = document.getElementById('status');
  var plural = kind === 'test' ? 'tests' : 'requirements';
  var csrf = '';
  // the protocol of the calling tool (OSLC Core 2.0 delegated UI)
  var windowName = location.hash.indexOf('oslc-core-windowName-1.0') >= 0;
  var returnURL = windowName ? window.name : '';

  function status(text, isError) {
    statusEl.textContent = text || '';
    statusEl.className = isError ? 'error' : 'muted';
  }
  function api(method, path, data) {
    var opts = { method: method, credentials: 'same-origin', headers: { Accept: 'application/json' } };
    if (data !== undefined) {
      opts.headers['Content-Type'] = 'application/json';
      opts.headers['X-CSRF-Token'] = csrf;
      opts.body = JSON.stringify(data);
    }
    return fetch('/api/v1' + path, opts).then(function (res) {
      return res.text().then(function (txt) {
        var json = null;
        try { json = txt ? JSON.parse(txt) : null; } catch (e) { json = null; }
        if (!res.ok) {
          var err = new Error((json && json.error) || res.statusText || String(res.status));
          err.status = res.status;
          throw err;
        }
        return json;
      });
    });
  }
  function respond(results) {
    var msg = JSON.stringify({ 'oslc:results': results });
    if (windowName) {
      window.name = msg;
      if (returnURL) { location.href = returnURL; }
      return;
    }
    var target = window.opener || (window.parent !== window ? window.parent : null);
    if (target) { target.postMessage('oslc-response:' + msg, '*'); }
  }
  function result(item) {
    return { 'oslc:label': item.key + ' ' + item.title, 'rdf:resource': resBase + encodeURIComponent(item.id) };
  }
  function fail(e) {
    if (e && e.status === 401) { showSignIn(); return; }
    status((T.failed || '{error}').replace('{error}', e && e.message ? e.message : String(e)), true);
  }
  function showSignIn() {
    document.getElementById('signin').hidden = false;
    status('');
  }

  function startSelect() {
    var search = document.getElementById('search'), pick = document.getElementById('pick');
    var list = document.getElementById('list'), ok = document.getElementById('ok');
    var items = {};
    search.hidden = false;
    pick.hidden = false;
    function count() {
      var n = list.querySelectorAll('input:checked').length;
      ok.disabled = n === 0;
      return n;
    }
    function load() {
      var q = document.getElementById('q').value.trim();
      status(T.loading);
      var expr = q ? '"' + q.replace(/"/g, '') + '"' : '';
      api('GET', '/projects/' + encodeURIComponent(pid) + '/' + plural + (expr ? '?q=' + encodeURIComponent(expr) : '')).then(function (res) {
        var data = (res && (res.items || res.data)) || res || [];
        list.textContent = '';
        items = {};
        data.slice(0, 200).forEach(function (it) {
          items[it.id] = it;
          var li = document.createElement('li');
          var label = document.createElement('label');
          var box = document.createElement('input');
          box.type = 'checkbox';
          box.value = it.id;
          box.addEventListener('change', count);
          var key = document.createElement('span');
          key.className = 'key';
          key.textContent = it.key;
          var title = document.createElement('span');
          title.textContent = it.title;
          label.appendChild(box);
          label.appendChild(key);
          label.appendChild(title);
          li.appendChild(label);
          list.appendChild(li);
        });
        status(data.length ? '' : T.none);
        count();
      }, fail);
    }
    search.addEventListener('submit', function (e) { e.preventDefault(); load(); });
    pick.addEventListener('submit', function (e) {
      e.preventDefault();
      var out = [];
      list.querySelectorAll('input:checked').forEach(function (b) { out.push(result(items[b.value])); });
      respond(out);
    });
    document.getElementById('cancel').addEventListener('click', function () { respond([]); });
    load();
  }

  function startCreate() {
    var form = document.getElementById('create');
    form.hidden = false;
    document.getElementById('title').focus();
    document.getElementById('cancel').addEventListener('click', function () { respond([]); });
    form.addEventListener('submit', function (e) {
      e.preventDefault();
      var ok = document.getElementById('ok');
      var data = {};
      ['title', 'description', 'obligation', 'preconditions', 'expected_result'].forEach(function (f) {
        var el = document.getElementById(f);
        if (el && el.value.trim()) { data[f] = el.value.trim(); }
      });
      if (kind === 'requirement') { data.create_test = false; }
      ok.disabled = true;
      status(T.loading);
      api('POST', '/projects/' + encodeURIComponent(pid) + '/' + plural, data).then(function (item) {
        status((T.created || '').replace('{key}', item.key));
        respond([result(item)]);
      }, function (err) { ok.disabled = false; fail(err); });
    });
  }

  function start() {
    document.getElementById('signin').hidden = true;
    api('GET', '/auth/me').then(function (me) {
      csrf = (me && me.csrf_token) || '';
      return api('GET', '/projects/' + encodeURIComponent(pid)).then(function (pr) {
        if (pr && pr.key) { document.getElementById('project').textContent = pr.key + ' – ' + pr.name; }
        if (mode === 'select') { startSelect(); } else { startCreate(); }
      });
    }, fail).catch(fail);
  }
  document.getElementById('retry').addEventListener('click', start);
  start();
})();
`
