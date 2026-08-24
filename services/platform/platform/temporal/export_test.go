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
