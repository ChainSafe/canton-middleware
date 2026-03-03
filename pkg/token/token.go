package token

// Type represents a token type for balance operations.
type Type string

const (
	Prompt Type = "prompt" // PROMPT (bridged) token
	Demo   Type = "demo"   // DEMO (native) token
)
