package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tuiItem struct {
	test     SuiteTest
	selected bool
	result   *TestResult
}

type testResultMsg struct{ result TestResult }
type allDoneMsg struct{}

type tuiModel struct {
	items  []*tuiItem
	cursor int

	phase    string // select, running, done
	spinner  spinner.Model
	viewport viewport.Model
	width    int
	height   int

	server *Server
	opts   Options

	results   chan TestResult
	cancel    context.CancelFunc
	total     int
	completed int
	passed    int
	failed    int
	err       error
}

var (
	tuiTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	tuiMuted = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	tuiOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	tuiBad   = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	tuiWarn  = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	tuiPanel = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1)
)

func runTUI(server *Server, opts Options, tests []SuiteTest) error {
	items := make([]*tuiItem, 0, len(tests))
	for _, test := range tests {
		items = append(items, &tuiItem{test: test, selected: true})
	}
	spin := spinner.New()
	spin.Spinner = spinner.Dot
	spin.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	m := &tuiModel{
		items:    items,
		phase:    "select",
		spinner:  spin,
		viewport: viewport.New(0, 0),
		width:    100,
		height:   30,
		server:   server,
		opts:     opts,
	}
	m.syncLogs()
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	if m.cancel != nil {
		m.cancel()
	}
	return err
}

func (m *tuiModel) Init() tea.Cmd { return m.spinner.Tick }

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.viewport.Width = maxInt(30, msg.Width/2-4)
		m.viewport.Height = maxInt(8, msg.Height-12)
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case testResultMsg:
		item := m.findItem(msg.result.File)
		if item != nil {
			result := msg.result
			item.result = &result
			if result.Passed {
				m.passed++
			} else {
				m.failed++
			}
		}
		m.completed++
		m.syncLogs()
		if m.completed >= m.total {
			m.phase = "done"
			return m, nil
		}
		return m, waitForResult(m.results)
	case allDoneMsg:
		m.phase = "done"
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.cancel != nil {
				m.cancel()
			}
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
				m.syncLogs()
			}
		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
				m.syncLogs()
			}
		case " ":
			if m.phase == "select" {
				m.items[m.cursor].selected = !m.items[m.cursor].selected
			}
		case "a":
			if m.phase == "select" {
				all := true
				for _, item := range m.items {
					if !item.selected {
						all = false
						break
					}
				}
				for _, item := range m.items {
					item.selected = !all
				}
			}
		case "enter":
			if m.phase == "select" || m.phase == "done" {
				return m.startRun()
			}
		}
	}
	return m, nil
}

func (m *tuiModel) startRun() (tea.Model, tea.Cmd) {
	var tests []SuiteTest
	for _, item := range m.items {
		if item.selected {
			tests = append(tests, item.test)
		}
	}
	if len(tests) == 0 {
		m.err = fmt.Errorf("select at least one test with space")
		return m, nil
	}
	for _, item := range m.items {
		if item.selected {
			item.result = nil
		}
	}
	m.phase = "running"
	m.err = nil
	m.total = len(tests)
	m.completed, m.passed, m.failed = 0, 0, 0
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	results := make(chan TestResult, len(tests))
	m.results = results

	go func() {
		defer close(results)
		for _, test := range tests {
			harness, err := NewHarness(m.server, m.opts)
			if err != nil {
				results <- TestResult{File: test.File, Name: test.Definition.Name, Err: err}
				continue
			}
			result := harness.RunTest(ctx, test)
			harness.Close()
			results <- result
		}
	}()

	m.syncLogs()
	return m, tea.Batch(m.spinner.Tick, waitForResult(results))
}

func waitForResult(results <-chan TestResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-results
		if !ok {
			return allDoneMsg{}
		}
		return testResultMsg{result: result}
	}
}

func (m *tuiModel) findItem(file string) *tuiItem {
	for _, item := range m.items {
		if item.test.File == file {
			return item
		}
	}
	return nil
}

func (m *tuiModel) syncLogs() {
	if len(m.items) == 0 {
		m.viewport.SetContent("No tests loaded.")
		return
	}
	m.viewport.SetContent(m.logsFor(m.items[m.cursor]))
	m.viewport.GotoTop()
}

func (m *tuiModel) logsFor(item *tuiItem) string {
	if item == nil {
		return ""
	}
	if item.result == nil {
		return tuiMuted.Render("Press enter to run the selected tests.") + "\n\n" + item.test.Definition.Description
	}
	result := item.result
	var lines []string
	status := tuiOK.Render("✔ passed")
	if !result.Passed {
		status = tuiBad.Render("✖ failed")
	}
	lines = append(lines, fmt.Sprintf("%s  %s", tuiTitle.Render(result.Name), status))
	if result.Description != "" {
		lines = append(lines, tuiMuted.Render(result.Description))
	}
	lines = append(lines, "")
	for _, step := range result.Steps {
		glyph := tuiOK.Render("✔")
		if step.Err != nil {
			glyph = tuiBad.Render("✖")
		}
		lines = append(lines, fmt.Sprintf("%s %s %s", glyph, step.Name, tuiMuted.Render(step.Duration.Round(time.Millisecond).String())))
		for _, log := range step.Logs {
			lines = append(lines, "  "+tuiMuted.Render(log))
		}
		if step.Err != nil {
			lines = append(lines, "  "+tuiBad.Render(step.Err.Error()))
		}
	}
	if result.Err != nil {
		lines = append(lines, "", tuiBad.Render(result.Err.Error()))
	}
	return strings.Join(lines, "\n")
}

func (m *tuiModel) View() string {
	if m.width == 0 {
		return "loading…"
	}
	header := tuiPanel.Width(maxInt(40, m.width-2)).Render(
		tuiTitle.Render("MailVoid Integration Runner") + "\n" +
			tuiMuted.Render("server  ") + m.server.BaseURL + "\n" +
			tuiMuted.Render("data    ") + m.server.DataDir,
	)

	list := m.renderList()
	logs := tuiPanel.Width(m.viewport.Width + 2).Height(m.viewport.Height + 2).Render(m.viewport.View())

	var body string
	if m.width >= 110 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, list, "  ", logs)
	} else {
		body = lipgloss.JoinVertical(lipgloss.Left, list, "", logs)
	}

	status := m.statusLine()
	help := tuiMuted.Render("space select · a all · enter run · ↑↓ move · q quit")
	if m.phase == "running" {
		help = tuiMuted.Render("running · q quit")
	} else if m.phase == "done" {
		help = tuiMuted.Render("enter rerun selected · space select · q quit")
	}
	errLine := ""
	if m.err != nil {
		errLine = "\n" + tuiBad.Render("✖ "+m.err.Error())
	}
	return header + "\n" + body + "\n" + status + errLine + "\n" + help
}

func (m *tuiModel) renderList() string {
	var lines []string
	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = tuiTitle.Render("› ")
		}
		marker := "◯"
		if item.selected {
			marker = "◉"
		}
		status := tuiMuted.Render("·")
		if m.phase == "running" && item.selected && item.result == nil {
			status = m.spinner.View()
		}
		if item.result != nil {
			if item.result.Passed {
				status = tuiOK.Render("✔")
			} else {
				status = tuiBad.Render("✖")
			}
		}
		description := item.test.Definition.Description
		if len(description) > 42 {
			description = description[:42] + "…"
		}
		line := fmt.Sprintf("%s%s %s %-28s %s", cursor, marker, status, item.test.Definition.Name, tuiMuted.Render(description))
		if i == m.cursor {
			line = lipgloss.NewStyle().Background(lipgloss.Color("236")).Render(line)
		}
		lines = append(lines, line)
	}
	return tuiPanel.Width(maxInt(44, m.width/2-2)).Render(strings.Join(lines, "\n"))
}

func (m *tuiModel) statusLine() string {
	switch m.phase {
	case "running":
		return fmt.Sprintf("%s running %d/%d  %s %d  %s %d", m.spinner.View(), m.completed, m.total, tuiOK.Render("✔"), m.passed, tuiBad.Render("✖"), m.failed)
	case "done":
		return fmt.Sprintf("%s %d passed  %s %d failed", tuiOK.Render("✔"), m.passed, tuiBad.Render("✖"), m.failed)
	default:
		return tuiMuted.Render("Ready. Select tests and press enter.")
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
