package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var completionShells = []string{"bash", "zsh", "fish", "powershell"}

func newCompletionCmd() *cobra.Command {
	var printOnly bool

	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Install or print the shell completion script",
		Long: `completion installs the shell completion script into your home directory.

With no argument the shell is detected from $SHELL. Pass a shell name to choose
one explicitly. Use --print (or pass a shell name with --print) to write the
script to stdout instead of installing it, e.g.:

  source <(lbmesh completion zsh --print)

Install locations:
  bash  ~/.local/share/bash-completion/completions/lbmesh
  zsh   ~/.zsh/completions/_lbmesh   (directory must be on your fpath)
  fish  ~/.config/fish/completions/lbmesh.fish`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: completionShells,
		// Completion must not create ~/.lbmesh or open the state database.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error { return nil },
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = args[0]
			} else {
				shell = filepath.Base(os.Getenv("SHELL"))
				if shell == "." || shell == "" {
					return fmt.Errorf("cannot detect shell from $SHELL; specify one of: %v", completionShells)
				}
			}

			var buf bytes.Buffer
			if err := genCompletion(cmd.Root(), shell, &buf); err != nil {
				return err
			}
			if printOnly {
				_, err := cmd.OutOrStdout().Write(buf.Bytes())
				return err
			}
			return installCompletion(cmd.OutOrStdout(), shell, buf.Bytes())
		},
	}

	cmd.Flags().BoolVar(&printOnly, "print", false, "write the script to stdout instead of installing it")
	return cmd
}

func genCompletion(root *cobra.Command, shell string, w io.Writer) error {
	switch shell {
	case "bash":
		return root.GenBashCompletionV2(w, true)
	case "zsh":
		return root.GenZshCompletion(w)
	case "fish":
		return root.GenFishCompletion(w, true)
	case "powershell":
		return root.GenPowerShellCompletionWithDesc(w)
	default:
		return fmt.Errorf("unsupported shell %q (supported: %v)", shell, completionShells)
	}
}

func installCompletion(out io.Writer, shell string, script []byte) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolving home directory: %w", err)
	}

	var path, hint string
	switch shell {
	case "bash":
		path = filepath.Join(home, ".local", "share", "bash-completion", "completions", "lbmesh")
		hint = "Requires bash-completion v2; restart your shell to activate."
	case "zsh":
		path = filepath.Join(home, ".zsh", "completions", "_lbmesh")
		hint = "Ensure ~/.zshrc contains, before compinit:\n  fpath=(~/.zsh/completions $fpath)\n  autoload -U compinit && compinit\nThen restart your shell."
	case "fish":
		path = filepath.Join(home, ".config", "fish", "completions", "lbmesh.fish")
		hint = "Restart your shell to activate."
	default:
		return fmt.Errorf("automatic install is not supported for %s; use: lbmesh completion %s --print", shell, shell)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, script, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	_, err = fmt.Fprintf(out, "Installed %s completion to %s\n%s\n", shell, path, hint)
	return err
}
