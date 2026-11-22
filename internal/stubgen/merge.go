package stubgen

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Action is what Merge did with an existing file.
type Action string

const (
	// Created: the file did not exist.
	Created Action = "created"
	// Regenerated: the body was still the generated stub, so the whole
	// file was replaced by the new stub.
	Regenerated Action = "regenerated"
	// HeaderUpdated: the body was changed (implemented); only the header
	// block was rewritten.
	HeaderUpdated Action = "header"
	// Unchanged: the file already is what would be written.
	Unchanged Action = "unchanged"
	// Kept: the file has no qtestframe markers (or those of another
	// test); it is left alone.
	Kept Action = "kept"
)

// bodyHash is the hash recorded in the header: of the body with line ends,
// trailing spaces and surrounding blank lines normalised.
func bodyHash(body string) string {
	sum := sha256.Sum256([]byte(normalise(body)))
	return hex.EncodeToString(sum[:8])
}

func normalise(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t\r")
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// block is the position of the header block in a file.
type block struct {
	start, end int // byte offsets: begin line start, after the end line
	key, fw    string
	hash       string
}

// findBlock locates the header block ("" key when there is none).
func findBlock(s string) block {
	i := strings.Index(s, beginMarker)
	if i < 0 {
		return block{}
	}
	start := strings.LastIndexByte(s[:i], '\n') + 1
	lineEnd := strings.IndexByte(s[i:], '\n')
	if lineEnd < 0 {
		return block{}
	}
	fields := strings.Fields(s[i+len(beginMarker) : i+lineEnd])
	b := block{start: start}
	for _, f := range fields {
		switch {
		case strings.HasPrefix(f, "body="):
			b.hash = strings.TrimPrefix(f, "body=")
		case b.key == "":
			b.key = f
		case b.fw == "":
			b.fw = f
		}
	}
	j := strings.Index(s[i:], endMarker)
	if j < 0 || b.key == "" {
		return block{}
	}
	j += i
	e := strings.IndexByte(s[j:], '\n')
	if e < 0 {
		b.end = len(s)
	} else {
		b.end = j + e + 1
	}
	return b
}

// Body returns the part of a stub after its header block (the whole text
// when there is none).
func Body(s string) string {
	b := findBlock(s)
	if b.key == "" {
		return s
	}
	return s[b.end:]
}

// Merge combines an existing file with a newly generated stub (existing nil:
// there is no file). It returns the content to write and what was done;
// for Kept and Unchanged the content is the existing file.
func Merge(existing []byte, gen File) (string, Action) {
	if existing == nil {
		return gen.Content, Created
	}
	old := string(existing)
	if old == gen.Content {
		return old, Unchanged
	}
	b := findBlock(old)
	if b.key == "" || !strings.EqualFold(b.key, gen.Key) {
		return old, Kept
	}
	if b.hash != "" && bodyHash(old[b.end:]) == b.hash {
		return gen.Content, Regenerated
	}
	ng := findBlock(gen.Content)
	out := old[:b.start] + gen.Content[ng.start:ng.end] + old[b.end:]
	if out == old {
		return old, Unchanged
	}
	return out, HeaderUpdated
}

// ScriptRefPrefix starts the script of a test whose code is a generated
// stub in the repository (see ScriptRef).
const ScriptRefPrefix = "qtestframe:stub "

// ScriptRef is what the script field of a test says when it is
// implemented in the repository at path: the server does not execute it,
// and the results come from CI.
func ScriptRef(path string) string { return ScriptRefPrefix + path }

// ScriptRefPath returns the path of a ScriptRef script.
func ScriptRefPath(script string) (string, bool) {
	s := strings.TrimSpace(script)
	if !strings.HasPrefix(s, ScriptRefPrefix) || strings.Contains(s, "\n") {
		return "", false
	}
	return strings.TrimSpace(s[len(ScriptRefPrefix):]), true
}
