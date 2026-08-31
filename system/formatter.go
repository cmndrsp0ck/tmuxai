package system

import (
	"fmt"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
)

// InfoFormatter provides consistent formatting for TmuxAI information displays
type InfoFormatter struct {
	// Color schemes
	HeaderColor  Style
	LabelColor   Style
	SuccessColor Style
	WarningColor Style
	ErrorColor   Style
	NeutralColor Style
	falseColor   Style
}

// NewInfoFormatter creates a new formatter using TmuxAI's default color theme.
func NewInfoFormatter() *InfoFormatter {
	return NewInfoFormatterFromTheme(config.DefaultThemeConfig())
}

// NewInfoFormatterFromTheme creates a new formatter using the given theme
// config, e.g. the user's loaded cfg.Theme.
func NewInfoFormatterFromTheme(theme config.ThemeConfig) *InfoFormatter {
	return &InfoFormatter{
		HeaderColor:  NewStyle(theme.Model, true),
		LabelColor:   NewStyle(theme.Neutral, true),
		SuccessColor: NewStyle(theme.Success, true),
		WarningColor: NewStyle(theme.Warning, true),
		ErrorColor:   NewStyle(theme.Error, true),
		NeutralColor: NewStyle(theme.Neutral, false),
		falseColor:   NewStyle(theme.State, false),
	}
}

// FormatSection prints a section header
func (f *InfoFormatter) FormatSection(title string) string {
	return fmt.Sprintf("%s\n%s\n",
		f.HeaderColor.Sprint(title),
		f.NeutralColor.Sprint(strings.Repeat("─", len(title))))
}

// FormatKeyValue prints a key-value pair with consistent formatting
func (f *InfoFormatter) FormatKeyValue(key string, value interface{}) string {
	return fmt.Sprintf("%s %s\n",
		f.LabelColor.Sprintf("%-16s:", key),
		fmt.Sprint(value))
}

// FormatProgressBar generates a visual indicator for percentage values
func (f *InfoFormatter) FormatProgressBar(percent float64, width int) string {
	if width <= 0 {
		width = 10
	}

	filled := int((percent / 100) * float64(width))
	if filled > width {
		filled = width
	}

	var bar string

	// Choose color based on percentage
	var barColor Style
	switch {
	case percent >= 90:
		barColor = f.ErrorColor
	case percent >= 70:
		barColor = f.WarningColor
	default:
		barColor = f.SuccessColor
	}

	// Generate the filled portion
	if filled > 0 {
		bar += barColor.Sprint(strings.Repeat("█", filled))
	}

	// Generate the empty portion
	if width-filled > 0 {
		bar += f.NeutralColor.Sprint(strings.Repeat("░", width-filled))
	}

	return fmt.Sprintf("%s %.1f%%", bar, percent)
}

// FormatBool formats boolean values with color
func (f *InfoFormatter) FormatBool(value bool) string {
	if value {
		return f.SuccessColor.Sprint("yes")
	}
	return f.falseColor.Sprint("no")
}
