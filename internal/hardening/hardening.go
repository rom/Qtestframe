// Package hardening applies in-process Linux security restrictions:
// PR_SET_NO_NEW_PRIVS, capability dropping, Landlock filesystem rules and
// a seccomp-bpf deny-list. On other platforms all functions are no-ops
// that report what was skipped, so development on macOS works unchanged.
package hardening

// Options selects which restrictions to apply.
type Options struct {
	NoNewPrivs       bool
	DropCapabilities bool
	// KeepCapabilities lists capability numbers to keep (e.g. 10 = CAP_NET_BIND_SERVICE).
	KeepCapabilities []int
	Landlock         bool
	ReadOnlyPaths    []string
	ReadWritePaths   []string
	// ExecPaths are directories from which executables may be run (runner).
	ExecPaths []string
	Seccomp   bool
	// DenyNetwork additionally blocks socket creation (sandboxed scripts).
	DenyNetwork bool
}

// Report describes the outcome of Apply.
type Report struct {
	Applied []string `json:"applied"`
	Skipped []string `json:"skipped"`
}

func (r *Report) ok(s string)   { r.Applied = append(r.Applied, s) }
func (r *Report) skip(s string) { r.Skipped = append(r.Skipped, s) }

// DeniedSyscalls is the seccomp deny-list. These calls are never needed by
// the server and are frequently used in privilege escalation and
// container escapes. They fail with EPERM.
var DeniedSyscalls = []string{
	"acct", "add_key", "bpf", "chroot", "clock_adjtime", "clock_settime",
	"delete_module", "finit_module", "fsconfig", "fsmount", "fsopen", "fspick",
	"init_module", "iopl", "ioperm", "kexec_file_load", "kexec_load", "keyctl",
	"lookup_dcookie", "mount", "mount_setattr", "move_mount", "name_to_handle_at",
	"open_by_handle_at", "open_tree", "perf_event_open", "personality", "pivot_root",
	"process_vm_readv", "process_vm_writev", "ptrace", "quotactl", "reboot",
	"request_key", "setdomainname", "sethostname", "setns", "settimeofday",
	"swapoff", "swapon", "syslog", "umount2", "unshare", "userfaultfd", "vhangup",
}

// NetworkSyscalls are additionally denied when DenyNetwork is set.
var NetworkSyscalls = []string{"socket", "socketpair", "connect", "bind", "listen", "accept", "accept4"}

// DefaultReadOnly are paths the server needs to read after start-up
// (TLS roots for OIDC/SAML, DNS resolver configuration, time zones).
var DefaultReadOnly = []string{
	"/etc/ssl", "/etc/pki", "/etc/ca-certificates", "/usr/share/ca-certificates",
	"/etc/resolv.conf", "/etc/hosts", "/etc/nsswitch.conf", "/etc/localtime",
	"/etc/gai.conf", "/etc/services", "/etc/protocols", "/usr/share/zoneinfo",
	"/etc/qtestframe", "/proc/self",
}
