package userstore

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/chainsafe/canton-middleware/pkg/user"
)

// UserDao is a data access object that maps directly to the 'users' table in PostgreSQL.
type UserDao struct {
	bun.BaseModel             `bun:"table:users,alias:u"`
	ID                        int64      `bun:"id,pk,autoincrement"`
	EVMAddress                string     `bun:"evm_address,unique,notnull,type:varchar(42)"`
	CantonParty               string     `bun:"canton_party,notnull,type:varchar(255)"`
	Fingerprint               string     `bun:"fingerprint,notnull,type:varchar(128)"`
	MappingCID                *string    `bun:"mapping_cid,type:varchar(255)"`
	PromptBalance             *string    `bun:"prompt_balance,nullzero,type:numeric(38,18)"`
	DemoBalance               *string    `bun:"demo_balance,nullzero,type:numeric(38,18)"`
	BalanceUpdatedAt          *time.Time `bun:"balance_updated_at"`
	CreatedAt                 time.Time  `bun:"created_at,nullzero,default:current_timestamp"`
	CantonPartyID             *string    `bun:"canton_party_id,type:varchar(255)"`
	CantonPrivateKeyEncrypted *string    `bun:"canton_private_key_encrypted,type:text"`
	CantonKeyCreatedAt        *time.Time `bun:"canton_key_created_at"`
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
	bun.BaseModel `bun:"table:whitelist,alias:w"`
	EVMAddress    string    `bun:"evm_address,pk,type:varchar(42)"`
	Note          *string   `bun:"note,type:varchar(500)"`
	CreatedAt     time.Time `bun:"created_at,nullzero,default:current_timestamp"`
}
