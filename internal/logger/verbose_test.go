package logger

import "testing"

func TestSetVerboseRoundTrip(t *testing.T) {
	// Ensure initial state is false.
	SetVerbose(false)
	if Verbose() {
		t.Error("Verbose() should be false after SetVerbose(false)")
	}

	SetVerbose(true)
	if !Verbose() {
		t.Error("Verbose() should be true after SetVerbose(true)")
	}

	SetVerbose(false)
	if Verbose() {
		t.Error("Verbose() should be false after second SetVerbose(false)")
	}
}
