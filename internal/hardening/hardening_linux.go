//go:build linux

package hardening

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"unsafe"

	seccomp "github.com/elastic/go-seccomp-bpf"
	"github.com/landlock-lsm/go-landlock/landlock"
	"golang.org/x/sys/unix"
)

// Supported reports whether in-process hardening is available.
func Supported() bool { return true }

// Apply applies the requested restrictions. Errors are returned only for
// failures that leave the process in an unexpected state; unsupported
// kernel features are recorded as skipped.
func Apply(o Options) (Report, error) {
	var r Report
	var errs []error
	if o.NoNewPrivs || o.Seccomp || o.Landlock {
		if err := allPrctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0); err != nil {
			errs = append(errs, fmt.Errorf("no_new_privs: %w", err))
		} else {
			r.ok("no_new_privs")
		}
	}
	if o.DropCapabilities {
		if err := dropCaps(o.KeepCapabilities); err != nil {
			r.skip("capabilities: " + err.Error())
		} else {
			r.ok("capabilities")
		}
	}
	if o.Landlock {
		if err := applyLandlock(o); err != nil {
			r.skip("landlock: " + err.Error())
		} else {
			r.ok("landlock")
		}
	}
	if o.Seccomp {
		if err := applySeccomp(o.DenyNetwork); err != nil {
			r.skip("seccomp: " + err.Error())
		} else {
			r.ok("seccomp")
		}
	}
	return r, errors.Join(errs...)
}

func dropCaps(keep []int) error {
	k := map[int]bool{}
	for _, c := range keep {
		k[c] = true
	}
	last := unix.CAP_LAST_CAP
	if b, err := os.ReadFile("/proc/sys/kernel/cap_last_cap"); err == nil {
		var n int
		if _, err := fmt.Sscanf(string(b), "%d", &n); err == nil {
			last = n
		}
	}
	// Dropping from the bounding set requires CAP_SETPCAP; when the
	// process has no capabilities at all there is nothing to drop.
	hdr := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	var data [2]unix.CapUserData
	if err := unix.Capget(&hdr, &data[0]); err != nil {
		return err
	}
	hasSetPCap := data[0].Effective&(1<<unix.CAP_SETPCAP) != 0
	if hasSetPCap {
		for c := 0; c <= last; c++ {
			if k[c] {
				continue
			}
			if err := allPrctl(unix.PR_CAPBSET_DROP, uintptr(c), 0); err != nil && !errors.Is(err, unix.EINVAL) {
				return fmt.Errorf("bounding set drop %d: %w", c, err)
			}
		}
	}
	var mask [2]uint32
	for c := range k {
		if c >= 0 && c < 64 {
			mask[c/32] |= 1 << (uint(c) % 32)
		}
	}
	for i := range data {
		data[i].Effective &= mask[i]
		data[i].Permitted &= mask[i]
		data[i].Inheritable &= mask[i]
	}
	// Capabilities are per thread: apply to every OS thread of the runtime.
	_, _, e := syscall.AllThreadsSyscall(unix.SYS_CAPSET, uintptr(unsafe.Pointer(&hdr)), uintptr(unsafe.Pointer(&data[0])), 0)
	if e == syscall.ENOTSUP {
		return errors.New("capset on all threads requires a CGO_ENABLED=0 build")
	}
	if e != 0 {
		return fmt.Errorf("capset: %w", e)
	}
	// Ambient capabilities would survive execve of the runner.
	_ = allPrctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0)
	return nil
}

// allPrctl performs prctl on all threads of the process.
func allPrctl(op int, a, b uintptr) error {
	_, _, e := syscall.AllThreadsSyscall(unix.SYS_PRCTL, uintptr(op), a, b)
	if e == syscall.ENOTSUP {
		// cgo builds cannot use AllThreadsSyscall; fall back to the
		// current thread (release builds use CGO_ENABLED=0).
		return unix.Prctl(op, a, b, 0, 0)
	}
	if e != 0 {
		return e
	}
	return nil
}

func applyLandlock(o Options) error {
	ro := append(append([]string{}, DefaultReadOnly...), o.ReadOnlyPaths...)
	rules := []landlock.Rule{
		landlock.RODirs(existing(ro, true)...).IgnoreIfMissing(),
		landlock.ROFiles(existing(ro, false)...).IgnoreIfMissing(),
		landlock.RWDirs(existing(o.ReadWritePaths, true)...).IgnoreIfMissing(),
		landlock.RWFiles(existing(o.ReadWritePaths, false)...).IgnoreIfMissing(),
		landlock.RWFiles("/dev/null", "/dev/urandom", "/dev/random", "/dev/tty").IgnoreIfMissing(),
	}
	if len(o.ExecPaths) > 0 {
		rules = append(rules, landlock.RODirs(existing(o.ExecPaths, true)...).IgnoreIfMissing())
	}
	return landlock.V5.BestEffort().RestrictPaths(rules...)
}

// existing filters paths to existing directories (dirs=true) or files.
func existing(paths []string, dirs bool) []string {
	var out []string
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() == dirs {
			out = append(out, p)
		}
	}
	return out
}

func applySeccomp(denyNet bool) error {
	if !seccomp.Supported() {
		return errors.New("kernel does not support seccomp filters")
	}
	names := append([]string{}, DeniedSyscalls...)
	if denyNet {
		names = append(names, NetworkSyscalls...)
	}
	names = knownSyscalls(names)
	f := seccomp.Filter{
		NoNewPrivs: true,
		Flag:       seccomp.FilterFlagTSync,
		Policy: seccomp.Policy{
			DefaultAction: seccomp.ActionAllow,
			Syscalls:      []seccomp.SyscallGroup{{Action: seccomp.ActionErrno, Names: names}},
		},
	}
	return seccomp.LoadFilter(f)
}

// knownSyscalls drops names unknown on this architecture.
func knownSyscalls(names []string) []string {
	var out []string
	for _, n := range names {
		p := seccomp.Policy{DefaultAction: seccomp.ActionAllow, Syscalls: []seccomp.SyscallGroup{{Action: seccomp.ActionErrno, Names: []string{n}}}}
		if _, err := p.Assemble(); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// SetResourceLimits applies rlimits to the current process (inherited by
// children across execve).
func SetResourceLimits(l Limits) error {
	set := func(res int, v uint64) error {
		if v == 0 {
			return nil
		}
		return unix.Setrlimit(res, &unix.Rlimit{Cur: v, Max: v})
	}
	return errors.Join(
		set(unix.RLIMIT_CPU, l.CPUSeconds),
		set(unix.RLIMIT_FSIZE, l.FileSize),
		set(unix.RLIMIT_NOFILE, l.OpenFiles),
		set(unix.RLIMIT_AS, l.AddressSpace),
		unix.Setrlimit(unix.RLIMIT_CORE, &unix.Rlimit{}),
	)
}
