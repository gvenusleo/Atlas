package tool

import (
	"context"
	"testing"
)

func TestTodoWriteDefinition(t *testing.T) {
	tw := TodoWrite{}
	def := tw.Definition()
	if def.Name != "todo_write" {
		t.Fatalf("Name = %q, want %q", def.Name, "todo_write")
	}
}

func TestTodoWriteRun(t *testing.T) {
	tests := []struct {
		name      string
		arguments string
		wantErr   bool
		wantCount int
	}{
		{
			name:      "empty list",
			arguments: `{"todos":[]}`,
			wantCount: 0,
		},
		{
			name:      "three items",
			arguments: `{"todos":[{"content":"Read file","status":"completed"},{"content":"Edit file","status":"in_progress"},{"content":"Run tests","status":"pending"}]}`,
			wantCount: 3,
		},
		{
			name:      "invalid status",
			arguments: `{"todos":[{"content":"Task","status":"unknown"}]}`,
			wantErr:   true,
		},
		{
			name:      "empty content",
			arguments: `{"todos":[{"content":"  ","status":"pending"}]}`,
			wantErr:   true,
		},
		{
			name:      "invalid json",
			arguments: `{`,
			wantErr:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tw := TodoWrite{}
			result, err := tw.Run(context.Background(), tt.arguments)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Run() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if result == "" {
				t.Fatalf("Run() returned empty result")
			}
		})
	}
}
