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
	addField(&meta, "Reviewer", config.EngineLabel(s.CLI, s.Model), width)
	addField(&meta, "Model", config.EngineLabel(s.CLI, s.Model), width)
	addField(&meta, "Effort", s.Effort, width)
	addField(&meta, "Mode", s.Mode, width)
	if s.Account != "" {
		addField(&meta, "Account", s.Account, width)
	}
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

	renderErrorSection(&meta, s, width)
	renderPromptSection(&meta, s, width, promptExpanded)
	meta.WriteString("\n")

	metaStr := meta.String()
	metaLines := strings.Count(metaStr, "\n") + 1

	logHeight := height - metaLines - 1
	if logHeight < 3 {
		logHeight = 3
	}

	lines := wrapLogLines(s, width)
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

	var essential strings.Builder

	id := s.GroupID
	if id == "" {
		id = s.ID
	}
	if len(id) > 8 {
		id = id[:8]
	}
	title := "Megareview"
	if groupIsPlan(item) {
		title = "Plan Review"
	}
	essential.WriteString(titleStyle.Render(fmt.Sprintf("%s %s", title, id)))
	essential.WriteString("\n\n")

	// Shared metadata from primary session — derived from the group's sessions so
	// a Sol + Fable plan group is not mislabelled a megareview.
	addField(&essential, "Models", groupCLIs(item), width)
	addField(&essential, "Effort", s.Effort, width)
	addField(&essential, "Mode", groupKindLabel(item), width)
	addStyledField(&essential, "Status", groupStatus(item), statusStyle(groupStatus(item)), width)
	addField(&essential, "WorkDir", s.WorkDir, width)
	addField(&essential, "Started", s.StartTime.Format("15:04:05"), width)
	elapsed := groupElapsed(item)
	if elapsed != "-" {
		addField(&essential, "Duration", elapsed, width)
	}
	if s.ReviewScope != "" {
		addField(&essential, "Review", s.ReviewScope, width)
	}

	// Logs are the primary content of a grouped detail view. Reserve a heading
	// and at least one content line for every member before spending space on
	// the shared prompt, so a normal terminal always exposes every model.
	minLogLines := len(item.Sessions) * 2
	maxMetaLines := height - minLogLines
	if maxMetaLines < 0 {
		maxMetaLines = 0
	}
	metaLines := renderedLines(essential.String())
	if len(metaLines) > maxMetaLines {
		metaLines = metaLines[:maxMetaLines]
	}

	if len(metaLines) < maxMetaLines {
		var prompt strings.Builder
		renderPromptSection(&prompt, s, width, promptExpanded)
		optional := renderedLines(prompt.String())
		room := maxMetaLines - len(metaLines)
		if len(optional) > room {
			optional = optional[:room]
		}
		metaLines = append(metaLines, optional...)
	}

	remaining := height - len(metaLines)
	for i, sess := range item.Sessions {
		membersLeft := len(item.Sessions) - i
		budget := remaining / membersLeft
		section := groupLogLines(sess, width, budget)
		metaLines = append(metaLines, section...)
		remaining -= len(section)
	}
	if len(metaLines) > height {
		metaLines = metaLines[:height]
	}
	return strings.Join(metaLines, "\n")
}

func renderedLines(text string) []string {
	text = strings.TrimRight(text, "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func groupLogLines(sess *session.Session, width, budget int) []string {
	if budget <= 0 {
		return nil
	}
	label := groupLogLabel(sess)
	if sess.Status == "failed" && sess.ErrorMsg != "" {
		label += " (FAILED)"
	}
	lines := []string{titleStyle.Render(fmt.Sprintf("=== %s ===", label))}
	if budget == 1 {
		return lines
	}

	if sess.Status == "failed" && sess.ErrorMsg != "" {
		message := config.PublicRuntimeError(sess.CLI, sess.Model, sess.ErrorMsg)
		for _, line := range wrapText(message, width) {
			lines = append(lines, failedStyle.Render(line))
			if len(lines) == budget {
				return lines
			}
		}
	}

	logLines := wrapLogLines(sess, width)
	if len(logLines) == 0 {
		return append(lines, labelStyle.Render("(empty log)"))
	}
	room := budget - len(lines)
	if len(logLines) > room {
		logLines = logLines[len(logLines)-room:]
	}
	return append(lines, logLines...)
}

func groupLogLabel(sess *session.Session) string {
	role := "REVIEW"
	if sess.Mode == "consilium" {
		role = "JUDGE"
	}
	return strings.ToUpper(groupEngineLabel(sess)) + " " + role
}

// renderErrorSection renders the full error message wrapped across as many
// lines as needed, in the failed (red) style. Unlike a single-line field it is
// never truncated, so long model-runtime errors plus any
// trailing detail stay fully readable.
func renderErrorSection(b *strings.Builder, s *session.Session, width int) {
	if s.ErrorMsg == "" {
		return
	}
	b.WriteString("\n")
	b.WriteString(titleStyle.Render("Error"))
	b.WriteString("\n")
	message := config.PublicRuntimeError(s.CLI, s.Model, s.ErrorMsg)
	for _, line := range wrapText(message, width) {
		b.WriteString(failedStyle.Render(line))
		b.WriteString("\n")
	}
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

// wrapLogLines reads one session log, applies public model naming, and wraps
// long lines to wrapWidth.
func wrapLogLines(s *session.Session, wrapWidth int) []string {
	data, err := os.ReadFile(s.LogFile)
	if err != nil {
		return nil
	}
	if len(data) == 0 {
		return nil
	}

	publicLog := config.PublicRuntimeLog(s.CLI, s.Model, string(data))
	rawLines := strings.Split(strings.TrimRight(publicLog, "\n"), "\n")

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
