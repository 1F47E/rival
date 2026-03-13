package skills

import "embed"

//go:embed all:rival-codex
//go:embed all:rival-gemini
//go:embed all:rival-megareview
var Files embed.FS

// Names lists all embedded skill directory names.
var Names = []string{"rival-codex", "rival-gemini", "rival-megareview"}
