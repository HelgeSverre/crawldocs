package main

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

// Color definitions
var (
	// Success messages (green)
	colorSuccess = color.New(color.FgGreen).SprintFunc()

	// Skip/warning messages (yellow)
	colorWarn = color.New(color.FgYellow).SprintFunc()

	// Error messages (red)
	colorError = color.New(color.FgRed).SprintFunc()

	// Info messages (cyan)
	colorInfo = color.New(color.FgCyan).SprintFunc()

	// Dim messages (gray)
	colorDim = color.New(color.Faint).SprintFunc()

	// Bold for emphasis
	colorBold = color.New(color.Bold).SprintFunc()

	// Rate limit messages (purple/magenta)
	colorRateLimit = color.New(color.FgMagenta).SprintFunc()
)

// Output prefixes
const (
	prefixSaved     = "✓"
	prefixSkipped   = "⚠"
	prefixError     = "✗"
	prefixVisiting  = "→"
	prefixProgress  = "◆"
	prefixInfo      = "ℹ"
	prefixRateLimit = "⏱"
)

// logSuccess prints a success message
func logSuccess(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorSuccess(prefixSaved), msg)
}

// logSkip prints a skip/warning message
func logSkip(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorWarn(prefixSkipped), msg)
}

// logWarn prints a warning message (alias for logSkip)
func logWarn(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorWarn(prefixSkipped), msg)
}

// logError prints an error message
func logError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorError(prefixError), msg)
}

// logVisit prints a URL visit message
func logVisit(url string) {
	fmt.Printf("%s %s\n", colorInfo(prefixVisiting), colorDim(url))
}

// logProgress prints a progress message
func logProgress(current, total int, percentage float64) {
	msg := fmt.Sprintf("Progress: %d/%d (%.1f%%)", current, total, percentage)
	fmt.Printf("%s %s\n", colorInfo(prefixProgress), msg)
}

// logInfo prints an informational message
func logInfo(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorInfo(prefixInfo), msg)
}

// logDim prints a dimmed message
func logDim(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Println(colorDim(msg))
}

// logRateLimit prints a rate limit message
func logRateLimit(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Printf("%s %s\n", colorRateLimit(prefixRateLimit), msg)
}

// isAlreadyVisitedError checks if the error is because URL was already visited
func isAlreadyVisitedError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "already visited")
}

// getHTTPStatusText returns a human-readable status text for common HTTP codes
func getHTTPStatusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Found"
	case 304:
		return "Not Modified"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 429:
		return "Too Many Requests"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	default:
		return "Unknown"
	}
}
