package server

import (
	"embed"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

//go:embed templates/index.html
var indexHTML embed.FS

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

const (
	defaultListLimit         = 100
	maxListLimit             = 500
	maxPromptBytes           = 256 << 10
	maxPromptSourceFileBytes = 2 << 20
	maxLogTailBytes          = 256 << 10
)

type sessionGroup struct {
	ID            string          `json:"id"`
	IsGroup       bool            `json:"is_group"`
	Kind          string          `json:"kind"` // group kind: "megareview" or "plan" ("" for solo)
	Sessions      []publicSession `json:"sessions"`
	Status        string          `json:"status"`
	CLI           string          `json:"cli"`
	Models        string          `json:"models"`
	Effort        string          `json:"effort"`
	Elapsed       string          `json:"elapsed"`
	WorkDir       string          `json:"work_dir"`
	PromptPreview string          `json:"prompt_preview"`
}

// publicSession is the bounded list-view contract. Full prompts, log paths,
// process IDs, and other executor internals never belong in the polling API.
type publicSession struct {
	ID            string     `json:"id"`
	CLI           string     `json:"cli"`
	Mode          string     `json:"mode"`
	Model         string     `json:"model"`
	Effort        string     `json:"effort"`
	ReviewScope   string     `json:"review_scope,omitempty"`
	PromptPreview string     `json:"prompt_preview,omitempty"`
	Status        string     `json:"status"`
	StartTime     time.Time  `json:"start_time"`
	QueuedAt      *time.Time `json:"queued_at,omitempty"`
	QueuePosition int        `json:"queue_position,omitempty"`
	EndTime       *time.Time `json:"end_time,omitempty"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	Duration      string     `json:"duration,omitempty"`
	WorkDir       string     `json:"work_dir"`
	OutputBytes   int64      `json:"output_bytes,omitempty"`
	OutputLines   int        `json:"output_lines,omitempty"`
	ErrorMsg      string     `json:"error,omitempty"`
	Account       string     `json:"account,omitempty"`
}

type stats struct {
	Running   int `json:"running"`
	Queued    int `json:"queued"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Total     int `json:"total"`
}

type apiResponse struct {
	Stats      stats          `json:"stats"`
	Groups     []sessionGroup `json:"groups"`
	Pagination pagination     `json:"pagination"`
	Revision   uint64         `json:"revision"`
	Version    string         `json:"version"`
}

type pagination struct {
	Shown       int  `json:"shown"`
	TotalGroups int  `json:"total_groups"`
	Limit       int  `json:"limit"`
	Offset      int  `json:"offset"`
	HasMore     bool `json:"has_more"`
	HasPrevious bool `json:"has_previous"`
}

type promptResponse struct {
	Prompt      string `json:"prompt"`
	Truncated   bool   `json:"truncated"`
	SourceBytes int64  `json:"source_bytes,omitempty"`
	PromptBytes int    `json:"prompt_bytes,omitempty"`
	PreviewOnly bool   `json:"preview_only,omitempty"`
}

func New(version string) *http.ServeMux {
	mux := http.NewServeMux()
	cache := newSessionCache(config.SessionDirPath())

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		data, err := indexHTML.ReadFile("templates/index.html")
		if err != nil {
			http.Error(w, "template not found", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		limit, offset, err := listPage(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		sessions, revision := cache.load()
		groups := groupSessions(sessions)
		totalGroups := len(groups)
		if offset > len(groups) {
			offset = len(groups)
		}
		end := min(offset+limit, len(groups))
		groups = groups[offset:end]
		if groups == nil {
			groups = []sessionGroup{}
		}

		var st stats
		st.Total = len(sessions)
		for _, s := range sessions {
			switch s.Status {
			case "running":
				st.Running++
			case "queued":
				st.Queued++
			case "completed":
				st.Completed++
			case "failed":
				st.Failed++
			}
		}

		resp := apiResponse{
			Stats:    st,
			Groups:   groups,
			Revision: revision,
			Version:  version,
			Pagination: pagination{
				Shown:       len(groups),
				TotalGroups: totalGroups,
				Limit:       limit,
				Offset:      offset,
				HasMore:     end < totalGroups,
				HasPrevious: offset > 0,
			},
		}

		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/run", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		id := r.URL.Query().Get("id")
		if id == "" || len(id) > 256 {
			http.Error(w, "invalid run id", http.StatusBadRequest)
			return
		}

		sessions, _ := cache.load()
		for _, group := range groupSessions(sessions) {
			if group.ID != id {
				continue
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(group)
			return
		}
		http.Error(w, "run not found", http.StatusNotFound)
	})

	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/")
		if len(parts) < 1 || len(parts) > 2 || (len(parts) == 2 && parts[1] != "log") {
			http.NotFound(w, r)
			return
		}
		id := parts[0]
		if !uuidRegex.MatchString(id) {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}

		s := cache.get(id)
		if s == nil {
			_, _ = cache.load()
			s = cache.get(id)
		}
		if s == nil {
			http.Error(w, "session metadata not found", http.StatusNotFound)
			return
		}

		if len(parts) == 1 {
			detail, err := loadPromptResponse(filepath.Join(config.SessionDirPath(), id+".json"), s)
			if err != nil {
				http.Error(w, "session detail not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(detail)
			return
		}

		logPath := filepath.Join(config.SessionDirPath(), id+".log")
		data, truncated, err := readLogTail(logPath, maxLogTailBytes)
		if err != nil {
			http.Error(w, "log not found", http.StatusNotFound)
			return
		}
		data = []byte(config.PublicRuntimeLog(s.CLI, s.Model, string(data)))
		if truncated {
			w.Header().Set("X-Rival-Log-Truncated", "true")
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(data)
	})

	return mux
}

func listPage(r *http.Request) (int, int, error) {
	raw := r.URL.Query().Get("limit")
	limit := defaultListLimit
	if raw == "" {
		raw = strconv.Itoa(defaultListLimit)
	}
	var err error
	limit, err = strconv.Atoi(raw)
	if err != nil || limit < 1 {
		return 0, 0, errors.New("limit must be a positive integer")
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	offset := 0
	if rawOffset := r.URL.Query().Get("offset"); rawOffset != "" {
		offset, err = strconv.Atoi(rawOffset)
		if err != nil || offset < 0 {
			return 0, 0, errors.New("offset must be a non-negative integer")
		}
	}
	return limit, offset, nil
}

func loadPromptResponse(path string, summary *session.Session) (promptResponse, error) {
	info, err := os.Stat(path)
	if err != nil {
		return promptResponse{}, err
	}
	if info.Size() > maxPromptSourceFileBytes {
		return promptResponse{
			Prompt:      summary.PromptPreview,
			Truncated:   true,
			SourceBytes: info.Size(),
			PromptBytes: len(summary.PromptPreview),
			PreviewOnly: true,
		}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return promptResponse{}, err
	}
	var stored struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return promptResponse{}, err
	}

	originalBytes := len(stored.Prompt)
	truncated := originalBytes > maxPromptBytes
	if truncated {
		stored.Prompt = strings.ToValidUTF8(stored.Prompt[:maxPromptBytes], "")
	}
	return promptResponse{
		Prompt:      stored.Prompt,
		Truncated:   truncated,
		SourceBytes: info.Size(),
		PromptBytes: originalBytes,
	}, nil
}

func readLogTail(path string, maxBytes int64) ([]byte, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, false, err
	}
	truncated := info.Size() > maxBytes
	if truncated {
		if _, err := f.Seek(-maxBytes, io.SeekEnd); err != nil {
			return nil, false, err
		}
	}

	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return nil, false, err
	}
	return []byte(strings.ToValidUTF8(string(data), "")), truncated, nil
}

func groupSessions(sessions []*session.Session) []sessionGroup {
	type bucket struct {
		sessions []*session.Session
	}
	groups := make(map[string]*bucket)
	var order []string

	for _, s := range sessions {
		if s.GroupID != "" {
			if g, ok := groups[s.GroupID]; ok {
				g.sessions = append(g.sessions, s)
			} else {
				groups[s.GroupID] = &bucket{sessions: []*session.Session{s}}
				order = append(order, s.GroupID)
			}
		} else {
			key := "solo:" + s.ID
			groups[key] = &bucket{sessions: []*session.Session{s}}
			order = append(order, key)
		}
	}

	result := make([]sessionGroup, 0, len(order))
	for _, key := range order {
		b := groups[key]
		session.SortGroupMembers(b.sessions)
		primary := b.sessions[0]
		g := sessionGroup{
			ID:            primary.ID,
			IsGroup:       len(b.sessions) > 1 || primary.GroupID != "",
			Sessions:      publicSessions(b.sessions),
			Status:        groupStatus(b.sessions),
			Effort:        primary.Effort,
			WorkDir:       primary.WorkDir,
			PromptPreview: primary.PromptPreview,
		}
		if primary.GroupID != "" {
			g.ID = primary.GroupID
		}

		if g.IsGroup {
			// Derive the group kind + models from the sessions so a plan group
			// (Sol + Fable) is not mislabelled a megareview.
			g.Kind = groupKind(b.sessions)
			g.CLI = groupCLIs(b.sessions)
			g.Models = groupModels(b.sessions)
		} else {
			g.CLI = config.EngineLabel(primary.CLI, primary.Model)
			g.Models = config.EngineLabel(primary.CLI, primary.Model)
		}

		g.Elapsed = groupElapsed(b.sessions)
		result = append(result, g)
	}
	return result
}

func groupStatus(sessions []*session.Session) string {
	// Tier: running > queued > failed > completed.
	for _, s := range sessions {
		if s.Status == "running" {
			return "running"
		}
	}
	for _, s := range sessions {
		if s.Status == "queued" {
			return "queued"
		}
	}
	for _, s := range sessions {
		if s.Status == "failed" {
			return "failed"
		}
	}
	return "completed"
}

// groupKind returns the group kind: "plan" if any session is a plan review,
// otherwise "megareview". Plan groups run Sol + Fable.
func groupKind(sessions []*session.Session) string {
	for _, s := range sessions {
		if s.Mode == "plan" {
			return "plan"
		}
	}
	return "megareview"
}

// groupEngineLabel names one session's model for group display.
func groupEngineLabel(s *session.Session) string {
	return config.EngineLabel(s.CLI, s.Model)
}

// groupCLIs returns the group's distinct public model names joined with "+".
func groupCLIs(sessions []*session.Session) string {
	seen := map[string]bool{}
	var clis []string
	for _, s := range sessions {
		label := groupEngineLabel(s)
		if label != "" && !seen[label] {
			seen[label] = true
			clis = append(clis, label)
		}
	}
	return strings.Join(clis, "+")
}

func groupModels(sessions []*session.Session) string {
	seen := map[string]bool{}
	var models []string
	for _, s := range sessions {
		label := config.EngineLabel(s.CLI, s.Model)
		if label != "" && !seen[label] {
			seen[label] = true
			models = append(models, label)
		}
	}
	return strings.Join(models, " + ")
}

// publicSessions returns shallow copies with public model names for the web
// API. Session files retain the exact runtime ids needed for execution and
// backwards compatibility.
func publicSessions(sessions []*session.Session) []publicSession {
	result := make([]publicSession, 0, len(sessions))
	for _, s := range sessions {
		label := config.EngineLabel(s.CLI, s.Model)
		result = append(result, publicSession{
			ID:            s.ID,
			CLI:           label,
			Mode:          s.Mode,
			Model:         label,
			Effort:        s.Effort,
			ReviewScope:   s.ReviewScope,
			PromptPreview: s.PromptPreview,
			Status:        s.Status,
			StartTime:     s.StartTime,
			QueuedAt:      s.QueuedAt,
			QueuePosition: s.QueuePosition,
			EndTime:       s.EndTime,
			ExitCode:      s.ExitCode,
			Duration:      s.Duration,
			WorkDir:       s.WorkDir,
			OutputBytes:   s.OutputBytes,
			OutputLines:   s.OutputLines,
			ErrorMsg:      config.PublicRuntimeError(s.CLI, s.Model, s.ErrorMsg),
			Account:       s.Account,
		})
	}
	return result
}

func publicLogData(id string, data []byte, sessions []*session.Session) ([]byte, bool) {
	for _, s := range sessions {
		if s.ID == id {
			return []byte(config.PublicRuntimeLog(s.CLI, s.Model, string(data))), true
		}
	}
	return nil, false
}

func groupElapsed(sessions []*session.Session) string {
	now := time.Now()
	var earliest, latest time.Time
	for _, s := range sessions {
		start := s.StartTime
		if s.QueuedAt != nil && (start.IsZero() || s.QueuedAt.Before(start)) {
			start = *s.QueuedAt
		}
		if start.IsZero() {
			continue
		}

		end := start
		switch {
		case s.Status == "running" || s.Status == "queued":
			end = now
		case s.EndTime != nil:
			end = *s.EndTime
		case s.Duration != "":
			if duration, err := time.ParseDuration(s.Duration); err == nil {
				end = start.Add(duration)
			}
		}
		if end.Before(start) {
			end = start
		}
		if earliest.IsZero() || start.Before(earliest) {
			earliest = start
		}
		if latest.IsZero() || end.After(latest) {
			latest = end
		}
	}
	if !earliest.IsZero() && latest.After(earliest) {
		return latest.Sub(earliest).Round(time.Second).String()
	}
	return "-"
}
