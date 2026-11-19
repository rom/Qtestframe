//go:build !linux

package hardening

// Supported reports whether in-process hardening is available.
func Supported() bool { return false }

// Apply is a no-op outside Linux.
func Apply(o Options) (Report, error) {
	var r Report
	for _, s := range []string{"no_new_privs", "capabilities", "landlock", "seccomp"} {
		r.skip(s + ": not supported on this platform")
	}
	return r, nil
}

// SetResourceLimits is a no-op outside Linux.
func SetResourceLimits(Limits) error { return nil }
