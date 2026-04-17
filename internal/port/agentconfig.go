package port

// MCPServerEntry describes an MCP server registration in an agent's config.
type MCPServerEntry struct {
	Command string   `json:"command" yaml:"command"`
	Args    []string `json:"args"    yaml:"args"`
}

// RegistrationAction describes what happened during MCP registration.
type RegistrationAction int

const (
	ActionCreated           RegistrationAction = iota // new config file created
	ActionAdded                                       // entry added to existing file
	ActionAlreadyRegistered                           // entry already present, no changes
	ActionRemoved                                     // entry removed from config
	ActionNotFound                                    // entry was not present, nothing to remove
)

// RegistrationResult reports the outcome of an MCP registration.
type RegistrationResult struct {
	ConfigPath string
	Action     RegistrationAction
}

// AgentConfigRegistrar registers or unregisters an MCP server in an agent's config file.
type AgentConfigRegistrar interface {
	Register(serverName string, entry MCPServerEntry) (RegistrationResult, error)
	// RegisterForce overwrites any existing registration unconditionally.
	RegisterForce(serverName string, entry MCPServerEntry) (RegistrationResult, error)
	Unregister(serverName string) (RegistrationResult, error)
}
