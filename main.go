package main

import (
	"crypto/md5"
	"embed"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/go-vgo/robotgo"
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

// typeCharacter simulates a single keystroke for the given rune.
// robotgo.TypeStr handles all printable ASCII and Unicode (for Base122).
func typeCharacter(char rune) {
	robotgo.TypeStr(string(char))
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
