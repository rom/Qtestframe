package hardening

import (
	"testing"
)

func TestDenyLists(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range append(append([]string{}, DeniedSyscalls...), NetworkSyscalls...) {
		if s == "" || seen[s] {
			t.Errorf("empty or duplicate syscall %q", s)
		}
		seen[s] = true
	}
	for _, must := range []string{"ptrace", "unshare", "mount", "bpf", "setns"} {
		if !seen[must] {
			t.Errorf("%s must be denied", must)
		}
	}
	if len(DefaultReadOnly) == 0 || DefaultLimits.CPUSeconds == 0 || DefaultLimits.OpenFiles == 0 || DefaultLimits.AddressSpace != 0 {
		t.Errorf("unexpected defaults: %v %+v", DefaultReadOnly, DefaultLimits)
	}
}

func TestReport(t *testing.T) {
	var r Report
	r.ok("a")
	r.skip("b")
	r.ok("c")
	if len(r.Applied) != 2 || r.Applied[1] != "c" || len(r.Skipped) != 1 || r.Skipped[0] != "b" {
		t.Errorf("%+v", r)
	}
}
