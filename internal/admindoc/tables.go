package admindoc

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rom/qtestframe/internal/chat"
	"github.com/rom/qtestframe/internal/db"
	"github.com/rom/qtestframe/internal/hardening"
	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/rbac"
	"github.com/rom/qtestframe/internal/store"
)

var roleText = map[string]text{
	rbac.RoleViewer: {"Viewer", "Läsare"}, rbac.RoleReviewer: {"Reviewer", "Granskare"}, rbac.RoleSME: {"SME", "Ämnesexpert"}, rbac.RoleTester: {"Tester", "Testare"},
	rbac.RoleTestLead: {"Test lead", "Testledare"}, rbac.RoleManager: {"Manager", "Ansvarig"}, rbac.RoleAdmin: {"Admin", "Administratör"},
}

var permHeader = text{"Permission", "Behörighet"}

// permissionMatrix is the default permission matrix of the built-in roles
// (rbac defaults), least privileged role first.
func permissionMatrix(lang string) (string, error) {
	roles := slices.Clone(rbac.Roles)
	slices.Reverse(roles)
	var b strings.Builder
	b.WriteString("| " + permHeader.in(lang))
	for _, r := range roles {
		t, ok := roleText[r]
		if !ok {
			return "", fmt.Errorf("no name for the role %q", r)
		}
		b.WriteString(" | " + t.in(lang))
	}
	b.WriteString(" |\n|---" + strings.Repeat("|:-:", len(roles)) + "|\n")
	for _, p := range rbac.AllPermissionList {
		fmt.Fprintf(&b, "| `%s`", p)
		for _, r := range roles {
			mark := ""
			if slices.Contains(rbac.DefaultPermissions(r), string(p)) {
				mark = "✔"
			}
			b.WriteString(" | " + mark)
		}
		b.WriteString(" |\n")
	}
	return b.String(), nil
}

// webhookEvents lists the events a webhook can subscribe to.
func webhookEvents(lang string) (string, error) {
	groups := map[string][]string{}
	var order []string
	for _, e := range model.WebhookEvents {
		g, _, _ := strings.Cut(e, ".")
		if _, ok := groups[g]; !ok {
			order = append(order, g)
		}
		groups[g] = append(groups[g], "`"+e+"`")
	}
	head := text{"| Object | Events |\n|---|---|", "| Objekt | Händelser |\n|---|---|"}
	var b strings.Builder
	b.WriteString(head.in(lang) + "\n")
	for _, g := range order {
		fmt.Fprintf(&b, "| %s | %s |\n", g, strings.Join(groups[g], ", "))
	}
	return b.String(), nil
}

// chatEvents lists the events of a chat channel by group (the groups of
// the channel settings, derived from the event registry).
func chatEvents(lang string) (string, error) {
	groups := map[string][]string{}
	for _, e := range model.WebhookEvents {
		g := chat.Group(e)
		groups[g] = append(groups[g], "`"+e+"` ("+chat.EventLabel(lang, e)+")")
	}
	head := text{"| Group | Events |\n|---|---|", "| Grupp | Händelser |\n|---|---|"}
	var b strings.Builder
	b.WriteString(head.in(lang) + "\n")
	for _, g := range chat.Groups {
		if len(groups[g]) > 0 {
			fmt.Fprintf(&b, "| %s | %s |\n", chat.GroupLabel(lang, g), strings.Join(groups[g], ", "))
		}
	}
	return b.String(), nil
}

// backupTables lists the tables of a backup and those left out.
func backupTables(lang string) (string, error) {
	in := text{"**In a backup** (%d tables, in restore order):", "**I en säkerhetskopia** (%d tabeller, i den ordning de återställs):"}
	out := text{"**Not in a backup:**", "**Inte i en säkerhetskopia:**"}
	quote := func(ts []string) string { return "`" + strings.Join(ts, "`, `") + "`" }
	return fmt.Sprintf(in.in(lang), len(store.BackupTables)) + " " + quote(store.BackupTables) + ".\n\n" +
		out.in(lang) + " " + quote(store.NotBackedUpTables()) + ".", nil
}

// migrationList lists the embedded schema migrations.
func migrationList(lang string) (string, error) {
	ms, err := db.Migrations()
	if err != nil {
		return "", err
	}
	head := text{"| Version | Migration |\n|---|---|", "| Version | Migrering |\n|---|---|"}
	var b strings.Builder
	b.WriteString(head.in(lang) + "\n")
	for _, m := range ms {
		fmt.Fprintf(&b, "| %d | `%s` |\n", m.Version, m.Name)
	}
	return b.String(), nil
}

// hardeningLists are the built-in lists of the in-process hardening.
func hardeningLists(lang string) (string, error) {
	t := text{
		"| List | Entries |\n|---|---|\n| Read-only paths after the lock-down (besides the configured files) | %s |\n| System calls denied by seccomp (`EPERM`) | %s |\n| Further system calls denied to scripts with `runner.deny_network` | %s |",
		"| Lista | Poster |\n|---|---|\n| Sökvägar som får läsas efter låsningen (utöver de konfigurerade filerna) | %s |\n| Systemanrop som seccomp nekar (`EPERM`) | %s |\n| Fler systemanrop som nekas skript med `runner.deny_network` | %s |",
	}
	q := func(ts []string) string { return "`" + strings.Join(ts, "`, `") + "`" }
	return fmt.Sprintf(t.in(lang), q(hardening.DefaultReadOnly), q(hardening.DeniedSyscalls), q(hardening.NetworkSyscalls)), nil
}
