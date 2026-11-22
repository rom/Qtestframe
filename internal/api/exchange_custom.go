package api

import (
	"context"
	"strings"
	"sync"

	"github.com/rom/qtestframe/internal/customfield"
	"github.com/rom/qtestframe/internal/interop"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

// The project's custom fields take part in the exchange (Excel round-trip
// and ReqIF import) as fields "cf:<key>": a select is an enumeration of
// its options, the ReqIF attribute mapping matches them by label like the
// built-in fields. The extension finds the store in the context
// (loadExchange puts it there), so one registration serves every server.

type exchangeStoreKey struct{}

var registerCustomOnce sync.Once

func registerCustomFieldExchange() {
	registerCustomOnce.Do(func() {
		interop.Register(interop.Extension{Name: "custom fields", Fields: customExchangeFields})
	})
}

func withExchangeStore(ctx context.Context, st *store.Store) context.Context {
	return context.WithValue(ctx, exchangeStoreKey{}, st)
}

func customExchangeFields(ctx context.Context, tenantID, projectID, kind string) ([]interop.Field, error) {
	st, _ := ctx.Value(exchangeStoreKey{}).(*store.Store)
	if st == nil {
		return nil, nil
	}
	defs, err := st.ListCustomFields(ctx, tenantID, projectID, kind)
	if err != nil {
		return nil, err
	}
	var out []interop.Field
	for i := range defs {
		out = append(out, customExchangeField(defs[i]))
	}
	return out, nil
}

func customValues(it *interop.Item) model.CustomValues {
	if it.Req != nil {
		return it.Req.CustomFields
	}
	return it.Test.CustomFields
}

func customExchangeField(def model.CustomField) interop.Field {
	f := &def
	fd := interop.Field{Key: "cf:" + def.Key, Label: customfield.Column(f), Aliases: []string{def.Label, def.Key}, Type: interop.Text}
	switch def.Type {
	case model.FieldSingleSelect:
		fd.Type, fd.Values = interop.Enum, def.Options
	case model.FieldMultiSelect:
		fd.Type, fd.Values = interop.List, def.Options
	case "textarea", "long_text", "markdown":
		fd.Type = interop.LongText
	}
	fd.Get = func(it *interop.Item) string { return customfield.Plain(customValues(it)[def.Key]) }
	parse := func(raw string) (any, error) {
		raw = strings.TrimSpace(raw)
		if def.Type == model.FieldDate && len(raw) > 10 && raw[4] == '-' && raw[10] == 'T' {
			raw = raw[:10] // a ReqIF DATE
		}
		if def.Type == model.FieldMultiSelect {
			raw = strings.Join(interop.SplitList(raw), ", ")
		}
		if (def.Type == model.FieldSingleSelect || def.Type == model.FieldMultiSelect) && raw != "" {
			// options match case-insensitively
			var vals []string
			for _, v := range interop.SplitList(raw) {
				for _, o := range def.Options {
					if strings.EqualFold(o, v) {
						v = o
					}
				}
				vals = append(vals, v)
			}
			if def.Type == model.FieldSingleSelect {
				raw = strings.Join(vals, " ")
			} else {
				raw = strings.Join(vals, ", ")
			}
		}
		return customfield.Parse(f, raw)
	}
	fd.Parse = func(raw string) (string, error) {
		v, err := parse(raw)
		if err != nil {
			return "", err
		}
		return customfield.Plain(v), nil
	}
	fd.Put = func(body map[string]any, value string, _ *interop.Item) error {
		v, err := parse(value)
		if err != nil {
			return err
		}
		cf, _ := body["custom_fields"].(map[string]any)
		if cf == nil {
			cf = map[string]any{}
			body["custom_fields"] = cf
		}
		cf[def.Key] = v
		return nil
	}
	return fd
}
