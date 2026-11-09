package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

type batchPrinter struct {
	tty     bool
	verbose bool
}

func newBatchPrinter(verbose bool) *batchPrinter {
	return &batchPrinter{
		tty:     term.IsTerminal(os.Stdout.Fd()) && os.Getenv("NO_COLOR") == "",
		verbose: verbose,
	}
}

func (p *batchPrinter) Header(baseURL string, count int) {
	if !p.tty {
		fmt.Printf("MailVoid integration runner\nserver: %s\ntests: %d\n\n", baseURL, count)
		return
	}
	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("8")).
		Padding(0, 2)
	title := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6")).Render("MailVoid integration runner")
	body := fmt.Sprintf("%s\n%s %s\n%s %d",
		title,
		lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("server"),
		baseURL,
		lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("tests"),
		count)
	fmt.Println(panel.Render(body))
	fmt.Println()
}

func (p *batchPrinter) Test(result TestResult) {
	status := "PASS"
	glyph := "✔"
	if !result.Passed {
		status = "FAIL"
		glyph = "✖"
	}
	if p.tty {
		style := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
		if !result.Passed {
			style = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
		}
		fmt.Printf("%s %-36s %s\n", style.Render(glyph), result.Name, lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(result.Duration.Round(time.Millisecond).String()))
	} else {
		fmt.Printf("[%s] %s (%s)\n", status, result.Name, result.Duration.Round(time.Millisecond))
	}
	if result.Passed && !p.verbose {
		return
	}
	if result.Err != nil {
		fmt.Printf("    %s\n", result.Err)
	}
	for _, step := range result.Steps {
		if !p.verbose && step.Err == nil {
			continue
		}
		if step.Err != nil {
			fmt.Printf("    %s %s: %v\n", "✖", step.Name, step.Err)
		} else if p.verbose {
			fmt.Printf("    %s %s\n", "→", step.Name)
		}
		for _, line := range step.Logs {
			fmt.Printf("      %s\n", line)
		}
	}
}

func (p *batchPrinter) Summary(passed, failed int, elapsed time.Duration) {
	fmt.Println()
	if p.tty {
		success := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Render(fmt.Sprintf("✔ %d passed", passed))
		text := success
		if failed > 0 {
			text += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Render(fmt.Sprintf("✖ %d failed", failed))
		}
		if elapsed > 0 {
			text += "  " + lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render(elapsed.Round(time.Millisecond).String())
		}
		fmt.Println(text)
		return
	}
	line := fmt.Sprintf("%d passed, %d failed", passed, failed)
	if elapsed > 0 {
		line += ", " + elapsed.Round(time.Millisecond).String()
	}
	fmt.Println(strings.TrimSpace(line))
}
