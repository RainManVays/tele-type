//go:build linux

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

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
