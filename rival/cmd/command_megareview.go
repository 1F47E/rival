package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/1F47E/rival/internal/config"
	"github.com/1F47E/rival/internal/executor"
	"github.com/1F47E/rival/internal/parser"
	"github.com/1F47E/rival/internal/session"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

const megareviewUsage = `Usage:
  /rival-megareview — review the entire project with both Codex and Gemini in parallel
  /rival-megareview src/api/ — review specific scope
  /rival-megareview -re xhigh src/api/ — review with xhigh reasoning effort
  /rival-megareview — show this usage info

Reasoning effort (-re): low, medium (default), high, xhigh`

var commandMegareviewCmd = &cobra.Command{
	Use:   "megareview",
	Short: "Run Codex and Gemini reviews in parallel",
	RunE:  commandMegareviewAction,
}

func init() {
	commandMegareviewCmd.Flags().String("workdir", ".", "working directory")
	commandCmd.AddCommand(commandMegareviewCmd)
}

type reviewResult struct {
	cli    string
	log    []byte
	err    error
	exCode int
}

func commandMegareviewAction(cmd *cobra.Command, args []string) error {
	workdir, _ := cmd.Flags().GetString("workdir")

	raw, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}

	parsed, err := parser.ParseReviewArgs(string(raw))
	if err != nil {
		_, _ = fmt.Fprintln(os.Stdout, err.Error())
		return &ExitCodeError{Code: 1, Err: err}
	}

	if parsed.IsEmpty {
		_, _ = fmt.Fprintln(os.Stdout, megareviewUsage)
		return nil
	}

	// Preflight both CLIs — warn but continue if one is missing.
	codexOK := true
	geminiOK := true
	if err := executor.CodexPreflight(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: codex unavailable: %v\n", err)
		codexOK = false
	}
	if err := executor.GeminiPreflight(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "warning: gemini unavailable: %v\n", err)
		geminiOK = false
	}
	if !codexOK && !geminiOK {
		return fmt.Errorf("both codex and gemini are unavailable")
	}

	groupID := uuid.New().String()

	var wg sync.WaitGroup
	results := make(chan reviewResult, 2)

	if codexOK {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runOneCLI("codex", groupID, parsed, workdir)
		}()
	}

	if geminiOK {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- runOneCLI("gemini", groupID, parsed, workdir)
		}()
	}

	wg.Wait()
	close(results)

	var codexRes, geminiRes *reviewResult
	for r := range results {
		r := r
		switch r.cli {
		case "codex":
			codexRes = &r
		case "gemini":
			geminiRes = &r
		}
	}

	anySuccess := false
	if codexRes != nil {
		_, _ = fmt.Fprintln(os.Stdout, "=== CODEX REVIEW ===")
		if codexRes.err != nil {
			_, _ = fmt.Fprintf(os.Stdout, "ERROR: %v\n", codexRes.err)
		} else {
			_, _ = os.Stdout.Write(codexRes.log)
			if codexRes.exCode == 0 {
				anySuccess = true
			}
		}
	}

	if geminiRes != nil {
		_, _ = fmt.Fprintln(os.Stdout, "\n=== GEMINI REVIEW ===")
		if geminiRes.err != nil {
			_, _ = fmt.Fprintf(os.Stdout, "ERROR: %v\n", geminiRes.err)
		} else {
			_, _ = os.Stdout.Write(geminiRes.log)
			if geminiRes.exCode == 0 {
				anySuccess = true
			}
		}
	}

	// Exit 1 only if no review succeeded.
	if !anySuccess {
		return &ExitCodeError{Code: 1, Err: fmt.Errorf("all reviews failed")}
	}

	return nil
}

func runOneCLI(cli, groupID string, parsed *parser.ParseResult, workdir string) reviewResult {
	model := config.CodexModel
	if cli == "gemini" {
		model = config.GeminiModel
	}

	sess, err := session.New(cli, "megareview", model, parsed.Effort, workdir, parsed.Prompt, parsed.ReviewScope, groupID)
	if err != nil {
		return reviewResult{cli: cli, err: fmt.Errorf("create session: %w", err)}
	}

	log.Info().Str("session", sess.ID).Str("cli", cli).Str("group", groupID).Msg("starting megareview")

	ctx := context.Background()
	var result *executor.Result
	switch cli {
	case "codex":
		result, err = executor.RunCodex(ctx, sess, parsed.Prompt, parsed.Effort, workdir, nil)
	case "gemini":
		result, err = executor.RunGemini(ctx, sess, parsed.Prompt, parsed.Effort, workdir, nil)
	}

	if err != nil {
		_ = sess.Fail(1, err.Error())
		return reviewResult{cli: cli, err: err}
	}

	if result.ExitCode != 0 {
		_ = sess.Fail(result.ExitCode, fmt.Sprintf("%s exited with code %d", cli, result.ExitCode))
	} else {
		_ = sess.Complete(result.ExitCode, result.OutputBytes, result.OutputLines)
	}

	logData, err := os.ReadFile(sess.LogFile)
	if err != nil {
		return reviewResult{cli: cli, err: fmt.Errorf("read log: %w", err), exCode: result.ExitCode}
	}

	return reviewResult{cli: cli, log: logData, exCode: result.ExitCode}
}
