//go:build linux

package hardening

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

const helperEnv = "QTF_HARDENING_HELPER"

// childSpec is passed (as JSON) to the helper process.
type childSpec struct {
	Opts    Options
	Limits  *Limits
	Inside  string // file path that must stay writable
	Outside string // file path that must not be writable under Landlock
}

// TestHelperProcess is not a real test: it is executed in a child process
// by runChild because Apply irreversibly restricts the calling process.
func TestHelperProcess(t *testing.T) {
	raw := os.Getenv(helperEnv)
	if raw == "" {
		t.Skip("helper process only")
	}
	var spec childSpec
	if err := json.Unmarshal([]byte(raw), &spec); err != nil {
		fmt.Println("ERROR", err)
		return
	}
	out := map[string]string{}
	if spec.Limits != nil {
		out["limits_err"] = errString(SetResourceLimits(*spec.Limits))
		var rl unix.Rlimit
		_ = unix.Getrlimit(unix.RLIMIT_NOFILE, &rl)
		out["nofile"] = strconv.FormatUint(rl.Cur, 10)
		_ = unix.Getrlimit(unix.RLIMIT_CORE, &rl)
		out["core"] = strconv.FormatUint(rl.Max, 10)
	}
	rep, err := Apply(spec.Opts)
	out["apply_err"] = errString(err)
	b, _ := json.Marshal(rep)
	out["report"] = string(b)
	for k, v := range procStatus() {
		out[k] = v
	}
	if spec.Inside != "" {
		out["write_inside"] = errString(os.WriteFile(spec.Inside, []byte("ok"), 0o600))
	}
	if spec.Outside != "" {
		out["write_outside"] = errString(os.WriteFile(spec.Outside, []byte("no"), 0o600))
	}
	out["unshare"] = errString(unix.Unshare(unix.CLONE_NEWUTS))
	_, _, e := syscall.Syscall(syscall.SYS_PTRACE, unix.PTRACE_PEEKDATA, 1, 0)
	out["ptrace"] = errString(errnoErr(e))
	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0)
	if err == nil {
		unix.Close(fd)
	}
	out["socket"] = errString(err)
	b, _ = json.Marshal(out)
	fmt.Println("RESULT " + string(b))
	// Exit directly: the sandboxed child may not write coverage data.
	os.Exit(0)
}

func errnoErr(e syscall.Errno) error {
	if e == 0 {
		return nil
	}
	return e
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func procStatus() map[string]string {
	out := map[string]string{}
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return out
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if ok {
			out[k] = strings.TrimSpace(v)
		}
	}
	return out
}

// runChild re-executes the test binary with spec and returns the result map.
func runChild(t *testing.T, spec childSpec) map[string]string {
	t.Helper()
	// Let the child write its coverage counters next to ours.
	covDir := ""
	if f := flag.Lookup("test.gocoverdir"); f != nil {
		covDir = f.Value.String()
	}
	if covDir != "" {
		spec.Opts.ReadWritePaths = append(spec.Opts.ReadWritePaths, covDir)
	}
	b, _ := json.Marshal(spec)
	args := []string{"-test.run=^TestHelperProcess$", "-test.count=1"}
	if covDir != "" {
		args = append(args, "-test.gocoverdir="+covDir)
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), helperEnv+"="+string(b))
	outb, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %v\n%s", err, outb)
	}
	for _, line := range strings.Split(string(outb), "\n") {
		if rest, ok := strings.CutPrefix(line, "RESULT "); ok {
			res := map[string]string{}
			if err := json.Unmarshal([]byte(rest), &res); err != nil {
				t.Fatal(err)
			}
			return res
		}
	}
	t.Fatalf("no result from helper:\n%s", outb)
	return nil
}

func report(t *testing.T, res map[string]string) Report {
	t.Helper()
	var r Report
	if err := json.Unmarshal([]byte(res["report"]), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

func contains(list []string, prefix string) bool {
	for _, s := range list {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// landlockABI returns the kernel's Landlock ABI version (0 = unavailable).
func landlockABI() int {
	v, _, e := syscall.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, 1 /* LANDLOCK_CREATE_RULESET_VERSION */)
	if e != 0 {
		return 0
	}
	return int(v)
}

func isRoot() bool { return os.Geteuid() == 0 }

func capEff(t *testing.T, res map[string]string) uint64 {
	t.Helper()
	v, err := strconv.ParseUint(res["CapEff"], 16, 64)
	if err != nil {
		t.Fatalf("CapEff %q: %v", res["CapEff"], err)
	}
	return v
}

func TestSupported(t *testing.T) {
	if !Supported() {
		t.Error("Supported() must be true on linux")
	}
}

func TestApplyNothing(t *testing.T) {
	// No options: nothing is touched, safe to run in-process.
	r, err := Apply(Options{})
	if err != nil || len(r.Applied) != 0 || len(r.Skipped) != 0 {
		t.Errorf("Apply(empty) = %+v, %v", r, err)
	}
}

func TestExisting(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	in := []string{dir, file, filepath.Join(dir, "missing")}
	if got := existing(in, true); len(got) != 1 || got[0] != dir {
		t.Errorf("dirs = %v", got)
	}
	if got := existing(in, false); len(got) != 1 || got[0] != file {
		t.Errorf("files = %v", got)
	}
	if got := existing(nil, true); got != nil {
		t.Errorf("nil = %v", got)
	}
}

func TestKnownSyscalls(t *testing.T) {
	got := knownSyscalls([]string{"ptrace", "definitely_not_a_syscall", "unshare"})
	if len(got) != 2 || got[0] != "ptrace" || got[1] != "unshare" {
		t.Errorf("knownSyscalls = %v", got)
	}
	all := knownSyscalls(append(append([]string{}, DeniedSyscalls...), NetworkSyscalls...))
	if len(all) < len(DeniedSyscalls)/2 {
		t.Errorf("too few known syscalls: %d", len(all))
	}
}

func TestAllPrctlRead(t *testing.T) {
	// PR_SET_KEEPCAPS with the current value is harmless.
	v, err := unix.PrctlRetInt(unix.PR_GET_KEEPCAPS, 0, 0, 0, 0)
	if err != nil {
		t.Skip(err)
	}
	if err := allPrctl(unix.PR_SET_KEEPCAPS, uintptr(v), 0); err != nil {
		t.Errorf("allPrctl: %v", err)
	}
	if err := allPrctl(-1, 0, 0); err == nil {
		t.Error("invalid prctl must fail")
	}
}

func TestChildNoNewPrivsOnly(t *testing.T) {
	res := runChild(t, childSpec{Opts: Options{NoNewPrivs: true}})
	if res["apply_err"] != "" {
		t.Fatal(res["apply_err"])
	}
	if res["NoNewPrivs"] != "1" || res["Seccomp"] != "0" {
		t.Errorf("status: NoNewPrivs=%s Seccomp=%s", res["NoNewPrivs"], res["Seccomp"])
	}
	if r := report(t, res); len(r.Applied) != 1 || r.Applied[0] != "no_new_privs" {
		t.Errorf("report %+v", r)
	}
}

func TestChildSeccomp(t *testing.T) {
	res := runChild(t, childSpec{Opts: Options{Seccomp: true}})
	r := report(t, res)
	if res["NoNewPrivs"] != "1" {
		t.Errorf("NoNewPrivs = %s", res["NoNewPrivs"])
	}
	if !contains(r.Applied, "seccomp") {
		t.Skipf("seccomp not applied: %+v", r)
	}
	if res["Seccomp"] != "2" {
		t.Errorf("Seccomp = %s", res["Seccomp"])
	}
	eperm := syscall.EPERM.Error()
	if res["unshare"] != eperm || res["ptrace"] != eperm {
		t.Errorf("denied syscalls: unshare=%q ptrace=%q", res["unshare"], res["ptrace"])
	}
	if res["socket"] != "" {
		t.Errorf("socket must be allowed without DenyNetwork: %q", res["socket"])
	}
}

func TestChildSeccompDenyNetwork(t *testing.T) {
	res := runChild(t, childSpec{Opts: Options{Seccomp: true, DenyNetwork: true}})
	if !contains(report(t, res).Applied, "seccomp") {
		t.Skip("seccomp unavailable")
	}
	if res["socket"] != syscall.EPERM.Error() {
		t.Errorf("socket = %q, want EPERM", res["socket"])
	}
}

func TestChildDropCapabilities(t *testing.T) {
	res := runChild(t, childSpec{Opts: Options{DropCapabilities: true, KeepCapabilities: []int{unix.CAP_NET_BIND_SERVICE, 99, -1}}})
	r := report(t, res)
	if !contains(r.Applied, "capabilities") {
		t.Fatalf("capabilities not applied: %+v", r)
	}
	eff := capEff(t, res)
	if eff&^(1<<unix.CAP_NET_BIND_SERVICE) != 0 {
		t.Errorf("CapEff = %x, only NET_BIND_SERVICE may remain", eff)
	}
	if isRoot() {
		if eff != 1<<unix.CAP_NET_BIND_SERVICE {
			t.Errorf("root: CapEff = %x, want NET_BIND_SERVICE kept", eff)
		}
		bnd, _ := strconv.ParseUint(res["CapBnd"], 16, 64)
		if bnd != 1<<unix.CAP_NET_BIND_SERVICE {
			t.Errorf("CapBnd = %x", bnd)
		}
		// Without CAP_SYS_ADMIN unshare fails even though seccomp is off.
		if res["unshare"] == "" {
			t.Error("unshare succeeded after dropping capabilities")
		}
	}
}

func TestChildLandlock(t *testing.T) {
	rw := t.TempDir()
	outside := t.TempDir()
	res := runChild(t, childSpec{
		Opts: Options{
			Landlock:       true,
			ReadOnlyPaths:  []string{"/etc/hostname", filepath.Join(outside, "missing")},
			ReadWritePaths: []string{rw},
			ExecPaths:      []string{"/usr", "/bin"},
		},
		Inside:  filepath.Join(rw, "in.txt"),
		Outside: filepath.Join(outside, "out.txt"),
	})
	r := report(t, res)
	if res["apply_err"] != "" || !contains(r.Applied, "landlock") {
		t.Fatalf("landlock: err=%q report=%+v", res["apply_err"], r)
	}
	if res["NoNewPrivs"] != "1" {
		t.Errorf("landlock implies no_new_privs")
	}
	if res["write_inside"] != "" {
		t.Errorf("write inside RW dir failed: %s", res["write_inside"])
	}
	if landlockABI() < 1 {
		t.Skip("kernel has no Landlock: cannot verify denial")
	}
	if res["write_outside"] == "" {
		t.Error("write outside allowed paths succeeded")
	}
	if _, err := os.Stat(filepath.Join(outside, "out.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("outside file exists: %v", err)
	}
}

func TestChildFullSandbox(t *testing.T) {
	rw := t.TempDir()
	res := runChild(t, childSpec{
		Opts: Options{
			NoNewPrivs: true, DropCapabilities: true, Landlock: true, Seccomp: true, DenyNetwork: true,
			ReadWritePaths: []string{rw},
		},
		Limits: &Limits{CPUSeconds: 30, FileSize: 1 << 20, OpenFiles: 64},
		Inside: filepath.Join(rw, "x"),
	})
	if res["limits_err"] != "" || res["nofile"] != "64" || res["core"] != "0" {
		t.Errorf("limits: err=%q nofile=%s core=%s", res["limits_err"], res["nofile"], res["core"])
	}
	r := report(t, res)
	if res["apply_err"] != "" || !contains(r.Applied, "no_new_privs") {
		t.Fatalf("apply: %q %+v", res["apply_err"], r)
	}
	for _, want := range []string{"capabilities", "landlock", "seccomp"} {
		if !contains(r.Applied, want) && !contains(r.Skipped, want) {
			t.Errorf("%s neither applied nor skipped: %+v", want, r)
		}
	}
	if contains(r.Applied, "seccomp") && res["socket"] != syscall.EPERM.Error() {
		t.Errorf("socket = %q", res["socket"])
	}
	if res["write_inside"] != "" {
		t.Errorf("write inside: %s", res["write_inside"])
	}
}

func TestChildResourceLimitsZero(t *testing.T) {
	// Zero values leave limits unchanged except RLIMIT_CORE.
	var before unix.Rlimit
	_ = unix.Getrlimit(unix.RLIMIT_NOFILE, &before)
	res := runChild(t, childSpec{Limits: &Limits{}})
	if res["limits_err"] != "" || res["core"] != "0" {
		t.Errorf("limits: %q core=%s", res["limits_err"], res["core"])
	}
	if res["nofile"] != strconv.FormatUint(before.Cur, 10) {
		t.Errorf("nofile changed: %s != %d", res["nofile"], before.Cur)
	}
}
