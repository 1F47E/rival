package queue

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestCrossProcessFIFO drives three real OS processes competing for a single
// queue slot and asserts they acquire it in enqueue order. Each child runs the
// helper-process branch below (the standard Go subprocess-test idiom), so this
// exercises actual flock-based cross-process coordination, not goroutines.
func TestCrossProcessFIFO(t *testing.T) {
	if os.Getenv("RIVAL_QUEUE_HELPER") != "" {
		runQueueHelper()
		return
	}

	dir := t.TempDir()
	labels := []string{"A", "B", "C"}
	procs := make([]*exec.Cmd, len(labels))
	outs := make([]*strings.Builder, len(labels))

	for i, label := range labels {
		cmd := exec.Command(os.Args[0], "-test.run=TestCrossProcessFIFO")
		cmd.Env = append(os.Environ(),
			"RIVAL_QUEUE_HELPER=1",
			"RIVAL_QUEUE_DIR="+dir,
			"RIVAL_QUEUE_LABEL="+label,
			"RIVAL_QUEUE_HOLD=400ms",
		)
		var buf strings.Builder
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		outs[i] = &buf
		if err := cmd.Start(); err != nil {
			t.Fatalf("start %s: %v", label, err)
		}
		procs[i] = cmd
		time.Sleep(60 * time.Millisecond) // stagger so enqueue order is deterministic
	}

	for i, cmd := range procs {
		if err := cmd.Wait(); err != nil {
			t.Fatalf("proc %s failed: %v\noutput:\n%s", labels[i], err, outs[i].String())
		}
	}

	// Collect each label's ACQUIRED timestamp and assert A < B < C.
	type acq struct {
		label string
		when  time.Time
	}
	var order []acq
	for i, buf := range outs {
		for _, line := range strings.Split(buf.String(), "\n") {
			if strings.Contains(line, "ACQUIRED") {
				ts := strings.TrimSpace(strings.TrimPrefix(line, labels[i]+" ACQUIRED "))
				when, err := time.Parse("15:04:05.000000", ts)
				if err != nil {
					t.Fatalf("parse acquire time %q: %v", ts, err)
				}
				order = append(order, acq{labels[i], when})
			}
		}
	}
	if len(order) != 3 {
		t.Fatalf("expected 3 ACQUIRED lines, got %d:\n%s%s%s", len(order),
			outs[0].String(), outs[1].String(), outs[2].String())
	}
	// Sort by time, assert label order is A,B,C.
	for i := 0; i < len(order); i++ {
		for j := i + 1; j < len(order); j++ {
			if order[j].when.Before(order[i].when) {
				order[i], order[j] = order[j], order[i]
			}
		}
	}
	got := order[0].label + order[1].label + order[2].label
	if got != "ABC" {
		t.Fatalf("FIFO violated: acquire order = %s, want ABC", got)
	}
}

func runQueueHelper() {
	dir := os.Getenv("RIVAL_QUEUE_DIR")
	label := os.Getenv("RIVAL_QUEUE_LABEL")
	hold, _ := time.ParseDuration(os.Getenv("RIVAL_QUEUE_HOLD"))

	m := &Manager{
		Dir:           dir,
		MaxConcurrent: 1,
		PollInterval:  20 * time.Millisecond,
		Timeout:       30 * time.Second,
		SessionLive:   func(string) bool { return false },
	}
	if _, err := m.Enqueue("", nil, "test", "/tmp"); err != nil {
		fmt.Printf("%s ENQUEUE_ERR %v\n", label, err)
		os.Exit(1)
	}
	if err := m.WaitForSlot(context.Background(), nil); err != nil {
		fmt.Printf("%s WAIT_ERR %v\n", label, err)
		os.Exit(1)
	}
	fmt.Printf("%s ACQUIRED %s\n", label, time.Now().Format("15:04:05.000000"))
	time.Sleep(hold)
	m.Release()
	os.Exit(0)
}
