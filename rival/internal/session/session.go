package session

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/1F47E/rival/internal/config"
	"github.com/google/uuid"
)

type Session struct {
	ID            string     `json:"id"`
	GroupID       string     `json:"group_id,omitempty"`
	CLI           string     `json:"cli"`
	Mode          string     `json:"mode"`
	Model         string     `json:"model"`
	Effort        string     `json:"effort"`
	ReviewScope   string     `json:"review_scope,omitempty"`
	Prompt        string     `json:"prompt,omitempty"`
	PromptPreview string     `json:"prompt_preview,omitempty"`
	PromptHash    string     `json:"prompt_hash,omitempty"`
	Status        string     `json:"status"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       *time.Time `json:"end_time,omitempty"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	Duration      string     `json:"duration,omitempty"`
	WorkDir       string     `json:"work_dir"`
	LogFile       string     `json:"log_file"`
	OutputBytes   int64      `json:"output_bytes"`
	OutputLines   int        `json:"output_lines"`
	ErrorMsg      string     `json:"error,omitempty"`
	PID           int        `json:"pid"`
}

// New creates a new session and writes the initial JSON file.
// groupID links sessions that belong together (e.g. megareview); pass "" for standalone.
func New(cli, mode, model, effort, workdir, prompt, reviewScope, groupID string) (*Session, error) {
	dir := config.SessionDirPath()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create session dir: %w", err)
	}

	id := uuid.New().String()
	logFile := filepath.Join(dir, id+".log")

	preview := prompt
	if len(preview) > config.PromptPreviewLen {
		preview = preview[:config.PromptPreviewLen]
	}

	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(prompt)))

	s := &Session{
		ID:            id,
		GroupID:       groupID,
		CLI:           cli,
		Mode:          mode,
		Model:         model,
		Effort:        effort,
		ReviewScope:   reviewScope,
		Prompt:        prompt,
		PromptPreview: preview,
		PromptHash:    hash,
		Status:        "running",
		StartTime:     time.Now(),
		WorkDir:       workdir,
		LogFile:       logFile,
	}

	if err := s.Save(); err != nil {
		return nil, err
	}
	return s, nil
}

// Save writes the session JSON atomically (tmp file + rename).
func (s *Session) Save() error {
	dir := config.SessionDirPath()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal session: %w", err)
	}

	tmp := filepath.Join(dir, s.ID+".json.tmp")
	final := filepath.Join(dir, s.ID+".json")

	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write session tmp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp) // clean up orphaned temp file
		return fmt.Errorf("rename session: %w", err)
	}
	return nil
}

// Complete marks the session as completed.
func (s *Session) Complete(exitCode int, outputBytes int64, outputLines int) error {
	now := time.Now()
	s.Status = "completed"
	s.ExitCode = &exitCode
	s.EndTime = &now
	s.Duration = now.Sub(s.StartTime).Round(time.Second).String()
	s.OutputBytes = outputBytes
	s.OutputLines = outputLines
	return s.Save()
}

// Fail marks the session as failed.
func (s *Session) Fail(exitCode int, errMsg string) error {
	now := time.Now()
	s.Status = "failed"
	s.ExitCode = &exitCode
	s.EndTime = &now
	s.Duration = now.Sub(s.StartTime).Round(time.Second).String()
	s.ErrorMsg = errMsg
	return s.Save()
}

// LoadAll reads and returns all sessions, sorted newest first.
func LoadAll() []*Session {
	dir := config.SessionDirPath()
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil
	}

	var sessions []*Session
	for _, path := range matches {
		if strings.HasSuffix(path, ".json.tmp") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var s Session
		if err := json.Unmarshal(data, &s); err != nil {
			continue
		}
		sessions = append(sessions, &s)
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions
}

// OpenLog opens the session log file for writing.
func (s *Session) OpenLog() (*os.File, error) {
	return os.OpenFile(s.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
}
