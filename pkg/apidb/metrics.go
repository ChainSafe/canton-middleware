package apidb

import (
	"database/sql"
	"fmt"
	"time"
)

// TokenMetrics represents the cached token metrics
type TokenMetrics struct {
	TotalSupply      string     `json:"total_supply"`
	LastReconciledAt *time.Time `json:"last_reconciled_at,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// GetTotalSupply returns the cached total supply
func (s *Store) GetTotalSupply() (string, error) {
	var totalSupply sql.NullString
	query := `SELECT total_supply FROM token_metrics WHERE id = 1`
	err := s.db.QueryRow(query).Scan(&totalSupply)
	if err == sql.ErrNoRows {
		return "0", nil
	}
	if err != nil {
		return "0", fmt.Errorf("failed to get total supply: %w", err)
	}
	if !totalSupply.Valid {
		return "0", nil
	}
	return totalSupply.String, nil
}

// GetTokenMetrics returns all token metrics
func (s *Store) GetTokenMetrics() (*TokenMetrics, error) {
	metrics := &TokenMetrics{}
	var totalSupply sql.NullString
	var lastReconciledAt sql.NullTime
	query := `SELECT total_supply, last_reconciled_at, updated_at FROM token_metrics WHERE id = 1`
	err := s.db.QueryRow(query).Scan(&totalSupply, &lastReconciledAt, &metrics.UpdatedAt)
	if err == sql.ErrNoRows {
		return &TokenMetrics{TotalSupply: "0"}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get token metrics: %w", err)
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

// SetTotalSupply sets the total supply to a specific value
func (s *Store) SetTotalSupply(value string) error {
	query := `
		INSERT INTO token_metrics (id, total_supply, updated_at)
		VALUES (1, $1, NOW())
		ON CONFLICT (id) DO UPDATE
		SET total_supply = $1, updated_at = NOW()
	`
	_, err := s.db.Exec(query, value)
	if err != nil {
		return fmt.Errorf("failed to set total supply: %w", err)
	}
	return nil
}

// IncrementTotalSupply adds amount to total supply atomically
func (s *Store) IncrementTotalSupply(amount string) error {
	query := `
		INSERT INTO token_metrics (id, total_supply, updated_at)
		VALUES (1, $1::DECIMAL, NOW())
		ON CONFLICT (id) DO UPDATE
		SET total_supply = COALESCE(token_metrics.total_supply, 0) + $1::DECIMAL, updated_at = NOW()
	`
	_, err := s.db.Exec(query, amount)
	if err != nil {
		return fmt.Errorf("failed to increment total supply: %w", err)
	}
	return nil
}

// DecrementTotalSupply subtracts amount from total supply atomically
func (s *Store) DecrementTotalSupply(amount string) error {
	query := `
		UPDATE token_metrics
		SET total_supply = COALESCE(total_supply, 0) - $1::DECIMAL, updated_at = NOW()
		WHERE id = 1
	`
	_, err := s.db.Exec(query, amount)
	if err != nil {
		return fmt.Errorf("failed to decrement total supply: %w", err)
	}
	return nil
}

// UpdateLastReconciled updates the last reconciliation timestamp
func (s *Store) UpdateLastReconciled() error {
	query := `
		UPDATE token_metrics
		SET last_reconciled_at = NOW(), updated_at = NOW()
		WHERE id = 1
	`
	_, err := s.db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to update last reconciled: %w", err)
	}
	return nil
}
