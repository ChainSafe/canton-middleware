package dao

import "time"

// WhitelistDao is a data access object that maps directly to the 'whitelist' table in PostgreSQL.
type WhitelistDao struct {
	tableName  struct{}  `bun:"table:whitelist"` // nolint
	EVMAddress string    `json:"evm_address" bun:",pk,type:varchar(42)"`
	Note       *string   `json:"note,omitempty" bun:",type:varchar(500)"`
	CreatedAt  time.Time `json:"created_at" bun:",nullzero,default:current_timestamp"`
}
