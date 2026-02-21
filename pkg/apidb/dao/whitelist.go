package dao

import "time"

// WhitelistDao is a data access object that maps directly to the 'whitelist' table in PostgreSQL.
type WhitelistDao struct {
	tableName  struct{} `pg:"whitelist"` // nolint
	EVMAddress string   `json:"evm_address" pg:",pk"`
	Note       *string  `json:"note,omitempty" pg:"note"`
	CreatedAt  time.Time `json:"created_at" pg:"default:now()"`
}
