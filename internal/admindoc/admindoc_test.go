package admindoc

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rom/qtestframe/internal/app"
	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/settings"
)

const root = "../.."

// The generated parts of the guide match the code (make admin-guide-ref).
func TestGuideUpToDate(t *testing.T) {
	var msgs []string
	if err := Run(root, true, func(s string) { msgs = append(msgs, s) }); err != nil {
		t.Fatalf("%v\n%s", err, strings.Join(msgs, "\n"))
	}
}

// Every configuration key and environment variable has a description, and
// no description names a key that is gone.
func TestEveryKeyDescribed(t *testing.T) {
	keys := map[string]bool{}
	for _, f := range Fields() {
		keys[f.Key] = true
		d, ok := keyDocs[f.Key]
		if !ok || d.en == "" || d.sv == "" {
			t.Errorf("%s: no description in both languages", f.Key)
		}
	}
	for k := range keyDocs {
		if !keys[k] {
			t.Errorf("description of %s, which is not a configuration key", k)
		}
	}
	for k := range defaultDocs {
		if !keys[k] {
			t.Errorf("default of %s, which is not a configuration key", k)
		}
	}
	for env, key := range config.EnvVars {
		if _, ok := envDocs[env]; !ok {
			t.Errorf("%s: no description", env)
		}
		if !keys[key] && key != "email.password" && key != "secrets.local.key" && key != "secrets.pkcs11.pin" && key != "metrics.token" && key != "tracing.headers" {
			t.Errorf("%s sets %s, which is not a configuration key", env, key)
		}
	}
	for env := range envDocs {
		if _, ok := config.EnvVars[env]; !ok {
			t.Errorf("description of %s, which config.EnvVars does not read", env)
		}
	}
}

// Every key of the configuration file is in the system settings (editable
// or shown read-only) or listed as file-only with a reason (the inventory
// of the administration view).
func TestSettingsInventory(t *testing.T) {
	inReg := map[string]bool{}
	for _, d := range settings.Registry {
		inReg[d.Key] = true
	}
	for _, f := range Fields() {
		k := f.Key
		if i := strings.Index(k, "[]"); i >= 0 {
			k = k[:i] // providers: the list is one setting
		}
		if inReg[k] || inReg[f.Key] {
			continue
		}
		if why, ok := settings.FileOnly[k]; !ok || why == "" {
			t.Errorf("%s: neither a system setting nor in settings.FileOnly", f.Key)
		}
	}
	for k := range settings.FileOnly {
		if inReg[k] {
			t.Errorf("%s: in settings.FileOnly and in the registry", k)
		}
	}
}

// Every command of `qtestframe help` is documented.
func TestCommandsCovered(t *testing.T) {
	var out bytes.Buffer
	if code := app.Run([]string{"help"}, app.Env{Stdout: &out, Stderr: &out, Ctx: context.Background()}); code != 0 {
		t.Fatal(out.String())
	}
	documented := map[string]bool{}
	for _, c := range commands {
		documented[strings.Join(c.args, " ")] = true
	}
	re := regexp.MustCompile(`(?m)^  qtestframe ([a-z-]+)(?: (create|add|verify)\b)?`)
	for _, m := range re.FindAllStringSubmatch(out.String(), -1) {
		name := strings.TrimSpace(m[1] + " " + m[2])
		if name == "secrets" {
			for _, sub := range regexp.MustCompile(`(?m)^  ([a-z-]+)\s`).FindAllStringSubmatch(secretsHelp(t), -1) {
				if !documented["secrets "+sub[1]] {
					t.Errorf("secrets %s is not documented", sub[1])
				}
			}
			continue
		}
		if !documented[name] {
			t.Errorf("%s is not documented", name)
		}
	}
}

func secretsHelp(t *testing.T) string {
	var out bytes.Buffer
	if code := app.Run([]string{"secrets", "help"}, app.Env{Stdout: &out, Stderr: &out, Ctx: context.Background()}); code != 0 {
		t.Fatal(out.String())
	}
	return out.String()
}

func TestFlagsOf(t *testing.T) {
	flags, err := flagsOf([]string{"serve"})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]cliFlag{}
	for _, f := range flags {
		names[f.name] = f
	}
	if names["port"].typ != "int" || names["config"].usage == "" || names["p"].name != "p" {
		t.Errorf("flags: %+v", flags)
	}
	if _, err := flagsOf([]string{"no-such-command"}); err == nil {
		t.Error("unknown command accepted")
	}
}

func TestReplace(t *testing.T) {
	page := "# T\n\n" + begin("x") + "\nold\n" + end("x") + "\n"
	got, err := Replace(page, "x", "new")
	if err != nil || !strings.Contains(got, begin("x")+"\n\nnew\n\n"+end("x")) {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := Replace("# T\n", "x", "new"); err == nil {
		t.Error("missing markers accepted")
	}
	if _, err := Replace(page+begin("x"), "x", "new"); err == nil {
		t.Error("repeated markers accepted")
	}
}

// Run writes stale pages, then finds them current; unknown blocks fail.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	for _, lang := range Languages {
		if err := os.MkdirAll(filepath.Join(dir, Dir, lang), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, b := range blocks() {
			path := filepath.Join(dir, Dir, lang, b.page)
			old, _ := os.ReadFile(path)
			old = append(old, []byte("\n"+begin(b.name)+"\n"+end(b.name)+"\n")...)
			if err := os.WriteFile(path, old, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	log := func(string) {}
	if err := Run(dir, true, log); err == nil || !strings.Contains(err.Error(), "out of date") {
		t.Fatalf("check of stale pages: %v", err)
	}
	if err := Run(dir, false, log); err != nil {
		t.Fatal(err)
	}
	if err := Run(dir, true, log); err != nil {
		t.Fatalf("check of current pages: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, Dir, "sv", "configuration.md"))
	if !strings.Contains(string(b), "| `server.listen` | text | `:8080` |") || !strings.Contains(string(b), "`QTF_SERVER_LISTEN`") {
		t.Errorf("configuration page:\n%.800s", b)
	}
	b, _ = os.ReadFile(filepath.Join(dir, Dir, "en", "cli-reference.md"))
	if !strings.Contains(string(b), "### `qtestframe serve`") || !strings.Contains(string(b), "| `-p` | `int` |") {
		t.Errorf("command line page:\n%.800s", b)
	}
	if err := os.WriteFile(filepath.Join(dir, Dir, "en", "extra.md"), []byte(begin("nope")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Run(dir, false, log); err == nil {
		t.Error("unknown block accepted")
	}
	if err := Run(filepath.Join(dir, "missing"), false, log); err == nil {
		t.Error("missing folder accepted")
	}
}
