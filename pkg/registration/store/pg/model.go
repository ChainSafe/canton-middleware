package pg

import (
	"time"
)

// UserDAO represents the database model for a user
type UserDAO struct {
	EVMAddress         string     `db:"evm_address"`
	CantonParty        string     `db:"canton_party"`
	Fingerprint        string     `db:"fingerprint"`
	MappingCID         string     `db:"mapping_cid"`
	CantonPartyID      string     `db:"canton_party_id"`
	CantonKeyCreatedAt *time.Time `db:"canton_key_created_at"`
}
