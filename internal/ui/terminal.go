package ui

import (
	"fmt"
	"sync"
	"time"

	"github.com/fatih/color"
)

const (
	maxLines    = 10
	lineWidth   = 80
	clearScreen = "\033[H\033[2J"
	moveUp      = "\033[%dA"
	clearLine   = "\033[K"
)

type TerminalOutput struct {
	mu            sync.Mutex
	lines         []string
	host          string
	gray          *color.Color
	lastSize      int
	lastLine      string
	lastUpdate    time.Time
	spinnerActive bool
	spinner       *StepSpinner
}

func NewTerminalOutput(host string) *TerminalOutput {
	return &TerminalOutput{
		lines:      make([]string, 0, maxLines),
		host:       host,
		gray:       color.New(color.FgHiBlack),
		lastSize:   0,
		lastLine:   "",
		lastUpdate: time.Now(),
	}
}

func (t *TerminalOutput) SetSpinnerActive(active bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.spinnerActive = active
}

func (t *TerminalOutput) WriteLine(format string, args ...interface{}) {
	t.mu.Lock()
	defer t.mu.Unlock()

	line := fmt.Sprintf(format, args...)
	if line == t.lastLine && time.Since(t.lastUpdate) < 100*time.Millisecond {
		return
	}

	t.lastLine = line
	t.lastUpdate = time.Now()

	timestamp := time.Now().Format("15:04:05")
	formattedLine := fmt.Sprintf("[%s] %s: %s", timestamp, t.host, line)

	t.lines = append(t.lines, formattedLine)
	if len(t.lines) > maxLines {
		t.lines = t.lines[1:]
	}

	var spinnerText string
	if t.spinner != nil {
		spinnerText = t.spinner.GetCurrentStep()
		t.spinner.Stop(false)
	}

	if t.lastSize > 0 {
		fmt.Printf(moveUp, t.lastSize)
		for i := 0; i < t.lastSize; i++ {
			fmt.Print(clearLine + "\n")
		}
		fmt.Printf(moveUp, t.lastSize)
	}

	for _, l := range t.lines {
		t.gray.Println(l)
	}

	if t.spinner != nil && spinnerText != "" {
		t.spinner.Start(spinnerText)
	}

	t.lastSize = len(t.lines)
}

func (t *TerminalOutput) Clear() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.lastSize > 0 {
		fmt.Printf(moveUp, t.lastSize)
		for i := 0; i < t.lastSize; i++ {
			fmt.Print(clearLine + "\n")
		}
		fmt.Printf(moveUp, t.lastSize)
	}

	t.lines = nil
	t.lastSize = 0
	t.lastLine = ""
}
