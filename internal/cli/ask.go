package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newAskCmd() *cobra.Command {
	return &cobra.Command{
		Use:   `ask "<prompt>"`,
		Short: "Pass a natural language request to the AI LLM planner",
		Long: `ask forwards your natural language prompt to the configured AI provider
(Ollama, OpenAI, Anthropic, or an OpenAI-compatible LiteLLM proxy). The LLM
responds with a structured JSON execution plan which lbmesh presents for
confirmation before deploying any containers.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(sub-task-11): call pkg/ai to build execution plan.
			fmt.Printf("ask: prompt=%q\n", args[0])
			return nil
		},
	}
}
