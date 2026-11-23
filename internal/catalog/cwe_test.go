package catalog

import "testing"

func TestCWEs(t *testing.T) {
	c, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	m := c.CWEs()
	if len(m) < 20 {
		t.Fatalf("only %d CWE ids", len(m))
	}
	if m["CWE-434"] == "" {
		t.Fatalf("CWE-434: %q", m["CWE-434"])
	}
	if _, ok := m["CWE-999999"]; ok {
		t.Fatal("unknown id")
	}
}
