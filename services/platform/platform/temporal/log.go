package temporal

import (
	"context"
	"log/slog"

	"github.com/Yelethe1st/prepeet/services/platform/platform/telemetry"
)

// logAdapter presents an slog.Logger as the interface the Temporal SDK expects.
//
// The adapter exists so SDK output goes through the same scrubbing and trace
// correlation as everything else. Without it the SDK writes its own lines in its
// own shape, and those lines carry connection strings on a failed dial, which is
// precisely the case somebody is reading logs during.
//
// It scrubs even though telemetry.NewLogger already does, because it is handed
// whatever logger the caller has and Dial falls back to slog.Default() when
// given none. That redundancy is only worth having if it is tested against a
// plain logger rather than a scrubbing one, which is what the test does; an
// earlier version tested it through NewLogger and could not have failed.
type logAdapter struct {
	inner *slog.Logger
}

func newLogAdapter(inner *slog.Logger) *logAdapter { return &logAdapter{inner: inner} }

// The SDK passes alternating keys and values. They are scrubbed here because the
// values are the SDK's, not ours: a namespace or an error string from a failed
// connection arrives through this path.
func (l *logAdapter) log(level slog.Level, message string, keyvals ...any) {
	scrubbed := make([]any, 0, len(keyvals))
	for _, value := range keyvals {
		if text, isText := value.(string); isText {
			scrubbed = append(scrubbed, telemetry.Scrub(text))
			continue
		}
		scrubbed = append(scrubbed, value)
	}
	l.inner.Log(context.Background(), level, telemetry.Scrub(message), scrubbed...)
}

func (l *logAdapter) Debug(message string, keyvals ...any) {
	l.log(slog.LevelDebug, message, keyvals...)
}
func (l *logAdapter) Info(message string, keyvals ...any) { l.log(slog.LevelInfo, message, keyvals...) }
func (l *logAdapter) Warn(message string, keyvals ...any) { l.log(slog.LevelWarn, message, keyvals...) }
func (l *logAdapter) Error(message string, keyvals ...any) {
	l.log(slog.LevelError, message, keyvals...)
}
