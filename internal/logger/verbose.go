package logger

import "sync/atomic"

var verboseFlag atomic.Bool

// SetVerbose sets the global verbose flag. When true, payload helpers (Task 4)
// will log full request/response bodies.
func SetVerbose(v bool) { verboseFlag.Store(v) }

// Verbose reports whether verbose (trace) logging is enabled.
func Verbose() bool { return verboseFlag.Load() }
