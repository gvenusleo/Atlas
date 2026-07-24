package tui

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/runtime"
)

func TestModelPickerSelectsModelAndReasoningEffort(t *testing.T) {
	models := pickerTestModels()
	var picker modelPicker
	picker.open(models, models[0].Value, "xhigh")

	if picker.modelIndex != 0 {
		t.Fatalf("initial model index = %d, want 0", picker.modelIndex)
	}
	if selection := picker.update("enter"); selection != nil || picker.stage != modelPickerReasoning || picker.effortIndex != 1 {
		t.Fatalf("reasoning picker state = %#v, selection = %#v", picker, selection)
	}
	picker.update("up")
	selection := picker.update("enter")
	if selection == nil || selection.model.Value != models[0].Value || selection.effort != "high" {
		t.Fatalf("selection = %#v", selection)
	}
	if picker.active() {
		t.Fatal("picker remained active after selection")
	}
}

func TestModelPickerUsesModelReasoningDefaults(t *testing.T) {
	models := pickerTestModels()
	var picker modelPicker
	picker.open(models, models[0].Value, "xhigh")
	picker.update("down")

	selection := picker.update("enter")
	if selection == nil || selection.model.Value != models[1].Value || selection.effort != "" {
		t.Fatalf("model without reasoning selection = %#v", selection)
	}

	picker.open(models, models[0].Value, "xhigh")
	picker.update("down")
	picker.update("down")
	selection = picker.update("enter")
	if selection == nil || selection.model.Value != models[2].Value || selection.effort != "medium" {
		t.Fatalf("single reasoning selection = %#v", selection)
	}
}

func TestModelPickerIgnoresEscape(t *testing.T) {
	models := pickerTestModels()
	var picker modelPicker
	picker.open(models, models[0].Value, "high")
	picker.update("enter")
	picker.update("esc")
	if picker.stage != modelPickerReasoning {
		t.Fatalf("picker stage after escape = %d, want reasoning", picker.stage)
	}
}

func TestModelPickerRenderIsBoundedAndKeepsSelectionVisible(t *testing.T) {
	models := make([]runtime.ModelOption, 20)
	for i := range models {
		models[i] = runtime.ModelOption{Value: strings.Repeat("model", 10) + string(rune('a'+i))}
	}
	var picker modelPicker
	picker.open(models, models[15].Value, "")

	rendered := picker.render(24, 5, false)
	lines := strings.Split(rendered.content, "\n")
	if len(lines) != 5 || !strings.Contains(ansi.Strip(rendered.content), "›") {
		t.Fatalf("picker render = %q", ansi.Strip(rendered.content))
	}
	for _, line := range lines {
		if got := ansi.StringWidth(line); got > 24 {
			t.Fatalf("rendered line width = %d, want at most 24: %q", got, ansi.Strip(line))
		}
	}
}

func TestModelPickerRenderSeparatesTitleAndUsesForegroundSelection(t *testing.T) {
	models := pickerTestModels()
	var picker modelPicker
	picker.open(models, models[0].Value, "")

	rendered := picker.render(60, 5, false)
	rawLines := strings.Split(rendered.content, "\n")
	if len(rawLines) != 5 || ansi.Strip(rawLines[0]) != "  Select model" || rawLines[1] != "" {
		t.Fatalf("picker title spacing = %q", ansi.Strip(rendered.content))
	}
	selected := ansi.Strip(rawLines[2])
	if rawLines[2] != lightTheme.highlight.Render(selected) {
		t.Fatalf("selected model does not use one foreground style: %q", rawLines[2])
	}
	if !reflect.DeepEqual(lightTheme.highlight.GetBackground(), lightTheme.text.GetBackground()) {
		t.Fatal("selected model style has a background color")
	}
}

func TestPickerDisplayNamesAreSingleLine(t *testing.T) {
	modelName := modelOptionName(runtime.ModelOption{Name: "\x1b[31mModel\nA\x1b[0m"})
	effortName := reasoningEffortName(runtime.ReasoningEffortOption{Name: "Extra\tHigh"})

	if modelName != "Model A" || effortName != "Extra High" {
		t.Fatalf("display names = model:%q effort:%q", modelName, effortName)
	}
}

func pickerTestModels() []runtime.ModelOption {
	return []runtime.ModelOption{
		{
			Value:         "provider/model-a",
			Name:          "Model A",
			ContextWindow: 1000,
			ReasoningEfforts: []runtime.ReasoningEffortOption{
				{Value: "high", Name: "High"},
				{Value: "xhigh", Name: "XHigh"},
			},
		},
		{Value: "provider/model-b", Name: "Model B", ContextWindow: 1500},
		{
			Value:         "provider/model-c",
			Name:          "Model C",
			ContextWindow: 2000,
			ReasoningEfforts: []runtime.ReasoningEffortOption{
				{Value: "medium", Name: "Medium"},
			},
		},
	}
}
