package ui

import (
	"fmt"
	"time"

	"github.com/briandowns/spinner"
)

type StepSpinner struct {
	spinner     *spinner.Spinner
	host        string
	output      *TerminalOutput
	currentStep string
}

func (s *StepSpinner) GetCurrentStep() string {
	return s.currentStep
}

func NewStepSpinner(host string, output *TerminalOutput) *StepSpinner {
	blueColor := "\033[34m"
	resetColor := "\033[0m"

	s := spinner.New(spinner.CharSets[11], 100*time.Millisecond)
	s.Prefix = fmt.Sprintf("%s[%s] %s", blueColor, host, resetColor)

	return &StepSpinner{
		spinner: s,
		host:    host,
		output:  output,
	}
}

func (s *StepSpinner) Start(step string) {
	s.currentStep = step
	if s.output != nil {
		s.output.SetSpinnerActive(true)
	}
	s.spinner.Suffix = fmt.Sprintf(" %s", step)
	s.spinner.Start()
}

func (s *StepSpinner) Stop(success bool) {
	s.spinner.Stop()
	if s.output != nil {
		s.output.SetSpinnerActive(false)
	}
	if success {
		fmt.Printf("[%s] ✅ %s\n", s.host, s.spinner.Suffix)
	} else {
		fmt.Printf("[%s] 🚨 %s\n", s.host, s.spinner.Suffix)
	}
}
