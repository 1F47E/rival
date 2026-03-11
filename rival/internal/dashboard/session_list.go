package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/1F47E/rival/internal/session"
)

func renderSessionList(sessions []*session.Session, selected int, width, height int) string {
	if len(sessions) == 0 {
		return labelStyle.Render("No sessions yet. Run rival to get started.")
	}

	var b strings.Builder

	// Header row.
	header := formatHeaderRow(width)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	maxItems := height - 2 // header + separator
	if maxItems < 1 {
		maxItems = 1
	}

	// Scroll offset.
	offset := 0
	if selected >= maxItems {
		offset = selected - maxItems + 1
	}

	for i := offset; i < len(sessions) && i-offset < maxItems; i++ {
		s := sessions[i]
		line := formatSessionRow(s, width)
		if i == selected {
			b.WriteString(selectedItemStyle.Render(line))
		} else {
			b.WriteString(normalItemStyle.Render(line))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func formatHeaderRow(width int) string {
	cols := calcColumns(width)
	return fmt.Sprintf(" %-*s %-*s %-*s %-*s %-*s %-*s %s",
		cols.status, "STATUS",
		cols.cli, "CLI",
		cols.model, "MODEL",
		cols.effort, "EFFORT",
		cols.elapsed, "TIME",
		cols.workdir, "WORKDIR",
		"PROMPT",
	)
}

func formatSessionRow(s *session.Session, width int) string {
	cols := calcColumns(width)

	// Status icon + text.
	icon := statusIcon(s.Status)
	statusText := fmt.Sprintf("%s %s", icon, s.Status)

	// Elapsed time.
	elapsed := formatElapsed(s)

	// Truncate workdir and prompt to fit.
	wd := truncatePath(s.WorkDir, cols.workdir)
	prompt := ""
	if cols.prompt > 0 {
		prompt = truncate(s.PromptPreview, cols.prompt)
	}

	// Build raw line without ANSI for proper alignment, then apply status color.
	rawStatus := fmt.Sprintf("%-*s", cols.status, statusText)
	coloredStatus := statusStyle(s.Status).Render(rawStatus)

	return fmt.Sprintf(" %s %-*s %-*s %-*s %-*s %-*s %s",
		coloredStatus,
		cols.cli, s.CLI,
		cols.model, truncate(s.Model, cols.model),
		cols.effort, s.Effort,
		cols.elapsed, elapsed,
		cols.workdir, wd,
		prompt,
	)
}

type columnWidths struct {
	status  int
	cli     int
	model   int
	effort  int
	elapsed int
	workdir int
	prompt  int
}

func calcColumns(width int) columnWidths {
	// Fixed columns.
	c := columnWidths{
		status:  12,
		cli:     8,
		model:   20,
		effort:  8,
		elapsed: 8,
	}

	// 2 for leading space + separators between columns (7 spaces for 8 columns).
	fixed := 2 + c.status + c.cli + c.model + c.effort + c.elapsed + 7
	remaining := width - fixed
	if remaining < 10 {
		remaining = 10
	}

	// Split remaining between workdir and prompt.
	c.workdir = remaining / 2
	c.prompt = remaining - c.workdir

	return c
}

func statusIcon(status string) string {
	switch status {
	case "running":
		return "●"
	case "completed":
		return "●"
	case "failed":
		return "●"
	default:
		return "○"
	}
}

func formatElapsed(s *session.Session) string {
	if s.Duration != "" {
		return s.Duration
	}
	if s.Status == "running" {
		d := time.Since(s.StartTime).Round(time.Second)
		return d.String()
	}
	return "-"
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}

func truncatePath(path string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(path)
	if len(runes) <= max {
		return path
	}
	if max <= 4 {
		return string(runes[:max])
	}
	return string(runes[:max-1]) + "…"
}
