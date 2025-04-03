package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	// TitleStyle defines the style for titles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FAFAFA")).
			Background(lipgloss.Color("#7D56F4")).
			PaddingLeft(2).
			PaddingRight(2).
			MarginBottom(1)

	// InfoStyle defines the style for informational text
	InfoStyle = lipgloss.NewStyle().
			Italic(true).
			Foreground(lipgloss.Color("#888888"))

	// SuccessStyle defines the style for success messages
	SuccessStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#04B575"))

	// ErrorStyle defines the style for error messages
	ErrorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF0000"))
)

// FormatTitle formats a string as a title
func FormatTitle(title string) string {
	return TitleStyle.Render(title)
}

// FormatInfo formats a string as informational text
func FormatInfo(info string) string {
	return InfoStyle.Render(info)
}

// FormatSuccess formats a string as a success message
func FormatSuccess(message string) string {
	return SuccessStyle.Render(message)
}

// FormatError formats a string as an error message
func FormatError(err string) string {
	return ErrorStyle.Render(err)
}

// ProgressBar creates a simple text-based progress bar
func ProgressBar(progress float64, width int) string {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	filled := int(float64(width) * progress)
	empty := width - filled

	bar := strings.Repeat("█", filled) + strings.Repeat("░", empty)
	percentage := fmt.Sprintf(" %3.0f%%", progress*100)

	return bar + percentage
}
