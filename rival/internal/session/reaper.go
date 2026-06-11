package session

import (
	"github.com/1F47E/rival/internal/procinfo"
	"github.com/rs/zerolog/log"
)

// ReapOrphans finds sessions stuck in "running" or "queued" whose process is
// dead, and marks them failed.
func ReapOrphans() {
	sessions := LoadAll()
	for _, s := range sessions {
		if s.Status != "running" && s.Status != "queued" {
			continue
		}
		if !procinfo.Alive(s.PID, s.PIDStart) {
			msg := "orphaned (process dead)"
			if s.Status == "queued" {
				msg = "orphaned while queued (process dead)"
			}
			log.Info().Str("session", s.ID).Int("pid", s.PID).Str("status", s.Status).Msg("reaping orphaned session")
			_ = s.Fail(1, msg)
		}
	}
}
