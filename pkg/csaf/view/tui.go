package view

import (
	"fmt"

	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// model represents the Bubble Tea model for the TUI
type model struct {
	document Document
	ready    bool
	err      error
}

// newModel creates a new Bubble Tea model with the given document
func newModel(doc Document) model {
	return model{
		document: doc,
		ready:    true,
	}
}

// Init implements the bubbletea.Model interface
func (m model) Init() tea.Cmd {
	return nil
}

// Update implements the bubbletea.Model interface
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements the bubbletea.Model interface
func (m model) View() string {
	if !m.ready {
		return "Loading..."
	}

	if m.err != nil {
		return fmt.Sprintf("Error: %v\n\nPress 'q' to quit.", m.err)
	}

	// Styling
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4"))
	
	labelStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#04B575"))
	
	valueStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFFFF"))

	// Format the document information
	var docType string
	category := m.document.Document.Category
	
	// TODO: Future enhancement - different rendering logic based on document type
	// For now, we render the same TUI for both types, but this is where
	// we could diverge the logic between 'csaf_vex' and 'csaf_security_advisory'
	switch category {
	case "csaf_vex":
		docType = "VEX Document"
	case "csaf_security_advisory":
		docType = "Security Advisory"
	default:
		docType = "CSAF Document"
	}

	content := fmt.Sprintf(
		"%s\n\n%s %s\n\n%s %s\n\n%s %s\n\n%s",
		titleStyle.Render(fmt.Sprintf("CSAF %s Viewer", docType)),
		labelStyle.Render("ID:"),
		valueStyle.Render(m.document.Document.Tracking.ID),
		labelStyle.Render("Title:"),
		valueStyle.Render(m.document.Document.Title),
		labelStyle.Render("Category:"),
		valueStyle.Render(category),
		lipgloss.NewStyle().Faint(true).Render("Press 'q' or Ctrl+C to quit"),
	)

	return content
}

// RunTUI starts the Bubble Tea TUI program for viewing a CSAF document
func RunTUI(doc Document) error {
	m := newModel(doc)
	p := tea.NewProgram(m, tea.WithAltScreen())
	
	_, err := p.Run()
	return err
}
