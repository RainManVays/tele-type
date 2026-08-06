//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// typeCharacter simulates a single keystroke by asking System Events to type
// the character as literal Unicode text (osascript keystroke), so the active
// OS keyboard layout has no effect on what character gets produced.
func typeCharacter(char rune) {
	s := string(char)
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	script := fmt.Sprintf(`tell application "System Events" to keystroke "%s"`, s)
	_ = exec.Command("osascript", "-e", script).Run()
}

// lockLatinLayout is a no-op on macOS: System Events' keystroke command
// injects Unicode text independently of the active input source, so there's
// no Linux-style keymap-remap hazard to guard against here.
func lockLatinLayout() (restore func()) {
	return func() {}
}
