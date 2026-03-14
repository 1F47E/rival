package session

import (
	"syscall"

	"github.com/rs/zerolog/log"
)

// ReapOrphans finds sessions stuck in "running" whose process is dead, and marks them failed.
func ReapOrphans() {
	sessions := LoadAll()
	for _, s := range sessions {
		if s.Status != "running" {
			continue
		}
		if s.PID <= 0 || !processAlive(s.PID) {
			log.Info().Str("session", s.ID).Int("pid", s.PID).Msg("reaping orphaned session")
			_ = s.Fail(1, "orphaned (process dead)")
		}
	}
}

func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
