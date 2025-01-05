package ui

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
)

type StepSpinner struct {
	spinner *spinner.Spinner
	host    string
}

func NewStepSpinner(host string) *StepSpinner {
	blueColor := "\033[34m"
	resetColor := "\033[0m"

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Prefix = fmt.Sprintf("%s[%s] %s", blueColor, host, resetColor)
	return &StepSpinner{
		spinner: s,
		host:    host,
	}
}

func (s *StepSpinner) Start(step string) {
	s.spinner.Suffix = fmt.Sprintf(" %s", step)
	s.spinner.Start()
}

func (s *StepSpinner) Stop(success bool) {
	s.spinner.Stop()
	if success {
		fmt.Printf("[%s] ✅ %s\n", s.host, s.spinner.Suffix)
	} else {
		fmt.Printf("[%s] 🚨 %s\n", s.host, s.spinner.Suffix)
	}
}
