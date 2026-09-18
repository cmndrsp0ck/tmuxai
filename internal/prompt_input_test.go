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
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
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
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
	m.ta.SetWidth(80)
	m = pressKey(m, tea.KeyCtrlD)

	if !m.eof {
		t.Fatalf("expected eof to be true on Ctrl+D with empty buffer")
	}
}

func TestPromptInputCtrlDWithTextDoesNotExit(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
	m.ta.SetWidth(80)
	m = typeText(m, "abc")
	m = pressKey(m, tea.KeyCtrlD)

	if m.eof {
		t.Fatalf("did not expect eof when buffer is non-empty")
	}
}

func TestPromptInputAltEnterInsertsNewlineInsteadOfSubmitting(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
	m.ta.SetWidth(80)
	m = typeText(m, "first line")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	m = updated.(promptInputModel)
	m = typeText(m, "second line")

	if m.submitted {
		t.Fatalf("alt+enter must not submit the line")
	}
	if got, want := m.ta.Value(), "first line\nsecond line"; got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
	if got := m.ta.LineCount(); got != 2 {
		t.Fatalf("expected 2 logical lines, got %d", got)
	}
}

func TestPromptInputCtrlCClearsLine(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
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

	m := newPromptInputModel("» ", history, nil, "", 20)
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
	m := newPromptInputModel("» ", simplehistory.New(), candidates, "", 20)
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
	m := newPromptInputModel("» ", simplehistory.New(), candidates, "", 20)
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
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
	m.ta.SetWidth(20)

	m = typeText(m, strings.Repeat("word ", 20))

	if got := m.ta.LineCount(); got != 1 {
		t.Fatalf("expected a single logical line (soft-wrapped), got %d logical lines", got)
	}
	if got := m.ta.LineInfo().Height; got <= 1 {
		t.Fatalf("expected the long line to visually wrap across multiple rows, got height %d", got)
	}
}

func TestPromptInputSignalsGrowWhenContentExceedsHeight(t *testing.T) {
	// Starting at promptInputStartHeight (2 rows), typing a line that wraps
	// to more than 2 rows must not resize the running program's textarea
	// (that desyncs Bubble Tea's renderer - see the comment on
	// wantedHeight); it must instead ask to be relaunched taller.
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", promptInputStartHeight)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 20})
	m = updated.(promptInputModel)

	m = typeText(m, strings.Repeat("word ", 20))

	if !m.needsResize {
		t.Fatalf("expected needsResize once wrapped content exceeds the fixed height")
	}
	if m.resizeTo <= m.height {
		t.Fatalf("expected resizeTo (%d) to exceed the original height (%d)", m.resizeTo, m.height)
	}
	if m.submitted || m.eof {
		t.Fatalf("a resize request must not also look like submit or EOF")
	}
}

func TestPromptInputShrinksWhenContentNoLongerNeedsTheHeight(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", promptInputStartHeight)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 20, Height: 20})
	m = updated.(promptInputModel)

	m = typeText(m, strings.Repeat("word ", 20))
	if !m.needsResize || m.resizeTo <= promptInputStartHeight {
		t.Fatalf("test setup: expected the long line to have grown the height first")
	}
	grownHeight := m.resizeTo

	// Simulate readPromptLine relaunching at the grown height, then the
	// user deleting most of the text back down.
	m2 := newPromptInputModel("» ", simplehistory.New(), nil, "", grownHeight)
	updated, _ = m2.Update(tea.WindowSizeMsg{Width: 20, Height: 20})
	m2 = updated.(promptInputModel)
	m2.ta.SetValue("word ")
	m2.ta.CursorEnd()

	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m2 = updated.(promptInputModel)

	if !m2.needsResize {
		t.Fatalf("expected needsResize once content shrinks below the current height")
	}
	if m2.resizeTo >= grownHeight {
		t.Fatalf("expected resizeTo (%d) to shrink below the grown height (%d)", m2.resizeTo, grownHeight)
	}
	if m2.resizeTo < promptInputStartHeight {
		t.Fatalf("expected resizeTo (%d) to never shrink below the start height (%d)", m2.resizeTo, promptInputStartHeight)
	}
}

func TestPromptInputDoesNotResizeWithinItsHeight(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", promptInputStartHeight)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m = updated.(promptInputModel)

	m = typeText(m, "short message")

	if m.needsResize {
		t.Fatalf("did not expect a resize request for content that fits the starting height")
	}
}

func TestPromptInputCollapsesLargePastes(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
	m.ta.SetWidth(80)

	pasted := strings.Repeat("line\n", 121) + "line" // 122 lines
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pasted), Paste: true})
	m = updated.(promptInputModel)

	got := m.ta.Value()
	if strings.Contains(got, "line\nline") {
		t.Fatalf("expected the raw pasted text to be collapsed, got %q", got)
	}
	if !strings.Contains(got, "[Pasted text #1 +122 lines]") {
		t.Fatalf("expected a collapsed-paste placeholder, got %q", got)
	}
	if len(m.pastes) != 1 {
		t.Fatalf("expected exactly one tracked paste, got %d", len(m.pastes))
	}

	expanded := expandPastes(got, m.pastes)
	if expanded != pasted {
		t.Fatalf("expected expandPastes to restore the original text, got %q", expanded)
	}
}

func TestPromptInputCollapsesLargePastesWithCarriageReturnLineEndings(t *testing.T) {
	// Terminals (confirmed via tmux paste-buffer) report line breaks within
	// a bracketed paste as \r, not \n - the same byte Enter itself sends.
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
	m.ta.SetWidth(80)

	pasted := strings.Repeat("line\r", 121) + "line"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pasted), Paste: true})
	m = updated.(promptInputModel)

	if !strings.Contains(m.ta.Value(), "[Pasted text #1 +122 lines]") {
		t.Fatalf("expected \\r-delimited pastes to be counted and collapsed, got %q", m.ta.Value())
	}
}

func TestPromptInputDoesNotCollapseSmallPastes(t *testing.T) {
	m := newPromptInputModel("» ", simplehistory.New(), nil, "", 20)
	m.ta.SetWidth(80)

	pasted := "line one\nline two\nline three"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(pasted), Paste: true})
	m = updated.(promptInputModel)

	if got := m.ta.Value(); got != pasted {
		t.Fatalf("expected a short paste to be inserted verbatim, got %q", got)
	}
	if len(m.pastes) != 0 {
		t.Fatalf("did not expect a short paste to be tracked, got %v", m.pastes)
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
