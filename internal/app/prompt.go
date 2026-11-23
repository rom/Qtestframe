package app

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rom/qtestframe/internal/prompt"
	"github.com/rom/qtestframe/internal/uploader"
)

// cmdPrompt builds a prompt for an AI assistant on a server (qtestframe
// prompt, see docs/user-guide/en/requirements.md "AI prompts") and prints
// it, or writes one Markdown file per part into --out. Nothing is sent to
// an AI service.
func cmdPrompt(args []string, env Env) error {
	fs := flag.NewFlagSet("prompt", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	var sf serverFlags
	sf.register(fs)
	var in uploader.PromptRequest
	fs.StringVar(&in.Kind, "kind", "architecture", "prompt kind: "+strings.Join(prompt.Kinds, ", "))
	fs.StringVar(&in.Query, "scope", "", "the requirements (or tests) to include, in the query language; \"\" for all")
	fs.StringVar(&in.Query, "query", "", "same as --scope")
	folder := fs.String("folder", "", "only this requirement folder and its subfolders (id, name or path A/B)")
	release := fs.String("release", "", "only the requirements of this release (id, name or version)")
	fs.StringVar(&in.Variant, "variant", "", "only the requirements that apply to this variant (id or name)")
	fs.StringVar(&in.Lang, "lang", "en", "language of the prompt: en or sv")
	fs.StringVar(&in.TestType, "test-type", "", "test type (design_tests, test_strategy): "+strings.Join(prompt.TestTypes, ", "))
	fs.StringVar(&in.Framework, "framework", "", "test framework or tools (implement, automate_tests, test_strategy)")
	fs.StringVar(&in.Notes, "notes", "", "additional instructions")
	fs.BoolVar(&in.Compact, "compact", false, "solution prompts: titles and acceptance criteria only")
	fs.StringVar(&in.Split, "split", "none", "solution prompts: split into parts by folder (top-level folder) or size, or none")
	fs.IntVar(&in.MaxTokens, "max-tokens", 0, fmt.Sprintf("solution prompts: estimated tokens per part (%d–%d; split by size defaults to %d)", prompt.MinPartTokens, prompt.MaxPartTokens, prompt.DefaultPartTokens))
	formats := fs.String("formats", "", "solution prompts: output formats besides Markdown, comma-separated ("+strings.Join(prompt.Formats, ", ")+"; default: the kind's, \"none\" for Markdown only)")
	out := fs.String("out", "", "directory to write the prompt to (one .md file per part); default: print to standard output")
	if _, err := parseArgs(fs, args); err != nil {
		return err
	}
	if sf.project == "" {
		return errors.New("--project KEY is required (or $QTF_PROJECT)")
	}
	switch strings.TrimSpace(*formats) {
	case "":
	case "none":
		in.Formats = []string{}
	default:
		in.Formats = splitList([]string{*formats})
	}
	c, err := sf.client()
	if err != nil {
		return err
	}
	p, err := c.ProjectByKey(env.Ctx, sf.project)
	if err != nil {
		return err
	}
	if *folder != "" {
		if in.FolderID, err = c.FindFolder(env.Ctx, p.ID, *folder); err != nil {
			return err
		}
		if in.FolderID == "" {
			return fmt.Errorf("no requirement folder %q in %s", *folder, p.Key)
		}
	}
	if *release != "" {
		if in.ReleaseID, err = c.FindRelease(env.Ctx, p.ID, *release); err != nil {
			return err
		}
		if in.ReleaseID == "" {
			return fmt.Errorf("no release %q in %s", *release, p.Key)
		}
	}
	res, err := c.Prompt(env.Ctx, p.ID, in)
	if err != nil {
		return err
	}
	parts := res.Parts
	if len(parts) == 0 {
		parts = []uploader.PromptPart{{Index: 1, Title: res.Title, Text: res.Text, Items: res.Items, Tokens: res.Tokens}}
	}
	if *out == "" {
		fmt.Fprint(env.Stdout, res.Text)
		fmt.Fprintf(env.Stderr, "%s: %d items, %d part(s), about %d tokens\n", res.Title, res.Items, len(parts), res.Tokens)
		return nil
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	base := fmt.Sprintf("%s-%s-%s", p.Key, strings.ReplaceAll(res.Kind, "_", "-"), time.Now().Format("2006-01-02"))
	for _, pt := range parts {
		name := base + ".md"
		if len(parts) > 1 {
			name = fmt.Sprintf("%s-part-%02d.md", base, pt.Index)
		}
		target := filepath.Join(*out, name)
		if err := os.WriteFile(target, []byte(pt.Text), 0o644); err != nil {
			return err
		}
		label := ""
		if pt.Label != "" {
			label = " (" + pt.Label + ")"
		}
		fmt.Fprintf(env.Stdout, "wrote %s%s: %d items, about %d tokens\n", target, label, pt.Items, pt.Tokens)
	}
	fmt.Fprintf(env.Stderr, "%s: %d items in %d file(s), about %d tokens\n", res.Title, res.Items, len(parts), res.Tokens)
	return nil
}
