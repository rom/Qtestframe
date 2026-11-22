package main

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	var log bytes.Buffer
	if err := run([]string{"-root", "../../../..", "-check"}, &log); err != nil {
		t.Fatalf("%v\n%s", err, log.String())
	}
	if err := run([]string{"-root", t.TempDir(), "-check"}, &log); err == nil {
		t.Error("a folder without the guide was accepted")
	}
	if err := run([]string{"-nope"}, &log); err == nil {
		t.Error("unknown flag accepted")
	}
}
