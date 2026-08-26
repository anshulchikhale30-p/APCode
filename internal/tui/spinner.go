package tui

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// SpinnerMode controls when a spinner animates.
type SpinnerMode int

const (
	// SpinnerAuto enables animation only for interactive writers.
	SpinnerAuto SpinnerMode = iota
	// SpinnerOn forces animation (used by tests).
	SpinnerOn
	// SpinnerOff disables all spinner output.
	SpinnerOff
)

const defaultSpinnerInterval = 100 * time.Millisecond

var defaultFrames = []string{"◐", "◓", "◑", "◒"}

// Spinner is a reusable single-line activity indicator. It animates a
// frame + message on one terminal line and clears itself on Stop. A
// spinner never writes to non-interactive writers, never leaks its
// goroutine (Stop waits for it), and is safe to Stop more than once.
type Spinner struct {
	Out         io.Writer
	Message     string
	Frames      []string
	Interval    time.Duration
	Mode        SpinnerMode
	Interactive bool // whether Out is a real terminal (set by the caller)
	Ctx         context.Context

	mu     sync.Mutex
	active bool
	stop   chan struct{}
	done   chan struct{}
}

// NewSpinner creates a spinner in auto mode. Interactive must reflect
// whether out is attached to a terminal; use IsTerminalWriter.
func NewSpinner(out io.Writer, message string, interactive bool) *Spinner {
	return &Spinner{
		Out:         out,
		Message:     message,
		Frames:      defaultFrames,
		Interval:    defaultSpinnerInterval,
		Mode:        SpinnerAuto,
		Interactive: interactive,
	}
}

func (s *Spinner) enabled() bool {
	switch s.Mode {
	case SpinnerOn:
		return true
	case SpinnerOff:
		return false
	default:
		return s.Interactive && s.Out != nil
	}
}

// Start begins animating. It is a no-op when disabled or already running.
func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active || !s.enabled() {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.stop = make(chan struct{})
	s.done = make(chan struct{})
	stop, done := s.stop, s.done
	out, msg := s.Out, s.Message
	frames := s.Frames
	if len(frames) == 0 {
		frames = defaultFrames
	}
	interval := s.Interval
	if interval <= 0 {
		interval = defaultSpinnerInterval
	}
	ctx := s.Ctx
	s.mu.Unlock()

	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		// Render the first frame immediately so short operations still
		// show activity.
		fmt.Fprintf(out, "\r%s %s  ", frames[0], msg)
		for i := 1; ; i = (i + 1) % len(frames) {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Fprintf(out, "\r%s %s  ", frames[i], msg)
			}
		}
	}()
}

// Stop halts the animation, waits until the goroutine has fully exited,
// and erases the spinner line. Safe to call multiple times and safe to
// call on a never-started spinner.
func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	close(s.stop)
	done, out, msg := s.done, s.Out, s.Message
	s.mu.Unlock()

	<-done
	// Erase the line with plain spaces; no escape sequences, so this is
	// safe even without color support.
	clearLen := len(msg) + 6
	fmt.Fprintf(out, "\r%s\r", strings.Repeat(" ", clearLen))
}

// Active reports whether the spinner is currently animating.
func (s *Spinner) Active() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}
