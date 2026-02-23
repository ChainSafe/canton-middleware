package dao

import "time"

// UserDao is a data access object that maps directly to the 'users' table in PostgreSQL.
type UserDao struct {
	tableName                 struct{}   `bun:"table:users"` // nolint
	ID                        int64      `json:"id" bun:",pk,autoincrement"`
	EVMAddress                string     `json:"evm_address" bun:",unique,notnull,type:varchar(42)"`
	CantonParty               string     `json:"canton_party" bun:",notnull,type:varchar(255)"`
	Fingerprint               string     `json:"fingerprint" bun:",notnull,type:varchar(128)"`
	MappingCID                *string    `json:"mapping_cid,omitempty" bun:",type:varchar(255)"`
	PromptBalance             *string    `json:"prompt_balance" bun:",nullzero,type:numeric(38,18)"`
	DemoBalance               *string    `json:"demo_balance" bun:",nullzero,type:numeric(38,18)"`
	BalanceUpdatedAt          *time.Time `json:"balance_updated_at,omitempty" bun:"balance_updated_at"`
	CreatedAt                 time.Time  `json:"created_at" bun:",nullzero,default:current_timestamp"`
	CantonPartyID             *string    `json:"canton_party_id,omitempty" bun:",type:varchar(255)"`
	CantonPrivateKeyEncrypted *string    `json:"-" bun:",type:text"`
	CantonKeyCreatedAt        *time.Time `json:"canton_key_created_at,omitempty" bun:"canton_key_created_at"`
}
