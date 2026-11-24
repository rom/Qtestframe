package settings

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBackupSettingsDecode(t *testing.T) {
	ok := map[string]string{
		"backup.schedule":   `"weekly sun 03:30"`,
		"backup.directory":  `"/var/backups/qtestframe"`,
		"backup.passphrase": `"<twelve or more>"`,
		"backup.s3.bucket":  `"qtf-backups"`,
		"backup.s3.prefix":  `"qtestframe/nightly/"`,
		"backup.s3.region":  `"eu-north-1"`,
		"backup.keep_last":  `0`,
		"backup.keep_days":  `3650`,
	}
	for k, raw := range ok {
		if _, err := def(t, k).Decode(json.RawMessage(raw)); err != nil {
			t.Errorf("%s %s: %v", k, raw, err)
		}
	}
	for _, k := range []string{"backup.directory", "backup.passphrase", "backup.s3.bucket", "backup.s3.prefix", "backup.s3.endpoint"} {
		if _, err := def(t, k).Decode(json.RawMessage(`""`)); err != nil {
			t.Errorf("%s empty: %v", k, err)
		}
	}
	bad := map[string][]string{
		"backup.schedule":    {`"hourly"`, `""`, `"every 1m"`},
		"backup.directory":   {`"relative/dir"`},
		"backup.passphrase":  {`"short"`},
		"backup.s3.bucket":   {`"Bad_Bucket"`, `"ab"`},
		"backup.s3.prefix":   {`"a b"`, `"../x"`},
		"backup.s3.endpoint": {`"ftp://x"`, `"https://s3.example.com/"`},
		"backup.keep_last":   {`-1`, `1001`},
	}
	for k, raws := range bad {
		for _, raw := range raws {
			if _, err := def(t, k).Decode(json.RawMessage(raw)); err == nil || !strings.Contains(err.Error(), k) {
				t.Errorf("%s %s accepted: %v", k, raw, err)
			}
		}
	}
	if err := checkDirectory("/a\nb"); err == nil {
		t.Error("line break accepted")
	}
	if d := def(t, "backup.directory"); d.Apply != Restart {
		t.Error("the directory is opened at start-up")
	}
}
