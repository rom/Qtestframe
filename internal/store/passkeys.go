package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/model"
)

// Passkeys (WebAuthn credentials) and the challenges of their ceremonies
// (see internal/api/passkeys.go).

// MaxPasskeysPerUser bounds the passkeys of one account.
const MaxPasskeysPerUser = 25

const passkeyCols = `id, user_id, credential_id, public_key, alg, sign_count, aaguid, transports, attestation, name,
	user_verified, backup_eligible, backed_up, created_at, last_used_at`

func scanPasskey(sc interface{ Scan(...any) error }) (*model.Passkey, error) {
	var p model.Passkey
	var transports string
	var count int64
	var used sql.NullTime
	if err := sc.Scan(&p.ID, &p.UserID, &p.CredentialID, &p.PublicKey, &p.Alg, &count, &p.AAGUID, &transports, &p.Attestation, &p.Name,
		&p.UserVerified, &p.BackupEligible, &p.BackedUp, &p.CreatedAt, &used); err != nil {
		return nil, err
	}
	p.SignCount = uint32(count)
	p.LastUsedAt = timePtr(used)
	p.Transports = []string{}
	_ = json.Unmarshal([]byte(transports), &p.Transports)
	return &p, nil
}

// AddPasskey stores a new passkey of a user and lifts the enrolment
// requirement of the user's sessions (a passkey is a second factor). A
// credential id that is already registered is a conflict.
func (s *Store) AddPasskey(ctx context.Context, p *model.Passkey) error {
	p.ID = newID()
	p.CreatedAt = s.now()
	p.Name = truncate(strings.TrimSpace(p.Name), 100)
	if p.Transports == nil {
		p.Transports = []string{}
	}
	tr, _ := json.Marshal(p.Transports)
	return s.tx(ctx, func(c conn) error {
		var n int
		if err := c.row(ctx, `SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, p.UserID).Scan(&n); err != nil {
			return err
		}
		if n >= MaxPasskeysPerUser {
			return Conflictf("you can register at most %d passkeys; delete one first", MaxPasskeysPerUser)
		}
		if err := c.row(ctx, `SELECT COUNT(*) FROM webauthn_credentials WHERE credential_id = ?`, p.CredentialID).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return Conflictf("this passkey is already registered")
		}
		if _, err := c.exec(ctx, `INSERT INTO webauthn_credentials (`+passkeyCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.UserID, p.CredentialID, p.PublicKey, p.Alg, int64(p.SignCount), p.AAGUID, string(tr), p.Attestation, p.Name,
			p.UserVerified, p.BackupEligible, p.BackedUp, p.CreatedAt, nil); err != nil {
			return mapErr(err, "passkey")
		}
		_, err := c.exec(ctx, `UPDATE sessions SET mfa_enrol_required = ? WHERE user_id = ?`, false, p.UserID)
		return err
	})
}

// ListPasskeys returns the passkeys of a user, oldest first.
func (s *Store) ListPasskeys(ctx context.Context, userID string) ([]model.Passkey, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT `+passkeyCols+` FROM webauthn_credentials WHERE user_id = ? ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Passkey{}
	for rows.Next() {
		p, err := scanPasskey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPasskeyByCredentialID returns the passkey with a credential id.
func (s *Store) GetPasskeyByCredentialID(ctx context.Context, credentialID string) (*model.Passkey, error) {
	p, err := scanPasskey(s.c(ctx).row(ctx, `SELECT `+passkeyCols+` FROM webauthn_credentials WHERE credential_id = ?`, credentialID))
	if err != nil {
		return nil, mapErr(err, "passkey")
	}
	return p, nil
}

// GetPasskey returns a passkey of a user.
func (s *Store) GetPasskey(ctx context.Context, userID, id string) (*model.Passkey, error) {
	p, err := scanPasskey(s.c(ctx).row(ctx, `SELECT `+passkeyCols+` FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID))
	if err != nil {
		return nil, mapErr(err, "passkey")
	}
	return p, nil
}

// RenamePasskey changes the name of a passkey of a user.
func (s *Store) RenamePasskey(ctx context.Context, userID, id, name string) error {
	res, err := s.c(ctx).exec(ctx, `UPDATE webauthn_credentials SET name = ? WHERE id = ? AND user_id = ?`, truncate(strings.TrimSpace(name), 100), id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "passkey")
	}
	return nil
}

// DeletePasskey removes a passkey of a user.
func (s *Store) DeletePasskey(ctx context.Context, userID, id string) error {
	res, err := s.c(ctx).exec(ctx, `DELETE FROM webauthn_credentials WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return mapErr(sql.ErrNoRows, "passkey")
	}
	return nil
}

// DeleteUserPasskeys removes all passkeys of a user and their pending
// ceremonies and returns how many passkeys there were.
func (s *Store) DeleteUserPasskeys(ctx context.Context, userID string) (int, error) {
	var n int64
	err := s.tx(ctx, func(c conn) error {
		res, err := c.exec(ctx, `DELETE FROM webauthn_credentials WHERE user_id = ?`, userID)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		_, err = c.exec(ctx, `DELETE FROM webauthn_challenges WHERE user_id = ?`, userID)
		return err
	})
	return int(n), err
}

// UsePasskey records a sign-in with a passkey: the time, the highest
// signature counter seen and its backup state.
func (s *Store) UsePasskey(ctx context.Context, id string, count uint32, backedUp bool) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE webauthn_credentials SET last_used_at = ?, backed_up = ?,
		sign_count = CASE WHEN sign_count < ? THEN ? ELSE sign_count END WHERE id = ?`,
		s.now(), backedUp, int64(count), int64(count), id)
	return err
}

// CountPasskeys returns the number of passkeys of a user.
func (s *Store) CountPasskeys(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.c(ctx).row(ctx, `SELECT COUNT(*) FROM webauthn_credentials WHERE user_id = ?`, userID).Scan(&n)
	return n, err
}

// PasskeyCounts returns the number of passkeys per user (users without
// passkeys are missing).
func (s *Store) PasskeyCounts(ctx context.Context) (map[string]int, error) {
	rows, err := s.c(ctx).query(ctx, `SELECT user_id, COUNT(*) FROM webauthn_credentials GROUP BY user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- challenges

// CreateWebAuthnChallenge stores an issued challenge (expired ones are
// dropped).
func (s *Store) CreateWebAuthnChallenge(ctx context.Context, ch *model.WebAuthnChallenge) error {
	ch.CreatedAt = s.now()
	return s.tx(ctx, func(c conn) error {
		if _, err := c.exec(ctx, `DELETE FROM webauthn_challenges WHERE expires_at <= ?`, ch.CreatedAt); err != nil {
			return err
		}
		_, err := c.exec(ctx, `INSERT INTO webauthn_challenges (id, purpose, user_id, binding, user_verification, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ch.ID, ch.Purpose, ch.UserID, ch.Binding, ch.UserVerification, ch.CreatedAt, ch.ExpiresAt.UTC())
		return mapErr(err, "passkey challenge")
	})
}

// TakeWebAuthnChallenge removes the challenge and returns it when it was
// issued for purpose and binding and has not expired. Of two requests
// with the same challenge only one gets it (single use).
func (s *Store) TakeWebAuthnChallenge(ctx context.Context, id, purpose, binding string) (*model.WebAuthnChallenge, error) {
	var ch model.WebAuthnChallenge
	err := s.tx(ctx, func(c conn) error {
		if err := c.row(ctx, `SELECT id, purpose, user_id, binding, user_verification, created_at, expires_at FROM webauthn_challenges WHERE id = ?`, id).
			Scan(&ch.ID, &ch.Purpose, &ch.UserID, &ch.Binding, &ch.UserVerification, &ch.CreatedAt, &ch.ExpiresAt); err != nil {
			return err
		}
		res, err := c.exec(ctx, `DELETE FROM webauthn_challenges WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return sql.ErrNoRows
		}
		return nil
	})
	if err != nil {
		return nil, mapErr(err, "passkey challenge")
	}
	if ch.Purpose != purpose || ch.Binding != binding || !ch.ExpiresAt.After(s.now()) {
		return nil, mapErr(sql.ErrNoRows, "passkey challenge")
	}
	return &ch, nil
}

// SetSessionReauth records a re-authentication in a session.
func (s *Store) SetSessionReauth(ctx context.Context, sessionID string, at time.Time) error {
	_, err := s.c(ctx).exec(ctx, `UPDATE sessions SET reauth_at = ? WHERE id = ?`, at.UTC(), sessionID)
	return err
}
