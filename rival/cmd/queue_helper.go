package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/queue"
	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
)

// waitForQueueSlot enqueues one ticket covering the given queued sessions and
// blocks until a slot is free, then marks the sessions running. Progress is
// printed to stderr with the strict "rival queue:" prefix (stdout is reserved
// for the final output the skills present verbatim). On cancel/timeout the
// sessions are failed with a clear message and an error is returned.
// The returned release func frees the slot; call it via defer.
func waitForQueueSlot(ctx context.Context, noQueue bool, sessions []*session.Session, mode, workdir string) (release func(), err error) {
	markRunning := func() error {
		for _, s := range sessions {
			if err := s.MarkRunning(); err != nil {
				return fmt.Errorf("mark session running: %w", err)
			}
		}
		return nil
	}

	if noQueue || config.QueueDisabled() {
		return func() {}, markRunning()
	}

	groupID := ""
	ids := make([]string, 0, len(sessions))
	for _, s := range sessions {
		ids = append(ids, s.ID)
		if s.GroupID != "" {
			groupID = s.GroupID
		}
	}

	m := queue.New()
	if _, enqErr := m.Enqueue(groupID, ids, mode, workdir); enqErr != nil {
		// A broken queue dir (permissions, disk) must not brick reviews —
		// degrade to unqueued execution with a loud warning.
		log.Warn().Err(enqErr).Msg("queue unavailable — running without queueing")
		return func() {}, markRunning()
	}

	start := time.Now()
	waitErr := m.WaitForSlot(ctx, func(pos, total, running int) {
		_, _ = fmt.Fprintf(os.Stderr, "rival queue: position %d/%d (%d running), waiting %s\n",
			pos, total, running, time.Since(start).Round(time.Second))
		for _, s := range sessions {
			_ = s.SetQueuePosition(pos)
		}
	})
	if waitErr != nil {
		m.Release()
		msg := "cancelled while queued"
		if errors.Is(waitErr, queue.ErrQueueTimeout) {
			msg = fmt.Sprintf("queue timeout after %s — queue may be wedged; inspect with 'rival queue', purge with 'rival queue clear'", m.Timeout)
		}
		for _, s := range sessions {
			_ = s.Fail(1, msg)
		}
		return nil, fmt.Errorf("rival queue: %s", msg)
	}

	if err := markRunning(); err != nil {
		m.Release()
		return nil, err
	}
	if waited := time.Since(start); waited >= time.Second {
		_, _ = fmt.Fprintf(os.Stderr, "rival queue: slot acquired after %s\n", waited.Round(time.Second))
	}
	return m.Release, nil
}
