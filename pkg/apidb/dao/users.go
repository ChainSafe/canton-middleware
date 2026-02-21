package dao

import "time"

// UserDao is a data access object that maps directly to the 'users' table in PostgreSQL.
type UserDao struct {
	tableName                 struct{}   `pg:"users"` // nolint
	ID                        int64      `json:"id" pg:",pk"`
	EVMAddress                string     `json:"evm_address" pg:",unique,notnull"`
	CantonParty               string     `json:"canton_party" pg:",notnull"`
	Fingerprint               string     `json:"fingerprint" pg:",notnull"`
	MappingCID                *string    `json:"mapping_cid,omitempty" pg:"mapping_cid"`
	PromptBalance             *string    `json:"prompt_balance" pg:"prompt_balance,use_zero"`
	DemoBalance               *string    `json:"demo_balance" pg:"demo_balance,use_zero"`
	BalanceUpdatedAt          *time.Time `json:"balance_updated_at,omitempty" pg:"balance_updated_at"`
	CreatedAt                 time.Time  `json:"created_at" pg:"default:now()"`
	CantonPartyID             *string    `json:"canton_party_id,omitempty" pg:"canton_party_id"`
	CantonPrivateKeyEncrypted *string    `json:"-" pg:"canton_private_key_encrypted"`
	CantonKeyCreatedAt        *time.Time `json:"canton_key_created_at,omitempty" pg:"canton_key_created_at"`
}
