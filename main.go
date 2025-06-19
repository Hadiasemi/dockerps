package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/table"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	// Clean, readable styles
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("86")).
			Bold(true).
			Padding(0, 1)

	filterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("205")).
			Bold(true)

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	runningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	stoppedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	pausedStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
)

type Container struct {
	ID      string `json:"ID"`
	Image   string `json:"Image"`
	Command string `json:"Command"`
	Created string `json:"CreatedAt"`
	Status  string `json:"Status"`
	Ports   string `json:"Ports"`
	Names   string `json:"Names"`
	State   string `json:"State"`
}

type model struct {
	table      table.Model
	containers []Container
	filter     textinput.Model
	filtering  bool
	err        error
	width      int
	height     int
}

func (m model) Init() tea.Cmd {
	return tea.Batch(loadContainers, textinput.Blink)
}

func loadContainers() tea.Msg {
	cmd := exec.Command("docker", "ps", "-a", "--format", "json")
	output, err := cmd.Output()
	if err != nil {
		return errMsg{err}
	}

	var containers []Container
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}
		var container Container
		if err := json.Unmarshal([]byte(line), &container); err != nil {
			continue
		}
		containers = append(containers, container)
	}

	return containersLoaded{containers}
}

type containersLoaded struct {
	containers []Container
}

type errMsg struct {
	err error
}

func (e errMsg) Error() string {
	return e.err.Error()
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

func formatStatus(state string) string {
	switch state {
	case "running":
		return runningStyle.Render("RUNNING")
	case "exited":
		return stoppedStyle.Render("STOPPED")
	case "paused":
		return pausedStyle.Render("PAUSED")
	case "restarting":
		return pausedStyle.Render("RESTART")
	case "removing":
		return stoppedStyle.Render("REMOVING")
	case "dead":
		return stoppedStyle.Render("DEAD")
	case "created":
		return pausedStyle.Render("CREATED")
	default:
		return strings.ToUpper(state)
	}
}

func formatTime(created string) string {
	if created == "" {
		return "—"
	}

	// Try to parse Docker timestamp
	t, err := time.Parse("2006-01-02 15:04:05 -0700 MST", created)
	if err != nil {
		return created[:10] // Just show date part if parsing fails
	}

	duration := time.Since(t)
	switch {
	case duration < time.Hour:
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	case duration < 24*time.Hour:
		return fmt.Sprintf("%dh", int(duration.Hours()))
	default:
		return fmt.Sprintf("%dd", int(duration.Hours()/24))
	}
}

func formatPorts(ports string) string {
	if ports == "" {
		return "—"
	}
	return truncate(strings.ReplaceAll(ports, "0.0.0.0:", ""), 25)
}

func (m model) containerToRow(c Container) table.Row {
	// Clean container name (remove leading slash if present)
	name := strings.TrimPrefix(c.Names, "/")

	// Get column widths from current table configuration
	cols := m.table.Columns()
	if len(cols) < 5 {
		// Fallback to default widths
		return table.Row{
			truncate(c.ID, 14),
			truncate(name, 25),
			truncate(c.Image, 30),
			formatStatus(c.State), // Don't truncate status - preserve color formatting
			truncate(formatPorts(c.Ports), 25),
		}
	}

	return table.Row{
		truncate(c.ID, cols[0].Width),
		truncate(name, cols[1].Width),
		truncate(c.Image, cols[2].Width),
		formatStatus(c.State), // Don't truncate status - preserve color formatting
		truncate(formatPorts(c.Ports), cols[4].Width),
	}
}

func filterContainers(containers []Container, filter string) []Container {
	if filter == "" {
		return containers
	}

	var filtered []Container
	filter = strings.ToLower(filter)

	for _, container := range containers {
		name := strings.ToLower(strings.TrimPrefix(container.Names, "/"))
		ports := strings.ToLower(container.Ports)

		if strings.Contains(name, filter) ||
			strings.Contains(strings.ToLower(container.Image), filter) ||
			strings.Contains(strings.ToLower(container.State), filter) ||
			strings.Contains(strings.ToLower(container.ID), filter) ||
			strings.Contains(ports, filter) {
			filtered = append(filtered, container)
		}
	}

	return filtered
}

func (m model) updateTable() model {
	filtered := filterContainers(m.containers, m.filter.Value())

	var rows []table.Row
	for _, container := range filtered {
		rows = append(rows, m.containerToRow(container))
	}

	m.table.SetRows(rows)
	return m
}

func (m model) updateTableSize() model {
	// Ensure minimum width
	if m.width < 80 {
		m.width = 80
	}

	// Fixed minimum widths for each column
	idWidth := 14     // Increased for better ID visibility
	statusWidth := 16 // Increased more for colored status text

	// Calculate remaining width for flexible columns
	fixedWidth := idWidth + statusWidth + 10 // 10 for padding/borders
	remainingWidth := m.width - fixedWidth

	// Distribute remaining width among name, image, and ports
	nameWidth := max(20, remainingWidth/3)
	imageWidth := max(25, remainingWidth/3)
	portsWidth := max(20, remainingWidth-nameWidth-imageWidth)

	columns := []table.Column{
		{Title: "ID", Width: idWidth},
		{Title: "NAME", Width: nameWidth},
		{Title: "IMAGE", Width: imageWidth},
		{Title: "STATUS", Width: statusWidth},
		{Title: "PORTS", Width: portsWidth},
	}

	m.table.SetColumns(columns)

	// Use most of the screen height for the table (leave space for title, filter, help)
	tableHeight := max(10, m.height-8)
	m.table.SetHeight(tableHeight)

	return m.updateTable()
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m = m.updateTableSize()
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.filtering = false
				m.filter.Blur()
				return m, nil
			case "enter":
				m.filtering = false
				m.filter.Blur()
				return m.updateTable(), nil
			default:
				m.filter, cmd = m.filter.Update(msg)
				m = m.updateTable()
				return m, cmd
			}
		} else {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "/":
				m.filtering = true
				return m, m.filter.Focus()
			case "r":
				return m, loadContainers
			default:
				m.table, cmd = m.table.Update(msg)
				return m, cmd
			}
		}

	case containersLoaded:
		m.containers = msg.containers
		m = m.updateTable()
		return m, nil

	case errMsg:
		m.err = msg.err
		return m, tea.Quit
	}

	return m, nil
}

func (m model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	var b strings.Builder

	// Title with container count
	b.WriteString(titleStyle.Render("🐳 Docker Container Manager"))
	b.WriteString(fmt.Sprintf(" (%d containers)\n\n", len(m.containers)))

	// Filter input
	if m.filtering {
		b.WriteString(filterStyle.Render("Filter: "))
		b.WriteString(m.filter.View())
		b.WriteString("\n\n")
	} else if m.filter.Value() != "" {
		b.WriteString(filterStyle.Render(fmt.Sprintf("Filter: %s", m.filter.Value())))
		b.WriteString("\n\n")
	}

	// Table
	b.WriteString(m.table.View())
	b.WriteString("\n\n")

	// Help
	if m.filtering {
		b.WriteString(helpStyle.Render("Enter: apply filter • Esc: cancel • Ctrl+C: quit"))
	} else {
		b.WriteString(helpStyle.Render("↑↓: navigate • /: filter • r: refresh • q: quit"))
	}
	b.WriteString("\n")

	return b.String()
}

func initialModel() model {
	// Create filter input
	filter := textinput.New()
	filter.Placeholder = "Type to filter containers..."
	filter.CharLimit = 50

	// Start with better default columns
	columns := []table.Column{
		{Title: "ID", Width: 14},
		{Title: "NAME", Width: 25},
		{Title: "IMAGE", Width: 30},
		{Title: "STATUS", Width: 16},
		{Title: "PORTS", Width: 25},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(15), // Will be updated dynamically
	)

	// Enhanced table styling for better readability
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true).
		Padding(0, 1) // Add padding for better spacing

	s.Selected = s.Selected.
		Foreground(lipgloss.Color("229")).
		Background(lipgloss.Color("57")).
		Bold(false)

	// Add cell styling for better spacing
	s.Cell = s.Cell.Padding(0, 1)

	t.SetStyles(s)

	return model{
		table:  t,
		filter: filter,
		width:  100, // Better default width
		height: 30,  // Better default height
	}
}

func main() {
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
