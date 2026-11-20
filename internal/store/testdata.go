package store

import (
	"encoding/json"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Limits of the test data of one test.
const (
	MaxParameters     = 50
	MaxDataRows       = 500
	MaxParameterName  = 50
	MaxDataRowName    = 200
	MaxDataValueChars = 2000
)

// paramName: a letter or underscore, then letters, digits, _ . or -.
var paramName = regexp.MustCompile(`^[\p{L}_][\p{L}\p{N}_.\-]*$`)

// placeholder matches {{name}} (spaces inside the braces are allowed).
var placeholder = regexp.MustCompile(`\{\{\s*([\p{L}_][\p{L}\p{N}_.\-]*)\s*\}\}`)

// ValidParameterName reports whether s can name a test parameter.
func ValidParameterName(s string) bool {
	return paramName.MatchString(s) && utf8.RuneCountInString(s) <= MaxParameterName
}

// ResolvePlaceholders replaces {{name}} with values[name]. Placeholders of
// unknown parameters are left as they are.
func ResolvePlaceholders(text string, values map[string]string) string {
	if len(values) == 0 || !strings.Contains(text, "{{") {
		return text
	}
	return placeholder.ReplaceAllStringFunc(text, func(m string) string {
		name := placeholder.FindStringSubmatch(m)[1]
		if v, ok := values[name]; ok {
			return v
		}
		return m
	})
}

// Placeholders lists the parameter names referenced in text (in order of
// first use, without duplicates).
func Placeholders(text string) []string {
	var out []string
	seen := map[string]bool{}
	for _, m := range placeholder.FindAllStringSubmatch(text, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

// ValidateTestData normalises and checks parameters and data rows: names
// are trimmed and must be unique (ignoring case), values of unknown
// parameters are dropped and missing values become "".
func ValidateTestData(t *model.TestCase) error {
	if t.Parameters == nil {
		t.Parameters = []string{}
	}
	if t.DataRows == nil {
		t.DataRows = []model.DataRow{}
	}
	if len(t.Parameters) > MaxParameters {
		return Invalidf("a test can have at most %d parameters", MaxParameters)
	}
	if len(t.DataRows) > MaxDataRows {
		return Invalidf("a test can have at most %d data rows", MaxDataRows)
	}
	seen := map[string]bool{}
	params := make([]string, 0, len(t.Parameters))
	for _, p := range t.Parameters {
		p = strings.TrimSpace(p)
		if !ValidParameterName(p) {
			return Invalidf("invalid parameter name %q (letters, digits, _ . -; at most %d characters, not starting with a digit)", p, MaxParameterName)
		}
		if seen[strings.ToLower(p)] {
			return Invalidf("duplicate parameter %q", p)
		}
		seen[strings.ToLower(p)] = true
		params = append(params, p)
	}
	t.Parameters = params
	if len(params) == 0 && len(t.DataRows) > 0 {
		return Invalidf("data rows need at least one parameter")
	}
	for i := range t.DataRows {
		r := &t.DataRows[i]
		r.Name = strings.TrimSpace(r.Name)
		if utf8.RuneCountInString(r.Name) > MaxDataRowName {
			return Invalidf("name of data row %d is longer than %d characters", i+1, MaxDataRowName)
		}
		vals := make(map[string]string, len(params))
		for _, p := range params {
			v := r.Values[p]
			if utf8.RuneCountInString(v) > MaxDataValueChars {
				return Invalidf("value of %s in data row %d is longer than %d characters", p, i+1, MaxDataValueChars)
			}
			vals[p] = v
		}
		r.Values = vals
	}
	return nil
}

func encodeJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func decodeParams(s string) []string {
	out := []string{}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []string{}
	}
	return out
}

func decodeRows(s string) []model.DataRow {
	out := []model.DataRow{}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = []model.DataRow{}
	}
	for i := range out {
		if out[i].Values == nil {
			out[i].Values = map[string]string{}
		}
	}
	return out
}

func decodeValues(s string) map[string]string {
	out := map[string]string{}
	_ = json.Unmarshal([]byte(s), &out)
	if out == nil {
		out = map[string]string{}
	}
	return out
}
