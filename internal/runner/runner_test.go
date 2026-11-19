package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/rom/qtestframe/internal/config"
)

// TestMain lets the test binary act as the `sandbox-exec` re-exec target,
// like the real server binary does.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "sandbox-exec" {
		c, err := ParseSandboxArgs(os.Args[2:], os.Stderr)
		if err == nil {
			err = SandboxExec(c)
		}
		fmt.Fprintln(os.Stderr, "sandbox-exec:", err)
		os.Exit(125)
	}
	os.Exit(m.Run())
}

func newRunner(t *testing.T) *Runner {
	t.Helper()
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh not available")
	}
	r := New(config.Runner{
		Enabled:        true,
		Timeout:        10 * time.Second,
		WorkDir:        filepath.Join(t.TempDir(), "work"),
		MaxOutputBytes: 1 << 16,
		Interpreters:   map[string]string{"sh": "/bin/sh"},
	})
	if r.Self == "" {
		t.Fatal("Self not set")
	}
	return r
}

func TestEnabled(t *testing.T) {
	var nilRunner *Runner
	if nilRunner.Enabled() {
		t.Error("nil runner enabled")
	}
	r := New(config.Runner{})
	if r.Enabled() {
		t.Error("disabled runner enabled")
	}
	if _, err := r.Run(context.Background(), "sh", "true", nil); !errors.Is(err, ErrDisabled) {
		t.Errorf("err = %v", err)
	}
}

func TestRunPassAndEnv(t *testing.T) {
	r := newRunner(t)
	res, err := r.Run(context.Background(), "sh", `echo "key=$QTF_TEST_KEY other=$QTF_MY_VAR_2 home=$HOME"; pwd`, map[string]string{"test_key": "v1", "my-var.2": "v2"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed || res.ExitCode != 0 || res.TimedOut || res.Truncated {
		t.Errorf("result %+v", res)
	}
	if !strings.Contains(res.Output, "key=v1 other=v2 home="+r.Cfg.WorkDir+"/run-") {
		t.Errorf("output %q", res.Output)
	}
	// The temporary run directory is removed afterwards.
	entries, _ := os.ReadDir(r.Cfg.WorkDir)
	if len(entries) != 0 {
		t.Errorf("work dir not cleaned: %v", entries)
	}
}

func TestRunFail(t *testing.T) {
	r := newRunner(t)
	res, err := r.Run(context.Background(), "sh", "echo oops >&2; exit 3", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed || res.ExitCode != 3 || !strings.Contains(res.Output, "oops") {
		t.Errorf("result %+v", res)
	}
}

func TestRunTimeout(t *testing.T) {
	r := newRunner(t)
	r.Cfg.Timeout = 200 * time.Millisecond
	start := time.Now()
	res, err := r.Run(context.Background(), "sh", "sleep 30 & sleep 30; echo done", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.TimedOut || res.ExitCode != -1 || res.Passed || !strings.Contains(res.Output, "timed out") {
		t.Errorf("result %+v", res)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("timeout took %s", time.Since(start))
	}
}

func TestRunDefaults(t *testing.T) {
	r := newRunner(t)
	r.Cfg.Timeout, r.Cfg.MaxOutputBytes = 0, 0
	res, err := r.Run(context.Background(), "sh", "echo hi", nil)
	if err != nil || !res.Passed || res.Output != "hi\n" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestRunTruncation(t *testing.T) {
	r := newRunner(t)
	r.Cfg.MaxOutputBytes = 10
	res, err := r.Run(context.Background(), "sh", `i=0; while [ $i -lt 50 ]; do echo 0123456789; i=$((i+1)); done`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Truncated || !strings.HasPrefix(res.Output, "0123456789\n[qtestframe] output truncated") {
		t.Errorf("result %+v", res)
	}
}

func TestRunErrors(t *testing.T) {
	r := newRunner(t)
	if _, err := r.Run(context.Background(), "cobol", "x", nil); err == nil || !strings.Contains(err.Error(), "no interpreter") {
		t.Errorf("unknown lang: %v", err)
	}
	r.Cfg.Interpreters["empty"] = ""
	if _, err := r.Run(context.Background(), "empty", "x", nil); err == nil {
		t.Error("empty interpreter accepted")
	}
	r.Cfg.Interpreters["sh"] = "/nonexistent/interpreter"
	if _, err := r.Run(context.Background(), "sh", "true", nil); err == nil || !strings.Contains(err.Error(), "runner: start") {
		t.Errorf("missing interpreter: %v", err)
	}
	r.Cfg.Interpreters["sh"] = "/bin/sh"
	file := filepath.Join(t.TempDir(), "file")
	os.WriteFile(file, nil, 0o600)
	r.Cfg.WorkDir = filepath.Join(file, "sub")
	if _, err := r.Run(context.Background(), "sh", "true", nil); err == nil || !strings.Contains(err.Error(), "work dir") {
		t.Errorf("bad work dir: %v", err)
	}
}

func TestBuildEnv(t *testing.T) {
	env := buildEnv("/w", map[string]string{"b": "2", "a": "1", "x-y": "3"})
	want := []string{"PATH=/usr/local/bin:/usr/bin:/bin", "HOME=/w", "TMPDIR=/w", "LANG=C.UTF-8", "QTF_A=1", "QTF_B=2", "QTF_X_Y=3"}
	if strings.Join(env, "|") != strings.Join(want, "|") {
		t.Errorf("env = %v", env)
	}
}

func TestLimitedBuffer(t *testing.T) {
	l := &limitedBuffer{max: 4}
	for _, p := range []string{"ab", "cdef", "", "gh"} {
		if n, err := l.Write([]byte(p)); n != len(p) || err != nil {
			t.Fatalf("Write(%q) = %d, %v", p, n, err)
		}
	}
	if l.buf.String() != "abcd" || !l.truncated {
		t.Errorf("buf=%q truncated=%v", l.buf.String(), l.truncated)
	}
	l2 := &limitedBuffer{max: 4}
	l2.Write([]byte("abcd"))
	l2.Write(nil)
	if l2.truncated || l2.String() != "abcd" {
		t.Errorf("exact fit: %q %v", l2.String(), l2.truncated)
	}
}

func TestParseSandboxArgs(t *testing.T) {
	var stderr bytes.Buffer
	c, err := ParseSandboxArgs([]string{"--workdir", "/w", "--cpu", "7", "--no-network", "--", "/bin/sh", "s.sh"}, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	if c.WorkDir != "/w" || c.CPU != 7 || !c.NoNet || len(c.Argv) != 2 || c.Argv[0] != "/bin/sh" {
		t.Errorf("%+v", c)
	}
	c, err = ParseSandboxArgs([]string{"--workdir", "/w", "cmd"}, &stderr)
	if err != nil || c.CPU != 120 || c.NoNet {
		t.Errorf("defaults: %+v %v", c, err)
	}
	for _, args := range [][]string{
		{"--", "/bin/sh"},
		{"--workdir", "/w"},
		{"--bogus"},
		{"--cpu", "lots", "--workdir", "/w", "x"},
	} {
		if _, err := ParseSandboxArgs(args, &stderr); err == nil {
			t.Errorf("%v: expected error", args)
		}
	}
}

func TestSandboxOptions(t *testing.T) {
	o := SandboxOptions(&SandboxConfig{WorkDir: "/w", NoNet: true})
	if !o.NoNewPrivs || !o.DropCapabilities || !o.Landlock || !o.Seccomp || !o.DenyNetwork {
		t.Errorf("%+v", o)
	}
	if len(o.ReadWritePaths) != 1 || o.ReadWritePaths[0] != "/w" || len(o.ExecPaths) == 0 {
		t.Errorf("paths: %+v", o)
	}
	if SandboxOptions(&SandboxConfig{WorkDir: "/w"}).DenyNetwork {
		t.Error("DenyNetwork without NoNet")
	}
}

func TestSandboxedRun(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sandbox requires linux")
	}
	r := newRunner(t)
	r.Cfg.Sandbox = true
	r.Self = os.Args[0]
	outside := t.TempDir()
	script := fmt.Sprintf(`echo inside > ./in.txt && cat ./in.txt
if { echo x > %s/escape.txt; } 2>/dev/null; then echo ESCAPED; fi
while read -r line; do case "$line" in NoNewPrivs:*|Seccomp:*) echo "$line";; esac; done < /proc/self/status
echo "v=$QTF_V"`, outside)
	res, err := r.Run(context.Background(), "sh", script, map[string]string{"v": "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Passed {
		t.Fatalf("sandboxed script failed: %+v", res)
	}
	if !strings.Contains(res.Output, "inside\n") || !strings.Contains(res.Output, "NoNewPrivs:\t1") || !strings.Contains(res.Output, "v=ok") {
		t.Errorf("output %q", res.Output)
	}
	if landlockAvailable() {
		if strings.Contains(res.Output, "ESCAPED") {
			t.Errorf("sandboxed script wrote outside its work dir: %q", res.Output)
		}
		if _, err := os.Stat(filepath.Join(outside, "escape.txt")); err == nil {
			t.Error("escape.txt was created")
		}
	}
}

func TestSandboxExecBadArgs(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("sandbox requires linux")
	}
	r := newRunner(t)
	r.Cfg.Sandbox = true
	r.Self = os.Args[0]
	// The interpreter does not exist: sandbox-exec fails after hardening.
	r.Cfg.Interpreters["sh"] = "/nonexistent/sh"
	res, err := r.Run(context.Background(), "sh", "true", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed || res.ExitCode != 125 || !strings.Contains(res.Output, "sandbox-exec:") {
		t.Errorf("result %+v", res)
	}
}
