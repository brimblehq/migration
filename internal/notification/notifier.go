package notification

import (
	"fmt"
	"os/exec"
	"runtime"
)

type Notifier interface {
	Send(title, message string) error
}

type DefaultNotifier struct{}

func New() *DefaultNotifier {
	return &DefaultNotifier{}
}

func (n *DefaultNotifier) Send(title, message string) error {
	switch runtime.GOOS {
	case "darwin":
		return sendOSXNotification(title, message)
	case "linux":
		return sendLinuxNotification(title, message)
	case "windows":
		return sendWindowsNotification(title, message)
	default:
		return fmt.Errorf("unsupported operating system: %s", runtime.GOOS)
	}
}

func sendOSXNotification(title, message string) error {
	script := fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)
	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}

func sendLinuxNotification(title, message string) error {
	cmd := exec.Command("notify-send", title, message)
	return cmd.Run()
}

func sendWindowsNotification(title, message string) error {
	psScript := fmt.Sprintf(`
		[reflection.assembly]::loadwithpartialname("System.Windows.Forms")
		$notify = new-object system.windows.forms.notifyicon
		$notify.icon = [System.Drawing.SystemIcons]::Information
		$notify.visible = $true
		$notify.showballoontip(10, "%s", "%s", [system.windows.forms.tooltipicon]::None)
	`, title, message)

	cmd := exec.Command("powershell", "-c", psScript)
	return cmd.Run()
}
