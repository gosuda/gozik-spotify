// Package notify shows a startup popup/notification for gozik-spotify,
// matching the tkinter popup used by gozik-yt-music.
package notify

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
)

// Startup informs the user that the plugin is running and where to find the
// web UI. It tries a graphical dialog first, then falls back to a desktop
// notification, then to the system log only.
func Startup(webUIPort int) {
	if !hasDisplay() {
		return
	}

	msg := fmt.Sprintf("The gozik Spotify plugin is running.\nManage authentication at http://127.0.0.1:%d", webUIPort)
	title := "gozik Spotify"

	// Prefer a real dialog window like gozik-yt-music's Tkinter popup.
	if tryZenity(title, msg, webUIPort) {
		return
	}
	if tryNotifySend(title, msg) {
		return
	}
	if tryXMessage(msg) {
		return
	}

	// macOS / Windows fallbacks.
	switch runtime.GOOS {
	case "darwin":
		_ = tryAppleScript(title, msg)
	case "windows":
		_ = tryWindowsMsg(title, msg)
	}
}

func hasDisplay() bool {
	return os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""
}

func tryZenity(title, msg string, port int) bool {
	if _, err := exec.LookPath("zenity"); err != nil {
		return false
	}
	cmd := exec.Command(
		"zenity", "--info",
		"--title", title,
		"--text", msg,
		"--ok-label", "Open Web UI",
	)
	if err := cmd.Start(); err != nil {
		return false
	}
	// The dialog may stay open; do not wait. If the user clicks OK, open the UI.
	go func() {
		if err := cmd.Wait(); err == nil {
			_ = openBrowser(fmt.Sprintf("http://127.0.0.1:%d", port))
		}
	}()
	return true
}

func tryNotifySend(title, msg string) bool {
	if _, err := exec.LookPath("notify-send"); err != nil {
		return false
	}
	cmd := exec.Command("notify-send", "-a", title, title, msg)
	if err := cmd.Run(); err != nil {
		log.Printf("notify-send failed: %v", err)
		return false
	}
	return true
}

func tryXMessage(msg string) bool {
	if _, err := exec.LookPath("xmessage"); err != nil {
		return false
	}
	cmd := exec.Command("xmessage", "-center", msg)
	_ = cmd.Start()
	return true
}

func tryAppleScript(title, msg string) bool {
	if _, err := exec.LookPath("osascript"); err != nil {
		return false
	}
	script := fmt.Sprintf(`display dialog %q with title %q buttons {"OK"}`, msg, title)
	cmd := exec.Command("osascript", "-e", script)
	_ = cmd.Start()
	return true
}

func tryWindowsMsg(title, msg string) bool {
	if _, err := exec.LookPath("msg"); err != nil {
		return false
	}
	cmd := exec.Command("msg", "*", "/TIME:10", title, msg)
	_ = cmd.Start()
	return true
}

func openBrowser(url string) error {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
		args = []string{url}
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		cmd = "xdg-open"
		args = []string{url}
	}
	return exec.Command(cmd, args...).Start()
}
