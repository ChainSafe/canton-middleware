package apidb

import (
	"database/sql"
	"fmt"
	"time"
)

// TokenMetrics represents the cached token metrics for a specific token
type TokenMetrics struct {
	TokenSymbol      string     `json:"token_symbol"`
	TotalSupply      string     `json:"total_supply"`
	LastReconciledAt *time.Time `json:"last_reconciled_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// GetTotalSupply returns the cached total supply for a specific token
func (s *Store) GetTotalSupply(tokenSymbol string) (string, error) {
	var totalSupply sql.NullString
	query := `SELECT total_supply FROM token_metrics WHERE token_symbol = $1`
	err := s.db.QueryRow(query, tokenSymbol).Scan(&totalSupply)
	if err == sql.ErrNoRows {
		return "0", nil
	}
	if err != nil {
		return "0", fmt.Errorf("failed to get total supply for %s: %w", tokenSymbol, err)
	}
	if !totalSupply.Valid {
		return "0", nil
	}
	return totalSupply.String, nil
}

// GetTokenMetrics returns token metrics for a specific token
func (s *Store) GetTokenMetrics(tokenSymbol string) (*TokenMetrics, error) {
	metrics := &TokenMetrics{TokenSymbol: tokenSymbol}
	var totalSupply sql.NullString
	var lastReconciledAt sql.NullTime
	query := `SELECT total_supply, last_reconciled_at, updated_at FROM token_metrics WHERE token_symbol = $1`
	err := s.db.QueryRow(query, tokenSymbol).Scan(&totalSupply, &lastReconciledAt, &metrics.UpdatedAt)
	if err == sql.ErrNoRows {
		return &TokenMetrics{TokenSymbol: tokenSymbol, TotalSupply: "0"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get token metrics for %s: %w", tokenSymbol, err)
	}
	if totalSupply.Valid {
		metrics.TotalSupply = totalSupply.String
	} else {
		metrics.TotalSupply = "0"
	}
	if lastReconciledAt.Valid {
		metrics.LastReconciledAt = &lastReconciledAt.Time
	}
	return metrics, nil
}

// SetTotalSupply sets the total supply for a specific token
func (s *Store) SetTotalSupply(tokenSymbol, value string) error {
	query := `
		INSERT INTO token_metrics (token_symbol, total_supply, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (token_symbol) DO UPDATE
		SET total_supply = $2, updated_at = NOW()
	`
	_, err := s.db.Exec(query, tokenSymbol, value)
	if err != nil {
		return fmt.Errorf("failed to set total supply for %s: %w", tokenSymbol, err)
	}
	return nil
}

// IncrementTotalSupply adds amount to total supply for a specific token atomically
func (s *Store) IncrementTotalSupply(tokenSymbol, amount string) error {
	query := `
		INSERT INTO token_metrics (token_symbol, total_supply, updated_at)
		VALUES ($1, $2::DECIMAL, NOW())
		ON CONFLICT (token_symbol) DO UPDATE
		SET total_supply = COALESCE(token_metrics.total_supply, 0) + $2::DECIMAL, updated_at = NOW()
	`
	_, err := s.db.Exec(query, tokenSymbol, amount)
	if err != nil {
		return fmt.Errorf("failed to increment total supply for %s: %w", tokenSymbol, err)
	}
	return nil
}

// DecrementTotalSupply subtracts amount from total supply for a specific token atomically
func (s *Store) DecrementTotalSupply(tokenSymbol, amount string) error {
	query := `
		UPDATE token_metrics
		SET total_supply = COALESCE(total_supply, 0) - $1::DECIMAL, updated_at = NOW()
		WHERE token_symbol = $2
	`
	_, err := s.db.Exec(query, amount, tokenSymbol)
	if err != nil {
		return fmt.Errorf("failed to decrement total supply for %s: %w", tokenSymbol, err)
	}
	return nil
}

// UpdateLastReconciled updates the last reconciliation timestamp for a specific token
func (s *Store) UpdateLastReconciled(tokenSymbol string) error {
	query := `
		UPDATE token_metrics
		SET last_reconciled_at = NOW(), updated_at = NOW()
		WHERE token_symbol = $1
	`
	_, err := s.db.Exec(query, tokenSymbol)
	if err != nil {
		return fmt.Errorf("failed to update last reconciled for %s: %w", tokenSymbol, err)
	}
	return nil
}
