package internal

import (
	"testing"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestSessionManager(t *testing.T) *Manager {
	t.Helper()
	t.Setenv("HOME", t.TempDir())

	return &Manager{
		Config:   &config.Config{MaxContextSize: 100000},
		Messages: []ChatMessage{},
	}
}

func TestSaveSession_EmptyHistory(t *testing.T) {
	m := newTestSessionManager(t)

	err := m.SaveSession("mywork")
	assert.Error(t, err)
}

func TestSaveAndLoadSession_RoundTrip(t *testing.T) {
	m := newTestSessionManager(t)

	m.Messages = []ChatMessage{
		{Content: "hello", FromUser: true, Timestamp: time.Now()},
		{Content: "hi there", FromUser: false, Timestamp: time.Now()},
	}

	require.NoError(t, m.SaveSession("mywork"))

	// Simulate a fresh manager with no history.
	m2 := newTestSessionManagerSameHome(t, m)
	m2.Messages = []ChatMessage{}

	require.NoError(t, m2.LoadSession("mywork"))
	require.Len(t, m2.Messages, 2)
	assert.Equal(t, "hello", m2.Messages[0].Content)
	assert.True(t, m2.Messages[0].FromUser)
	assert.Equal(t, "hi there", m2.Messages[1].Content)
	assert.False(t, m2.Messages[1].FromUser)
}

func TestLoadSession_NotFound(t *testing.T) {
	m := newTestSessionManager(t)

	err := m.LoadSession("does-not-exist")
	assert.Error(t, err)
}

func TestSaveSession_DefaultName(t *testing.T) {
	m := newTestSessionManager(t)
	m.Messages = []ChatMessage{{Content: "hi", FromUser: true, Timestamp: time.Now()}}

	require.NoError(t, m.SaveSession(defaultSessionName))

	sessions, err := m.ListSessions()
	require.NoError(t, err)
	assert.Contains(t, sessions, defaultSessionName)
}

func TestListSessions_Empty(t *testing.T) {
	m := newTestSessionManager(t)

	sessions, err := m.ListSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestAutoSaveSession_DefaultEnabled(t *testing.T) {
	m := newTestSessionManager(t)
	m.Config.Session = config.SessionConfig{AutoSave: true, AutoSaveName: "last"}
	m.Messages = []ChatMessage{{Content: "auto-saved", FromUser: true, Timestamp: time.Now()}}

	m.autoSaveSession()

	sessions, err := m.ListSessions()
	require.NoError(t, err)
	assert.Contains(t, sessions, "last")
}

func TestAutoSaveSession_Disabled(t *testing.T) {
	m := newTestSessionManager(t)
	m.Config.Session = config.SessionConfig{AutoSave: false, AutoSaveName: "last"}
	m.Messages = []ChatMessage{{Content: "should not save", FromUser: true, Timestamp: time.Now()}}

	m.autoSaveSession()

	sessions, err := m.ListSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestAutoSaveSession_EmptyHistorySkipped(t *testing.T) {
	m := newTestSessionManager(t)
	m.Config.Session = config.SessionConfig{AutoSave: true, AutoSaveName: "last"}

	m.autoSaveSession() // no messages; must not error or create a file

	sessions, err := m.ListSessions()
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

func TestSanitizeSessionName(t *testing.T) {
	assert.Equal(t, "my-work_1", sanitizeSessionName("my-work_1"))
	assert.Equal(t, "my-work", sanitizeSessionName("my/work"))
	assert.Equal(t, defaultSessionName, sanitizeSessionName(""))
}

// newTestSessionManagerSameHome builds a new Manager reusing the same HOME
// directory already configured on t (via the parent test), so it shares the
// sessions directory with an existing manager.
func newTestSessionManagerSameHome(t *testing.T, existing *Manager) *Manager {
	t.Helper()
	return &Manager{
		Config:   existing.Config,
		Messages: []ChatMessage{},
	}
}
