package admindoc

import (
	"reflect"
	"strings"
	"time"
)

// Field is one key of the configuration file.
type Field struct {
	// Key is the dotted YAML key; keys of the elements of a list of
	// providers are written "auth.oidc[].name".
	Key string
	// Type is the value type as the reference shows it.
	Type string
	// Default is the built-in value (from config.Default()); invalid
	// inside lists, where each element starts empty.
	Default reflect.Value
}

var durationType = reflect.TypeOf(time.Duration(0))

// walk returns the YAML keys of the struct type t in declaration order; v
// is the default value (invalid inside lists).
func walk(t reflect.Type, v reflect.Value, prefix string) []Field {
	var out []Field
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name, opts, _ := strings.Cut(f.Tag.Get("yaml"), ",")
		if name == "-" || !f.IsExported() {
			continue
		}
		var fv reflect.Value
		if v.IsValid() {
			fv = v.Field(i)
		}
		if opts == "inline" {
			out = append(out, walk(f.Type, fv, prefix)...)
			continue
		}
		if name == "" {
			name = strings.ToLower(f.Name)
		}
		key := name
		if prefix != "" {
			key = prefix + "." + name
		}
		ft := f.Type
		switch {
		case ft.Kind() == reflect.Struct && ft != durationType:
			out = append(out, walk(ft, fv, key)...)
		case ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Struct:
			out = append(out, Field{Key: key, Type: "list", Default: fv})
			out = append(out, walk(ft.Elem(), reflect.Value{}, key+"[]")...)
		default:
			out = append(out, Field{Key: key, Type: typeName(ft), Default: fv})
		}
	}
	return out
}

func typeName(t reflect.Type) string {
	if t == durationType {
		return "duration"
	}
	switch t.Kind() {
	case reflect.Pointer:
		return typeName(t.Elem())
	case reflect.String:
		return "string"
	case reflect.Bool:
		return "bool"
	case reflect.Int, reflect.Int64, reflect.Uint, reflect.Uint64, reflect.Int32, reflect.Uint32:
		return "integer"
	case reflect.Slice:
		return "list"
	case reflect.Map:
		return "map"
	}
	return t.Kind().String()
}
