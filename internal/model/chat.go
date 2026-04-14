package model

// ChatMessage is a single turn in an LLM conversation.
type ChatMessage struct {
	Role    string
	Content string
}

// ChatOptions controls LLM generation parameters for a single call.
type ChatOptions struct {
	Temperature float64
	MaxTokens   int
}
