package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
)

// defaultSessionName is used when the user doesn't provide a name to /save or /load.
const defaultSessionName = "last"

// invalidSessionNameChars matches characters that aren't safe to use in a filename.
var invalidSessionNameChars = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

// sanitizeSessionName normalizes a user-provided session name into a safe filename stem.
func sanitizeSessionName(name string) string {
	name = invalidSessionNameChars.ReplaceAllString(name, "-")
	if name == "" {
		name = defaultSessionName
	}
	return name
}

// sessionFilePath returns the on-disk path for a given session name.
func sessionFilePath(name string) string {
	return filepath.Join(config.GetSessionsDir(), sanitizeSessionName(name)+".json")
}

// SavedSession is the on-disk representation of a saved chat session.
// Only the user/agent message history is persisted; tmux pane state,
// exec history, and loaded KBs/skills are intentionally not saved.
type SavedSession struct {
	SavedAt  time.Time     `json:"saved_at"`
	Messages []ChatMessage `json:"messages"`
}

// SaveSession writes the current chat history to disk under the given name.
func (m *Manager) SaveSession(name string) error {
	if len(m.Messages) == 0 {
		return fmt.Errorf("no chat history to save")
	}

	session := SavedSession{
		SavedAt:  time.Now(),
		Messages: m.Messages,
	}

	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode session: %w", err)
	}

	path := sessionFilePath(name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

// LoadSession reads a previously saved chat history from disk and replaces
// the manager's current message history with it.
func (m *Manager) LoadSession(name string) error {
	path := sessionFilePath(name)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no saved session named %q", sanitizeSessionName(name))
		}
		return fmt.Errorf("failed to read session file: %w", err)
	}

	var session SavedSession
	if err := json.Unmarshal(data, &session); err != nil {
		return fmt.Errorf("failed to parse session file: %w", err)
	}

	m.Messages = session.Messages

	// A restored conversation may already be close to (or over) the context
	// budget; squash it immediately if needed rather than waiting for the
	// next message round-trip to discover that.
	if m.needSquash() {
		m.squashHistory()
	}

	return nil
}

// PrintChatHistory writes the current chat history to the terminal, formatted
// the same way messages are shown as they happen. It's meant to be called
// after LoadSession, since restored messages otherwise never get printed
// (unlike live messages, which appear on screen as they're typed/received).
func (m *Manager) PrintChatHistory() {
	for _, msg := range m.Messages {
		if msg.FromUser {
			fmt.Println(m.GetPrompt() + msg.Content)
		} else {
			fmt.Println(system.Cosmetics(msg.Content))
		}
	}
}

// ListSessions returns the names (without extension) of all saved sessions,
// sorted alphabetically.
func (m *Manager) ListSessions() ([]string, error) {
	entries, err := os.ReadDir(config.GetSessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read sessions directory: %w", err)
	}

	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		names = append(names, entry.Name()[:len(entry.Name())-len(".json")])
	}

	sort.Strings(names)
	return names, nil
}
