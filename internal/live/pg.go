package live

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/stdlib"
)

// maxPayload stays below PostgreSQL's NOTIFY limit of 8000 bytes.
const maxPayload = 7800

// PGBus carries events between instances through PostgreSQL
// LISTEN/NOTIFY. The channel is derived from the database and schema, so
// several installations (or test schemas) on one server do not mix.
type PGBus struct {
	DB  *sql.DB
	Log *slog.Logger
	// Channel is set by NewPGBus.
	Channel string
	// Backoff is the first wait before reconnecting (default 1s).
	Backoff time.Duration
}

// NewPGBus returns a bus on the database's channel.
func NewPGBus(ctx context.Context, db *sql.DB, log *slog.Logger) (*PGBus, error) {
	var dbName, schema string
	if err := db.QueryRowContext(ctx, `SELECT current_database(), current_schema()`).Scan(&dbName, &schema); err != nil {
		return nil, fmt.Errorf("live: %w", err)
	}
	sum := sha256.Sum256([]byte(dbName + "." + schema))
	return &PGBus{DB: db, Log: log, Channel: "qtf_live_" + hex.EncodeToString(sum[:8])}, nil
}

// Send notifies the other instances; a notification for many users is
// split so each payload fits.
func (b *PGBus) Send(ctx context.Context, ev Event) error {
	body, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if len(body) > maxPayload {
		if len(ev.Users) > 1 {
			a, c := ev, ev
			a.Users, c.Users = ev.Users[:len(ev.Users)/2], ev.Users[len(ev.Users)/2:]
			return errors.Join(b.Send(ctx, a), b.Send(ctx, c))
		}
		return fmt.Errorf("live: event %s too large for NOTIFY", ev.ID)
	}
	_, err = b.DB.ExecContext(ctx, `SELECT pg_notify($1, $2)`, b.Channel, string(body))
	if err != nil && b.Log != nil {
		b.Log.Warn("live: notify failed", "err", err)
	}
	return err
}

// Run listens on the channel until ctx ends, reconnecting with backoff;
// after a reconnection the subscribers are told to reload (h.Resync).
func (b *PGBus) Run(ctx context.Context, h *Hub) {
	wait := b.Backoff
	if wait <= 0 {
		wait = time.Second
	}
	first := true
	for ctx.Err() == nil {
		err := b.listen(ctx, h, func() {
			if !first {
				h.Resync()
			}
			first = false
			wait = b.Backoff
			if wait <= 0 {
				wait = time.Second
			}
		})
		if ctx.Err() != nil {
			return
		}
		if b.Log != nil {
			b.Log.Warn("live: listener disconnected", "err", err, "retry_in", wait.String())
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if wait *= 2; wait > time.Minute {
			wait = time.Minute
		}
	}
}

func (b *PGBus) listen(ctx context.Context, h *Hub, connected func()) error {
	conn, err := b.DB.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return conn.Raw(func(dc any) error {
		sc, ok := dc.(*stdlib.Conn)
		if !ok {
			return errors.New("live: not a pgx connection")
		}
		pc := sc.Conn()
		if _, err := pc.Exec(ctx, `LISTEN "`+b.Channel+`"`); err != nil {
			return err
		}
		connected()
		defer func() {
			// the connection goes back to the pool: stop listening there
			c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = pc.Exec(c, `UNLISTEN *`)
		}()
		for {
			n, err := pc.WaitForNotification(ctx)
			if err != nil {
				return err
			}
			var ev Event
			if json.Unmarshal([]byte(n.Payload), &ev) != nil || ev.Origin == h.instance {
				continue
			}
			h.Deliver(ev)
		}
	})
}
