//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32        = syscall.NewLazyDLL("user32.dll")
	procSendInput = user32.NewProc("SendInput")
)

const (
	inputKeyboard    = 1
	keyEventFUnicode = 0x0004
	keyEventFKeyUp   = 0x0002
)

// keybdInput mirrors Win32's KEYBDINPUT struct.
type keybdInput struct {
	wVk         uint16
	wScan       uint16
	dwFlags     uint32
	time        uint32
	dwExtraInfo uintptr
}

// input mirrors Win32's INPUT struct. The trailing padding brings the union
// member up to MOUSEINPUT's size (32 bytes on amd64), which is what the real
// C struct's union occupies — SendInput validates cbSize against that exact
// layout, not just KEYBDINPUT's own (smaller) size.
type input struct {
	inputType uint32
	ki        keybdInput
	padding   uint64
}

func sendUnicodeKey(char rune, keyUp bool) {
	flags := uint32(keyEventFUnicode)
	if keyUp {
		flags |= keyEventFKeyUp
	}
	in := input{
		inputType: inputKeyboard,
		ki: keybdInput{
			wScan:   uint16(char),
			dwFlags: flags,
		},
	}
	procSendInput.Call(
		uintptr(1),
		uintptr(unsafe.Pointer(&in)),
		unsafe.Sizeof(in),
	)
}

// typeCharacter simulates a single keystroke by injecting the character
// directly at the Unicode level (KEYEVENTF_UNICODE), so the active OS
// keyboard layout has no effect on what character gets produced.
func typeCharacter(char rune) {
	sendUnicodeKey(char, false)
	sendUnicodeKey(char, true)
}

// lockLatinLayout is a no-op on Windows: SendInput's KEYEVENTF_UNICODE path
// injects characters independently of the active keyboard layout, so there's
// no Linux-style keymap-remap hazard to guard against here.
func lockLatinLayout() (restore func()) {
	return func() {}
}
