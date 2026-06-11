package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	StateWaiting = "waiting"
	StateRunning = "running"
)

// Ticket is one queued review. Tickets live as JSON files in ~/.rival/queue/
// named <unixnano>-<pid>-<id8>.json so a plain filename sort yields FIFO order.
// A ticket is written exactly twice in its life (created as waiting, promoted
// to running) and removed once — there are no post-promotion writes.
type Ticket struct {
	ID         string     `json:"id"`
	GroupID    string     `json:"group_id,omitempty"`
	SessionIDs []string   `json:"session_ids,omitempty"`
	Mode       string     `json:"mode"`
	PID        int        `json:"pid"`
	PIDStart   int64      `json:"pid_start,omitempty"` // owner process start time (Unix ns); guards against PID reuse
	State      string     `json:"state"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	WorkDir    string     `json:"work_dir,omitempty"`

	file string // filename within the queue dir, not serialized
}

// writeTicket persists a ticket atomically (tmp file + rename), matching the
// session.Save pattern so scanners never see a half-written file.
func writeTicket(dir string, t *Ticket) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal ticket: %w", err)
	}
	tmp := filepath.Join(dir, t.file+".tmp")
	final := filepath.Join(dir, t.file)
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return fmt.Errorf("write ticket tmp: %w", err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename ticket: %w", err)
	}
	return nil
}

func ticketFilename(now time.Time, pid int, id string) string {
	short := id
	if len(short) > 8 {
		short = short[:8]
	}
	// Nano is zero-padded so lexicographic filename sort == chronological FIFO.
	return fmt.Sprintf("%019d-%d-%s.json", now.UnixNano(), pid, short)
}
