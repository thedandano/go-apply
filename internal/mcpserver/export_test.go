package mcpserver

import "context"

// GetSessionStateForTest returns the state string and existence flag for the
// given session ID in the package-level memory store. Intended only for blackbox test files.
func GetSessionStateForTest(id string) (string, bool) {
	sess, ok, _ := sessions.Get(context.Background(), id)
	if !ok {
		return "", false
	}
	return string(sess.State), true
}
