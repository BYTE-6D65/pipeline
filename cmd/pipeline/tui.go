package main

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/BYTE-6D65/pipeline/pkg/testdata"
)

// View states
type viewState int

const (
	viewMainMenu viewState = iota
	viewTestMenu
	viewRunningTest
	viewResults
)

// Message types
type messageType int

const (
	msgInfo messageType = iota
	msgWarning
	msgError
	msgSuccess
)

// UserMessage represents a dynamic message to the user
type userMessage struct {
	msgType messageType
	text    string
}

// Model holds the state of the TUI
type model struct {
	state      viewState
	cursor     int
	choices    []string
	selected   map[int]struct{}
	width      int
	height     int

	// Test execution
	testScenario testdata.TestScenario
	testRunning  bool
	testResults  *testdata.PerformanceMetrics
	testError    error

	// Test progress
	testProgress *testProgressMsg

	// Animation
	spinnerFrame int

	// Dynamic user messages
	userMessage *userMessage
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#7D56F4")).
		PaddingLeft(2)

	menuItemStyle = lipgloss.NewStyle().
		PaddingLeft(4)

	selectedItemStyle = lipgloss.NewStyle().
		PaddingLeft(2).
		Foreground(lipgloss.Color("#7D56F4")).
		Bold(true)

	helpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#626262")).
		PaddingTop(1).
		PaddingLeft(2)

	resultsStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#7D56F4")).
		Padding(1, 2)

	infoMessageStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00A9E0")).
		Foreground(lipgloss.Color("#00A9E0")).
		Padding(0, 2).
		MarginTop(1).
		MarginLeft(2)

	warningMessageStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FFB800")).
		Foreground(lipgloss.Color("#FFB800")).
		Padding(0, 2).
		MarginTop(1).
		MarginLeft(2)

	errorMessageStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#FF5555")).
		Foreground(lipgloss.Color("#FF5555")).
		Padding(0, 2).
		MarginTop(1).
		MarginLeft(2)

	successMessageStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#50FA7B")).
		Foreground(lipgloss.Color("#50FA7B")).
		Padding(0, 2).
		MarginTop(1).
		MarginLeft(2)
)

// Messages
type testCompleteMsg struct {
	results *testdata.PerformanceMetrics
	err     error
}

type testProgressMsg struct {
	eventCount    int
	totalEvents   int
	currentRate   float64
	elapsedTime   time.Duration
}

type tickMsg struct{}

// Global program reference for sending progress updates
var globalProgram *tea.Program

func initialModel() model {
	return model{
		state:    viewMainMenu,
		cursor:   0,
		selected: make(map[int]struct{}),
		choices: []string{
			"🧪 Run Performance Tests",
			"❌ Exit",
		},
		spinnerFrame: 0,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func tick() tea.Cmd {
	return tea.Tick(100*1000000, func(t time.Time) tea.Msg { // 100ms
		return tickMsg{}
	})
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyPress(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case testCompleteMsg:
		m.testRunning = false
		m.testResults = msg.results
		m.testError = msg.err
		m.state = viewResults

		// Set success message if test passed
		if msg.err == nil && msg.results != nil {
			m.userMessage = &userMessage{
				msgType: msgSuccess,
				text:    fmt.Sprintf("Test completed successfully!\n   Processed %d events in %v",
					msg.results.EventCount,
					msg.results.Duration.Round(time.Millisecond)),
			}
		}
		return m, nil

	case testProgressMsg:
		m.testProgress = &msg
		return m, nil

	case tickMsg:
		if m.state == viewRunningTest {
			m.spinnerFrame = (m.spinnerFrame + 1) % 10
			return m, tick()
		}
	}

	return m, nil
}

func (m model) handleKeyPress(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case viewMainMenu:
		return m.handleMainMenuKeys(msg)
	case viewTestMenu:
		return m.handleTestMenuKeys(msg)
	case viewResults:
		return m.handleResultsKeys(msg)
	}
	return m, nil
}

func (m model) handleMainMenuKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		// Clear message when navigating
		m.userMessage = nil

	case "down", "j":
		if m.cursor < len(m.choices)-1 {
			m.cursor++
		}
		// Clear message when navigating
		m.userMessage = nil

	case "enter", " ":
		switch m.cursor {
		case 0: // Performance Tests
			m.state = viewTestMenu
			m.cursor = 0
			m.userMessage = nil
			m.choices = []string{
				"📈 Normal Load Test (1,000 events)",
				"💪 Massive Payload Test (100 events @ 1MB each)",
				"🔥 Adversarial Test (500 events)",
				"⬅️  Back to Main Menu",
			}
		case 1: // Exit
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) handleTestMenuKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}

	case "down", "j":
		if m.cursor < len(m.choices)-1 {
			m.cursor++
		}

	case "enter", " ":
		if m.cursor == 3 { // Back button
			m.state = viewMainMenu
			m.cursor = 0
			m.choices = []string{
				"🧪 Run Performance Tests",
				"❌ Exit",
			}
		} else {
			// Run test
			var scenario testdata.TestScenario
			var eventCount int

			switch m.cursor {
			case 0: // Normal
				scenario = testdata.ScenarioNormal
				eventCount = 1000
			case 1: // Massive
				scenario = testdata.ScenarioMassive
				eventCount = 100
			case 2: // Adversarial
				scenario = testdata.ScenarioAdversarial
				eventCount = 500
			}

			m.testScenario = scenario
			m.testRunning = true
			m.state = viewRunningTest
			m.testProgress = nil // Reset progress
			return m, tea.Batch(runTest(scenario, eventCount), tick())
		}
	}
	return m, nil
}

func (m model) handleResultsKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit

	case "enter", " ", "esc":
		// Go back to test menu
		m.state = viewTestMenu
		m.cursor = 0
		m.testResults = nil
		m.testError = nil
		m.userMessage = nil // Clear message when going back
		m.choices = []string{
			"📈 Normal Load Test (1,000 events)",
			"💪 Massive Payload Test (100 events @ 1MB each)",
			"🔥 Adversarial Test (500 events)",
			"⬅️  Back to Main Menu",
		}
	}
	return m, nil
}

func (m model) View() string {
	switch m.state {
	case viewMainMenu:
		return m.renderMainMenu()
	case viewTestMenu:
		return m.renderTestMenu()
	case viewRunningTest:
		return m.renderRunningTest()
	case viewResults:
		return m.renderResults()
	}
	return ""
}

func (m model) renderMainMenu() string {
	s := titleStyle.Render("🎮 Pipeline Demo - Interactive Menu") + "\n\n"

	for i, choice := range m.choices {
		cursor := "  "
		if m.cursor == i {
			cursor = "▶ "
			s += selectedItemStyle.Render(cursor + choice) + "\n"
		} else {
			s += menuItemStyle.Render(cursor + choice) + "\n"
		}
	}

	s += helpStyle.Render("\nUse ↑/↓ or j/k to navigate • Enter to select • q to quit")

	// Render user message if present
	if m.userMessage != nil {
		s += "\n" + m.renderUserMessage()
	}

	return s
}

func (m model) renderTestMenu() string {
	s := titleStyle.Render("🧪 Performance Test Scenarios") + "\n\n"

	for i, choice := range m.choices {
		cursor := "  "
		if m.cursor == i {
			cursor = "▶ "
			s += selectedItemStyle.Render(cursor + choice) + "\n"
		} else {
			s += menuItemStyle.Render(cursor + choice) + "\n"
		}
	}

	s += helpStyle.Render("\nUse ↑/↓ or j/k to navigate • Enter to select • q to quit")
	return s
}

func (m model) renderRunningTest() string {
	s := titleStyle.Render(fmt.Sprintf("⚡ Running %s test...", m.testScenario)) + "\n\n"

	// Show spinner
	s += "  " + m.spinner() + " Processing events...\n\n"

	// Show progress if available
	if m.testProgress != nil {
		progress := m.testProgress
		percentage := float64(progress.eventCount) / float64(progress.totalEvents) * 100

		// Progress bar
		barWidth := 40
		filled := int(percentage / 100 * float64(barWidth))
		bar := "["
		for i := 0; i < barWidth; i++ {
			if i < filled {
				bar += "█"
			} else {
				bar += "░"
			}
		}
		bar += "]"

		s += fmt.Sprintf("  %s %.1f%%\n\n", bar, percentage)
		s += fmt.Sprintf("  Events:     %d / %d\n", progress.eventCount, progress.totalEvents)
		s += fmt.Sprintf("  Elapsed:    %v\n", progress.elapsedTime.Round(time.Millisecond))
		s += fmt.Sprintf("  Rate:       %.0f events/sec\n", progress.currentRate)
	} else {
		s += "  Initializing...\n"
	}

	s += "\n"
	s += helpStyle.Render("Running... Press Ctrl+C to cancel")
	return s
}

func (m model) renderResults() string {
	if m.testError != nil {
		errMsg := errorMessageStyle.Render("❌ Test failed!\n   " + m.testError.Error())
		return titleStyle.Render("❌ Test Failed") + "\n\n" +
			errMsg + "\n\n" +
			helpStyle.Render("Press Enter to go back")
	}

	if m.testResults == nil {
		return "No results available"
	}

	header := titleStyle.Render("✅ Test Complete") + "\n"

	// Show success message if present
	var successMsg string
	if m.userMessage != nil {
		successMsg = "\n" + m.renderUserMessage() + "\n"
	}

	metrics := resultsStyle.Render(testdata.FormatMetrics(m.testResults))
	footer := helpStyle.Render("\nPress Enter to run another test • q to quit")

	return header + successMsg + metrics + footer
}

func (m model) spinner() string {
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	return frames[m.spinnerFrame]
}

func (m model) renderUserMessage() string {
	if m.userMessage == nil {
		return ""
	}

	var style lipgloss.Style
	var icon string

	switch m.userMessage.msgType {
	case msgInfo:
		style = infoMessageStyle
		icon = "ℹ️ "
	case msgWarning:
		style = warningMessageStyle
		icon = "⚠️  "
	case msgError:
		style = errorMessageStyle
		icon = "❌ "
	case msgSuccess:
		style = successMessageStyle
		icon = "✅ "
	}

	return style.Render(icon + m.userMessage.text)
}

func runTest(scenario testdata.TestScenario, eventCount int) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()

		// Run test with progress callback
		results, err := testdata.RunTestScenarioWithProgress(
			ctx,
			scenario,
			eventCount,
			func(count, total int, rate float64, elapsed time.Duration) {
				// Send progress update to the global program
				if globalProgram != nil {
					globalProgram.Send(testProgressMsg{
						eventCount:  count,
						totalEvents: total,
						currentRate: rate,
						elapsedTime: elapsed,
					})
				}
			},
		)

		return testCompleteMsg{
			results: results,
			err:     err,
		}
	}
}

func startTUI() error {
	m := initialModel()
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Set the global program reference for progress updates
	globalProgram = p

	_, err := p.Run()
	return err
}
