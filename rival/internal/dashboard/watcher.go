package dashboard

import (
	"context"
	"os"
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// SessionEvent is sent when sessions change.
type SessionEvent struct {
	Sessions []*session.Session
}

// WatchSessions watches the session directory and sends events on changes.
// The goroutine exits when ctx is cancelled.
func WatchSessions(ctx context.Context, events chan<- SessionEvent) error {
	dir := config.SessionDirPath()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return err
	}

	// Send initial state.
	sessions := session.LoadAll()
	events <- SessionEvent{Sessions: sessions}

	go func() {
		defer func() { _ = watcher.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) {
					isJSON := strings.HasSuffix(event.Name, ".json") && !strings.HasSuffix(event.Name, ".json.tmp")
					isLog := strings.HasSuffix(event.Name, ".log")
					if isJSON || isLog {
						sessions := session.LoadAll()
						events <- SessionEvent{Sessions: sessions}
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Warn().Err(err).Msg("watcher error")
			}
		}
	}()

	return nil
}
