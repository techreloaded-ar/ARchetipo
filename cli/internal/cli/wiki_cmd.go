package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/techreloaded-ar/ARchetipo/cli/internal/config"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/iox"
	"github.com/techreloaded-ar/ARchetipo/cli/internal/wiki"
)

func newWikiCmd(s streams) *cobra.Command {
	root := &cobra.Command{Use: "wiki", Short: "Living project Wiki operations"}
	root.AddCommand(newWikiInitCmd(s), newWikiStatusCmd(s), newWikiValidateCmd(s), newWikiSearchCmd(s), newWikiAffectedCmd(s), newWikiPublishCmd(s), newWikiMigrateCmd(s))
	return root
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
			pages, err := wiki.Load(root, false)
			if err != nil {
				return nil, iox.NewInternal("loading Wiki", err)
			}
			counts := map[string]int{}
			for _, p := range pages {
				counts[string(p.Meta.Status)]++
			}
			report := wiki.Validate(cfg.ProjectRoot, root)
			return map[string]any{"root": root, "pages": len(pages), "statuses": counts, "ok": report.OK, "findings": report.Findings}, nil
		})
	}}
}

func newWikiValidateCmd(s streams) *cobra.Command {
	return &cobra.Command{Use: "validate", Short: "Validate Wiki structure, links and evidence", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "validation_result", true, func(cfg config.Config, root string) (any, error) { return wiki.Validate(cfg.ProjectRoot, root), nil })
	}}
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
			items, err := wiki.Search(root, query, pageType, status, includeSources)
			if err != nil {
				return nil, iox.NewInternal("searching Wiki", err)
			}
			return map[string]any{"query": query, "items": items, "count": len(items)}, nil
		})
	}}
	cmd.Flags().StringVar(&pageType, "type", "", "filter by page type")
	cmd.Flags().StringVar(&status, "status", "", "filter by lifecycle status")
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
	return &cobra.Command{Use: "publish", Short: "Atomically promote valid draft pages", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_publish_result", true, func(cfg config.Config, root string) (any, error) {
			count, err := wiki.Publish(cfg.ProjectRoot, root)
			if err != nil {
				if strings.Contains(err.Error(), "validation failed") {
					return nil, iox.NewConflict("Wiki validation failed", "run `archetipo wiki validate` and repair error findings", err)
				}
				return nil, iox.NewInternal("publishing Wiki", err)
			}
			return map[string]any{"published": count, "root": root}, nil
		})
	}}
}

func newWikiMigrateCmd(s streams) *cobra.Command {
	var prd, codemap string
	cmd := &cobra.Command{Use: "migrate", Short: "Archive legacy PRD and Codemap sources", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		return withWiki(cmd, s, "wiki_migration_result", false, func(cfg config.Config, root string) (any, error) {
			if prd == "" {
				prd = cfg.Paths.PRD
			}
			if codemap == "" {
				codemap = "docs/CODEMAP.md"
			}
			items, err := wiki.Migrate(cfg.ProjectRoot, root, prd, codemap)
			if err != nil {
				return nil, iox.NewInternal("migrating Wiki sources", err)
			}
			return map[string]any{"imported": items, "count": len(items), "requires_semantic_ingest": len(items) > 0}, nil
		})
	}}
	cmd.Flags().StringVar(&prd, "prd", "", "legacy PRD path")
	cmd.Flags().StringVar(&codemap, "codemap", "", "legacy Codemap path")
	return cmd
}
