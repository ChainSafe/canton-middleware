package dao

import "time"

// UserDao is a data access object that maps directly to the 'users' table in PostgreSQL.
type UserDao struct {
	tableName                 struct{}   `pg:"users"` // nolint
	ID                        int64      `json:"id" pg:",pk"`
	EVMAddress                string     `json:"evm_address" pg:",unique,notnull,type:VARCHAR(42)"`
	CantonParty               string     `json:"canton_party" pg:",notnull,type:VARCHAR(255)"`
	Fingerprint               string     `json:"fingerprint" pg:",notnull,type:VARCHAR(64)"`
	MappingCID                *string    `json:"mapping_cid,omitempty" pg:"mapping_cid,type:VARCHAR(128)"`
	PromptBalance             *string    `json:"prompt_balance" pg:"prompt_balance,use_zero,type:NUMERIC(38,18)"`
	DemoBalance               *string    `json:"demo_balance" pg:"demo_balance,use_zero,type:NUMERIC(38,18)"`
	BalanceUpdatedAt          *time.Time `json:"balance_updated_at,omitempty" pg:"balance_updated_at"`
	CreatedAt                 time.Time  `json:"created_at" pg:"default:now()"`
	CantonPartyID             *string    `json:"canton_party_id,omitempty" pg:"canton_party_id,type:VARCHAR(255)"`
	CantonPrivateKeyEncrypted *string    `json:"-" pg:"canton_private_key_encrypted"`
	CantonKeyCreatedAt        *time.Time `json:"canton_key_created_at,omitempty" pg:"canton_key_created_at"`
}
