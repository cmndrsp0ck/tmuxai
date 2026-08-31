package system

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/charmbracelet/lipgloss"
)

// Style wraps a lipgloss.Style behind the small Sprint/Sprintf API that the
// rest of the codebase already used via fatih/color, so callers didn't need
// to change beyond swapping how the Style is constructed.
type Style struct {
	style lipgloss.Style
}

// NewStyle builds a Style that colors text with the given foreground color
// (an ANSI number like "2" or a hex string like "#22c55e").
func NewStyle(fg string, bold bool) Style {
	s := lipgloss.NewStyle()
	if fg != "" {
		s = s.Foreground(lipgloss.Color(fg))
	}
	if bold {
		s = s.Bold(true)
	}
	return Style{style: s}
}

// OnBackground returns a copy of the Style with the given background color
// applied (an ANSI number like "0" or a hex string like "#2a2a3d"). Passing
// an empty string returns the Style unchanged.
func (s Style) OnBackground(bg string) Style {
	if bg == "" {
		return s
	}
	return Style{style: s.style.Background(lipgloss.Color(bg))}
}

// Sprint renders its arguments styled, mirroring (*color.Color).Sprint.
func (s Style) Sprint(a ...interface{}) string {
	return s.style.Render(fmt.Sprint(a...))
}

// Sprintf renders a formatted string styled, mirroring (*color.Color).Sprintf.
func (s Style) Sprintf(format string, a ...interface{}) string {
	return s.style.Render(fmt.Sprintf(format, a...))
}

// Theme holds the named styles used throughout TmuxAI's TUI: the input
// prompt, confirmation dialogs, countdowns, and info panels.
type Theme struct {
	Primary Style
	Accent  Style
	Model   Style
	State   Style
	Success Style
	Warning Style
	Error   Style
	Neutral Style
}

// NewTheme builds a Theme from a config.ThemeConfig, as loaded from the
// user's TmuxAI config file (or its defaults).
func NewTheme(cfg config.ThemeConfig) Theme {
	return Theme{
		Primary: NewStyle(cfg.Primary, cfg.Bold),
		Accent:  NewStyle(cfg.Accent, cfg.Bold),
		Model:   NewStyle(cfg.Model, cfg.Bold),
		State:   NewStyle(cfg.State, cfg.Bold),
		Success: NewStyle(cfg.Success, cfg.Bold),
		Warning: NewStyle(cfg.Warning, cfg.Bold),
		Error:   NewStyle(cfg.Error, cfg.Bold),
		Neutral: NewStyle(cfg.Neutral, false),
	}
}

// DefaultTheme returns the Theme built from TmuxAI's default color config.
// Used where no user config is available, e.g. package-level defaults.
func DefaultTheme() Theme {
	return NewTheme(config.DefaultThemeConfig())
}

// BackgroundSequence returns the raw ANSI SGR escape sequence that sets a
// background color (an ANSI number like "5" or a hex string like
// "#2a2a3d"). It returns "" when bg is empty or unparsable.
//
// This is used to color the readline input area itself (as opposed to
// Style.OnBackground, which colors discrete pieces of already-rendered
// text such as the prompt label): go-readline-ny's Editor.DefaultColor
// takes a plain escape-sequence string it applies while the user types,
// rather than a lipgloss.Style.
func BackgroundSequence(bg string) string {
	if bg == "" {
		return ""
	}
	if strings.HasPrefix(bg, "#") {
		hex := strings.TrimPrefix(bg, "#")
		if len(hex) != 6 {
			return ""
		}
		v, err := strconv.ParseUint(hex, 16, 32)
		if err != nil {
			return ""
		}
		r, g, b := (v>>16)&0xFF, (v>>8)&0xFF, v&0xFF
		return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", r, g, b)
	}
	if n, err := strconv.Atoi(bg); err == nil && n >= 0 && n <= 255 {
		return fmt.Sprintf("\x1b[48;5;%dm", n)
	}
	return ""
}
