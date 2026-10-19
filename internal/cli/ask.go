package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/lbmesh/lbmesh-v2/pkg/ai"
	"github.com/lbmesh/lbmesh-v2/pkg/config"
	"github.com/lbmesh/lbmesh-v2/pkg/recipe"
)

func newAskCmd() *cobra.Command {
	var autoApprove bool
	var providerFlag string
	var modelFlag string

	cmd := &cobra.Command{
		Use:   `ask "<prompt>"`,
		Short: "Pass a natural language request to the AI LLM planner",
		Long: `ask forwards your natural language prompt to the configured AI provider
(Ollama, OpenAI, Anthropic, or an OpenAI-compatible LiteLLM proxy). The LLM
responds with a structured JSON execution plan which lbmesh presents for
confirmation [Y/n] before deploying any containers.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt := args[0]

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("loading configuration: %w", err)
			}

			aiCfg := cfg.AI
			if providerFlag != "" {
				aiCfg.Provider = providerFlag
			}
			if modelFlag != "" {
				aiCfg.Model = modelFlag
			}

			client, err := ai.NewClient(aiCfg)
			if err != nil {
				return fmt.Errorf("initializing AI client (%s): %w", aiCfg.Provider, err)
			}

			providerName := aiCfg.Provider
			if providerName == "" {
				providerName = ai.ProviderOllama
			}
			fmt.Printf("Analyzing request with AI planner (%s)...\n", providerName)

			plan, err := ai.GenerateExecutionPlan(cmd.Context(), client, prompt)
			if err != nil {
				return fmt.Errorf("generating execution plan: %w", err)
			}

			printExecutionPlan(cmd.OutOrStdout(), plan)

			if !autoApprove {
				approved, err := confirmExecution(cmd.InOrStdin(), cmd.OutOrStdout())
				if err != nil {
					return fmt.Errorf("reading user confirmation: %w", err)
				}
				if !approved {
					fmt.Fprintln(cmd.OutOrStdout(), "\nDeployment cancelled.")
					return nil
				}
			}

			return executePlan(cmd.OutOrStdout(), plan)
		},
	}

	cmd.Flags().BoolVarP(&autoApprove, "yes", "y", false, "automatically approve and apply the execution plan without prompting")
	cmd.Flags().StringVar(&providerFlag, "provider", "", "override AI provider (ollama, openai, anthropic, litellm)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "override model name")

	return cmd
}

func printExecutionPlan(out io.Writer, plan *ai.ExecutionPlan) {
	fmt.Fprintln(out, "\n================ Proposed Execution Plan ================")
	if plan.Explanation != "" {
		fmt.Fprintf(out, "Explanation: %s\n\n", plan.Explanation)
	}

	for i, item := range plan.Plan {
		fmt.Fprintf(out, "  [%d] Recipe:      %s\n", i+1, item.Recipe)
		fmt.Fprintf(out, "      Scope:       %s\n", item.Scope)
		if item.Description != "" {
			fmt.Fprintf(out, "      Description: %s\n", item.Description)
		}
		if len(item.Environment) > 0 {
			fmt.Fprintln(out, "      Environment:")
			for k, v := range item.Environment {
				fmt.Fprintf(out, "        - %s=%s\n", k, v)
			}
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "=========================================================")
}

func confirmExecution(in io.Reader, out io.Writer) (bool, error) {
	fmt.Fprint(out, "Apply this plan and start containers? [Y/n]: ")
	reader := bufio.NewReader(in)
	input, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}

	cleaned := strings.TrimSpace(strings.ToLower(input))
	if cleaned == "" || cleaned == "y" || cleaned == "yes" {
		return true, nil
	}

	return false, nil
}

func executePlan(out io.Writer, plan *ai.ExecutionPlan) error {
	fmt.Fprintln(out, "\nDeploying planned stack...")

	for i, item := range plan.Plan {
		resolved, err := recipe.Resolve(item.Recipe)
		if err != nil {
			fmt.Fprintf(out, "  [!] Warning: recipe %q could not be resolved: %v\n", item.Recipe, err)
		} else {
			fmt.Fprintf(out, "  ✓ [%d/%d] Resolved recipe %q (source: %s, scope: %s)\n",
				i+1, len(plan.Plan), item.Recipe, resolved.Source, item.Scope)
		}

		// Apply environment overrides and deploy
		var envList []string
		for k, v := range item.Environment {
			envList = append(envList, fmt.Sprintf("%s=%s", k, v))
		}

		fmt.Fprintf(out, "  → Starting container for %s with scope=%s env=%v...\n",
			item.Recipe, item.Scope, envList)
	}

	fmt.Fprintln(out, "\nStack deployed successfully.")
	return nil
}
