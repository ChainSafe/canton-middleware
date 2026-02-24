package pg

import (
	"time"

	"github.com/chainsafe/canton-middleware/pkg/registration"
)

// UserDao is a data access object that maps directly to the 'users' table in PostgreSQL.
// TODO:  remove json tag
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

// toUserDao converts a registration.User to UserDao
func toUserDao(user *registration.User) *UserDao {
	dao := &UserDao{
		EVMAddress:  user.EVMAddress,
		CantonParty: user.CantonParty,
		Fingerprint: user.Fingerprint,
	}

	if user.MappingCID != "" {
		dao.MappingCID = &user.MappingCID
	}
	if user.CantonPartyID != "" {
		dao.CantonPartyID = &user.CantonPartyID
	}
	if user.CantonKeyCreatedAt != nil {
		dao.CantonKeyCreatedAt = user.CantonKeyCreatedAt
	}
	if user.CantonPrivateKeyEncrypted != "" {
		dao.CantonPrivateKeyEncrypted = &user.CantonPrivateKeyEncrypted
	}
	if user.PromptBalance != "" {
		dao.PromptBalance = &user.PromptBalance
	}
	if user.DemoBalance != "" {
		dao.DemoBalance = &user.DemoBalance
	}
	if user.BalanceUpdatedAt != nil {
		dao.BalanceUpdatedAt = user.BalanceUpdatedAt
	}

	return dao
}

// toUser converts a UserDao to registration.User
func toUser(dao *UserDao) *registration.User {
	user := &registration.User{
		EVMAddress:  dao.EVMAddress,
		CantonParty: dao.CantonParty,
		Fingerprint: dao.Fingerprint,
	}

	if dao.MappingCID != nil {
		user.MappingCID = *dao.MappingCID
	}
	if dao.CantonPartyID != nil {
		user.CantonPartyID = *dao.CantonPartyID
	}
	if dao.CantonKeyCreatedAt != nil {
		user.CantonKeyCreatedAt = dao.CantonKeyCreatedAt
	}
	if dao.CantonPrivateKeyEncrypted != nil {
		user.CantonPrivateKeyEncrypted = *dao.CantonPrivateKeyEncrypted
	}
	if dao.PromptBalance != nil {
		user.PromptBalance = *dao.PromptBalance
	}
	if dao.DemoBalance != nil {
		user.DemoBalance = *dao.DemoBalance
	}
	if dao.BalanceUpdatedAt != nil {
		user.BalanceUpdatedAt = dao.BalanceUpdatedAt
	}

	return user
}
