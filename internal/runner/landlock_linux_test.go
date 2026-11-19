//go:build linux

package runner

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// landlockAvailable reports whether the kernel enforces Landlock.
func landlockAvailable() bool {
	v, _, e := syscall.Syscall(unix.SYS_LANDLOCK_CREATE_RULESET, 0, 0, 1)
	return e == 0 && v >= 1
}
