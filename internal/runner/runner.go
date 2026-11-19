// Package runner executes automated test scripts in a constrained child
// process. When sandboxing is enabled the server re-executes itself as
// `qtestframe sandbox-exec`, which applies rlimits, no_new_privs, Landlock
// and seccomp before exec'ing the interpreter.
package runner

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/hardening"
)

// ErrDisabled is returned when script execution is turned off.
var ErrDisabled = errors.New("runner: script execution is disabled")

// Result is the outcome of one script run.
type Result struct {
	ExitCode  int           `json:"exit_code"`
	Output    string        `json:"output"`
	Duration  time.Duration `json:"duration"`
	TimedOut  bool          `json:"timed_out"`
	Truncated bool          `json:"truncated"`
	Passed    bool          `json:"passed"`
}

// Runner runs scripts according to configuration.
type Runner struct {
	Cfg config.Runner
	// Self is the path of the server binary used for sandbox re-exec.
	Self string
}

// New creates a runner.
func New(cfg config.Runner) *Runner {
	self, _ := os.Executable()
	return &Runner{Cfg: cfg, Self: self}
}

// Enabled reports whether scripts may be executed.
func (r *Runner) Enabled() bool { return r != nil && r.Cfg.Enabled }

var extensions = map[string]string{"sh": ".sh", "bash": ".sh", "python": ".py", "javascript": ".js"}

// Run executes script with the interpreter configured for lang. env
// entries are added as QTF_<KEY>=value.
func (r *Runner) Run(ctx context.Context, lang, script string, env map[string]string) (*Result, error) {
	if !r.Enabled() {
		return nil, ErrDisabled
	}
	interp, ok := r.Cfg.Interpreters[lang]
	if !ok || interp == "" {
		return nil, fmt.Errorf("runner: no interpreter configured for %q", lang)
	}
	if err := os.MkdirAll(r.Cfg.WorkDir, 0o700); err != nil {
		return nil, fmt.Errorf("runner: work dir: %w", err)
	}
	dir, err := os.MkdirTemp(r.Cfg.WorkDir, "run-")
	if err != nil {
		return nil, fmt.Errorf("runner: temp dir: %w", err)
	}
	defer os.RemoveAll(dir)
	file := filepath.Join(dir, "script"+extensions[lang])
	if err := os.WriteFile(file, []byte(script), 0o600); err != nil {
		return nil, fmt.Errorf("runner: write script: %w", err)
	}
	timeout := r.Cfg.Timeout
	if timeout <= 0 {
		timeout = time.Minute
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	if r.Cfg.Sandbox {
		args := []string{"sandbox-exec", "--workdir", dir, "--cpu", strconv.Itoa(int(timeout.Seconds()) + 1), "--", interp, file}
		cmd = exec.CommandContext(cctx, r.Self, args...)
	} else {
		cmd = exec.CommandContext(cctx, interp, file)
	}
	cmd.Dir = dir
	cmd.Env = buildEnv(dir, env)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// kill the whole process group
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		return nil
	}
	max := r.Cfg.MaxOutputBytes
	if max <= 0 {
		max = 1 << 20
	}
	out := &limitedBuffer{max: max}
	cmd.Stdout, cmd.Stderr = out, out
	cmd.Stdin = nil
	start := time.Now()
	err = cmd.Run()
	res := &Result{Duration: time.Since(start), Output: out.String(), Truncated: out.truncated}
	if cctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = -1
		res.Output += fmt.Sprintf("\n[qtestframe] script timed out after %s\n", timeout)
		return res, nil
	}
	var ee *exec.ExitError
	switch {
	case err == nil:
		res.Passed = true
	case errors.As(err, &ee):
		res.ExitCode = ee.ExitCode()
	default:
		return nil, fmt.Errorf("runner: start: %w", err)
	}
	return res, nil
}

func buildEnv(dir string, extra map[string]string) []string {
	env := []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"HOME=" + dir,
		"TMPDIR=" + dir,
		"LANG=C.UTF-8",
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		name := strings.Map(func(r rune) rune {
			if r >= 'a' && r <= 'z' {
				return r - 32
			}
			if r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
				return r
			}
			return '_'
		}, k)
		env = append(env, "QTF_"+name+"="+extra[k])
	}
	return env
}

// limitedBuffer stores at most max bytes and discards the rest.
type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := l.max - l.buf.Len(); room > 0 {
		if len(p) > room {
			l.buf.Write(p[:room])
			l.truncated = true
		} else {
			l.buf.Write(p)
		}
	} else if len(p) > 0 {
		l.truncated = true
	}
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	s := l.buf.String()
	if l.truncated {
		s += "\n[qtestframe] output truncated\n"
	}
	return s
}

// SandboxConfig is parsed from sandbox-exec arguments.
type SandboxConfig struct {
	WorkDir string
	CPU     uint64
	NoNet   bool
	Argv    []string
}

// ParseSandboxArgs parses `sandbox-exec [flags] -- interpreter script`.
func ParseSandboxArgs(args []string, stderr io.Writer) (*SandboxConfig, error) {
	fs := flag.NewFlagSet("sandbox-exec", flag.ContinueOnError)
	fs.SetOutput(stderr)
	c := &SandboxConfig{}
	fs.StringVar(&c.WorkDir, "workdir", "", "writable working directory")
	fs.Uint64Var(&c.CPU, "cpu", 120, "CPU time limit in seconds")
	fs.BoolVar(&c.NoNet, "no-network", false, "deny socket creation")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	c.Argv = fs.Args()
	if c.WorkDir == "" || len(c.Argv) == 0 {
		return nil, errors.New("sandbox-exec: --workdir and a command are required")
	}
	return c, nil
}

// SandboxOptions returns the hardening options for a sandboxed script.
func SandboxOptions(c *SandboxConfig) hardening.Options {
	return hardening.Options{
		NoNewPrivs:       true,
		DropCapabilities: true,
		Landlock:         true,
		ReadOnlyPaths:    []string{"/etc", "/usr", "/lib", "/lib64", "/bin", "/sbin", "/opt"},
		ReadWritePaths:   []string{c.WorkDir},
		ExecPaths:        []string{"/usr", "/bin", "/lib", "/lib64", "/opt"},
		Seccomp:          true,
		DenyNetwork:      c.NoNet,
	}
}

// SandboxExec applies restrictions to the current process and replaces
// it with the target command. It only returns on error.
func SandboxExec(c *SandboxConfig) error {
	l := hardening.DefaultLimits
	l.CPUSeconds = c.CPU
	if err := hardening.SetResourceLimits(l); err != nil {
		return fmt.Errorf("sandbox-exec: rlimits: %w", err)
	}
	if _, err := hardening.Apply(SandboxOptions(c)); err != nil {
		return fmt.Errorf("sandbox-exec: %w", err)
	}
	if err := os.Chdir(c.WorkDir); err != nil {
		return err
	}
	bin, err := exec.LookPath(c.Argv[0])
	if err != nil {
		return err
	}
	return syscall.Exec(bin, c.Argv, os.Environ())
}
