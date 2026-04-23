package mcpserver

// GetSessionStateForTest returns the state string and existence flag for the
// given session ID. Intended only for use in blackbox test files.
func GetSessionStateForTest(id string) (string, bool) {
	sess := sessions.Get(id)
	if sess == nil {
		return "", false
	}
	return sess.State.String(), true
}
