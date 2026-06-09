package server

import (
	"embed"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
)

//go:embed templates/index.html
var indexHTML embed.FS

var uuidRegex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type sessionGroup struct {
	ID           string             `json:"id"`
	IsGroup      bool               `json:"is_group"`
	Sessions     []*session.Session `json:"sessions"`
	Status       string             `json:"status"`
	CLI          string             `json:"cli"`
	Models       string             `json:"models"`
	Effort       string             `json:"effort"`
	Elapsed      string             `json:"elapsed"`
	WorkDir      string             `json:"work_dir"`
	PromptPreview string            `json:"prompt_preview"`
}

type stats struct {
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
	Total     int `json:"total"`
}

type apiResponse struct {
	Stats   stats          `json:"stats"`
	Groups  []sessionGroup `json:"groups"`
	Version string         `json:"version"`
}

func New(version string) *http.ServeMux {
	mux := http.NewServeMux()

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
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})

	mux.HandleFunc("/api/sessions", func(w http.ResponseWriter, r *http.Request) {
		sessions := session.LoadAll()
		groups := groupSessions(sessions)

		var st stats
		st.Total = len(sessions)
		for _, s := range sessions {
			switch s.Status {
			case "running":
				st.Running++
			case "completed":
				st.Completed++
			case "failed":
				st.Failed++
			}
		}

		resp := apiResponse{
			Stats:   st,
			Groups:  groups,
			Version: version,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/api/sessions/", func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/sessions/"), "/")
		if len(parts) != 2 || parts[1] != "log" {
			http.NotFound(w, r)
			return
		}
		id := parts[0]
		if !uuidRegex.MatchString(id) {
			http.Error(w, "invalid session id", http.StatusBadRequest)
			return
		}

		logPath := filepath.Join(config.SessionDirPath(), id+".log")
		data, err := os.ReadFile(logPath)
		if err != nil {
			http.Error(w, "log not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write(data)
	})

	return mux
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
		primary := b.sessions[0]
		g := sessionGroup{
			ID:            primary.ID,
			IsGroup:       len(b.sessions) > 1,
			Sessions:      b.sessions,
			Status:        groupStatus(b.sessions),
			Effort:        primary.Effort,
			WorkDir:       primary.WorkDir,
			PromptPreview: primary.PromptPreview,
		}

		if g.IsGroup {
			g.CLI = "mega"
			g.Models = groupModels(b.sessions)
		} else {
			g.CLI = primary.CLI
			g.Models = primary.Model
		}

		g.Elapsed = groupElapsed(b.sessions)
		result = append(result, g)
	}
	return result
}

func groupStatus(sessions []*session.Session) string {
	for _, s := range sessions {
		if s.Status == "running" {
			return "running"
		}
	}
	for _, s := range sessions {
		if s.Status == "failed" {
			return "failed"
		}
	}
	return "completed"
}

func groupModels(sessions []*session.Session) string {
	seen := map[string]bool{}
	var models []string
	for _, s := range sessions {
		if s.Model != "" && !seen[s.Model] {
			seen[s.Model] = true
			models = append(models, s.Model)
		}
	}
	return strings.Join(models, " + ")
}

func groupElapsed(sessions []*session.Session) string {
	var maxDur time.Duration
	for _, s := range sessions {
		var d time.Duration
		if s.Status == "running" {
			d = time.Since(s.StartTime)
		} else if s.EndTime != nil {
			d = s.EndTime.Sub(s.StartTime)
		}
		if d > maxDur {
			maxDur = d
		}
	}
	if maxDur > 0 {
		return maxDur.Round(time.Second).String()
	}
	return "-"
}
