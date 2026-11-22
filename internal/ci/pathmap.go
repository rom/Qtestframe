package ci

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// PathRule maps changed files to tests, tags and requirements: a file
// that matches one of Paths selects the tests (keys), the tests with one
// of the tags and the tests of the requirements (keys).
type PathRule struct {
	Paths        []string `yaml:"paths"`
	Tests        []string `yaml:"tests"`
	Tags         []string `yaml:"tags"`
	Requirements []string `yaml:"requirements"`
}

// PathMap is a list of rules, read from a YAML file in the repository
// (e.g. qtestframe-map.yml):
//
//   - paths: ["src/cart/**", "api/cart.go"]
//     tests: [SHOP-TC-3]
//     tags: [cart]
//     requirements: [SHOP-REQ-4]
type PathMap []PathRule

// ParsePathMap reads a path map.
func ParsePathMap(data []byte) (PathMap, error) {
	var m PathMap
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("path map: %w", err)
	}
	for i, r := range m {
		if len(r.Paths) == 0 {
			return nil, fmt.Errorf("path map: rule %d has no paths", i+1)
		}
		for _, p := range r.Paths {
			if _, err := path.Match(strings.ReplaceAll(p, "**", "*"), ""); err != nil {
				return nil, fmt.Errorf("path map: invalid pattern %q", p)
			}
		}
	}
	return m, nil
}

// MatchPath reports whether a slash-separated file path matches a pattern
// where * matches within a path segment and ** any number of segments; a
// pattern ending in / matches everything below it.
func MatchPath(pattern, file string) bool {
	pattern = strings.TrimPrefix(pattern, "./")
	file = strings.TrimPrefix(file, "./")
	if strings.HasSuffix(pattern, "/") {
		pattern += "**"
	}
	return matchSegs(strings.Split(pattern, "/"), strings.Split(file, "/"))
}

func matchSegs(pat, segs []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			if len(pat) == 1 {
				return true
			}
			for i := 0; i <= len(segs); i++ {
				if matchSegs(pat[1:], segs[i:]) {
					return true
				}
			}
			return false
		}
		if len(segs) == 0 {
			return false
		}
		if ok, _ := path.Match(pat[0], segs[0]); !ok {
			return false
		}
		pat, segs = pat[1:], segs[1:]
	}
	return len(segs) == 0
}

// Hit is what the path map selects for changed files.
type Hit struct {
	Tests, Tags, Requirements []string
	// Why: rule pattern → the first changed file it matched.
	Why map[string]string
}

// Match applies the map to changed files.
func (m PathMap) Match(files []string) Hit {
	h := Hit{Why: map[string]string{}}
	add := func(dst *[]string, v []string) {
		for _, x := range v {
			if x = strings.TrimSpace(x); x != "" && !contains(*dst, x) {
				*dst = append(*dst, x)
			}
		}
	}
	for _, r := range m {
		for _, p := range r.Paths {
			for _, f := range files {
				if MatchPath(p, f) {
					add(&h.Tests, r.Tests)
					add(&h.Tags, r.Tags)
					add(&h.Requirements, r.Requirements)
					if _, ok := h.Why[p]; !ok {
						h.Why[p] = f
					}
					break
				}
			}
		}
	}
	return h
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
