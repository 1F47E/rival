package dashboard

import (
	"fmt"
	"os"
	"strings"

	"github.com/1F47E/rival/internal/session"
)

func renderDetailView(s *session.Session, width, height, scrollOffset int) string {
	if s == nil {
		return labelStyle.Render("Select a session to view details")
	}

	var b strings.Builder

	id := s.ID
	if len(id) > 8 {
		id = id[:8]
	}
	b.WriteString(titleStyle.Render(fmt.Sprintf("Session %s", id)))
	b.WriteString("\n\n")

	// Metadata.
	addField(&b, "CLI", s.CLI, width)
	addField(&b, "Model", s.Model, width)
	addField(&b, "Effort", s.Effort, width)
	addField(&b, "Mode", s.Mode, width)
	addField(&b, "Status", statusStyle(s.Status).Render(s.Status), width)
	addField(&b, "WorkDir", s.WorkDir, width)
	addField(&b, "Started", s.StartTime.Format("15:04:05"), width)
	if s.Duration != "" {
		addField(&b, "Duration", s.Duration, width)
	}
	if s.ExitCode != nil {
		addField(&b, "Exit", fmt.Sprintf("%d", *s.ExitCode), width)
	}
	if s.OutputBytes > 0 {
		addField(&b, "Output", fmt.Sprintf("%d bytes, %d lines", s.OutputBytes, s.OutputLines), width)
	}
	if s.ReviewScope != "" {
		addField(&b, "Review", s.ReviewScope, width)
	}
	if s.PromptPreview != "" {
		addField(&b, "Prompt", s.PromptPreview, width)
	}
	if s.ErrorMsg != "" {
		addField(&b, "Error", failedStyle.Render(s.ErrorMsg), width)
	}

	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Log"))
	b.WriteString("\n")

	// Tail the log file.
	logLines := readLogTail(s.LogFile, height-20, scrollOffset)
	b.WriteString(logLines)

	return b.String()
}

func addField(b *strings.Builder, label, value string, width int) {
	l := labelStyle.Render(fmt.Sprintf("%-10s", label))
	v := valueStyle.Render(value)
	line := fmt.Sprintf("%s %s", l, v)
	b.WriteString(truncate(line, width-2))
	b.WriteString("\n")
}

func readLogTail(path string, maxLines, scrollOffset int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return labelStyle.Render("(no log file)")
	}
	if len(data) == 0 {
		return labelStyle.Render("(empty log)")
	}

	lines := strings.Split(string(data), "\n")

	// Apply scroll offset from the bottom.
	end := len(lines) - scrollOffset
	if end < 0 {
		end = 0
	}
	start := end - maxLines
	if start < 0 {
		start = 0
	}

	if start >= end {
		return labelStyle.Render("(scrolled past top)")
	}

	visible := lines[start:end]
	return strings.Join(visible, "\n")
}
