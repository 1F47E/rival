package dashboard

import (
	"fmt"
	"os"
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/session"
	"github.com/charmbracelet/lipgloss"
)

func renderDetailView(item *displayItem, width, height int, promptExpanded bool) string {
	if item == nil || item.Primary() == nil {
		return labelStyle.Render("Select a session to view details")
	}

	if item.IsGroup() {
		return renderGroupDetailView(item, width, height, promptExpanded)
	}
	return renderSingleDetailView(item.Primary(), width, height, promptExpanded)
}

func renderSingleDetailView(s *session.Session, width, height int, promptExpanded bool) string {
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
	if s.ErrorMsg != "" {
		addStyledField(&meta, "Error", s.ErrorMsg, failedStyle, width)
	}

	renderPromptSection(&meta, s, width, promptExpanded)
	meta.WriteString("\n")

	metaStr := meta.String()
	metaLines := strings.Count(metaStr, "\n") + 1

	logHeight := height - metaLines - 1
	if logHeight < 3 {
		logHeight = 3
	}

	lines := wrapLogLines(s.LogFile, width)
	logTitle := "Log"
	if len(lines) > logHeight {
		logTitle = "Log (recent)"
	}
	logTitleStr := titleStyle.Render(logTitle) + "\n"

	var logContent string
	if len(lines) == 0 {
		logContent = labelStyle.Render("(empty log)")
	} else if len(lines) <= logHeight {
		logContent = strings.Join(lines, "\n")
	} else {
		logContent = strings.Join(lines[len(lines)-logHeight:], "\n")
	}

	full := metaStr + logTitleStr + logContent
	result := strings.Split(full, "\n")
	if len(result) > height {
		result = result[:height]
	}
	return strings.Join(result, "\n")
}

func renderGroupDetailView(item *displayItem, width, height int, promptExpanded bool) string {
	s := item.Primary()

	var meta strings.Builder

	id := s.ID
	if len(id) > 8 {
		id = id[:8]
	}
	meta.WriteString(titleStyle.Render(fmt.Sprintf("Megareview %s", id)))
	meta.WriteString("\n\n")

	// Shared metadata from primary session.
	addField(&meta, "CLI", "codex+gemini", width)
	addField(&meta, "Effort", s.Effort, width)
	addField(&meta, "Mode", "megareview", width)
	addStyledField(&meta, "Status", groupStatus(item), statusStyle(groupStatus(item)), width)
	addField(&meta, "WorkDir", s.WorkDir, width)
	addField(&meta, "Started", s.StartTime.Format("15:04:05"), width)
	elapsed := groupElapsed(item)
	if elapsed != "-" {
		addField(&meta, "Duration", elapsed, width)
	}
	if s.ReviewScope != "" {
		addField(&meta, "Review", s.ReviewScope, width)
	}

	renderPromptSection(&meta, s, width, promptExpanded)
	meta.WriteString("\n")

	metaStr := meta.String()
	metaLines := strings.Count(metaStr, "\n") + 1

	// Split remaining height between the two logs.
	remaining := height - metaLines
	if remaining < 6 {
		remaining = 6
	}

	var logSections strings.Builder

	for _, sess := range item.Sessions {
		label := strings.ToUpper(sess.CLI) + " REVIEW"
		if sess.Status == "failed" && sess.ErrorMsg != "" {
			label += " (FAILED)"
		}
		logSections.WriteString(titleStyle.Render(fmt.Sprintf("=== %s ===", label)))
		logSections.WriteString("\n")

		if sess.Status == "failed" && sess.ErrorMsg != "" {
			logSections.WriteString(failedStyle.Render(sess.ErrorMsg))
			logSections.WriteString("\n")
		}

		perLogHeight := remaining/len(item.Sessions) - 2 // title + gap
		if perLogHeight < 3 {
			perLogHeight = 3
		}

		lines := wrapLogLines(sess.LogFile, width)
		if len(lines) == 0 {
			logSections.WriteString(labelStyle.Render("(empty log)"))
		} else if len(lines) <= perLogHeight {
			logSections.WriteString(strings.Join(lines, "\n"))
		} else {
			logSections.WriteString(strings.Join(lines[len(lines)-perLogHeight:], "\n"))
		}
		logSections.WriteString("\n\n")
	}

	full := metaStr + logSections.String()
	result := strings.Split(full, "\n")
	if len(result) > height {
		result = result[:height]
	}
	return strings.Join(result, "\n")
}

func renderPromptSection(b *strings.Builder, s *session.Session, width int, promptExpanded bool) {
	prompt := s.Prompt
	if prompt == "" {
		prompt = s.PromptPreview
	}
	if prompt == "" {
		return
	}
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Prompt"))
	b.WriteString("\n")
	promptLines := wrapText(prompt, width)
	if !promptExpanded && len(promptLines) > config.PromptDetailMaxLines {
		for _, line := range promptLines[:config.PromptDetailMaxLines] {
			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString(labelStyle.Render("... (p to expand)"))
		b.WriteString("\n")
	} else {
		for _, line := range promptLines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func addField(b *strings.Builder, label, value string, width int) {
	addStyledField(b, label, value, valueStyle, width)
}

func addStyledField(b *strings.Builder, label, value string, style lipgloss.Style, width int) {
	maxValWidth := width - 13
	if maxValWidth < 5 {
		maxValWidth = 5
	}
	rawVal := truncate(value, maxValWidth)
	l := labelStyle.Render(fmt.Sprintf("%-10s", label))
	v := style.Render(rawVal)
	fmt.Fprintf(b, "%s %s\n", l, v)
}

// wrapText word-wraps a string to the given width.
func wrapText(text string, wrapWidth int) []string {
	if wrapWidth <= 0 {
		return []string{text}
	}
	var result []string
	for _, paragraph := range strings.Split(text, "\n") {
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		words := strings.Fields(paragraph)
		if len(words) == 0 {
			result = append(result, "")
			continue
		}
		line := words[0]
		for _, w := range words[1:] {
			if len(line)+1+len(w) > wrapWidth {
				result = append(result, line)
				line = w
			} else {
				line += " " + w
			}
		}
		result = append(result, line)
	}
	return result
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

	rawLines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")

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
