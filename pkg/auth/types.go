package auth

type NonceResponse struct {
	Nonce string `json:"nonce"`
}

type LoginRequest struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"` // unix seconds
}
