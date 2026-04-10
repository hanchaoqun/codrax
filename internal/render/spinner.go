package render

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

// Spinner shows an animated indicator on the terminal while work is in
// progress. It overwrites its own line on each tick so it appears to
// animate in place. Stop() clears the line, leaving room for the
// final status line written by the caller.
type Spinner struct {
	mu      sync.Mutex
	writeMu sync.Mutex // guards all writes to out, shared with Renderer
	out     io.Writer
	msg     string
	detail  string
	running bool
	done    chan struct{}
	s       *style
}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func newSpinner(out io.Writer, s *style) *Spinner {
	return &Spinner{
		out:  out,
		s:    s,
		done: make(chan struct{}),
	}
}

// Start begins the animation with the given message.
func (sp *Spinner) Start(msg string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.msg = msg
	sp.detail = ""
	if sp.running {
		return
	}
	sp.running = true
	sp.done = make(chan struct{})
	go sp.loop()
}

// Update changes the spinner message without restarting.
func (sp *Spinner) Update(msg string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.msg = msg
}

// SetDetail sets a secondary detail line shown after the main message.
func (sp *Spinner) SetDetail(detail string) {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	sp.detail = detail
}

// Stop halts the animation and clears the spinner line.
func (sp *Spinner) Stop() {
	sp.mu.Lock()
	if !sp.running {
		sp.mu.Unlock()
		return
	}
	sp.running = false
	sp.mu.Unlock()
	<-sp.done
	// Clear spinner line.
	sp.writeMu.Lock()
	fmt.Fprintf(sp.out, "\r%s\r", strings.Repeat(" ", 80))
	sp.writeMu.Unlock()
}

func (sp *Spinner) loop() {
	defer close(sp.done)
	tick := time.NewTicker(80 * time.Millisecond)
	defer tick.Stop()
	frame := 0
	for {
		sp.mu.Lock()
		if !sp.running {
			sp.mu.Unlock()
			return
		}
		msg := sp.msg
		detail := sp.detail
		sp.mu.Unlock()

		f := spinnerFrames[frame%len(spinnerFrames)]
		line := fmt.Sprintf("\r  %s %s", sp.s.Cyan(f), msg)
		if detail != "" {
			line += sp.s.Dim(" "+detail)
		}
		// Pad to clear previous content, then write.
		padded := line + strings.Repeat(" ", max(0, 80-visibleLen(line)))
		sp.writeMu.Lock()
		fmt.Fprint(sp.out, padded)
		sp.writeMu.Unlock()

		frame++
		<-tick.C
	}
}

// visibleLen estimates the visible character count by stripping ANSI escapes.
func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		n++
	}
	return n
}
