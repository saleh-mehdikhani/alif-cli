package color

import (
	"fmt"
	"runtime"
)

// ANSI color codes
const (
	Reset    = "\033[0m"
	Red      = "\033[31m"
	Green    = "\033[32m"
	Yellow   = "\033[33m"
	Blue     = "\033[34m"
	Cyan     = "\033[36m"
	BoldCode = "\033[1m"
	Dim      = "\033[2m"
	Magenta  = "\033[35m"
	White    = "\033[37m"
	BoldCyan = "\033[1;36m"
)

var colorsEnabled = true

func init() {
	// Disable colors on Windows versions that don't support ANSI
	// Modern Windows 10+ supports ANSI, so we'll keep them enabled
	if runtime.GOOS == "windows" {
		// Could add version detection here if needed
		// For now, assume modern Windows
	}
}

// Success prints a green success message
func Success(format string, args ...interface{}) {
	if colorsEnabled {
		fmt.Printf(Green+format+Reset+"\n", args...)
	} else {
		fmt.Printf(format+"\n", args...)
	}
}

// Error prints a red error message
func Error(format string, args ...interface{}) {
	if colorsEnabled {
		fmt.Printf(Red+format+Reset+"\n", args...)
	} else {
		fmt.Printf(format+"\n", args...)
	}
}

// Warning prints a yellow warning message
func Warning(format string, args ...interface{}) {
	if colorsEnabled {
		fmt.Printf(Yellow+format+Reset+"\n", args...)
	} else {
		fmt.Printf(format+"\n", args...)
	}
}

// Info prints a cyan info message
func Info(format string, args ...interface{}) {
	if colorsEnabled {
		fmt.Printf(Cyan+format+Reset+"\n", args...)
	} else {
		fmt.Printf(format+"\n", args...)
	}
}

// Bold prints a bold message
func Bold(format string, args ...interface{}) {
	if colorsEnabled {
		fmt.Printf(BoldCode+format+Reset+"\n", args...)
	} else {
		fmt.Printf(format+"\n", args...)
	}
}

// Sprintf returns a colored string without printing
func Sprintf(colorCode, format string, args ...interface{}) string {
	if colorsEnabled {
		return fmt.Sprintf(colorCode+format+Reset, args...)
	}
	return fmt.Sprintf(format, args...)
}

// DisableColors turns off color output
func DisableColors() {
	colorsEnabled = false
}

// EnableColors turns on color output
func EnableColors() {
	colorsEnabled = true
}
