package store

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// SystemSetting is one runtime override of a configuration key (see
// internal/settings); Value is JSON.
type SystemSetting struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedBy string          `json:"updated_by"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// SystemSettings returns all stored overrides by key.
func (s *Store) SystemSettings(ctx context.Context) (map[string]SystemSetting, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT key, value, updated_by, updated_at FROM system_settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]SystemSetting{}
	for rows.Next() {
		var x SystemSetting
		var v string
		if err := rows.Scan(&x.Key, &v, &x.UpdatedBy, &x.UpdatedAt); err != nil {
			return nil, err
		}
		x.Value, x.UpdatedAt = json.RawMessage(v), x.UpdatedAt.UTC()
		out[x.Key] = x
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	// secrets (the SMTP password, OIDC client secrets) are stored encrypted
	for k, x := range out {
		if _, ok := SecretSettings[k]; !ok {
			continue
		}
		v, err := transformSetting(k, x.Value, func(_, old, ad string) (string, error) { return s.unseal(ctx, old, ad) })
		if err != nil {
			return nil, fmt.Errorf("setting %s: %w", k, err)
		}
		x.Value = v
		out[k] = x
	}
	return out, nil
}

// SaveSystemSettings stores overrides (a nil value deletes the key's
// override) in one transaction.
func (s *Store) SaveSystemSettings(ctx context.Context, values map[string]json.RawMessage, by string) error {
	stored := make(map[string]json.RawMessage, len(values))
	for k, v := range values {
		if _, ok := SecretSettings[k]; ok && v != nil {
			var err error
			if v, err = transformSetting(k, v, func(_, old, ad string) (string, error) { return s.seal(old, ad) }); err != nil {
				return err
			}
		}
		stored[k] = v
	}
	values = stored
	return s.tx(ctx, func(c conn) error {
		for k, v := range values {
			if _, err := c.exec(ctx, `DELETE FROM system_settings WHERE key = ?`, k); err != nil {
				return err
			}
			if v == nil {
				continue
			}
			if _, err := c.exec(ctx, `INSERT INTO system_settings (key, value, updated_by, updated_at) VALUES (?, ?, ?, ?)`, k, string(v), by, s.now()); err != nil {
				return mapErr(err, "setting")
			}
		}
		return nil
	})
}
