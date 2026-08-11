package sessionview

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/1F47E/rival/internal/session"
)

type cachedSession struct {
	size    int64
	modTime int64
	session *session.Session
}

// Cache keeps the dashboards responsive without changing the session storage
// format used by the CLI. Only files whose size or mtime changed are reparsed,
// and parsed summaries never retain full prompts. Both front ends share one
// instance per directory, so they cannot read different data.
type Cache struct {
	mu       sync.Mutex
	dir      string
	files    map[string]cachedSession
	revision uint64
}

// New returns a Cache over the session directory dir.
func New(dir string) *Cache {
	return &Cache{
		dir:   dir,
		files: make(map[string]cachedSession),
	}
}

// Load returns every cached session, newest first, plus a revision counter.
// The revision increments only when this call observes a file that was added,
// removed, or changed by size or mtime. Two calls with no file change return
// the same number, so a caller can skip redundant work.
func (c *Cache) Load() ([]*session.Session, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, c.revision
		}
		return cachedSessionValues(c.files), c.revision
	}

	seen := make(map[string]bool, len(entries))
	changed := false
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.tmp") {
			continue
		}
		seen[name] = true

		info, err := entry.Info()
		if err != nil {
			continue
		}
		modTime := info.ModTime().UnixNano()
		if cached, ok := c.files[name]; ok && cached.size == info.Size() && cached.modTime == modTime {
			continue
		}

		s, err := session.LoadSummaryFile(filepath.Join(c.dir, name), info.Size())
		if err != nil {
			continue
		}
		c.files[name] = cachedSession{
			size:    info.Size(),
			modTime: modTime,
			session: s,
		}
		changed = true
	}

	for name := range c.files {
		if !seen[name] {
			delete(c.files, name)
			changed = true
		}
	}
	if changed {
		c.revision++
	}

	return cachedSessionValues(c.files), c.revision
}

// Get returns one cached session by ID, or nil when it is not cached.
func (c *Cache) Get(id string) *session.Session {
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.files[id+".json"]; ok {
		return cached.session
	}
	return nil
}

func cachedSessionValues(files map[string]cachedSession) []*session.Session {
	sessions := make([]*session.Session, 0, len(files))
	for _, cached := range files {
		sessions = append(sessions, cached.session)
	}
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})
	return sessions
}
