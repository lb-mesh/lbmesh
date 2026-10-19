package ai

import (
	"context"
	"strings"
	"testing"
)

func TestParseExecutionPlan_Object(t *testing.T) {
	raw := `{
		"explanation": "Spin up Postgres and Redis together.",
		"plan": [
			{
				"recipe": "postgresql",
				"scope": "system",
				"environment": {
					"POSTGRES_DB": "testdb"
				},
				"description": "Primary relational database"
			},
			{
				"recipe": "redis",
				"scope": "project",
				"description": "Cache store"
			}
		]
	}`

	plan, err := ParseExecutionPlan(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing plan: %v", err)
	}

	if plan.Explanation != "Spin up Postgres and Redis together." {
		t.Errorf("unexpected explanation: %s", plan.Explanation)
	}
	if len(plan.Plan) != 2 {
		t.Fatalf("expected 2 items, got %d", len(plan.Plan))
	}
	if plan.Plan[0].Recipe != "postgresql" || plan.Plan[0].Scope != "system" {
		t.Errorf("unexpected item 0: %+v", plan.Plan[0])
	}
	if plan.Plan[0].Environment["POSTGRES_DB"] != "testdb" {
		t.Errorf("unexpected env: %+v", plan.Plan[0].Environment)
	}
	if plan.Plan[1].Recipe != "redis" || plan.Plan[1].Scope != "project" {
		t.Errorf("unexpected item 1: %+v", plan.Plan[1])
	}
}

func TestParseExecutionPlan_MarkdownFenced(t *testing.T) {
	raw := "Here is the proposed deployment plan:\n\n```json\n" + `{
		"explanation": "Spin up Nginx server.",
		"plan": [
			{
				"recipe": "nginx",
				"scope": "project"
			}
		]
	}` + "\n```\nEnjoy!"

	plan, err := ParseExecutionPlan(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing plan: %v", err)
	}

	if len(plan.Plan) != 1 || plan.Plan[0].Recipe != "nginx" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestParseExecutionPlan_Array(t *testing.T) {
	raw := `[
		{
			"recipe": "postgresql",
			"environment": {
				"POSTGRES_PASSWORD": "secret"
			}
		}
	]`

	plan, err := ParseExecutionPlan(raw)
	if err != nil {
		t.Fatalf("unexpected error parsing plan: %v", err)
	}

	if len(plan.Plan) != 1 || plan.Plan[0].Recipe != "postgresql" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Plan[0].Scope != "project" {
		t.Errorf("expected default scope 'project', got %s", plan.Plan[0].Scope)
	}
}

func TestParseExecutionPlan_Invalid(t *testing.T) {
	_, err := ParseExecutionPlan("I am not sure what to deploy.")
	if err == nil {
		t.Fatal("expected error parsing invalid text, got nil")
	}
}

type fakeClient struct {
	response string
}

func (f *fakeClient) Generate(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if !strings.Contains(systemPrompt, "LBMesh AI Orchestrator") {
		return "", nil
	}
	return f.response, nil
}

func TestGenerateExecutionPlan(t *testing.T) {
	client := &fakeClient{
		response: `{"explanation":"Deploy Postgres","plan":[{"recipe":"postgresql"}]}`,
	}

	plan, err := GenerateExecutionPlan(context.Background(), client, "deploy postgres")
	if err != nil {
		t.Fatalf("unexpected error generating plan: %v", err)
	}

	if len(plan.Plan) != 1 || plan.Plan[0].Recipe != "postgresql" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}
