package store

import (
	"errors"
	"testing"
)

type rows int64

func (r rows) LastInsertId() (int64, error) { return 0, nil }
func (r rows) RowsAffected() (int64, error) { return int64(r), nil }

func TestSameVersion(t *testing.T) {
	if err := sameVersion(rows(1)); err != nil {
		t.Fatal(err)
	}
	err := sameVersion(rows(0))
	if !errors.Is(err, ErrStale) || !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
