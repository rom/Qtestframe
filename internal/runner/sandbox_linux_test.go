//go:build linux

package runner

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sandboxHelperEnv = "QTF_RUNNER_SANDBOX_HELPER"

// TestHelperSandboxExec runs SandboxExec in a child process with a command
// that cannot be executed, so SandboxExec returns (after hardening) instead
// of replacing the process.
func TestHelperSandboxExec(t *testing.T) {
	spec := os.Getenv(sandboxHelperEnv)
	if spec == "" {
		t.Skip("helper process only")
	}
	workdir, argv0, _ := strings.Cut(spec, "|")
	err := SandboxExec(&SandboxConfig{WorkDir: workdir, CPU: 30, Argv: []string{argv0}})
	fmt.Printf("SANDBOX_ERR %v\n", err)
}

// runSandboxHelper executes the helper. Coverage counters are written into
// the (Landlock-writable) work dir and copied to our coverage directory so
// that SandboxExec counts towards the package coverage.
func runSandboxHelper(t *testing.T, workdir, argv0 string) string {
	t.Helper()
	args := []string{"-test.run=^TestHelperSandboxExec$", "-test.count=1"}
	parentCov := ""
	if f := flag.Lookup("test.gocoverdir"); f != nil {
		parentCov = f.Value.String()
	}
	childCov := filepath.Join(workdir, "cov")
	// A coverage-instrumented child always needs a writable coverage
	// directory: under Landlock it cannot create one in $TMPDIR itself.
	if testing.CoverMode() != "" {
		if err := os.MkdirAll(childCov, 0o700); err != nil {
			t.Fatal(err)
		}
		args = append(args, "-test.gocoverdir="+childCov)
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), sandboxHelperEnv+"="+workdir+"|"+argv0)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	if parentCov != "" {
		entries, _ := os.ReadDir(childCov)
		for _, e := range entries {
			copyFile(t, filepath.Join(childCov, e.Name()), filepath.Join(parentCov, e.Name()))
		}
	}
	for _, l := range strings.Split(string(out), "\n") {
		if rest, ok := strings.CutPrefix(l, "SANDBOX_ERR "); ok {
			return rest
		}
	}
	t.Fatalf("no result:\n%s", out)
	return ""
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	if _, err := os.Stat(dst); err == nil {
		return // meta-data file shared with this process
	}
	in, err := os.Open(src)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
}

func TestSandboxExecReturnsErrors(t *testing.T) {
	// Command not found.
	if got := runSandboxHelper(t, t.TempDir(), "definitely-not-a-command-qtf"); !strings.Contains(got, "not found") {
		t.Errorf("lookpath: %q", got)
	}
	// Executable that the kernel refuses to exec (no ELF, no shebang).
	wd := t.TempDir()
	bad := filepath.Join(wd, "bad")
	if err := os.WriteFile(bad, []byte{0, 1, 2, 3}, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := runSandboxHelper(t, wd, bad); got == "<nil>" || got == "" {
		t.Errorf("exec of garbage returned %q", got)
	}
}
