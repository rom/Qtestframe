package store

import (
	"context"
	"encoding/json"
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
	return out, rows.Err()
}

// SaveSystemSettings stores overrides (a nil value deletes the key's
// override) in one transaction.
func (s *Store) SaveSystemSettings(ctx context.Context, values map[string]json.RawMessage, by string) error {
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
