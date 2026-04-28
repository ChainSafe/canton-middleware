package token

// Type identifies a token for balance operations.
type Type string

const (
	Prompt Type = "PROMPT"
	Demo   Type = "DEMO"
)

// TokenItem is a single token entry in the list response.
type TokenItem struct {
	Address  string `json:"address"`
	Name     string `json:"name"`
	Symbol   string `json:"symbol"`
	Decimals int    `json:"decimals"`
}

// TokensPage is the cursor-paginated response for GET /tokens.
type TokensPage struct {
	Items      []TokenItem `json:"items"`
	NextCursor string      `json:"next_cursor,omitempty"`
	HasMore    bool        `json:"has_more"`
}
