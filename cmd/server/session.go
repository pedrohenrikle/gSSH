package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
)

type BashSession struct {
	Id              string
	TerminalCommand *exec.Cmd
	Ptmx            *os.File
	InUse           bool
}

func (*BashSession) New(s *Server, sessionId string) (*BashSession, error) {
	// Initialize a bash session and a PTY session
	bashSession := exec.Command("bash")
	ptmx, err := pty.Start(bashSession)
	if err != nil {
		fmt.Printf("Failed to start bash session for %s: %v\n", sessionId, err)
		return nil, err
	}

	// Disable the "echo" from commands
	var termState *unix.Termios
	if termState, err = unix.IoctlGetTermios(int(ptmx.Fd()), unix.TCGETS); err != nil {
		fmt.Printf("Failed to get terminal attributes for %s: %v\n", sessionId, err)
		return nil, err
	}
	termState.Lflag &^= unix.ECHO
	if err = unix.IoctlSetTermios(int(ptmx.Fd()), unix.TCSETS, termState); err != nil {
		fmt.Printf("Failed to set terminal attributes for %s: %v\n", sessionId, err)
		return nil, err
	}
	defer func() { _ = unix.IoctlSetTermios(int(ptmx.Fd()), unix.TCSETS, termState) }()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGWINCH)

	// This ensures that the PTY adjusts to terminal window size changes.
	go func() {
		for range ch {
			if err := pty.InheritSize(os.Stdin, ptmx); err != nil {
				log.Fatalf("error trying to resize the PTY: %v", err)
			}
		}
	}()
	ch <- syscall.SIGWINCH
	defer func() { signal.Stop(ch); close(ch) }()

	return &BashSession{
		Id:              sessionId,
		TerminalCommand: bashSession,
		Ptmx:            ptmx,
		InUse:           true,
	}, nil
}
