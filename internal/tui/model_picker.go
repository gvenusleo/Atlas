package tui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/liuyuxin/atlas/internal/runtime"
)

type modelPickerStage uint8

const (
	modelPickerClosed modelPickerStage = iota
	modelPickerModels
	modelPickerReasoning
)

type modelPicker struct {
	stage         modelPickerStage
	models        []runtime.ModelOption
	modelIndex    int
	effortIndex   int
	currentModel  string
	currentEffort string
}

type modelSelection struct {
	model  runtime.ModelOption
	effort string
}

func (p *modelPicker) open(models []runtime.ModelOption, currentModel, currentEffort string) {
	if len(models) == 0 {
		return
	}
	p.stage = modelPickerModels
	p.models = models
	p.modelIndex = 0
	p.effortIndex = 0
	p.currentModel = currentModel
	p.currentEffort = currentEffort
	for i, option := range models {
		if option.Value == currentModel {
			p.modelIndex = i
			break
		}
	}
}

func (p modelPicker) active() bool {
	return p.stage != modelPickerClosed
}

func (p *modelPicker) close() {
	p.stage = modelPickerClosed
}

// update handles one modal picker key and returns a completed selection when available.
func (p *modelPicker) update(key string) *modelSelection {
	if !p.active() {
		return nil
	}
	switch key {
	case "up":
		p.move(-1)
	case "down":
		p.move(1)
	case "esc":
		if p.stage == modelPickerReasoning {
			p.stage = modelPickerModels
		} else {
			p.close()
		}
	case "enter":
		return p.selectCurrent()
	}
	return nil
}

func (p *modelPicker) move(delta int) {
	count := len(p.models)
	index := &p.modelIndex
	if p.stage == modelPickerReasoning {
		count = len(p.models[p.modelIndex].ReasoningEfforts)
		index = &p.effortIndex
	}
	if count == 0 {
		return
	}
	*index = min(max(*index+delta, 0), count-1)
}

// selectCurrent advances to reasoning selection or completes the model selection.
func (p *modelPicker) selectCurrent() *modelSelection {
	if len(p.models) == 0 || p.modelIndex < 0 || p.modelIndex >= len(p.models) {
		return nil
	}
	selectedModel := p.models[p.modelIndex]
	if p.stage == modelPickerModels {
		switch len(selectedModel.ReasoningEfforts) {
		case 0:
			p.close()
			return &modelSelection{model: selectedModel}
		case 1:
			p.close()
			return &modelSelection{model: selectedModel, effort: selectedModel.ReasoningEfforts[0].Value}
		default:
			p.stage = modelPickerReasoning
			p.effortIndex = 0
			if selectedModel.Value == p.currentModel {
				for i, effort := range selectedModel.ReasoningEfforts {
					if effort.Value == p.currentEffort {
						p.effortIndex = i
						break
					}
				}
			}
			return nil
		}
	}

	if p.effortIndex < 0 || p.effortIndex >= len(selectedModel.ReasoningEfforts) {
		return nil
	}
	selection := &modelSelection{
		model:  selectedModel,
		effort: selectedModel.ReasoningEfforts[p.effortIndex].Value,
	}
	p.close()
	return selection
}

// render displays a bounded window around the selected model or reasoning effort.
func (p modelPicker) render(width, maxRows int) composerRender {
	maxRows = max(maxRows, 1)
	contentWidth := max(width-3, 1)
	title := "Select model"
	items := make([]string, len(p.models))
	selected := p.modelIndex
	if p.stage == modelPickerReasoning && len(p.models) > 0 {
		model := p.models[p.modelIndex]
		title = "Select reasoning effort for " + modelOptionName(model)
		items = make([]string, len(model.ReasoningEfforts))
		for i, effort := range model.ReasoningEfforts {
			items[i] = reasoningEffortName(effort)
		}
		selected = p.effortIndex
	} else {
		for i, model := range p.models {
			items[i] = modelOptionName(model)
		}
	}

	lines := make([]string, 0, maxRows)
	itemRows := maxRows
	if maxRows > 1 {
		lines = append(lines, "  "+messageStyle.Bold(true).Render(ansi.Truncate(title, contentWidth, "…")))
		itemRows--
	}
	start := pickerWindowStart(len(items), selected, itemRows)
	end := min(start+itemRows, len(items))
	for i := start; i < end; i++ {
		label := ansi.Truncate(items[i], contentWidth, "…")
		prefix := "  "
		if i == selected {
			prefix = userStyle.Render("› ")
			label = userStyle.Render(label)
		}
		lines = append(lines, prefix+label)
	}
	return composerRender{content: strings.Join(lines, "\n"), height: max(len(lines), 1)}
}

func pickerWindowStart(total, selected, rows int) int {
	if rows <= 0 || total <= rows {
		return 0
	}
	start := selected - rows/2
	return min(max(start, 0), total-rows)
}

func modelOptionName(option runtime.ModelOption) string {
	if option.Name != "" {
		return singleLineDisplayText(option.Name)
	}
	return singleLineDisplayText(option.Value)
}

func reasoningEffortName(option runtime.ReasoningEffortOption) string {
	if option.Name != "" {
		return singleLineDisplayText(option.Name)
	}
	return singleLineDisplayText(option.Value)
}

// singleLineDisplayText removes terminal controls and whitespace that would break row accounting.
func singleLineDisplayText(value string) string {
	return strings.Join(strings.Fields(ansi.Strip(value)), " ")
}
