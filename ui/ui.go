package ui

import (
	"strings"

	"sptly/lyrics"

	tea "github.com/charmbracelet/bubbletea"
	gloss "github.com/charmbracelet/lipgloss"
)

// Holds the state for the lyrics UI
type Model struct {
	Lines      []lyrics.LRCLine
	Index      int
	Width      int
	Height     int
	UpdateChan chan struct{}

	beforeStyle  gloss.Style
	currentStyle gloss.Style
	afterStyle   gloss.Style
}

// Creates a Model with optional initial lines
func New(lines []lyrics.LRCLine) *Model {
	return &Model{
		Lines:       lines,
		beforeStyle: gloss.NewStyle().Foreground(gloss.Color("#585b70")),
		currentStyle: gloss.NewStyle().
			Foreground(gloss.Color("#89b4fa")).Bold(true),
		afterStyle: gloss.NewStyle().Foreground(gloss.Color("#cdd6f4")),
	}
}

// Starts listening for updates
func (m *Model) Init() tea.Cmd {
	return waitForUpdate(m.UpdateChan)
}

// Handles messages: window size, keypress, or update signals
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width, m.Height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return m, tea.Quit
		}
	case struct{}:
		// received signal to redraw
	}
	return m, waitForUpdate(m.UpdateChan)
}

// Renders the lyrics UI
func (m *Model) View() string {
	if m.Width == 0 || m.Height == 0 || len(m.Lines) == 0 {
		return ""
	}

	// Determine which line to center
	centerIdx := m.Index
	if centerIdx < 0 {
		centerIdx = 0
	} else if centerIdx >= len(m.Lines) {
		centerIdx = len(m.Lines) - 1
	}

	// Style current line
	var currentStyle gloss.Style
	if m.Index >= 0 {
		currentStyle = m.currentStyle
	} else {
		currentStyle = m.afterStyle
	}

	currentLine := currentStyle.Width(m.Width).Align(gloss.Center).Render(m.Lines[centerIdx].Text)
	currentLines := strings.Split(currentLine, "\n")
	currentHeight := len(currentLines)

	topPadding := (m.Height - currentHeight) / 2
	bottomPadding := m.Height - topPadding - currentHeight

	lines := make([]string, m.Height)

	// Fill lines above the current
	for i := 0; i < topPadding; i++ {
		idx := centerIdx - topPadding + i
		if idx >= 0 {
			lines[i] = m.beforeStyle.Width(m.Width).Align(gloss.Center).Render(m.Lines[idx].Text)
		} else {
			lines[i] = ""
		}
	}

	// Add current line(s)
	for i, l := range currentLines {
		lines[topPadding+i] = l
	}

	// Fill lines below the current
	for i := 0; i < bottomPadding; i++ {
		idx := centerIdx + i + 1
		if idx < len(m.Lines) {
			lines[topPadding+currentHeight+i] = m.afterStyle.Width(m.Width).Align(gloss.Center).Render(m.Lines[idx].Text)
		} else {
			lines[topPadding+currentHeight+i] = ""
		}
	}

	return gloss.JoinVertical(gloss.Center, lines...)
}

// waits for a signal on the update channel
func waitForUpdate(ch chan struct{}) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}
