package app

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rom/qtestframe/internal/config"
	"github.com/rom/qtestframe/internal/secrets"
	"github.com/rom/qtestframe/internal/store"
)

// setupSecrets opens the configured key provider and makes st encrypt the
// stored secrets; secrets still stored as plaintext (written by an earlier
// version) are encrypted in place. note receives messages for the operator.
func setupSecrets(ctx context.Context, cfg *config.Config, st *store.Store, note func(string)) (secrets.Provider, error) {
	o, err := secrets.Open(cfg, true)
	if err != nil {
		return nil, err
	}
	for _, n := range o.Notes {
		note(n)
	}
	if err := st.SetupSecrets(ctx, o.Provider); err != nil {
		o.Provider.Close()
		return nil, err
	}
	stats, err := st.MigrateSecrets(ctx)
	if err != nil {
		o.Provider.Close()
		return nil, fmt.Errorf("secrets: encrypt the secrets stored as plaintext: %w", err)
	}
	if stats.Changed > 0 {
		note(fmt.Sprintf("encrypted %d secret(s) that were stored as plaintext", stats.Changed))
	}
	return o.Provider, nil
}

const secretsUsage = `usage: qtestframe secrets <command> [--config FILE]

  status         show the key provider, the data-encryption keys and how many
                 stored secrets are encrypted
  migrate        encrypt the secrets still stored as plaintext (also done at
                 every start of the server)
  rotate         create a new data-encryption key and re-encrypt every secret
                 with it; --prune deletes the old keys afterwards (stop or
                 restart the other server processes first)
  rewrap         wrap the data-encryption keys with the configured key
                 provider; --from-config OLD.yaml names the configuration of
                 the provider that wrapped them (e.g. moving from the local key
                 file to the TPM or an HSM, or to a new key file)
  decrypt --yes  store every secret as plaintext again (before a downgrade)
  generate-key   print a new random key for QTF_SECRETS_KEY or a key file
`

func cmdSecrets(args []string, env Env) error {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, secretsUsage)
		return errors.New("secrets: a command is required")
	}
	fs, cfgPath := newFlags("secrets "+args[0], env)
	prune := fs.Bool("prune", false, "rotate: delete the old data-encryption keys no value uses any more")
	from := fs.String("from-config", "", "rewrap: configuration file of the key provider that wrapped the keys")
	yes := fs.Bool("yes", false, "decrypt: confirm that the secrets are stored as plaintext")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	ctx := env.Ctx
	switch args[0] {
	case "generate-key":
		fmt.Fprint(env.Stdout, secrets.FormatKey(secrets.NewKey()))
		return nil
	case "status":
		return secretsStatus(ctx, env, *cfgPath)
	case "migrate":
		cfg, st, err := openStore(ctx, *cfgPath, env.Stderr)
		if err != nil {
			return err
		}
		defer st.DB.Close()
		defer st.Keyring().Provider().Close()
		stats, err := st.SecretStatus(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "provider %s (%s): %s\n", cfg.Secrets.Provider, st.Keyring().Provider().KeyID(), describeStats(stats))
		return nil
	case "rotate":
		_, st, err := openStore(ctx, *cfgPath, env.Stderr)
		if err != nil {
			return err
		}
		defer st.DB.Close()
		defer st.Keyring().Provider().Close()
		id, stats, pruned, err := st.RotateSecrets(ctx, *prune)
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "new data-encryption key %s; re-encrypted %d secret(s)\n", id, stats.Changed)
		if len(pruned) > 0 {
			fmt.Fprintf(env.Stdout, "deleted the old key(s) %s\n", strings.Join(pruned, ", "))
		} else if !*prune {
			fmt.Fprintln(env.Stdout, "the old keys are kept (other server processes may still use them); restart the servers, then run `secrets rotate --prune` or delete them later")
		}
		return nil
	case "rewrap":
		return secretsRewrap(ctx, env, *cfgPath, *from)
	case "decrypt":
		if !*yes {
			return errors.New("decrypt stores every secret as plaintext; pass --yes to confirm")
		}
		_, st, err := openStore(ctx, *cfgPath, env.Stderr)
		if err != nil {
			return err
		}
		defer st.DB.Close()
		defer st.Keyring().Provider().Close()
		stats, err := st.DecryptSecrets(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(env.Stdout, "decrypted %d secret(s); they are encrypted again at the next start of this version\n", stats.Changed)
		return nil
	case "help", "--help", "-h":
		fmt.Fprint(env.Stdout, secretsUsage)
		return nil
	}
	fmt.Fprint(env.Stderr, secretsUsage)
	return fmt.Errorf("secrets: unknown command %q", args[0])
}

func describeStats(s *store.SecretStats) string {
	enc := 0
	keys := make([]string, 0, len(s.Encrypted))
	for k, n := range s.Encrypted {
		enc += n
		keys = append(keys, fmt.Sprintf("%s: %d", k, n))
	}
	sort.Strings(keys)
	out := fmt.Sprintf("%d secret(s) encrypted, %d stored as plaintext", enc, s.Plaintext)
	if len(keys) > 0 {
		out += " (" + strings.Join(keys, ", ") + ")"
	}
	return out
}

// secretsStatus reports the provider, the keys and the stored values
// without changing anything (no key file is created).
func secretsStatus(ctx context.Context, env Env, cfgPath string) error {
	cfg, st, err := openDB(ctx, cfgPath)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	fmt.Fprintf(env.Stdout, "provider:  %s\n", cfg.Secrets.Provider)
	o, perr := secrets.Open(&cfg, false)
	if perr == nil {
		defer o.Provider.Close()
		fmt.Fprintf(env.Stdout, "key:       %s\n", o.Provider.KeyID())
		info := o.Provider.Info()
		names := make([]string, 0, len(info))
		for k := range info {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			fmt.Fprintf(env.Stdout, "  %-12s %s\n", k+":", info[k])
		}
	} else {
		fmt.Fprintf(env.Stdout, "key:       not available: %v\n", perr)
	}
	keys, err := st.SecretKeys(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "data-encryption keys: %d\n", len(keys))
	for _, k := range keys {
		state := ""
		if k.Active {
			state = " (active)"
		}
		ok := ""
		if perr == nil {
			ok = "  ok"
			if k.KEKID != o.Provider.KeyID() {
				ok = "  wrapped by another key"
			}
		}
		fmt.Fprintf(env.Stdout, "  %s%s  %s %s, created %s%s\n", k.ID, state, k.Provider, k.KEKID, k.CreatedAt.Format("2006-01-02 15:04"), ok)
	}
	stats, err := st.SecretStatus(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "stored secrets: %s\n", describeStats(stats))
	fields := make([]string, 0, len(stats.Fields))
	for f := range stats.Fields {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, f := range fields {
		fmt.Fprintf(env.Stdout, "  %-40s %d\n", f, stats.Fields[f])
	}
	return nil
}

// secretsRewrap wraps the stored data-encryption keys with the configured
// provider, unwrapping them with the provider of fromPath (default: the
// configured one).
func secretsRewrap(ctx context.Context, env Env, cfgPath, fromPath string) error {
	cfg, st, err := openDB(ctx, cfgPath)
	if err != nil {
		return err
	}
	defer st.DB.Close()
	to, err := secrets.Open(&cfg, true)
	if err != nil {
		return err
	}
	defer to.Provider.Close()
	for _, n := range to.Notes {
		fmt.Fprintf(env.Stderr, "secrets: %s\n", n)
	}
	src := to.Provider
	if fromPath != "" {
		fromCfg, err := config.Load(fromPath)
		if err != nil {
			return fmt.Errorf("--from-config: %w", err)
		}
		// the database of the old configuration is not used; the key file
		// default follows the database of this configuration
		fromCfg.Database = cfg.Database
		from, err := secrets.Open(&fromCfg, false)
		if err != nil {
			return fmt.Errorf("--from-config: %w", err)
		}
		defer from.Provider.Close()
		src = from.Provider
	}
	n, err := st.RewrapSecretKeys(ctx, src, to.Provider)
	if err != nil {
		return err
	}
	if _, err := st.MigrateSecrets(ctx); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "re-wrapped %d data-encryption key(s) from %s to %s %s\n", n, src.KeyID(), to.Provider.Name(), to.Provider.KeyID())
	if fromPath != "" {
		fmt.Fprintln(env.Stdout, "keep the old key until you have checked that the server starts with the new one; backups taken before need the old key")
	}
	return nil
}
