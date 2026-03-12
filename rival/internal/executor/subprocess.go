package executor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/1F47E/rival/internal/session"
	"github.com/rs/zerolog/log"
)

// Result holds subprocess execution results.
type Result struct {
	ExitCode    int
	OutputBytes int64
	OutputLines int
}

// syncWriter wraps an io.Writer with a mutex for concurrent use.
type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (sw *syncWriter) Write(p []byte) (int, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	return sw.w.Write(p)
}

// RunSubprocess executes a command, pipes prompt to stdin, tees stdout to log + optional mirror.
func RunSubprocess(ctx context.Context, sess *session.Session, binary string, args []string, env []string, prompt string, mirror io.Writer) (*Result, error) {
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), env...)
	cmd.Dir = sess.WorkDir

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("stderr pipe: %w", err)
	}

	logFile, err := sess.OpenLog()
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer func() {
		if closeErr := logFile.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Str("session", sess.ID).Msg("failed to close log file")
		}
	}()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", binary, err)
	}

	// Record PID.
	sess.PID = cmd.Process.Pid
	if saveErr := sess.Save(); saveErr != nil {
		log.Warn().Err(saveErr).Str("session", sess.ID).Msg("failed to save session with PID")
	}

	// Thread-safe log writer for concurrent stdout/stderr goroutines.
	safeLog := &syncWriter{w: logFile}

	var outputBytes int64
	var outputLines int
	var stdinErr, scannerErr, stderrErr error
	var wg sync.WaitGroup

	// Write prompt to stdin, then close.
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { _ = stdin.Close() }()
		if _, writeErr := io.WriteString(stdin, prompt); writeErr != nil {
			stdinErr = writeErr
			log.Error().Err(writeErr).Str("session", sess.ID).Msg("failed to write prompt to stdin")
		}
	}()

	// Tee stdout to log file + counter + optional mirror.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var writers []io.Writer
		writers = append(writers, safeLog)
		if mirror != nil {
			writers = append(writers, mirror)
		}
		mw := io.MultiWriter(writers...)
		reader := bufio.NewReader(stdout)
		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				outputLines++
				n, writeErr := mw.Write(line)
				if writeErr != nil {
					log.Error().Err(writeErr).Str("session", sess.ID).Msg("failed to write output line")
				}
				outputBytes += int64(n)
			}
			if err != nil {
				if err != io.EOF {
					scannerErr = err
					log.Warn().Err(err).Str("session", sess.ID).Msg("stdout read error — output may be truncated")
				}
				break
			}
		}
	}()

	// Stderr goes to log file only.
	wg.Add(1)
	go func() {
		defer wg.Done()
		if _, copyErr := io.Copy(safeLog, stderr); copyErr != nil {
			stderrErr = copyErr
			log.Error().Err(copyErr).Str("session", sess.ID).Msg("failed to copy stderr to log")
		}
	}()

	wg.Wait()
	err = cmd.Wait()

	// Check for I/O errors from goroutines.
	if stdinErr != nil {
		return nil, fmt.Errorf("write prompt to stdin: %w", stdinErr)
	}
	if scannerErr != nil {
		log.Warn().Err(scannerErr).Str("session", sess.ID).Msg("stdout was truncated")
	}
	if stderrErr != nil {
		log.Warn().Err(stderrErr).Str("session", sess.ID).Msg("stderr capture failed")
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return nil, fmt.Errorf("subprocess %s: %w", binary, err)
		}
	}

	return &Result{
		ExitCode:    exitCode,
		OutputBytes: outputBytes,
		OutputLines: outputLines,
	}, nil
}
