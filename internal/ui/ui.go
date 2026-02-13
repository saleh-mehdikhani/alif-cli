package ui

import (
	"fmt"
	"sync"
	"time"

	"alif-cli/internal/color"
)

// Header prints a bold cyan section title without "STEP:" prefix
func Header(title string) {
	fmt.Printf("\n%s\n", color.Sprintf(color.BoldCyan, "%s", title))
}

// Item prints a key-value pair in list format
func Item(key, value string) {
	// Key is dim, value is white
	// We handle padding manually for alignment
	k := color.Sprintf(color.Dim, "  • %-12s", key+":")
	fmt.Printf("%s %s\n", k, value)
}

// Spinner handles loading animation
type Spinner struct {
	msg    string
	stop   chan struct{}
	wg     sync.WaitGroup
	mu     sync.Mutex
	active bool
}

// StartSpinner starts the animation in background
func StartSpinner(msg string) *Spinner {
	s := &Spinner{
		msg:    msg,
		stop:   make(chan struct{}),
		active: true,
	}
	s.wg.Add(1)
	go s.run()
	return s
}

func (s *Spinner) run() {
	defer s.wg.Done()
	chars := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	i := 0
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()

	// Initial print
	// fmt.Printf("\r  %s %s ", color.Sprintf(color.Yellow, "%s", chars[i]), color.Sprintf(color.Dim, "%s", s.msg))

	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			frame := color.Sprintf(color.Yellow, "%s", chars[i])
			// Check msg for updates (not implemented here, dynamic msg requires mutex)
			text := color.Sprintf(color.Dim, "%s", s.msg)
			// \033[2K clears line first to avoid artifacts
			fmt.Printf("\r\033[2K  %s %s ", frame, text)
			i = (i + 1) % len(chars)
		}
	}
}

// Succeed stops spinner with green checkmark
func (s *Spinner) Succeed(finalMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.active = false
	close(s.stop)
	s.wg.Wait()

	fmt.Printf("\r\033[2K") // Clear line
	if finalMsg == "" {
		finalMsg = s.msg
	}
	fmt.Printf("  %s %s\n", color.Sprintf(color.Green, "✓"), finalMsg)
}

// Fail stops spinner with red cross
func (s *Spinner) Fail(finalMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return
	}
	s.active = false
	close(s.stop)
	s.wg.Wait()

	fmt.Printf("\r\033[2K") // Clear line
	if finalMsg == "" {
		finalMsg = s.msg
	}
	fmt.Printf("  %s %s\n", color.Sprintf(color.Red, "✖"), finalMsg)
}

// Info prints a simple info line (e.g. for sub-steps or logs)
func Info(msg string) {
	fmt.Printf("  %s %s\n", color.Sprintf(color.Blue, "ℹ"), msg)
}

// Warn prints a warning line
func Warn(msg string) {
	fmt.Printf("  %s %s\n", color.Sprintf(color.Yellow, "!"), msg)
}

// Error prints error line
func Error(msg string) {
	fmt.Printf("  %s %s\n", color.Sprintf(color.Red, "✖"), msg)
}

// Success prints success line
func Success(msg string) {
	fmt.Printf("  %s %s\n", color.Sprintf(color.Green, "✓"), msg)
}
