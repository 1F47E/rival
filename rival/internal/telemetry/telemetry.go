package telemetry

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/posthog/posthog-go"
)

const (
	sentryDSN  = "https://4cade01be5cad580635e873f91df96f5@o4506162959220736.ingest.us.sentry.io/4511041118797825"
	posthogKey = "phc_g4pN5JLI7ASt1TE5dMuMhZ4SeRk2kn3bUpRs3r1R2rh"
)

var (
	enabled    bool
	phClient   posthog.Client
	distinctID string
	appVersion string
)

// Init initialises Sentry and PostHog. Call once from main.
func Init(version string) {
	if !Enabled() {
		return
	}
	appVersion = version
	distinctID = loadOrCreateID()

	// Sentry — sync transport for CLI (async drops events on exit).
	_ = sentry.Init(sentry.ClientOptions{
		Dsn:              sentryDSN,
		Release:          "rival@" + version,
		Environment:      "production",
		SendDefaultPII:   false,
		TracesSampleRate: 0,
		Transport:        sentry.NewHTTPSyncTransport(),
	})

	// PostHog
	phClient, _ = posthog.NewWithConfig(posthogKey, posthog.Config{
		Endpoint: "https://us.i.posthog.com",
	})

	enabled = true
}

// Flush must be deferred in main to ensure events are sent before exit.
func Flush() {
	if !enabled {
		return
	}
	sentry.Flush(2 * time.Second)
	if phClient != nil {
		_ = phClient.Close()
	}
}

// Enabled returns false when telemetry is opted out.
func Enabled() bool {
	for _, key := range []string{"DO_NOT_TRACK", "RIVAL_NO_TELEMETRY", "CI"} {
		if v := os.Getenv(key); v != "" && v != "0" && v != "false" {
			return false
		}
	}
	return true
}

// SessionData holds the fields we track. Avoids importing session package (circular).
type SessionData struct {
	CLI       string
	Mode      string
	Model     string
	Effort    string
	Status    string // "completed" or "failed"
	ExitCode  int
	Duration  time.Duration
	ErrorMsg  string
}

// TrackSession sends a telemetry event for a finished session.
func TrackSession(d SessionData) {
	if !enabled {
		return
	}

	event := "session_" + d.Status
	durationSec := int(d.Duration.Seconds())

	props := map[string]interface{}{
		"app":        "rival",
		"cmd":        d.CLI,
		"mode":       d.Mode,
		"model":      d.Model,
		"effort":     d.Effort,
		"status":     d.Status,
		"exit_code":  d.ExitCode,
		"duration_s": durationSec,
		"version":    appVersion,
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
	}

	// PostHog
	if phClient != nil {
		phProps := posthog.NewProperties()
		for k, v := range props {
			phProps.Set(k, v)
		}
		_ = phClient.Enqueue(posthog.Capture{
			DistinctId: distinctID,
			Event:      event,
			Properties: phProps,
		})
	}

	// Sentry — only for failures.
	if d.Status == "failed" {
		sentry.WithScope(func(scope *sentry.Scope) {
			for k, v := range props {
				scope.SetTag(k, fmt.Sprintf("%v", v))
			}
			if d.ErrorMsg != "" {
				scope.SetTag("error_msg", truncate(d.ErrorMsg, 200))
			}
			sentry.CaptureMessage(event)
		})
	}
}

// RecoverPanic wraps sentry.Recover for use in defer.
func RecoverPanic() {
	if !enabled {
		return
	}
	sentry.Recover()
}

func loadOrCreateID() string {
	home, _ := os.UserHomeDir()
	idFile := filepath.Join(home, ".rival", ".telemetry-id")

	if data, err := os.ReadFile(idFile); err == nil && len(data) > 0 {
		return string(data)
	}

	// Generate from hostname hash.
	hostname, _ := os.Hostname()
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("rival-"+hostname+"-"+strconv.FormatInt(time.Now().UnixNano(), 10))))
	id := hash[:16]

	_ = os.MkdirAll(filepath.Dir(idFile), 0700)
	_ = os.WriteFile(idFile, []byte(id), 0600)
	return id
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
