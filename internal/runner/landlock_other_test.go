//go:build !linux

package runner

func landlockAvailable() bool { return false }
