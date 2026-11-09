package main

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

func testTUI() *tuiModel {
	spin := spinner.New()
	return &tuiModel{
		items: []*tuiItem{
			{test: SuiteTest{File: "a.json", Definition: TestDefinition{Name: "Alpha", Description: "first"}}, selected: true},
			{test: SuiteTest{File: "b.json", Definition: TestDefinition{Name: "Beta", Description: "second"}}, selected: true},
		},
		phase:    "select",
		spinner:  spin,
		viewport: viewport.New(40, 10),
		width:    120,
		height:   40,
		server:   &Server{BaseURL: "http://example.test", DataDir: "/tmp/data"},
	}
}

func TestTUIViewsAndSelection(t *testing.T) {
	model := testTUI()
	if model.Init() == nil {
		t.Fatal("expected spinner command")
	}
	if !strings.Contains(model.View(), "Integration Runner") || !strings.Contains(model.View(), "Alpha") {
		t.Fatalf("view = %s", model.View())
	}
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if updated == nil {
		t.Fatal("nil model")
	}
	model.Update(tea.KeyMsg{Type: tea.KeySpace})
	if model.items[0].selected {
		t.Fatal("space should deselect")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	if !model.items[0].selected {
		t.Fatal("a should select all")
	}
	model.items[1].result = &TestResult{Name: "Beta", File: "b.json", Passed: false, Err: context.Canceled}
	model.cursor = 1
	model.syncLogs()
	if !strings.Contains(model.logsFor(model.items[1]), "failed") {
		t.Fatal("expected failure log")
	}
	model.phase = "done"
	model.passed, model.failed = 1, 1
	if !strings.Contains(model.statusLine(), "passed") {
		t.Fatal("status line")
	}
	if model.findItem("missing") != nil || model.findItem("a.json") == nil {
		t.Fatal("find item")
	}
}

func TestWaitForResult(t *testing.T) {
	results := make(chan TestResult, 1)
	results <- TestResult{Name: "x"}
	close(results)
	msg := waitForResult(results)()
	if _, ok := msg.(testResultMsg); !ok {
		t.Fatalf("msg = %T", msg)
	}
	if _, ok := waitForResult(results)().(allDoneMsg); !ok {
		t.Fatal("expected allDone")
	}
}

func TestBatchPrinter(t *testing.T) {
	printer := &batchPrinter{}
	printer.Header("http://example.test", 2)
	printer.Test(TestResult{Name: "ok", Passed: true, Duration: 1})
	printer.Test(TestResult{Name: "bad", Passed: false, Err: context.Canceled, Steps: []StepResult{{Name: "step", Err: context.Canceled, Logs: []string{"log"}}}})
	printer.Summary(1, 1, 0)
}
