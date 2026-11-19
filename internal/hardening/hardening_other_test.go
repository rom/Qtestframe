//go:build !linux

package hardening

import "testing"

func TestStubs(t *testing.T) {
	if Supported() {
		t.Error("Supported() must be false")
	}
	r, err := Apply(Options{NoNewPrivs: true, Seccomp: true})
	if err != nil || len(r.Applied) != 0 || len(r.Skipped) != 4 {
		t.Errorf("Apply = %+v, %v", r, err)
	}
	if err := SetResourceLimits(DefaultLimits); err != nil {
		t.Error(err)
	}
}
