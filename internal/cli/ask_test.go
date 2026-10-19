package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/lbmesh/lbmesh-v2/pkg/ai"
)

func TestConfirmExecution_Approve(t *testing.T) {
	cases := []string{"y\n", "Y\n", "yes\n", "YES\n", "\n"}

	for _, input := range cases {
		in := strings.NewReader(input)
		var out bytes.Buffer

		approved, err := confirmExecution(in, &out)
		if err != nil {
			t.Fatalf("unexpected error on input %q: %v", input, err)
		}
		if !approved {
			t.Errorf("expected approval for input %q, got false", input)
		}
	}
}

func TestConfirmExecution_Decline(t *testing.T) {
	cases := []string{"n\n", "N\n", "no\n", "NO\n", "other\n"}

	for _, input := range cases {
		in := strings.NewReader(input)
		var out bytes.Buffer

		approved, err := confirmExecution(in, &out)
		if err != nil {
			t.Fatalf("unexpected error on input %q: %v", input, err)
		}
		if approved {
			t.Errorf("expected decline for input %q, got true", input)
		}
	}
}

func TestPrintExecutionPlan(t *testing.T) {
	var out bytes.Buffer
	plan := &ai.ExecutionPlan{
		Explanation: "Deploy postgres and redis",
		Plan: []ai.PlanItem{
			{
				Recipe:      "postgresql",
				Scope:       "system",
				Description: "Primary DB",
				Environment: map[string]string{
					"POSTGRES_DB": "testdb",
				},
			},
			{
				Recipe: "redis",
				Scope:  "project",
			},
		},
	}

	printExecutionPlan(&out, plan)
	output := out.String()

	if !strings.Contains(output, "Proposed Execution Plan") {
		t.Errorf("missing header: %s", output)
	}
	if !strings.Contains(output, "postgresql") || !strings.Contains(output, "redis") {
		t.Errorf("missing recipes: %s", output)
	}
	if !strings.Contains(output, "POSTGRES_DB=testdb") {
		t.Errorf("missing env vars: %s", output)
	}
}

func TestExecutePlan(t *testing.T) {
	var out bytes.Buffer
	plan := &ai.ExecutionPlan{
		Plan: []ai.PlanItem{
			{
				Recipe: "postgresql",
				Scope:  "system",
				Environment: map[string]string{
					"POSTGRES_DB": "testdb",
				},
			},
		},
	}

	err := executePlan(&out, plan)
	if err != nil {
		t.Fatalf("unexpected error executing plan: %v", err)
	}

	output := out.String()
	if !strings.Contains(output, "Stack deployed successfully") {
		t.Errorf("unexpected output: %s", output)
	}
}

func TestAskCmdFlags(t *testing.T) {
	cmd := newAskCmd()
	if cmd.Use != `ask "<prompt>"` {
		t.Errorf("unexpected use: %s", cmd.Use)
	}

	yesFlag := cmd.Flag("yes")
	if yesFlag == nil || yesFlag.Shorthand != "y" {
		t.Errorf("expected -y/--yes flag")
	}

	providerFlag := cmd.Flag("provider")
	if providerFlag == nil {
		t.Errorf("expected --provider flag")
	}
}
