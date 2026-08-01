package cmd

import (
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/gitscope"
	"github.com/1F47E/rival/internal/parser"
	"github.com/rs/zerolog/log"
)

// buildDiffPreamble builds the DiffReviewPreamble block (changed-file list +
// diff stats) for workdir. files=="" means git detected no changes; the raw
// files list is returned alongside so callers can record it as the review
// scope without a second gitscope.Resolve fork.
func buildDiffPreamble(workdir string) (preamble, files string) {
	files = gitscope.Resolve(workdir)
	if files == "" {
		return "", ""
	}
	preamble = strings.ReplaceAll(config.DiffReviewPreamble, "{FILES}", files)
	if diffStat := gitscope.DiffStat(workdir); diffStat != "" {
		preamble = strings.ReplaceAll(preamble, "{DIFFSTAT}", "\nDiff stats:\n```\n"+diffStat+"\n```\n")
	} else {
		preamble = strings.ReplaceAll(preamble, "{DIFFSTAT}", "")
	}
	return preamble, files
}

// resolveGitScope auto-detects changed files via git and updates the parsed result.
// If git finds files, it rebuilds the prompt with DiffReviewPreamble + ReviewPrompt.
// If git finds nothing, it falls back to "the entire project".
func resolveGitScope(parsed *parser.ParseResult, workdir string) {
	preamble, files := buildDiffPreamble(workdir)
	if files == "" {
		log.Debug().Msg("git scope: no changes detected, falling back to full project")
		return // keep "the entire project" default
	}

	log.Info().Str("files", files).Msg("git scope: auto-detected changed files")
	parsed.AutoScope = false
	parsed.ReviewScope = files
	review := strings.ReplaceAll(config.ReviewPrompt, "{SCOPE}", "the changed files listed above")
	parsed.Prompt = preamble + review
}
