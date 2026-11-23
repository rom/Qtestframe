package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/rom/qtestframe/internal/model"
	"github.com/rom/qtestframe/internal/store"
)

const auditUsage = `usage: qtestframe audit verify [--config FILE] [--tenant SLUG] [--json]
       qtestframe audit verify --file EXPORT.json [--json]

  verify   check the hash chain of the audit trail: every entry's hash, the
           links between entries, the sequence, the end of the chain and the
           signed checkpoints (with --tenant one organisation, otherwise
           every organisation and the system entries). Exit 0 when intact,
           1 at the first broken link.
           --file checks an export (Administration → Audit trail → Export
           with hashes, or GET /api/v1/audit/archive) without the database:
           its entries, links, end and checkpoint hashes (the checkpoint
           signatures need the server's key).
`

func cmdAudit(args []string, env Env) error {
	if len(args) == 0 || args[0] != "verify" {
		fmt.Fprint(env.Stderr, auditUsage)
		return errors.New("audit: expected sub-command verify")
	}
	fs, cfgPath := newFlags("audit verify", env)
	slug := fs.String("tenant", "", "organisation (slug); default every organisation")
	file := fs.String("file", "", "verify an exported audit trail (JSON) instead of the database")
	asJSON := fs.Bool("json", false, "print the result as JSON")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	var results []namedVerification
	if *file != "" {
		f, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer f.Close()
		v, err := verifyArchive(f)
		if err != nil {
			return err
		}
		results = append(results, namedVerification{Name: *file, AuditVerification: v})
	} else {
		ctx := env.Ctx
		_, st, err := openStore(ctx, *cfgPath, env.Stderr)
		if err != nil {
			return err
		}
		defer st.DB.Close()
		var tenants []model.Tenant
		if *slug != "" {
			t, err := st.GetTenantBySlug(ctx, *slug)
			if err != nil {
				return fmt.Errorf("organisation %q: %w", *slug, err)
			}
			tenants = []model.Tenant{*t}
		} else {
			if tenants, err = st.ListTenants(ctx); err != nil {
				return err
			}
			tenants = append(tenants, model.Tenant{Slug: "(system)"})
		}
		for _, t := range tenants {
			v, err := st.VerifyAudit(ctx, t.ID)
			if err != nil {
				return fmt.Errorf("%s: %w", t.Slug, err)
			}
			results = append(results, namedVerification{Name: t.Slug, AuditVerification: v})
		}
	}
	broken := 0
	for _, r := range results {
		if !r.OK {
			broken++
		}
	}
	if *asJSON {
		enc := json.NewEncoder(env.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(results); err != nil {
			return err
		}
	} else {
		for _, r := range results {
			if r.OK {
				fmt.Fprintf(env.Stdout, "%s: OK, %d entries, %d checkpoints, last entry %d %s\n", r.Name, r.Entries, r.Checkpoints, r.LastSeq, short(r.LastHash))
				continue
			}
			fmt.Fprintf(env.Stdout, "%s: BROKEN at entry %d (%s): %s\n", r.Name, r.Broken.Seq, r.Broken.Problem, r.Broken.Message)
			if r.Broken.EntryID != "" {
				fmt.Fprintf(env.Stdout, "  entry id %s\n", r.Broken.EntryID)
			}
		}
	}
	if broken > 0 {
		return &exitError{code: 1, err: fmt.Errorf("audit trail: %d of %d chains broken", broken, len(results))}
	}
	return nil
}

type namedVerification struct {
	Name string `json:"name"`
	*model.AuditVerification
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

// verifyArchive checks an exported audit trail (JSON).
func verifyArchive(r io.Reader) (*model.AuditVerification, error) {
	var a store.AuditArchive
	dec := json.NewDecoder(r)
	if err := dec.Decode(&a); err != nil {
		return nil, fmt.Errorf("not an audit trail export: %w", err)
	}
	if a.Format != store.AuditArchiveFormat {
		return nil, fmt.Errorf("not an audit trail export (format %q, want %q)", a.Format, store.AuditArchiveFormat)
	}
	if a.Chain != store.AuditChainVersion {
		return nil, fmt.Errorf("unknown chain version %q (this version checks %q)", a.Chain, store.AuditChainVersion)
	}
	return store.VerifyAuditArchive(&a), nil
}
