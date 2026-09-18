package internal

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/nyaosorg/go-readline-ny/simplehistory"
)

// errEOF is returned by readPromptLine when the user signals end-of-input
// (Ctrl+D on an empty line), mirroring the io.EOF contract the previous
// go-readline-ny based loop relied on.
var errEOF = errors.New("eof")

// candidatesFunc mirrors the signature go-readline-ny's completion package
// used, so newCompleter's logic can be reused unchanged.
type candidatesFunc func(fieldsBeforeCursor []string) (completionSet []string, listingSet []string)

// promptInputModel is a small Bubble Tea program whose only job is to read
// a single (possibly multi-line, line-wrapped) line of input, reusing the
// history, tab-completion and external-editor behavior of the previous
// go-readline-ny based prompt.
type promptInputModel struct {
	ta          textarea.Model
	promptWidth int

	history    *simplehistory.Container
	historyPos int
	draft      string

	candidates candidatesFunc

	completionList []string
	editorFile     string

	submitted bool
	value     string
	eof       bool
}

type editorFinishedMsg struct{ err error }

// promptInputHeight is the fixed number of rows the input viewport shows.
// Bubble Tea's default renderer diffs frames by line count: it moves the
// cursor up by the previous frame's row count, then rewrites. If the
// textarea's rendered height changes between two Updates of the same
// running Program, and the terminal has to scroll to make room for the
// extra rows, that scroll isn't reflected in the renderer's bookkeeping and
// every following frame is drawn at the wrong offset, silently clobbering
// already-typed text. Keeping the height constant for the life of the
// Program avoids that entirely; input longer than this degrades to the
// textarea's own cursor-following internal scroll instead of corrupting
// the display.
const promptInputHeight = 6

func newPromptInputModel(promptText string, history *simplehistory.Container, candidates candidatesFunc, bg string) promptInputModel {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(promptInputHeight)
	ta.EndOfBufferCharacter = ' '

	promptWidth := lipgloss.Width(promptText)
	continuation := strings.Repeat(" ", promptWidth)
	ta.SetPromptFunc(promptWidth, func(lineIdx int) string {
		if lineIdx == 0 {
			return promptText
		}
		return continuation
	})

	if bg != "" {
		bgColor := lipgloss.Color(bg)
		ta.FocusedStyle.Text = ta.FocusedStyle.Text.Background(bgColor)
		ta.FocusedStyle.CursorLine = ta.FocusedStyle.CursorLine.Background(bgColor)
		ta.FocusedStyle.Placeholder = ta.FocusedStyle.Placeholder.Background(bgColor)
		ta.FocusedStyle.Prompt = ta.FocusedStyle.Prompt.Background(bgColor)
		ta.FocusedStyle.EndOfBuffer = ta.FocusedStyle.EndOfBuffer.Background(bgColor)
	}

	ta.Focus()

	return promptInputModel{
		ta:          ta,
		promptWidth: promptWidth,
		history:     history,
		historyPos:  history.Len(),
		candidates:  candidates,
	}
}

func (m promptInputModel) Init() tea.Cmd {
	return textarea.Blink
}

func (m promptInputModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	return m.handleMsg(msg)
}

func (m promptInputModel) handleMsg(msg tea.Msg) (promptInputModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// Leave one spare column so a fully-padded wrapped row never lands
		// exactly on the terminal's last column, which can trip a
		// terminal's own deferred-wrap handling and desync Bubble Tea's
		// line-based repaint.
		width := msg.Width - 1
		if width < 1 {
			width = 1
		}
		m.ta.SetWidth(width)
		return m, nil

	case editorFinishedMsg:
		next, cmd := m.finishEditor(msg.err)
		return next.(promptInputModel), cmd

	case tea.KeyMsg:
		m.completionList = nil

		switch msg.String() {
		case "enter":
			m.submitted = true
			m.value = m.ta.Value()
			return m, tea.Quit

		case "ctrl+c":
			m.ta.SetValue("")
			m.historyPos = m.history.Len()
			return m, nil

		case "ctrl+d":
			if m.ta.Value() == "" {
				m.eof = true
				return m, tea.Quit
			}
			return m, nil

		case "tab":
			m.completeAtCursor()
			return m, nil

		case "ctrl+o", "alt+e":
			next, cmd := m.openExternalEditor()
			return next.(promptInputModel), cmd

		case "up":
			if m.atFirstDisplayRow() {
				m.historyPrev()
				return m, nil
			}

		case "down":
			if m.atLastDisplayRow() {
				m.historyNext()
				return m, nil
			}
		}
	}

	var cmd tea.Cmd
	m.ta, cmd = m.ta.Update(msg)
	return m, cmd
}

func (m promptInputModel) View() string {
	view := m.ta.View()
	if len(m.completionList) > 0 {
		view += "\n" + strings.Join(m.completionList, "  ")
	}
	return view
}

// atFirstDisplayRow reports whether the cursor sits on the very first
// visual row of the buffer (accounting for soft-wrapped lines), i.e. the
// point at which pressing Up should recall history instead of moving the
// cursor within the text.
func (m promptInputModel) atFirstDisplayRow() bool {
	li := m.ta.LineInfo()
	return m.ta.Line() == 0 && li.RowOffset == 0
}

// atLastDisplayRow is the Down-key equivalent of atFirstDisplayRow.
func (m promptInputModel) atLastDisplayRow() bool {
	li := m.ta.LineInfo()
	return m.ta.Line() == m.ta.LineCount()-1 && li.RowOffset == li.Height-1
}

func (m *promptInputModel) historyPrev() {
	if m.history.Len() == 0 || m.historyPos == 0 {
		return
	}
	if m.historyPos == m.history.Len() {
		m.draft = m.ta.Value()
	}
	m.historyPos--
	m.ta.SetValue(m.history.At(m.historyPos))
	m.ta.CursorEnd()
}

func (m *promptInputModel) historyNext() {
	if m.historyPos >= m.history.Len() {
		return
	}
	m.historyPos++
	if m.historyPos == m.history.Len() {
		m.ta.SetValue(m.draft)
	} else {
		m.ta.SetValue(m.history.At(m.historyPos))
	}
	m.ta.CursorEnd()
}

// completeAtCursor implements the same single-match/common-prefix/list
// behavior as go-readline-ny's completion.Complete, but operates on the
// plain buffer text and only completes at the end of the input (tab
// completion is only ever invoked while typing forward).
func (m *promptInputModel) completeAtCursor() {
	if m.candidates == nil {
		return
	}
	text := m.ta.Value()
	fields, lastWordStart := splitFields(text)
	if len(fields) == 0 {
		return
	}

	list, baseList := m.candidates(fields)
	if len(baseList) == 0 {
		baseList = list
	}
	list, baseList = removeUnmatchedCandidates(list, baseList, fields[len(fields)-1])
	if len(list) == 0 {
		return
	}

	if len(list) == 1 {
		m.ta.SetValue(text[:lastWordStart] + list[0] + " ")
		m.ta.CursorEnd()
		return
	}

	prefix := commonCandidatePrefix(list)
	if strings.EqualFold(fields[len(fields)-1], prefix) {
		m.completionList = baseList
		return
	}
	m.ta.SetValue(text[:lastWordStart] + prefix)
	m.ta.CursorEnd()
}

func (m promptInputModel) openExternalEditor() (tea.Model, tea.Cmd) {
	fname, err := writeTempPromptFile(m.ta.Value())
	if err != nil {
		return m, nil
	}
	m.editorFile = fname

	editorBin := os.Getenv("EDITOR")
	if editorBin == "" {
		editorBin = "vim"
	}
	cmd := exec.Command(editorBin, fname)

	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return editorFinishedMsg{err: err}
	})
}

func (m promptInputModel) finishEditor(runErr error) (tea.Model, tea.Cmd) {
	fname := m.editorFile
	m.editorFile = ""
	defer func() { _ = os.Remove(fname) }()

	if runErr != nil || fname == "" {
		return m, nil
	}

	result, err := readTempPromptFile(fname)
	if err != nil {
		return m, nil
	}
	m.ta.SetValue(result)
	m.ta.CursorEnd()
	return m, nil
}

// writeTempPromptFile and readTempPromptFile replace the previous
// startEditor helper, split in two so the external editor can be run via
// tea.ExecProcess (which needs to release the terminal before the command
// starts and reclaim it once the command exits).
func writeTempPromptFile(source string) (string, error) {
	fd, err := os.CreateTemp("", "tmuxai-prompt-*.txt")
	if err != nil {
		return "", err
	}
	fname := fd.Name()
	if _, err := fmt.Fprint(fd, source); err != nil {
		_ = fd.Close()
		_ = os.Remove(fname)
		return "", err
	}
	if err := fd.Close(); err != nil {
		_ = os.Remove(fname)
		return "", err
	}
	return fname, nil
}

func readTempPromptFile(fname string) (string, error) {
	data, err := os.ReadFile(fname)
	if err != nil {
		return "", err
	}
	data = bytes.TrimSuffix(data, []byte{'\n'})
	data = bytes.TrimSuffix(data, []byte{'\r'})
	return string(data), nil
}

// readPromptLine runs a scoped Bubble Tea program that reads a single line
// (which may be soft-wrapped across multiple terminal rows) of input,
// reusing history, tab-completion and the external-editor shortcut. It runs
// inline (no alt screen) so prior chat history stays visible while
// composing; see growHeight for why it may transparently relaunch itself at
// a taller fixed height as the input grows.
func readPromptLine(ctx context.Context, promptText string, history *simplehistory.Container, candidates candidatesFunc, bg string) (string, error) {
	model := newPromptInputModel(promptText, history, candidates, bg)

	p := tea.NewProgram(model, tea.WithContext(ctx), tea.WithOutput(os.Stdout), tea.WithInput(os.Stdin))
	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	// Bubble Tea's inline renderer only erases the single row the cursor
	// ends up on when the Program stops; the other promptInputHeight-1
	// reserved rows (blank padding, or stale wrapped-text rows from before
	// the final keystroke) are left on screen. Reclaim that whole block
	// ourselves so the caller can print a single clean line in its place.
	if promptInputHeight > 1 {
		fmt.Fprintf(os.Stdout, "\x1b[%dA\x1b[J", promptInputHeight-1)
	}

	m := finalModel.(promptInputModel)
	if m.eof {
		return "", errEOF
	}
	if !m.submitted {
		return "", ctx.Err()
	}
	return m.value, nil
}

// --- completion helpers, ported from go-readline-ny's completion package to
// operate on plain string+cursor state instead of a readline.Buffer. ---

func splitFields(text string) (fields []string, lastWordStart int) {
	i := 0
	n := len(text)
	for i < n {
		for i < n && text[i] == ' ' {
			i++
			if i >= n {
				fields = append(fields, "")
				lastWordStart = i
				return
			}
		}
		start := i
		for i < n && text[i] != ' ' {
			i++
		}
		fields = append(fields, text[start:i])
		lastWordStart = start
	}
	return
}

func removeUnmatchedCandidates(full, base []string, prefix string) (newFull, newBase []string) {
	for i, name := range full {
		if len(name) >= len(prefix) && strings.EqualFold(prefix, name[:len(prefix)]) {
			newFull = append(newFull, name)
			newBase = append(newBase, base[i])
		}
	}
	return
}

func commonCandidatePrefix(list []string) string {
	if len(list) < 1 {
		return ""
	}
	common := []rune(list[0])
	minimumLength := len(list[0])
	minimumIndex := 0
	for index, f := range list[1:] {
		fr := []rune(f)
		i := 0
		for i < len(common) && i < len(fr) && unicode.ToUpper(common[i]) == unicode.ToUpper(fr[i]) {
			i++
		}
		common = common[:i]
		if len(f) < minimumLength {
			minimumLength = len(f)
			minimumIndex = index + 1
		}
	}
	return string([]rune(list[minimumIndex])[:len(common)])
}
