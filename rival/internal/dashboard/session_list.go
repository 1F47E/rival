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
	b.WriteString(titleStyle.Render("Sessions"))
	b.WriteString("\n\n")

	maxItems := height - 4 // title + padding
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
		line := formatSessionLine(s)
		if i == selected {
			b.WriteString(selectedItemStyle.Render(truncate(line, width-4)))
		} else {
			b.WriteString(normalItemStyle.Render(truncate(line, width-4)))
		}
		b.WriteString("\n")
	}

	return b.String()
}

func formatSessionLine(s *session.Session) string {
	status := statusStyle(s.Status).Render(s.Status)
	elapsed := formatElapsed(s)
	return fmt.Sprintf(" %s  %-7s  %-6s  %s  %s", status, s.CLI, s.Effort, elapsed, s.Model)
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
	// Count runes for proper unicode handling.
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
