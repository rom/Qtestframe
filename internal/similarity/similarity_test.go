package similarity

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStemEnglish(t *testing.T) {
	cases := map[string]string{
		"running": "run", "generously": "generous", "communication": "communic", "happiness": "happi",
		"caresses": "caress", "ponies": "poni", "ties": "tie", "agreed": "agre", "hopping": "hop",
		"relational": "relat", "conditional": "condit", "national": "nation", "requirements": "requir",
		"logged": "log", "users": "user", "encrypted": "encrypt", "skies": "sky", "news": "news",
		"proceed": "proceed", "is": "is", "fully": "fulli", "controllers": "control", "hopeful": "hope",
	}
	for w, want := range cases {
		if got := StemEnglish(w); got != want {
			t.Errorf("%s: %s, want %s", w, got, want)
		}
	}
	if Stem("de", "laufen") != "laufen" || Stem("en", "users") != "user" || Stem("sv", "kraven") != "krav" {
		t.Error("Stem by language")
	}
}

func TestStemSwedish(t *testing.T) {
	cases := map[string]string{
		"användaren": "använd", "användarna": "använd", "lösenordet": "lösenord", "inloggningen": "inloggning",
		"kraven": "krav", "sekunder": "sekund", "krypterade": "krypter", "systemet": "system", "försöken": "försök",
		"lyckligt": "lyck", "klockan": "klockan", "ja": "ja", "hoppfullt": "hoppfull",
	}
	for w, want := range cases {
		if got := StemSwedish(w); got != want {
			t.Errorf("%s: %s, want %s", w, got, want)
		}
	}
}

func TestTerms(t *testing.T) {
	keys := func(text, lang string) string {
		var out []string
		for _, x := range terms(text, lang) {
			out = append(out, x.key)
		}
		return strings.Join(out, " ")
	}
	cases := []struct{ text, lang, want string }{
		{"The user MUST sign in with e-mail", "en", "c:user c:login c:email"},
		{"Users shall log in using their email address", "en", "c:user c:login w:use c:email"},
		{"Användaren ska logga in med e-post", "sv", "c:user c:login c:email"},
		{"Användarinloggning", "sv", "c:user c:login"},
		{"Lösenordslängd minst 12 tecken", "sv", "c:password w:läng w:minst n:12 w:teck"},
		{"five attempts", "en", "n:5 c:attempt"},
	}
	for _, c := range cases {
		if got := keys(c.text, c.lang); got != c.want {
			t.Errorf("%q: %s, want %s", c.text, got, c.want)
		}
	}
	if Language("Användaren ska kunna logga in") != "sv" || Language("The user shall sign in") != "en" {
		t.Error("language")
	}
}

func TestSentences(t *testing.T) {
	got := sentences("First 2.5 s. Second!\nThird; fourth")
	var parts []string
	text := "First 2.5 s. Second!\nThird; fourth"
	for _, s := range got {
		parts = append(parts, text[s.start:s.end])
	}
	if strings.Join(parts, "|") != "First 2.5 s.|Second!|Third;|fourth" {
		t.Fatal(parts)
	}
}

func TestConstraints(t *testing.T) {
	cases := []struct {
		text     string
		negative bool
		op       string
		value    float64
		dim      string
	}{
		{"respond within 2 seconds", false, opLE, 2, "time"},
		{"in at least 5 s", false, opGE, 5, "time"},
		{"at most 2000 ms", false, opLE, 2, "time"},
		{"≤ 2 s", false, opLE, 2, "time"},
		{"minst 12 tecken", false, opGE, 12, "chars"},
		{"högst 15 minuters inaktivitet", false, opLE, 900, "time"},
		{"must not exceed 2 seconds", true, opLE, 2, "time"},
		{"no more than 3 attempts", true, opLE, 3, "count:attempt"},
		{"8 characters or more", false, opGE, 8, "chars"},
		{"mer än 10 MB", false, opGT, 10e6, "bytes"},
		{"inom 1,5 timmar", false, opLE, 5400, "time"},
	}
	for _, c := range cases {
		cs := parseConstraints(c.text, c.negative)
		if len(cs) != 1 || cs[0].op != c.op || math.Abs(cs[0].value-c.value) > 1e-6 || cs[0].dim != c.dim {
			t.Errorf("%q: %+v", c.text, cs)
		}
	}
	for _, text := range []string{"ISO27001", "TLS 1.2 or later", "version 3", "12 characters"} {
		if cs := parseConstraints(text, false); len(cs) != 0 {
			t.Errorf("%q: %+v", text, cs)
		}
	}
	le := func(v float64) constraint { return constraint{op: opLE, value: v, dim: "time"} }
	ge := func(v float64) constraint { return constraint{op: opGE, value: v, dim: "time"} }
	if !disjoint(le(2), ge(5)) || disjoint(le(5), ge(2)) || disjoint(le(2), ge(2)) || !disjoint(constraint{op: opLT, value: 2, dim: "time"}, ge(2)) {
		t.Error("intervals")
	}
	if disjoint(le(2), constraint{op: opGE, value: 5, dim: "chars"}) || !disjoint(constraint{op: opEQ, value: 3, dim: "x"}, constraint{op: opEQ, value: 4, dim: "x"}) {
		t.Error("dimensions and equality")
	}
}

func TestIndex(t *testing.T) {
	x := NewIndex([]Doc{
		{ID: "a", Title: "Login", Description: "The user MUST sign in with a password."},
		{ID: "b", Title: "Log in", Description: "Users shall log in using a password."},
		{ID: "c", Title: "Export", Description: "Reports MAY be exported as PDF."},
		{ID: "d", Title: "export", Description: "reports may be exported as pdf"},
		{ID: "e", Title: "", Description: ""},
	})
	if x.Len() != 5 || !x.Has("a") || x.Has("z") || x.Score("a", "z") != 0 {
		t.Fatal("index")
	}
	if s := x.Score("c", "d"); s != 1 {
		t.Fatalf("identical normalised texts: %.2f", s)
	}
	if s := x.Score("a", "b"); s < DefaultDuplicateThreshold {
		t.Errorf("paraphrase %.2f", s)
	}
	if s := x.Score("a", "c"); s > 0.1 {
		t.Errorf("unrelated %.2f", s)
	}
	m := x.Similar(Doc{Title: "Sign in", Description: "sign in with password"}, 2, 0.1, func(id string) bool { return id == "b" })
	if len(m) != 1 || m[0].ID != "a" {
		t.Fatal(m)
	}
	if m := x.Similar(Doc{Title: "export", Description: "reports may be exported as pdf"}, 0, 0, nil); len(m) < 2 || m[0].Score != 1 {
		t.Fatal(m)
	}
	pairs := x.Pairs(0.5)
	if len(pairs) != 2 || pairs[0].Score < pairs[1].Score {
		t.Fatal(pairs)
	}
	if (Doc{Title: " A ", Description: "B", Details: []string{" ", "C"}}).Text() != "A\nB\nC" {
		t.Error("text")
	}
}

func TestConflictKinds(t *testing.T) {
	docs := []ConflictDoc{
		{Doc: Doc{ID: "1", Title: "Data export", Description: "The user MUST be able to export personal data."}},
		{Doc: Doc{ID: "2", Title: "Data export", Description: "The user MUST NOT be able to export personal data."}},
		{Doc: Doc{ID: "3", Title: "Export never", Description: "Personal data must never be exported by the user."}, Obligation: "must"},
		{Doc: Doc{ID: "4", Title: "Agreeing", Description: "The user MUST NOT export data without consent."}},
		{Doc: Doc{ID: "5", Title: "Unrelated", Description: "Reports SHOULD be printable."}},
	}
	var ds []Doc
	for _, d := range docs {
		ds = append(ds, d.Doc)
	}
	x := NewIndex(ds)
	found := map[string]Conflict{}
	for _, c := range Conflicts(x, docs, 0.5, nil) {
		found[c.A+c.B] = c
	}
	if c, ok := found["12"]; !ok || c.Kind != ConflictObligation || c.Note != "MUST ↔ MUST NOT" {
		t.Fatalf("1-2: %+v", found)
	}
	if c, ok := found["13"]; !ok || c.Kind != ConflictObligation || !strings.Contains(c.Note, "never") {
		t.Fatalf("1-3: %+v", found)
	}
	if _, ok := found["23"]; ok {
		t.Error("2 and 3 agree")
	}
	for k := range found {
		if strings.Contains(k, "5") {
			t.Error("unrelated", k)
		}
	}
	if got := Conflicts(x, docs, 0.5, func(a, b string) bool { return true }); len(got) != 0 {
		t.Error("skip")
	}
}

func TestEmbeddingsClient(t *testing.T) {
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if r.Header.Get("Authorization") != "Bearer k" || req.Model != "m" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		got = append(got, req.Input...)
		type d struct {
			Index     int       `json:"index"`
			Embedding []float32 `json:"embedding"`
		}
		var out struct {
			Data []d `json:"data"`
		}
		// answered in reverse order: the client sorts by index
		for i := len(req.Input) - 1; i >= 0; i-- {
			out.Data = append(out.Data, d{Index: i, Embedding: []float32{float32(len(req.Input[i])), 0, 3}})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	c := &Client{URL: srv.URL, Key: "k", ModelName: "m", Batch: 2}
	vs, err := c.Embed(context.Background(), []string{"a", "bbbb", "cc"})
	if err != nil || len(vs) != 3 || len(got) != 3 || c.Model() != "m" {
		t.Fatal(vs, err, got)
	}
	if math.Abs(float64(vs[1][0])-0.8) > 1e-6 { // (4, 0, 3) normalised
		t.Fatal(vs[1])
	}
	if _, err := (&Client{URL: srv.URL, Key: "x", ModelName: "m"}).Embed(context.Background(), []string{"a"}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatal(err)
	}
	if (&Client{}).Model() != "default" {
		t.Error("default model")
	}
	enc := EncodeVector(vs[0])
	if back, err := DecodeVector(enc); err != nil || len(back) != 3 || back[2] != vs[0][2] {
		t.Fatal(back, err)
	}
	if _, err := DecodeVector("!!"); err == nil {
		t.Error("invalid vector")
	}
	if TextHash("a") == TextHash("b") || len(TextHash("a")) != 32 {
		t.Error("hash")
	}
	x := NewDenseIndex([]string{"a", "b", "c"}, vs)
	if x.Score("a", "c") <= 0 || x.Score("a", "z") != 0 {
		t.Error("dense score")
	}
	if n := x.Nearest([]float32{4, 0, 3}, 1, 0, nil); len(n) != 1 || n[0].ID != "b" {
		t.Fatal(n)
	}
	if p := x.Pairs(0.9); len(p) == 0 || p[0].Score < 0.9 {
		t.Fatal(p)
	}
}
