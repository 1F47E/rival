package cmd

import (
	"strings"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/gitscope"
	"github.com/1F47E/rival/internal/parser"
	"github.com/rs/zerolog/log"
)

// resolveGitScope auto-detects changed files via git and updates the parsed result.
// If git finds files, it rebuilds the prompt with DiffReviewPreamble + ReviewPrompt.
// If git finds nothing, it falls back to "the entire project".
func resolveGitScope(parsed *parser.ParseResult, workdir string) {
	files := gitscope.Resolve(workdir)
	if files == "" {
		log.Debug().Msg("git scope: no changes detected, falling back to full project")
		return // keep "the entire project" default
	}

	log.Info().Str("files", files).Msg("git scope: auto-detected changed files")
	parsed.AutoScope = false
	parsed.ReviewScope = files
	// Preamble lists the files in a code block; ReviewPrompt uses them as scope.
	preamble := strings.ReplaceAll(config.DiffReviewPreamble, "{FILES}", files)
	review := strings.ReplaceAll(config.ReviewPrompt, "{SCOPE}", "the changed files listed above")
	parsed.Prompt = preamble + review
}
