// Package mention implements "@name" references to people.
//
// A mention is "@" followed by a handle of letters, digits, ".", "_" and
// "-" (it must not end in punctuation and must not follow a letter or digit,
// so e-mail addresses are not mentions). Users of the organisation get the
// local part of their e-mail address as handle ("anna.berg@example.com" →
// "anna.berg"; a second "anna.berg" in another domain becomes
// "anna.berg-2"). People who are named in a project but have no account
// (an owner or assignee typed as plain text) get their name with spaces
// replaced by dots ("Anna Berg" → "anna.berg").
package mention

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Kinds of people.
const (
	KindUser   = "user"   // a member of the organisation
	KindPerson = "person" // a name used in the project without an account
)

// Person is someone that can be mentioned in a project.
type Person struct {
	Handle string `json:"handle"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	UserID string `json:"user_id,omitempty"`
	Email  string `json:"email,omitempty"`
	Avatar string `json:"avatar,omitempty"`
	// Uses counts how often a person's name appears in the project's owner
	// and assignee fields.
	Uses int `json:"uses,omitempty"`
}

func handleRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
}

// Slug turns a name or e-mail local part into a handle: lower case, runs of
// other characters → ".", no leading or trailing punctuation.
func Slug(s string) string {
	var b strings.Builder
	dot := false
	for _, r := range strings.ToLower(strings.TrimSpace(s)) {
		if handleRune(r) {
			if dot && b.Len() > 0 {
				b.WriteByte('.')
			}
			dot = false
			b.WriteRune(r)
		} else {
			dot = true
		}
	}
	return strings.Trim(b.String(), "._-")
}

// EmailHandle is the handle of a user: the slug of the e-mail local part.
func EmailHandle(email string) string {
	local, _, _ := strings.Cut(email, "@")
	if h := Slug(local); h != "" {
		return h
	}
	return Slug(email)
}

// Extract returns the handles mentioned in text, lower case, in order of
// first appearance and without duplicates.
func Extract(text string) []string {
	var out []string
	seen := map[string]bool{}
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '@' || (i > 0 && (unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1]))) {
			continue
		}
		j := i + 1
		for j < len(runes) && handleRune(runes[j]) {
			j++
		}
		h := strings.ToLower(strings.TrimRight(string(runes[i+1:j]), "._-"))
		if h != "" && !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
		i = j - 1
	}
	return out
}

// User is the input for Directory: a member of the organisation.
type User struct {
	ID, Email, Name, Avatar string
}

// Directory builds the people of a project: every user (with a unique
// handle) and every other name in names (values of owner and assignee
// fields; a value that is a mention of a user counts for that user). The
// result is sorted by kind (users first), then by name.
func Directory(users []User, names []string) []Person {
	us := append([]User(nil), users...)
	sort.Slice(us, func(i, j int) bool { return strings.ToLower(us[i].Email) < strings.ToLower(us[j].Email) })
	taken := map[string]bool{}
	var out []Person
	byHandle := map[string]int{}
	byName := map[string]int{}
	for _, u := range us {
		base := EmailHandle(u.Email)
		h := base
		for n := 2; taken[h] || h == ""; n++ {
			h = base + "-" + strconv.Itoa(n)
		}
		taken[h] = true
		name := u.Name
		if name == "" {
			name = u.Email
		}
		byHandle[h] = len(out)
		byName[strings.ToLower(name)] = len(out)
		byName[strings.ToLower(u.Email)] = len(out)
		out = append(out, Person{Handle: h, Name: name, Kind: KindUser, UserID: u.ID, Email: u.Email, Avatar: u.Avatar})
	}
	for _, raw := range names {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if hs := Extract(v); len(hs) > 0 && strings.HasPrefix(v, "@") {
			for _, h := range hs {
				if i, ok := byHandle[h]; ok {
					out[i].Uses++
				} else {
					addPerson(&out, byHandle, byName, taken, h, h)
				}
			}
			continue
		}
		if i, ok := byName[strings.ToLower(v)]; ok {
			out[i].Uses++
			continue
		}
		addPerson(&out, byHandle, byName, taken, Slug(v), v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind == KindUser
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func addPerson(out *[]Person, byHandle, byName map[string]int, taken map[string]bool, handle, name string) {
	if handle == "" {
		return
	}
	if i, ok := byHandle[handle]; ok {
		(*out)[i].Uses++
		return
	}
	taken[handle] = true
	byHandle[handle] = len(*out)
	byName[strings.ToLower(name)] = len(*out)
	*out = append(*out, Person{Handle: handle, Name: name, Kind: KindPerson, Uses: 1})
}
