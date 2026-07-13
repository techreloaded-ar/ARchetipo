package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

func newWikiCmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "wiki", Short: "Living project Wiki operations"}
	root.AddCommand(newWikiInitCmd(s), newWikiInspectCmd(s), newWikiStatusCmd(s), newWikiValidateCmd(s), newWikiSearchCmd(s), newWikiAffectedCmd(s), newWikiCatalogCmd(s), newWikiApproveCmd(s), newWikiResetCmd(s), newWikiPublishCmd(s))
	return root
}

func newWikiInspectCmd(s streams) *cobra.Command {
	return &cobra.Command{Use: "inspect", Short: "Inventory codebase evidence for Wiki bootstrap", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_inspection_result", false, func(cfg config.Config, root string) (any, error) {
			inspection, err := wiki.Inspect(cfg.ProjectRoot, root, cfg.Paths.PRD)
			if errors.Is(err, wiki.ErrNoProjectEvidence) {
				return nil, iox.NewPrecondition("Project has no code, manifest, or documentation evidence", "add project evidence before bootstrapping the Wiki", err)
			}
			if err != nil {
				return nil, iox.NewInternal("inspecting project", err)
			}
			return inspection, nil
		})
	}}
}

func withWiki(cmd *cobra.Command, s streams, kind string, require bool, fn func(config.Config, string) (any, error)) error {
	cwd, err := os.Getwd()
	if err != nil {
		return iox.NewInternal("cwd unavailable", err)
	}
	cfg, err := config.Load(cwd)
	if err != nil {
		return iox.NewInvalidInput(err.Error(), "fix .archetipo/config.yaml", err)
	}
	root := cfg.Paths.Wiki
	if !filepath.IsAbs(root) {
		root = filepath.Join(cfg.ProjectRoot, filepath.FromSlash(root))
	}
	if require {
		if _, err := os.Stat(root); errors.Is(err, fs.ErrNotExist) {
			return iox.NewPrecondition("Wiki is not initialized", "run `archetipo wiki init` first", err)
		} else if err != nil {
			return iox.NewInternal("reading Wiki root", err)
		}
	}
	data, err := fn(cfg, root)
	if err != nil {
		return err
	}
	if err := iox.WriteOK(s.out, kind, data); err != nil {
		return iox.NewInternal("encoding output", err)
	}
	return nil
}

func newWikiInitCmd(s streams) *cobra.Command {
	return &cobra.Command{Use: "init", Short: "Create the Wiki scaffold", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_init_result", false, func(cfg config.Config, root string) (any, error) {
			created, err := wiki.Init(root)
			if err != nil {
				return nil, iox.NewInternal("initializing Wiki", err)
			}
			return map[string]any{"root": root, "created": created}, nil
		})
	}}
}

func newWikiStatusCmd(s streams) *cobra.Command {
	return &cobra.Command{Use: "status", Short: "Summarize Wiki health and lifecycle state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_status", true, func(cfg config.Config, root string) (any, error) {
			pages, err := wiki.Load(root)
			if err != nil {
				return nil, iox.NewInternal("loading Wiki", err)
			}
			counts := map[string]int{}
			items := []map[string]any{}
			for _, p := range pages {
				state := wiki.PageState(cfg.ProjectRoot, p)
				counts[state]++
				items = append(items, map[string]any{"id": p.Meta.ID, "path": p.Path, "state": state, "issues": p.Meta.Issues})
			}
			report := wiki.Validate(cfg.ProjectRoot, root)
			return map[string]any{"root": root, "pages": len(pages), "states": counts, "items": items, "ok": report.OK, "findings": report.Findings}, nil
		})
	}}
}

func newWikiValidateCmd(s streams) *cobra.Command {
	var profile string
	cmd := &cobra.Command{Use: "validate", Short: "Validate Wiki structure, links and evidence", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if profile != "" && profile != "bootstrap" {
			return iox.NewInvalidInput("unknown Wiki validation profile: "+profile, "use --profile bootstrap or omit the flag", nil)
		}
		if profile == "bootstrap" {
			return withWiki(cmd, s, "validation_result", true, func(cfg config.Config, root string) (any, error) {
				report, err := wiki.ValidateBootstrap(cfg.ProjectRoot, root, cfg.Paths.PRD)
				if errors.Is(err, wiki.ErrNoProjectEvidence) {
					return nil, iox.NewPrecondition("Project has no code, manifest, or documentation evidence", "add project evidence before validating bootstrap coverage", err)
				}
				if err != nil {
					return nil, iox.NewInternal("validating bootstrap coverage", err)
				}
				return report, nil
			})
		}
		return withWiki(cmd, s, "validation_result", true, func(cfg config.Config, root string) (any, error) { return wiki.Validate(cfg.ProjectRoot, root), nil })
	}}
	cmd.Flags().StringVar(&profile, "profile", "", "additional validation profile (bootstrap)")
	return cmd
}

func newWikiSearchCmd(s streams) *cobra.Command {
	var pageType, status string
	var includeSources bool
	cmd := &cobra.Command{Use: "search [query]", Short: "Search the Wiki catalog and metadata", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) > 0 {
			query = args[0]
		}
		return withWiki(cmd, s, "wiki_search_result", true, func(cfg config.Config, root string) (any, error) {
			items, err := wiki.Search(cfg.ProjectRoot, root, query, pageType, status, includeSources)
			if err != nil {
				return nil, iox.NewInternal("searching Wiki", err)
			}
			return map[string]any{"query": query, "items": items, "count": len(items)}, nil
		})
	}}
	cmd.Flags().StringVar(&pageType, "type", "", "filter by page type")
	cmd.Flags().StringVar(&status, "status", "", "filter by derived state (generated, reviewed, stale, attention)")
	cmd.Flags().BoolVar(&includeSources, "include-sources", false, "include archived source documents")
	return cmd
}

func newWikiAffectedCmd(s streams) *cobra.Command {
	var base, head string
	var files []string
	cmd := &cobra.Command{Use: "affected", Short: "Find pages whose evidence intersects changed files", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_affected_result", true, func(cfg config.Config, root string) (any, error) {
			changed := files
			var err error
			if len(changed) == 0 {
				changed, err = wiki.GitChangedFiles(cfg.ProjectRoot, base, head)
				if err != nil {
					return nil, iox.NewInvalidInput("cannot resolve Git diff", "pass --file or valid --base/--head revisions", err)
				}
			}
			items, err := wiki.Affected(cfg.ProjectRoot, root, changed)
			if err != nil {
				return nil, iox.NewInternal("resolving affected Wiki pages", err)
			}
			return map[string]any{"files": changed, "items": items, "count": len(items)}, nil
		})
	}}
	cmd.Flags().StringVar(&base, "base", "", "base Git revision (default HEAD~1)")
	cmd.Flags().StringVar(&head, "head", "", "head Git revision (default HEAD)")
	cmd.Flags().StringSliceVar(&files, "file", nil, "changed project path; repeat or comma-separate")
	return cmd
}

func newWikiPublishCmd(s streams) *cobra.Command {
	cmd := &cobra.Command{Use: "publish", Short: "Deprecated alias for approving all generated pages", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_publish_result", true, func(cfg config.Config, root string) (any, error) {
			count, err := wiki.Publish(cfg.ProjectRoot, root)
			if err != nil {
				if errors.Is(err, wiki.ErrValidationFailed) || errors.Is(err, wiki.ErrUnresolvedIssues) || errors.Is(err, wiki.ErrMissingEvidence) {
					return nil, iox.NewConflict("Wiki approval blocked", "repair validation findings and resolve page issues before approval", err)
				}
				return nil, iox.NewInternal("publishing Wiki", err)
			}
			return map[string]any{"published": count, "root": root}, nil
		})
	}}
	cmd.Deprecated = "use `archetipo wiki approve`"
	return cmd
}

func newWikiApproveCmd(s streams) *cobra.Command {
	return &cobra.Command{Use: "approve [page-id...]", Short: "Mark generated pages as explicitly reviewed", Args: cobra.ArbitraryArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_approve_result", true, func(cfg config.Config, root string) (any, error) {
			count, err := wiki.Approve(cfg.ProjectRoot, root, args)
			if err != nil {
				if errors.Is(err, wiki.ErrValidationFailed) || errors.Is(err, wiki.ErrUnresolvedIssues) || errors.Is(err, wiki.ErrMissingEvidence) {
					return nil, iox.NewConflict("Wiki approval blocked", "repair validation findings and resolve page issues before approval", err)
				}
				if errors.Is(err, wiki.ErrPageNotFound) {
					return nil, iox.NewInvalidInput(err.Error(), "pass an existing Wiki page ID", err)
				}
				return nil, iox.NewInternal("approving Wiki pages", err)
			}
			return map[string]any{"approved": count, "root": root, "pages": args}, nil
		})
	}}
}

func newWikiResetCmd(s streams) *cobra.Command {
	return &cobra.Command{Use: "reset <page-id...>", Short: "Return reviewed pages to generated before updating them", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_reset_result", true, func(cfg config.Config, root string) (any, error) {
			count, err := wiki.Reset(cfg.ProjectRoot, root, args)
			if err != nil {
				if errors.Is(err, wiki.ErrPageNotFound) {
					return nil, iox.NewInvalidInput(err.Error(), "pass an existing Wiki page ID", err)
				}
				return nil, iox.NewInternal("resetting Wiki pages", err)
			}
			return map[string]any{"reset": count, "root": root, "pages": args}, nil
		})
	}}
}

func newWikiCatalogCmd(s streams) *cobra.Command {
	return &cobra.Command{Use: "catalog", Short: "Rebuild the Wiki index without changing review state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_catalog_result", true, func(cfg config.Config, root string) (any, error) {
			count, err := wiki.Catalog(cfg.ProjectRoot, root)
			if err != nil {
				return nil, iox.NewInternal("cataloging Wiki", err)
			}
			return map[string]any{"cataloged": count, "root": root}, nil
		})
	}}
}
