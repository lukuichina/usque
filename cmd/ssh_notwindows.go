//go:build !windows

package cmd

import (
	"golang.org/x/term"
	"golang.org/x/sys/unix"
)

func getTerminalSize() (width, height int) {
	w, h, err := term.GetSize(int(unix.Stdin))
	if err == nil && w > 0 && h > 0 {
		return w, h
	}
	return 80, 40
}

func setupTerminal() (func(), error) {
	fd := int(unix.Stdin)

	termState, err := term.GetState(fd)
	if err != nil {
		return nil, err
	}

	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return nil, err
	}

	originalTermios := *termios
	// Disable local echo and canonical mode so keystrokes are forwarded
	// immediately to the remote PTY. The remote side will handle echo
	// because we request ECHO=1. Keeping ISIG enabled allows Ctrl-C and
	// similar signals to still be sent. Turn off ICRNL so the local ENTER
	// key (CR) is forwarded as CR instead of being converted to LF; the
	// remote Windows shell expects CR as the line terminator.
	termios.Lflag &^= (unix.ECHO | unix.ICANON)
	termios.Lflag |= unix.ISIG
	termios.Iflag &^= unix.ICRNL

	if err := unix.IoctlSetTermios(fd, unix.TCSETS, termios); err != nil {
		return nil, err
	}

	return func() {
		_ = unix.IoctlSetTermios(fd, unix.TCSETS, &originalTermios)
		_ = term.Restore(fd, termState)
	}, nil
}
