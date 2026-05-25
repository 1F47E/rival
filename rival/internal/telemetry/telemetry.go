package telemetry

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

const sentryDSN = "https://4cade01be5cad580635e873f91df96f5@o4506162959220736.ingest.us.sentry.io/4511041118797825"

var enabled bool

func Init(version string) {
	if !Enabled() {
		return
	}
	_ = sentry.Init(sentry.ClientOptions{
		Dsn:              sentryDSN,
		Release:          "rival@" + version,
		Environment:      "production",
		SendDefaultPII:   false,
		TracesSampleRate: 0,
		Transport:        sentry.NewHTTPSyncTransport(),
	})
	enabled = true
}

func Flush() {
	if enabled {
		sentry.Flush(2 * time.Second)
	}
}

func Enabled() bool {
	for _, key := range []string{"DO_NOT_TRACK", "RIVAL_NO_TELEMETRY", "CI"} {
		if v := os.Getenv(key); v != "" && v != "0" && v != "false" {
			return false
		}
	}
	return true
}

func RecoverPanic() {
	if enabled {
		sentry.Recover()
	}
}
