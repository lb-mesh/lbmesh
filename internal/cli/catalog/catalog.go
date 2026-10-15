// Package catalog provides the `lbmesh catalog` sub-command group.
package catalog

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/dirs"
	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
)

// NewCatalogCmd returns the parent `catalog` command with its sub-commands attached.
func NewCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Manage recipe catalogs",
		Long:  `catalog groups operations for browsing and synchronising recipe template catalogs.`,
	}

	cmd.AddCommand(newListCmd(), newSyncCmd())
	return cmd
}

func newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Display all discovered local, custom, tap, and upstream templates",
		Long: `list scans every configured catalog source — the local custom directory,
connected git taps, the cached upstream catalog, and the embedded default
catalog — and prints a consolidated table of available recipes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			type row struct {
				name     string
				category string
				source   string
			}

			var rows []row

			// 1. Embedded defaults.
			embeddedNames, err := recipe.EmbeddedNames()
			if err == nil {
				for _, n := range embeddedNames {
					rows = append(rows, row{name: n, category: "-", source: "embedded"})
				}
			}

			// 2. User custom templates: ~/.lbmesh/custom/templates/<name>/
			root, err := dirs.LocalRoot()
			if err == nil {
				customDir := filepath.Join(root, "custom", "templates")
				if entries, err := os.ReadDir(customDir); err == nil {
					for _, e := range entries {
						if e.IsDir() {
							rows = append(rows, row{name: e.Name(), category: "-", source: "custom"})
						}
					}
				}

				// 3. Cached upstream catalog templates: ~/.lbmesh/catalog/templates/<category>/<name>/
				catDir := filepath.Join(root, "catalog", "templates")
				if cats, err := os.ReadDir(catDir); err == nil {
					for _, cat := range cats {
						if !cat.IsDir() {
							continue
						}
						catPath := filepath.Join(catDir, cat.Name())
						if names, err := os.ReadDir(catPath); err == nil {
							for _, n := range names {
								if n.IsDir() {
									rows = append(rows, row{name: n.Name(), category: cat.Name(), source: "catalog"})
								}
							}
						}
					}
				}

				// 4. Git tap templates: ~/.lbmesh/taps/<host>/<owner>/<repo>/templates/<name>/
				taps, err := recipe.TapList()
				if err == nil {
					for _, tap := range taps {
						tapTemplates := filepath.Join(tap.Path, "templates")
						if names, err := os.ReadDir(tapTemplates); err == nil {
							for _, n := range names {
								if n.IsDir() {
									rows = append(rows, row{name: n.Name(), category: "-", source: "tap:" + tap.Slug})
								}
							}
						}
					}
				}
			}

			if len(rows) == 0 {
				fmt.Println("No recipes found. Run `lbmesh catalog sync` to fetch the upstream catalog.")
				return nil
			}

			fmt.Printf("%-30s  %-14s  %s\n", "NAME", "CATEGORY", "SOURCE")
			fmt.Printf("%-30s  %-14s  %s\n", "----", "--------", "------")
			for _, r := range rows {
				fmt.Printf("%-30s  %-14s  %s\n", r.name, r.category, r.source)
			}
			return nil
		},
	}
}

func newSyncCmd() *cobra.Command {
	var indexURL string

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Synchronise official upstream JSON indices to local disk",
		Long: `sync downloads the standardised upstream recipe catalog JSON index from
the CDN endpoint and stores it under ~/.lbmesh/catalog/.

Each recipe referenced in the index is also fetched and cached locally so
that recipes are available offline after the first sync.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			progress := func(msg string) {
				fmt.Println(msg)
			}

			if err := recipe.CatalogSync(indexURL, progress); err != nil {
				return err
			}

			return nil
		},
	}

	cmd.Flags().StringVar(
		&indexURL,
		"index-url",
		recipe.DefaultCatalogIndexURL,
		"upstream catalog index URL",
	)

	return cmd
}
