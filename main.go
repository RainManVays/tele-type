package main

import (
	"crypto/md5"
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed assets/*
var assets embed.FS

func main() {
	svc := NewTeleTypeService()

	app := application.New(application.Options{
		Name:        "TeleType",
		Description: "Transfer files by simulating keyboard input",
		Services: []application.Service{
			application.NewService(svc),
		},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "TeleType",
		Width:           900,
		Height:          640,
		MinWidth:        640,
		MinHeight:       480,
		URL:             "/",
		DevToolsEnabled: true,
		EnableFileDrop:  true,
	})

	// Native OS drag-and-drop delivers real filesystem paths; the browser's
	// HTML5 DataTransfer API never exposes File.path outside Electron.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(e *application.WindowEvent) {
		if files := e.Context().DroppedFiles(); len(files) > 0 {
			app.Event.Emit("teletype:filedropped", files[0])
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// ── Shared helpers ────────────────────────────────────────────────────────────

func calculateMD5(filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// typeCharacter simulates a single keystroke layout-independently.
// xdotool key U{hex} sends a Unicode keysym directly, so the active OS keyboard
// layout (Cyrillic, etc.) has no effect on what character gets produced.
func typeCharacter(char rune) {
	_ = exec.Command("xdotool", "key", "--clearmodifiers", fmt.Sprintf("U%04X", char)).Run()
}

// lockLatinLayout temporarily forces a single "us" keyboard layout so every
// keysym xdotool needs (the Base64/85/91/122 alphabet) already exists in the
// active group and X never has to switch groups or remap keys mid-keystroke.
//
// Root cause: XTestFakeKeyEvent sends a raw keycode, and which character it
// produces depends on the currently active keyboard group. With a multi-group
// layout (e.g. "us,ru"), typing while a non-Latin group is active forces
// xdotool to switch groups or dynamically remap on every keystroke — done
// rapidly, that can wedge the X server's input pipeline badly enough to take
// keyboard AND mouse down with it, requiring a hard reboot to recover.
//
// Returns a restore func — call it (once) when the typing session ends to
// reinstate the user's original layout exactly as captured.
func lockLatinLayout() (restore func()) {
	out, err := exec.Command("setxkbmap", "-query").Output()
	if err != nil {
		return func() {}
	}

	var restoreArgs []string
	for _, line := range strings.Split(string(out), "\n") {
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "model":
			restoreArgs = append(restoreArgs, "-model", val)
		case "layout":
			restoreArgs = append(restoreArgs, "-layout", val)
		case "variant":
			if val != "" {
				restoreArgs = append(restoreArgs, "-variant", val)
			}
		case "options":
			for _, opt := range strings.Split(val, ",") {
				if opt != "" {
					restoreArgs = append(restoreArgs, "-option", opt)
				}
			}
		}
	}

	_ = exec.Command("setxkbmap", "us").Run()

	return func() {
		if len(restoreArgs) > 0 {
			_ = exec.Command("setxkbmap", restoreArgs...).Run()
		}
	}
}

func saveOffset(filePath string, offset int) {
	_ = os.WriteFile(filePath, []byte(fmt.Sprintf("%d", offset)), 0644)
}

func loadOffset(filePath string) int {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 0
	}
	var offset int
	fmt.Sscanf(string(data), "%d", &offset)
	return offset
}
