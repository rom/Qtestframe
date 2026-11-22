// Package customfield implements the rules of custom field definitions and
// values: checking a definition, normalising and validating the values of
// an item against the definitions, the text form used by exports, reports
// and the query language, and parsing that text back (imports).
//
// Values are stored per item as one JSON object (field key → value):
//
//	text, long_text, date, single_select, user, url  string
//	number                                           float64
//	checkbox                                         bool
//	multi_select                                     []string
//
// Dates are YYYY-MM-DD, users are e-mail addresses of members of the
// organisation, URLs are absolute http(s) URLs. An empty value (null, "",
// [] or false for a checkbox that is not required) is not stored.
package customfield

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rom/qtestframe/internal/model"
)

// Limits.
const (
	MaxFields      = 100     // definitions per project and item kind
	MaxOptions     = 200     // options of a select field
	MaxOptionLen   = 200     // characters of one option
	MaxLabelLen    = 100     // characters of a label
	MaxHelpLen     = 1000    // characters of the help text
	MaxUnitLen     = 20      // characters of a unit
	MaxTextLen     = 1000    // characters of a text value
	MaxLongTextLen = 100_000 // characters of a long text value
	MaxURLLen      = 2000    // characters of a URL value
)

var keyRe = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

// ErrInvalid wraps every validation error.
var ErrInvalid = errors.New("invalid")

func invalidf(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

// KeyFromLabel derives a key from a label ("Mass (kg)" → "mass_kg").
func KeyFromLabel(label string) string {
	var b strings.Builder
	under := false
	for _, r := range strings.ToLower(label) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			under = false
		case r == 'å' || r == 'ä':
			b.WriteRune('a')
			under = false
		case r == 'ö':
			b.WriteRune('o')
			under = false
		default:
			if !under && b.Len() > 0 {
				b.WriteByte('_')
				under = true
			}
		}
	}
	k := strings.Trim(b.String(), "_")
	if k == "" || k[0] < 'a' || k[0] > 'z' {
		k = "f_" + k
	}
	if len(k) > 40 {
		k = strings.TrimRight(k[:40], "_")
	}
	return k
}

// ValidateDefinition normalises and checks a field definition. The key is
// derived from the label when empty.
func ValidateDefinition(f *model.CustomField) error {
	f.Label = strings.TrimSpace(f.Label)
	if f.Label == "" || utf8.RuneCountInString(f.Label) > MaxLabelLen {
		return invalidf("label is required (at most %d characters)", MaxLabelLen)
	}
	f.Key = strings.TrimSpace(f.Key)
	if f.Key == "" {
		f.Key = KeyFromLabel(f.Label)
	}
	if !keyRe.MatchString(f.Key) {
		return invalidf("key must start with a-z and contain only a-z, 0-9 and _ (at most 40 characters)")
	}
	if !slices.Contains(model.CustomFieldEntities, f.EntityType) {
		return invalidf("entity_type must be one of %s", strings.Join(model.CustomFieldEntities, ", "))
	}
	if !slices.Contains(model.CustomFieldTypes, f.Type) {
		return invalidf("type must be one of %s", strings.Join(model.CustomFieldTypes, ", "))
	}
	f.Unit = strings.TrimSpace(f.Unit)
	if f.Type != model.FieldNumber {
		f.Unit = ""
	}
	if utf8.RuneCountInString(f.Unit) > MaxUnitLen {
		return invalidf("unit may have at most %d characters", MaxUnitLen)
	}
	f.Help = strings.TrimSpace(f.Help)
	if utf8.RuneCountInString(f.Help) > MaxHelpLen {
		return invalidf("help may have at most %d characters", MaxHelpLen)
	}
	if isSelect(f.Type) {
		seen := map[string]bool{}
		var opts []string
		for _, o := range f.Options {
			o = strings.TrimSpace(o)
			if o == "" || seen[strings.ToLower(o)] {
				continue
			}
			if utf8.RuneCountInString(o) > MaxOptionLen {
				return invalidf("an option may have at most %d characters", MaxOptionLen)
			}
			seen[strings.ToLower(o)] = true
			opts = append(opts, o)
		}
		if len(opts) == 0 {
			return invalidf("a select field needs at least one option")
		}
		if len(opts) > MaxOptions {
			return invalidf("at most %d options", MaxOptions)
		}
		f.Options = opts
	} else {
		f.Options = []string{}
	}
	if f.Default != nil {
		v, err := Normalize(f, f.Default)
		if err != nil {
			return invalidf("default: %v", strings.TrimPrefix(err.Error(), ErrInvalid.Error()+": "))
		}
		if f.Type == model.FieldUser && v != nil {
			return invalidf("a user field has no default")
		}
		f.Default = v
	}
	return nil
}

func isSelect(t string) bool { return t == model.FieldSingleSelect || t == model.FieldMultiSelect }

// Normalize converts a value to the stored form of the field's type and
// checks it (without the membership check of user fields). nil means empty.
func Normalize(f *model.CustomField, v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch f.Type {
	case model.FieldText, model.FieldLongText:
		s, ok := v.(string)
		if !ok {
			return nil, invalidf("%s: text expected", f.Key)
		}
		if f.Type == model.FieldText {
			s = strings.TrimSpace(s)
		}
		if strings.TrimSpace(s) == "" {
			return nil, nil
		}
		max := MaxTextLen
		if f.Type == model.FieldLongText {
			max = MaxLongTextLen
		}
		if utf8.RuneCountInString(s) > max {
			return nil, invalidf("%s: at most %d characters", f.Key, max)
		}
		return s, nil
	case model.FieldNumber:
		var n float64
		switch x := v.(type) {
		case float64:
			n = x
		case int:
			n = float64(x)
		case string:
			x = strings.TrimSpace(x)
			if x == "" {
				return nil, nil
			}
			p, err := parseNumber(x)
			if err != nil {
				return nil, invalidf("%s: %q is not a number", f.Key, x)
			}
			n = p
		default:
			return nil, invalidf("%s: number expected", f.Key)
		}
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, invalidf("%s: number expected", f.Key)
		}
		return n, nil
	case model.FieldDate:
		s, ok := v.(string)
		if !ok {
			return nil, invalidf("%s: date expected (YYYY-MM-DD)", f.Key)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		if len(s) > 10 {
			// accept a timestamp and keep its date
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				return t.UTC().Format(time.DateOnly), nil
			}
		}
		if _, err := time.Parse(time.DateOnly, s); err != nil {
			return nil, invalidf("%s: date expected (YYYY-MM-DD)", f.Key)
		}
		return s, nil
	case model.FieldSingleSelect:
		s, ok := v.(string)
		if !ok {
			return nil, invalidf("%s: one of the options expected", f.Key)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		o, ok := option(f, s)
		if !ok {
			return nil, invalidf("%s: %q is not an option (%s)", f.Key, s, strings.Join(f.Options, ", "))
		}
		return o, nil
	case model.FieldMultiSelect:
		var list []string
		switch x := v.(type) {
		case []string:
			list = x
		case []any:
			for _, e := range x {
				s, ok := e.(string)
				if !ok {
					return nil, invalidf("%s: list of options expected", f.Key)
				}
				list = append(list, s)
			}
		case string:
			list = SplitList(x)
		default:
			return nil, invalidf("%s: list of options expected", f.Key)
		}
		var out []string
		for _, s := range list {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			o, ok := option(f, s)
			if !ok {
				return nil, invalidf("%s: %q is not an option (%s)", f.Key, s, strings.Join(f.Options, ", "))
			}
			if !slices.Contains(out, o) {
				out = append(out, o)
			}
		}
		if len(out) == 0 {
			return nil, nil
		}
		// keep the order of the options
		sort.SliceStable(out, func(i, j int) bool {
			return slices.Index(f.Options, out[i]) < slices.Index(f.Options, out[j])
		})
		return out, nil
	case model.FieldCheckbox:
		switch x := v.(type) {
		case bool:
			return x, nil
		case string:
			b, ok := parseBool(x)
			if !ok {
				return nil, invalidf("%s: true or false expected", f.Key)
			}
			return b, nil
		}
		return nil, invalidf("%s: true or false expected", f.Key)
	case model.FieldUser:
		s, ok := v.(string)
		if !ok {
			return nil, invalidf("%s: e-mail address expected", f.Key)
		}
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return nil, nil
		}
		if !strings.Contains(s, "@") || len(s) > 320 || strings.ContainsAny(s, " \t\n") {
			return nil, invalidf("%s: e-mail address expected", f.Key)
		}
		return s, nil
	case model.FieldURL:
		s, ok := v.(string)
		if !ok {
			return nil, invalidf("%s: URL expected", f.Key)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		u, err := url.Parse(s)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(s) > MaxURLLen {
			return nil, invalidf("%s: absolute http(s) URL expected", f.Key)
		}
		return s, nil
	}
	return nil, invalidf("%s: unknown type %q", f.Key, f.Type)
}

func option(f *model.CustomField, s string) (string, bool) {
	for _, o := range f.Options {
		if strings.EqualFold(o, s) {
			return o, true
		}
	}
	return "", false
}

// parseNumber accepts "2.5", "2,5" and "1 200".
func parseNumber(s string) (float64, error) {
	s = strings.ReplaceAll(strings.ReplaceAll(s, " ", ""), " ", "")
	if strings.Count(s, ",") == 1 && !strings.Contains(s, ".") {
		s = strings.Replace(s, ",", ".", 1)
	}
	return strconv.ParseFloat(s, 64)
}

func parseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "yes", "y", "1", "x", "ja", "j", "sant", "✓":
		return true, true
	case "false", "no", "n", "0", "", "nej", "falskt":
		return false, true
	}
	return false, false
}

// SplitList splits the text form of a multi-select value (comma or
// semicolon separated, or one per line).
func SplitList(s string) []string {
	f := func(r rune) bool { return r == ',' || r == ';' || r == '\n' }
	var out []string
	for _, p := range strings.FieldsFunc(s, f) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Options of Validate.
type Options struct {
	// Create applies defaults and requires every required field; on
	// update a required field may only stay empty when it was empty.
	Create bool
	// Lenient skips the required check of a new item (items created
	// automatically, such as runs of a result import, get the defaults).
	Lenient bool
	// Old are the stored values (update).
	Old model.CustomValues
	// IsUser reports whether an e-mail address belongs to a member of the
	// organisation (nil: not checked).
	IsUser func(email string) bool
}

// Validate normalises the values of an item against the definitions of its
// project and kind. Unknown keys are an error unless their value is
// unchanged from Old (a value kept from before the item moved). The result
// holds only non-empty values (nil when there are none).
func Validate(defs []model.CustomField, values model.CustomValues, o Options) (model.CustomValues, error) {
	byKey := make(map[string]*model.CustomField, len(defs))
	for i := range defs {
		byKey[defs[i].Key] = &defs[i]
	}
	out := model.CustomValues{}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := values[k]
		f, ok := byKey[k]
		if !ok {
			if v == nil {
				continue
			}
			if old, had := o.Old[k]; had && Equal(old, v) {
				out[k] = old
				continue
			}
			return nil, invalidf("unknown custom field %q", k)
		}
		n, err := Normalize(f, v)
		if err != nil {
			// a stored value that no longer fits the definition (an option
			// was removed) is kept until it is changed
			if old, had := o.Old[k]; had && Equal(old, v) {
				out[k] = old
				continue
			}
			return nil, err
		}
		if n == nil {
			continue
		}
		if b, isBool := n.(bool); isBool && !b && !f.Required {
			continue // an unticked optional checkbox is empty
		}
		if f.Type == model.FieldUser && o.IsUser != nil {
			if old, had := o.Old[k]; !(had && Equal(old, n)) && !o.IsUser(n.(string)) {
				return nil, invalidf("%s: %s is not a member of the organisation", k, n)
			}
		}
		out[k] = n
	}
	for i := range defs {
		f := &defs[i]
		if _, ok := out[f.Key]; ok {
			continue
		}
		_, inInput := values[f.Key]
		if o.Create && !inInput && f.Default != nil {
			if d, err := Normalize(f, f.Default); err == nil && d != nil {
				out[f.Key] = d
				continue
			}
		}
		if !f.Required {
			continue
		}
		_, hadValue := o.Old[f.Key]
		if (o.Create && !o.Lenient) || hadValue {
			return nil, invalidf("%s (%s) is required", f.Label, f.Key)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// Equal compares two values in any of their forms ([]string and []any).
func Equal(a, b any) bool {
	return fmt.Sprint(Strings(a)) == fmt.Sprint(Strings(b)) && kind(a) == kind(b)
}

func kind(v any) string {
	switch v.(type) {
	case []any, []string:
		return "list"
	case float64, int:
		return "number"
	case bool:
		return "bool"
	case nil:
		return "nil"
	}
	return "string"
}

// Strings returns the query form of a value: numbers without trailing
// zeros, true/false, one string per selected option.
func Strings(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		return []string{x}
	case float64:
		return []string{strconv.FormatFloat(x, 'f', -1, 64)}
	case int:
		return []string{strconv.Itoa(x)}
	case bool:
		return []string{strconv.FormatBool(x)}
	case []string:
		return x
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, Strings(e)...)
		}
		return out
	}
	return []string{fmt.Sprint(v)}
}

// Text returns the display form of a value used by exports, reports and
// documents: numbers with the unit, Yes/No (Ja/Nej in Swedish), the
// options comma separated.
func Text(f *model.CustomField, v any, lang string) string {
	if v == nil {
		return ""
	}
	switch f.Type {
	case model.FieldCheckbox:
		b, _ := v.(bool)
		if lang == "sv" {
			if b {
				return "Ja"
			}
			return "Nej"
		}
		if b {
			return "Yes"
		}
		return "No"
	case model.FieldNumber:
		s := strings.Join(Strings(v), "")
		if f.Unit != "" {
			s += " " + f.Unit
		}
		return s
	}
	return strings.Join(Strings(v), ", ")
}

// Plain returns the re-importable text form of a value (no unit, true/false).
func Plain(v any) string { return strings.Join(Strings(v), ", ") }

// Parse reads the text form of a value (a CSV or spreadsheet cell). The
// unit of a number field may follow the number.
func Parse(f *model.CustomField, s string) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	if f.Type == model.FieldNumber && f.Unit != "" {
		s = strings.TrimSpace(strings.TrimSuffix(s, f.Unit))
	}
	if f.Type == model.FieldCheckbox {
		if b, ok := parseBool(s); ok {
			return b, nil
		}
	}
	return Normalize(f, s)
}

// Column returns the export column header of a field: "Label" or "Label
// (unit)".
func Column(f *model.CustomField) string {
	if f.Unit != "" {
		return f.Label + " (" + f.Unit + ")"
	}
	return f.Label
}

// ForEntity returns the definitions of one item kind, in order.
func ForEntity(defs []model.CustomField, entity string) []model.CustomField {
	var out []model.CustomField
	for _, d := range defs {
		if d.EntityType == entity {
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Position < out[j].Position })
	return out
}
