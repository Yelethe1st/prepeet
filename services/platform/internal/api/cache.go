package api

// The cacheability values the contract declares.
//
// Named here rather than typed at each call site so that the value a handler
// sends and the value the contract promises are the same string, and so the
// test that compares the two has something to compare against.
//
// Every value in this file comes from the caching conventions in ADR-0004.
const (
	// NoStore is for anything derived from a person's own data: a session, a
	// profile, an evaluation. It must not be written to disk by an
	// intermediary, and a failure must not be cached either, because a cached
	// failure outlives the thing that caused it.
	NoStore = "no-store"
)
