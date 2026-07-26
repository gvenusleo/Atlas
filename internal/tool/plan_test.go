package tool

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestUpdatePlanDefinition(t *testing.T) {
	definition := (UpdatePlan{}).Definition()
	if definition.Name != "update_plan" {
		t.Fatalf("Name = %q, want %q", definition.Name, "update_plan")
	}
	properties := definition.Parameters["properties"].(map[string]any)
	if _, ok := properties["plan"]; !ok {
		t.Fatalf("parameters missing plan property: %#v", definition.Parameters)
	}
}

func TestUpdatePlanRun(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		wantErr   bool
	}{
		{name: "empty plan", arguments: `{"plan":[]}`},
		{name: "three steps", arguments: `{"plan":[{"step":"Read file","status":"completed"},{"step":"Edit file","status":"in_progress"},{"step":"Run tests","status":"pending"}]}`},
		{name: "missing plan", arguments: `{}`, wantErr: true},
		{name: "null plan", arguments: `{"plan":null}`, wantErr: true},
		{name: "multiple active steps", arguments: `{"plan":[{"step":"Edit file","status":"in_progress"},{"step":"Run tests","status":"in_progress"}]}`, wantErr: true},
		{name: "control character", arguments: `{"plan":[{"step":"safe\rspoofed","status":"pending"}]}`, wantErr: true},
		{name: "invalid status", arguments: `{"plan":[{"step":"Task","status":"unknown"}]}`, wantErr: true},
		{name: "empty step", arguments: `{"plan":[{"step":"  ","status":"pending"}]}`, wantErr: true},
		{name: "invalid json", arguments: `{`, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := (UpdatePlan{}).Run(context.Background(), test.arguments)
			if test.wantErr {
				if err == nil {
					t.Fatal("Run() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result != "Plan updated" {
				t.Fatalf("Run() = %q, want %q", result, "Plan updated")
			}
		})
	}
}

func TestUpdatePlanRejectsOversizedPlan(t *testing.T) {
	plan := make([]map[string]string, maxPlanSteps+1)
	for i := range plan {
		plan[i] = map[string]string{"step": "Task", "status": "pending"}
	}
	arguments, err := json.Marshal(map[string]any{"plan": plan})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if _, err := ParsePlan(string(arguments)); err == nil {
		t.Fatal("ParsePlan() accepted too many steps")
	}

	longStep := strings.Repeat("界", maxPlanStepRunes+1)
	arguments, err = json.Marshal(map[string]any{"plan": []map[string]string{{"step": longStep, "status": "pending"}}})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if _, err := ParsePlan(string(arguments)); err == nil {
		t.Fatal("ParsePlan() accepted an oversized step")
	}
}

func TestUpdatePlanMetadata(t *testing.T) {
	metadata := (UpdatePlan{}).Metadata(`{"plan":[{"step":"Edit file","status":"in_progress"}]}`, "")
	if len(metadata.Plan) != 1 || metadata.Plan[0].Step != "Edit file" {
		t.Fatalf("Metadata() = %#v", metadata)
	}
}
