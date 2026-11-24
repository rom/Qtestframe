package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// FilePrefix starts the names of the files of scheduled backups; only
// files named like that are ever deleted by the retention rules.
const FilePrefix = "qtestframe-backup-"

const stampLayout = "20060102-150405"

var fileName = regexp.MustCompile(`^qtestframe-backup-(\d{8}-\d{6})(?:-\d{1,3})?\.zip(?:\.enc)?$`)

// FileName is the name of a backup file written at t.
func FileName(t time.Time, encrypted bool) string {
	n := FilePrefix + t.UTC().Format(stampLayout) + ".zip"
	if encrypted {
		n += EncryptedExt
	}
	return n
}

// IsBackupFile reports whether name is the name of a scheduled backup file
// (no directories, see FileName).
func IsBackupFile(name string) bool { return fileName.MatchString(name) }

// FreeName returns FileName(t, encrypted) in dir, or the name with a
// counter when a file of that name exists already.
func FreeName(dir string, t time.Time, encrypted bool) string {
	name := FileName(t, encrypted)
	for i := 1; i < 1000; i++ {
		if _, err := os.Lstat(filepath.Join(dir, name)); os.IsNotExist(err) {
			return name
		}
		n := FilePrefix + t.UTC().Format(stampLayout) + fmt.Sprintf("-%d", i) + ".zip"
		if encrypted {
			n += EncryptedExt
		}
		name = n
	}
	return name
}

type stamped struct {
	name string
	at   time.Time
}

// Prune applies the retention rules to the backup files in dir and returns
// the names of the deleted files: keepLast > 0 keeps the newest keepLast
// files, keepDays > 0 deletes files older than keepDays days; with both, a
// file goes when either rule says so. The newest file is always kept, and
// files not named like scheduled backups are never touched.
func Prune(dir string, keepLast, keepDays int, now time.Time) ([]string, error) {
	if keepLast <= 0 && keepDays <= 0 {
		return nil, nil
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []stamped
	for _, e := range ents {
		m := fileName.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		at, err := time.Parse(stampLayout, m[1])
		if err != nil {
			continue
		}
		files = append(files, stamped{e.Name(), at})
	}
	sort.Slice(files, func(i, j int) bool {
		if !files[i].at.Equal(files[j].at) {
			return files[i].at.After(files[j].at)
		}
		return files[i].name > files[j].name
	})
	cutoff := now.UTC().AddDate(0, 0, -keepDays)
	var deleted []string
	for i, f := range files {
		if i == 0 {
			continue
		}
		if (keepLast > 0 && i >= keepLast) || (keepDays > 0 && f.at.Before(cutoff)) {
			if err := os.Remove(filepath.Join(dir, f.name)); err != nil && !os.IsNotExist(err) {
				return deleted, err
			}
			deleted = append(deleted, f.name)
		}
	}
	return deleted, nil
}

// FileSHA256 returns the hex SHA-256 and the size of the file at path.
func FileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), n, nil
}
