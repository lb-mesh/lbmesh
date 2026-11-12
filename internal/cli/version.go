package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/internal/version"
)

func newVersionCmd() *cobra.Command {
	var short bool
	var output string

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the lbmesh version and build information",
		Long: `version prints the lbmesh release version together with the build date,
Go toolchain, and platform the binary was built for. Use -o json for the
full build information, including the git commit.`,
		Args: cobra.NoArgs,
		// Reporting the version must not create ~/.lbmesh or open the state
		// database, so override the root command's setup hook.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			return printVersion(cmd.OutOrStdout(), version.Get(), short, output)
		},
	}

	cmd.Flags().BoolVar(&short, "short", false, "print only the version number")
	cmd.Flags().StringVarP(&output, "output", "o", "text", "output format: text | json")

	return cmd
}

func printVersion(out io.Writer, info version.Info, short bool, output string) error {
	switch output {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(info)
	case "text", "":
	default:
		return fmt.Errorf("unknown output format %q (supported: text, json)", output)
	}

	if short {
		_, err := fmt.Fprintln(out, info.Version)
		return err
	}
	_, err := fmt.Fprintf(out, "lbmesh %s\n  built:    %s\n  go:       %s\n  platform: %s\n",
		info.Version, readableDate(info.Date), info.GoVersion, info.Platform)
	return err
}

// readableDate renders an RFC 3339 build timestamp as "Jan 2, 2006".
// Values that don't parse (e.g. "unknown") are returned unchanged.
func readableDate(s string) string {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return s
	}
	return t.UTC().Format("Jan 2, 2006")
}
