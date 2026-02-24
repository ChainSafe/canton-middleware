package userstore

import (
	"time"

	"github.com/chainsafe/canton-middleware/pkg/user"
)

// UserDao is a data access object that maps directly to the 'users' table in PostgreSQL.
// TODO:  remove json tag
type UserDao struct {
	tableName                 struct{}   `bun:"table:users"` //nolint:unused // Bun uses this marker field for table mapping via struct tags.
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

// toUserDao converts a user.User to UserDao.
func toUserDao(usr *user.User) *UserDao {
	dao := &UserDao{
		EVMAddress:  usr.EVMAddress,
		CantonParty: usr.CantonParty,
		Fingerprint: usr.Fingerprint,
	}

	if usr.MappingCID != "" {
		dao.MappingCID = &usr.MappingCID
	}
	if usr.CantonPartyID != "" {
		dao.CantonPartyID = &usr.CantonPartyID
	}
	if usr.CantonKeyCreatedAt != nil {
		dao.CantonKeyCreatedAt = usr.CantonKeyCreatedAt
	}
	if usr.CantonPrivateKeyEncrypted != "" {
		dao.CantonPrivateKeyEncrypted = &usr.CantonPrivateKeyEncrypted
	}
	if usr.PromptBalance != "" {
		dao.PromptBalance = &usr.PromptBalance
	}
	if usr.DemoBalance != "" {
		dao.DemoBalance = &usr.DemoBalance
	}
	if usr.BalanceUpdatedAt != nil {
		dao.BalanceUpdatedAt = usr.BalanceUpdatedAt
	}

	return dao
}

// toUser converts a UserDao to user.User.
func toUser(dao *UserDao) *user.User {
	usr := &user.User{
		EVMAddress:  dao.EVMAddress,
		CantonParty: dao.CantonParty,
		Fingerprint: dao.Fingerprint,
	}

	if dao.MappingCID != nil {
		usr.MappingCID = *dao.MappingCID
	}
	if dao.CantonPartyID != nil {
		usr.CantonPartyID = *dao.CantonPartyID
	}
	if dao.CantonKeyCreatedAt != nil {
		usr.CantonKeyCreatedAt = dao.CantonKeyCreatedAt
	}
	if dao.CantonPrivateKeyEncrypted != nil {
		usr.CantonPrivateKeyEncrypted = *dao.CantonPrivateKeyEncrypted
	}
	if dao.PromptBalance != nil {
		usr.PromptBalance = *dao.PromptBalance
	}
	if dao.DemoBalance != nil {
		usr.DemoBalance = *dao.DemoBalance
	}
	if dao.BalanceUpdatedAt != nil {
		usr.BalanceUpdatedAt = dao.BalanceUpdatedAt
	}

	return usr
}

// WhitelistDao is a data access object that maps directly to the 'whitelist' table in PostgreSQL.
type WhitelistDao struct {
	tableName  struct{}  `bun:"table:whitelist"` //nolint:unused // Bun uses this marker field for table mapping via struct tags.
	EVMAddress string    `json:"evm_address" bun:",pk,type:varchar(42)"`
	Note       *string   `json:"note,omitempty" bun:",type:varchar(500)"`
	CreatedAt  time.Time `json:"created_at" bun:",nullzero,default:current_timestamp"`
}
