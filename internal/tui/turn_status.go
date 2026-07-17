package tui

import (
	"fmt"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type turnPhase uint8

const (
	turnPhaseIdle turnPhase = iota
	turnPhaseWorking
	turnPhaseThinking
)

type turnStatus struct {
	phase     turnPhase
	startedAt time.Time
	spinner   spinner.Model
}

func newTurnStatus() turnStatus {
	return turnStatus{
		spinner: spinner.New(
			spinner.WithSpinner(spinner.MiniDot),
			spinner.WithStyle(mutedStyle),
		),
	}
}

func (s *turnStatus) start(now time.Time) tea.Cmd {
	s.phase = turnPhaseWorking
	s.startedAt = now
	return s.spinner.Tick
}

func (s *turnStatus) stop() {
	s.phase = turnPhaseIdle
	s.startedAt = time.Time{}
}

func (s turnStatus) active() bool {
	return s.phase != turnPhaseIdle
}

func (s *turnStatus) setPhase(phase turnPhase) {
	if s.active() {
		s.phase = phase
	}
}

func (s *turnStatus) update(msg spinner.TickMsg) tea.Cmd {
	if !s.active() {
		return nil
	}
	var cmd tea.Cmd
	s.spinner, cmd = s.spinner.Update(msg)
	return cmd
}

// viewAt renders the transient status row using wall-clock turn duration.
func (s turnStatus) viewAt(width int, now time.Time) string {
	if !s.active() {
		return ""
	}
	label := "Working"
	if s.phase == turnPhaseThinking {
		label = "Thinking"
	}
	elapsed := max(now.Sub(s.startedAt), 0)
	line := s.spinner.View() + " " + messageStyle.Bold(true).Render(label)
	line += " " + subtleStyle.Render(fmt.Sprintf("(%s • esc to interrupt)", formatTurnElapsed(elapsed)))
	return ansi.Truncate(line, max(width, 0), "…")
}

func formatTurnElapsed(elapsed time.Duration) string {
	seconds := int64(elapsed / time.Second)
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	if seconds < 3600 {
		return fmt.Sprintf("%dm %02ds", seconds/60, seconds%60)
	}
	return fmt.Sprintf("%dh %02dm %02ds", seconds/3600, seconds%3600/60, seconds%60)
}
