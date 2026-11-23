package auth

import (
	"errors"
	"testing"
)

func TestPasswordPolicy(t *testing.T) {
	if CharClasses("abc") != 1 || CharClasses("aB1!") != 4 || CharClasses("äÖ") != 2 || CharClasses("日本1") != 2 {
		t.Error(CharClasses("aB1!"))
	}
	pol := PasswordPolicy{MinLength: 12, MinClasses: 3}
	for pw, ok := range map[string]bool{"lowercase-only-1": true, "lowercaseonly11": false, "Upper-and-lower": true, "short-A1": false, "UPPER-LOWER-x": true} {
		err := pol.Validate(pw)
		if (err == nil) != ok {
			t.Errorf("%s: %v", pw, err)
		}
		if err != nil && !errors.Is(err, ErrWeakPassword) {
			t.Errorf("%s: %v is not ErrWeakPassword", pw, err)
		}
	}
	if err := (PasswordPolicy{MinClasses: 4}).Validate("abcdefgh-1A"); err != nil {
		t.Error(err)
	}
	if err := (PasswordPolicy{MinClasses: 4}).Validate("abcdefghij-1"); err == nil || err.Error() != "password must be at least 10 characters and use at least 4 of: lower-case letters, upper-case letters, digits, symbols" {
		t.Error(err)
	}
	h1, _ := HashPassword("first-password-1")
	h2, _ := HashPassword("second-password-2")
	if !Reused("first-password-1", []string{h2, h1}, 2) || Reused("first-password-1", []string{h2, h1}, 1) || Reused("other", []string{h1}, 5) {
		t.Error("Reused")
	}
}

func TestMapRolesSeveral(t *testing.T) {
	valid := func(r string) bool { return r != "bogus" }
	m := map[string]string{"qa": "tester, reviewer", "lead": "manager,bogus"}
	got := MapRoles([]string{"qa", "lead", "qa"}, m, "viewer", valid)
	if len(got) != 3 || got[0] != "tester" || got[1] != "reviewer" || got[2] != "manager" {
		t.Fatal(got)
	}
	if got := MapRoles(nil, m, "tester, reviewer", valid); len(got) != 2 {
		t.Fatal(got)
	}
	if got := MapRoles(nil, m, "bogus", valid); len(got) != 1 || got[0] != "bogus" {
		t.Fatal(got)
	}
	if len(SplitRoles(" a, ,b ")) != 2 || SplitRoles("") != nil {
		t.Error(SplitRoles(" a, ,b "))
	}
}
