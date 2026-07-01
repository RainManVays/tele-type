package main

import (
	"crypto/md5"
	"embed"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"

	"github.com/wailsapp/wails/v3/pkg/application"
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

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:           "TeleType",
		Width:           900,
		Height:          640,
		MinWidth:        640,
		MinHeight:       480,
		URL:             "/",
		DevToolsEnabled: true,
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
