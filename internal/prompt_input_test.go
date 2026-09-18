package internal

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
)

func typeText(m promptInputModel, s string) promptInputModel {
	for _, r := range s {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(promptInputModel)
	}
	return m
}

func pressKey(m promptInputModel, keyType tea.KeyType) promptInputModel {
	updated, _ := m.Update(tea.KeyMsg{Type: keyType})
	return updated.(promptInputModel)
}

func TestPromptInputSubmit(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "")
	m.ta.SetWidth(80)
	m = typeText(m, "hello world")
	m = pressKey(m, tea.KeyEnter)

	if !m.submitted {
		t.Fatalf("expected submitted to be true")
	}
	if m.value != "hello world" {
		t.Fatalf("expected value %q, got %q", "hello world", m.value)
	}
}

func TestPromptInputCtrlDOnEmptyLineSignalsEOF(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "")
	m.ta.SetWidth(80)
	m = pressKey(m, tea.KeyCtrlD)

	if !m.eof {
		t.Fatalf("expected eof to be true on Ctrl+D with empty buffer")
	}
}

func TestPromptInputCtrlDWithTextDoesNotExit(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "")
	m.ta.SetWidth(80)
	m = typeText(m, "abc")
	m = pressKey(m, tea.KeyCtrlD)

	if m.eof {
		t.Fatalf("did not expect eof when buffer is non-empty")
	}
}

func TestPromptInputCtrlCClearsLine(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "")
	m.ta.SetWidth(80)
	m = typeText(m, "some text")
	m = pressKey(m, tea.KeyCtrlC)

	if m.ta.Value() != "" {
		t.Fatalf("expected buffer to be cleared, got %q", m.ta.Value())
	}
	if m.submitted {
		t.Fatalf("Ctrl+C must not submit the line")
	}
}

func TestPromptInputHistoryNavigation(t *testing.T) {
	history := simplehistory.New()
	history.Add("first command")
	history.Add("second command")

	m := newPromptInputModel("» ", history, nil, "")
	m.ta.SetWidth(80)
	m = typeText(m, "draft in progress")

	m = pressKey(m, tea.KeyUp)
	if got := m.ta.Value(); got != "second command" {
		t.Fatalf("expected most recent history entry, got %q", got)
	}

	m = pressKey(m, tea.KeyUp)
	if got := m.ta.Value(); got != "first command" {
		t.Fatalf("expected oldest history entry, got %q", got)
	}

	// At the oldest entry, Up should have no further effect.
	m = pressKey(m, tea.KeyUp)
	if got := m.ta.Value(); got != "first command" {
		t.Fatalf("expected to stay on oldest entry, got %q", got)
	}

	m = pressKey(m, tea.KeyDown)
	if got := m.ta.Value(); got != "second command" {
		t.Fatalf("expected to move forward to second command, got %q", got)
	}

	m = pressKey(m, tea.KeyDown)
	if got := m.ta.Value(); got != "draft in progress" {
		t.Fatalf("expected to restore draft, got %q", got)
	}
}

func TestPromptInputTabCompletionSingleMatch(t *testing.T) {
	candidates := func(fields []string) ([]string, []string) {
		return []string{"/help"}, []string{"/help"}
	}
	m := newPromptInputModel("» ", simplehistory.New(), candidates, "")
	m.ta.SetWidth(80)
	m = typeText(m, "/hel")
	m = pressKey(m, tea.KeyTab)

	if got := m.ta.Value(); got != "/help " {
		t.Fatalf("expected single match to autocomplete with trailing space, got %q", got)
	}
}

func TestPromptInputTabCompletionMultipleMatchesShowsList(t *testing.T) {
	candidates := func(fields []string) ([]string, []string) {
		return []string{"/help", "/history"}, []string{"/help", "/history"}
	}
	m := newPromptInputModel("» ", simplehistory.New(), candidates, "")
	m.ta.SetWidth(80)
	m = typeText(m, "/h")
	m = pressKey(m, tea.KeyTab)

	if got := m.ta.Value(); got != "/h" {
		t.Fatalf("expected common prefix already typed to leave value unchanged, got %q", got)
	}
	if len(m.completionList) != 2 {
		t.Fatalf("expected completion list with 2 entries, got %v", m.completionList)
	}
}

func TestPromptInputWrapsLongLinesInsteadOfScrolling(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "")
	m.ta.SetWidth(20)

	m = typeText(m, strings.Repeat("word ", 20))

	if got := m.ta.LineCount(); got != 1 {
		t.Fatalf("expected a single logical line (soft-wrapped), got %d logical lines", got)
	}
	if got := m.ta.LineInfo().Height; got <= 1 {
		t.Fatalf("expected the long line to visually wrap across multiple rows, got height %d", got)
	}
}

func TestSplitFields(t *testing.T) {
	cases := []struct {
		text          string
		wantFields    []string
		wantLastStart int
	}{
		{"", nil, 0},
		{"/config", []string{"/config"}, 0},
		{"/config set", []string{"/config", "set"}, 8},
		{"/config set ", []string{"/config", "set", ""}, 12},
	}
	for _, c := range cases {
		fields, lastStart := splitFields(c.text)
		if len(fields) != len(c.wantFields) {
			t.Fatalf("splitFields(%q) = %v, want %v", c.text, fields, c.wantFields)
		}
		for i := range fields {
			if fields[i] != c.wantFields[i] {
				t.Fatalf("splitFields(%q) = %v, want %v", c.text, fields, c.wantFields)
			}
		}
		if lastStart != c.wantLastStart {
			t.Fatalf("splitFields(%q) lastWordStart = %d, want %d", c.text, lastStart, c.wantLastStart)
		}
	}
}

func TestCommonCandidatePrefix(t *testing.T) {
	got := commonCandidatePrefix([]string{"/help", "/history"})
	if got != "/h" {
		t.Fatalf("expected common prefix %q, got %q", "/h", got)
	}
}
