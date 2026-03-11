package dashboard

import (
	"fmt"
	"os"
	"strings"

	"github.com/1F47E/rival/internal/session"
	"github.com/charmbracelet/lipgloss"
)

func renderDetailView(s *session.Session, width, height, scrollTop int) string {
	if s == nil {
		return labelStyle.Render("Select a session to view details")
	}

	var meta strings.Builder

	id := s.ID
	if len(id) > 8 {
		id = id[:8]
	}
	meta.WriteString(titleStyle.Render(fmt.Sprintf("Session %s", id)))
	meta.WriteString("\n\n")

	// Metadata fields.
	addField(&meta, "CLI", s.CLI, width)
	addField(&meta, "Model", s.Model, width)
	addField(&meta, "Effort", s.Effort, width)
	addField(&meta, "Mode", s.Mode, width)
	addStyledField(&meta, "Status", s.Status, statusStyle(s.Status), width)
	addField(&meta, "WorkDir", s.WorkDir, width)
	addField(&meta, "Started", s.StartTime.Format("15:04:05"), width)
	if s.Duration != "" {
		addField(&meta, "Duration", s.Duration, width)
	}
	if s.ExitCode != nil {
		addField(&meta, "Exit", fmt.Sprintf("%d", *s.ExitCode), width)
	}
	if s.OutputBytes > 0 {
		addField(&meta, "Output", fmt.Sprintf("%d bytes, %d lines", s.OutputBytes, s.OutputLines), width)
	}
	if s.ReviewScope != "" {
		addField(&meta, "Review", s.ReviewScope, width)
	}
	if s.PromptPreview != "" {
		addField(&meta, "Prompt", s.PromptPreview, width)
	}
	if s.ErrorMsg != "" {
		addStyledField(&meta, "Error", s.ErrorMsg, failedStyle, width)
	}

	meta.WriteString("\n")
	meta.WriteString(titleStyle.Render("Log"))
	meta.WriteString("\n")

	metaStr := meta.String()
	metaLines := strings.Count(metaStr, "\n") + 1

	// Remaining height for log content.
	logHeight := height - metaLines
	if logHeight < 3 {
		logHeight = 3
	}

	lines := wrapLogLines(s.LogFile, width)

	// Clamp scrollTop.
	maxScroll := len(lines) - logHeight
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scrollTop > maxScroll {
		scrollTop = maxScroll
	}
	if scrollTop < 0 {
		scrollTop = 0
	}

	// Extract visible window.
	end := scrollTop + logHeight
	if end > len(lines) {
		end = len(lines)
	}
	var logContent string
	if len(lines) == 0 {
		logContent = labelStyle.Render("(empty log)")
	} else {
		logContent = strings.Join(lines[scrollTop:end], "\n")
	}

	// Scroll indicator.
	if len(lines) > logHeight {
		pct := 0
		if maxScroll > 0 {
			pct = scrollTop * 100 / maxScroll
		}
		indicator := labelStyle.Render(fmt.Sprintf(" [%d%%]", pct))
		// Replace the "Log" title line with "Log [xx%]".
		metaStr = strings.Replace(metaStr,
			titleStyle.Render("Log"),
			titleStyle.Render("Log")+indicator,
			1)
	}

	// Combine and cap to height.
	full := metaStr + logContent
	result := strings.Split(full, "\n")
	if len(result) > height {
		result = result[:height]
	}

	return strings.Join(result, "\n")
}

func addField(b *strings.Builder, label, value string, width int) {
	addStyledField(b, label, value, valueStyle, width)
}

func addStyledField(b *strings.Builder, label, value string, style lipgloss.Style, width int) {
	// Truncate raw value before styling to avoid counting ANSI escapes as width.
	maxValWidth := width - 13 // 10 label + 2 padding + 1 space
	if maxValWidth < 5 {
		maxValWidth = 5
	}
	rawVal := truncate(value, maxValWidth)
	l := labelStyle.Render(fmt.Sprintf("%-10s", label))
	v := style.Render(rawVal)
	fmt.Fprintf(b, "%s %s\n", l, v)
}

// wrapLogLines reads a log file and wraps long lines to wrapWidth.
func wrapLogLines(path string, wrapWidth int) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}

	rawLines := strings.Split(string(data), "\n")

	var lines []string
	for _, rawLine := range rawLines {
		runes := []rune(rawLine)
		if wrapWidth > 0 && len(runes) > wrapWidth {
			for len(runes) > wrapWidth {
				lines = append(lines, string(runes[:wrapWidth]))
				runes = runes[wrapWidth:]
			}
			lines = append(lines, string(runes))
		} else {
			lines = append(lines, rawLine)
		}
	}

	return lines
}
