//go:build windows

package cmd

import (
	"runtime"
	"syscall"
	"unsafe"
)

func getTerminalSize() (width, height int) {
	const (
		_STD_OUTPUT_HANDLE = uint32(0xFFFFFFF5)
		_FILE_TYPE_CHAR    = 0x0002
	)

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleScreenBufferInfo := kernel32.NewProc("GetConsoleScreenBufferInfo")
	getFileType := kernel32.NewProc("GetFileType")

	// Best-effort: if there is no console, bail out instead of crashing.
	handle, _, _ := getStdHandle.Call(uintptr(_STD_OUTPUT_HANDLE))
	if handle == 0 || handle == uintptr(^uint32(0)) {
		return 80, 40
	}
	fileType, _, _ := getFileType.Call(handle)
	if fileType != _FILE_TYPE_CHAR {
		return 80, 40
	}

	type smallRect struct {
		left, top, right, bottom int16
	}
	type coord struct {
		x, y int16
	}
	type consoleScreenBufferInfo struct {
		size                coord
		cursorPosition      coord
		attributes          uint32
		window              smallRect
		maximumWindowSize   coord
	}

	var csbi consoleScreenBufferInfo
	ret, _, _ := getConsoleScreenBufferInfo.Call(handle, uintptr(unsafe.Pointer(&csbi)))
	if ret == 0 {
		return 80, 40
	}

	width = int(csbi.window.right - csbi.window.left)
	height = int(csbi.window.bottom - csbi.window.top)
	if width <= 0 {
		width = int(csbi.size.x)
	}
	if height <= 0 {
		height = int(csbi.size.y)
	}

	// Sanity-check: if the values look like a malformed buffer state
	// (for example 1-tall or extremely narrow), treat it as unknown
	// so the caller can fall back to a sane default instead of sending
	// a broken size to the remote PTY.
	if width < 20 || height < 2 {
		return 80, 40
	}
	return width, height
}

// enableVirtualTerminalProcessing enables VT100/ANSI processing on the
// Windows console so CSI sequences (cursor movement, colors, etc.) are
// interpreted by conhost instead of being printed as raw bytes.
func enableVirtualTerminalProcessing() {
	if runtime.GOOS != "windows" {
		return
	}

	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	const (
		_STD_OUTPUT_HANDLE               = uint32(0xFFFFFFF5)
		_ENABLE_VIRTUAL_TERMINAL_PROCESSING = uint32(0x0004)
	)

	handle, _, _ := getStdHandle.Call(uintptr(_STD_OUTPUT_HANDLE))
	if handle == 0 || handle == uintptr(^uint32(0)) {
		return
	}

	var mode uint32
	ret, _, _ := getConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return
	}

	mode |= _ENABLE_VIRTUAL_TERMINAL_PROCESSING
	_, _, _ = setConsoleMode.Call(handle, uintptr(mode))
}

// windowsDisableInputEcho disables local console input echo on Windows
// without switching the console into full raw mode.
func setupTerminal() (func(), error) {
	enableVirtualTerminalProcessing()

	restoreEcho, err := windowsDisableInputEcho()
	if err != nil {
		return nil, err
	}
	return restoreEcho, nil
}

func windowsDisableInputEcho() (func(), error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getStdHandle := kernel32.NewProc("GetStdHandle")
	getConsoleMode := kernel32.NewProc("GetConsoleMode")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")

	const (
		_STD_INPUT_HANDLE  = uint32(0xFFFFFFF6)
		_ENABLE_ECHO_INPUT = uint32(0x0004)
		_ENABLE_LINE_INPUT = uint32(0x0002)
	)

	handle, _, _ := getStdHandle.Call(uintptr(_STD_INPUT_HANDLE))
	if handle == 0 || handle == uintptr(^uint32(0)) {
		return func() {}, nil
	}

	var mode uint32
	ret, _, _ := getConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if ret == 0 {
		return func() {}, nil
	}

	originalMode := mode
	mode = mode &^ (_ENABLE_ECHO_INPUT | _ENABLE_LINE_INPUT)
	_, _, _ = setConsoleMode.Call(handle, uintptr(mode))

	return func() {
		// Best-effort restore; ignore errors.
		_, _, _ = setConsoleMode.Call(handle, uintptr(originalMode))
	}, nil
}
