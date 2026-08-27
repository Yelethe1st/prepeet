package temporal

import "log/slog"

// LogForTest exposes the SDK log adapter to the external test package.
//
// The adapter is unexported because nothing outside this package should build
// one, and it is reachable here because "the adapter scrubs" is the assertion
// that matters and testing it only through a failed dial would make it depend
// on what the SDK happens to log.
func LogForTest(logger *slog.Logger, message string, keyvals ...any) {
	if logger == nil {
		logger = slog.Default()
	}
	newLogAdapter(logger).Error(message, keyvals...)
}

// LogEveryLevelForTest drives the adapter at each level the SDK uses.
//
// The SDK picks the level, not us, so every one of them must scrub: a
// level that forwarded a raw message would be a hole the size of whatever
// the SDK decided to log that day.
func LogEveryLevelForTest(logger *slog.Logger, message string, keyvals ...any) {
	adapter := newLogAdapter(logger)
	adapter.Debug(message, keyvals...)
	adapter.Info(message, keyvals...)
	adapter.Warn(message, keyvals...)
	adapter.Error(message, keyvals...)
}
