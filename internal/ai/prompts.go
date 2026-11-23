package ai

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"text/template"
)

// The prompt templates (prompts/<lang>/*.tmpl), embedded. Each feature has
// a "<feature>.system" and a "<feature>.user" template; "rules" and "data"
// are shared. PromptVersion names the version of the templates: change it
// whenever a template changes (TestPromptVersion checks the digest).
//
//go:embed prompts
var promptFS embed.FS

// PromptVersion is the version of the embedded prompt templates; it is
// returned with every answer and written to the audit log.
const PromptVersion = "2026-11.1"

// promptDigest is the digest of the templates of PromptVersion.
const promptDigest = "5a8778a7e017e1b4"

// Languages of the prompts and answers.
var Languages = []string{"en", "sv"}

var templates = func() map[string]*template.Template {
	out := map[string]*template.Template{}
	for _, lang := range Languages {
		t := template.Must(template.New(lang).Option("missingkey=error").ParseFS(promptFS, "prompts/"+lang+"/*.tmpl"))
		out[lang] = t
	}
	return out
}()

// PromptsDigest returns the digest of the embedded templates.
func PromptsDigest() string {
	var names []string
	_ = fs.WalkDir(promptFS, "prompts", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, p)
		}
		return nil
	})
	sort.Strings(names)
	h := sha256.New()
	for _, n := range names {
		b, _ := promptFS.ReadFile(n)
		h.Write([]byte(n))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// Lang returns a supported language ("en" for anything else).
func Lang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if i := strings.IndexAny(lang, "-_"); i > 0 {
		lang = lang[:i]
	}
	for _, l := range Languages {
		if l == lang {
			return l
		}
	}
	return "en"
}

// promptVars are the values of a template.
type promptVars struct {
	// Data is the JSON of the items (HTML-escaped: no "<" can close the
	// data block).
	Data string
	Max  int
	// mappings
	Kind         string
	Frameworks   []string
	FrameworkIDs string
	// retry
	Error string
}

// encodeData returns the item data as indented JSON. encoding/json escapes
// <, > and & (as < ...), so the text of an item can never contain
// the closing </item_data> tag.
func encodeData(v any) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(true)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSpace(b.String()), nil
}

func render(lang, name string, v promptVars) (string, error) {
	t := templates[Lang(lang)]
	var b bytes.Buffer
	if err := t.ExecuteTemplate(&b, name, v); err != nil {
		return "", fmt.Errorf("ai: prompt %s: %w", name, err)
	}
	return strings.TrimSpace(b.String()), nil
}
