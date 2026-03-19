package store

import (
	"time"

	"github.com/uptrace/bun"

	"github.com/chainsafe/canton-middleware/pkg/relayer"
)

// TransferDao maps to the 'transfers' table.
type TransferDao struct {
	bun.BaseModel     `bun:"table:transfers"`
	ID                string     `bun:",pk,type:varchar(255)"`
	Direction         string     `bun:",notnull,type:varchar(50)"`
	Status            string     `bun:",notnull,type:varchar(50)"`
	SourceChain       string     `bun:",notnull,type:varchar(100)"`
	DestinationChain  string     `bun:",notnull,type:varchar(100)"`
	SourceTxHash      string     `bun:",notnull,type:varchar(255)"`
	DestinationTxHash *string    `bun:",type:varchar(255)"`
	TokenAddress      string     `bun:",notnull,type:varchar(255)"`
	Amount            string     `bun:",notnull,type:varchar(255)"`
	Sender            string     `bun:",notnull,type:varchar(255)"`
	Recipient         string     `bun:",notnull,type:varchar(255)"`
	Nonce             int64      `bun:",notnull"`
	SourceBlockNumber uint64     `bun:",notnull"`
	RetryCount        int        `bun:",notnull,default:0"`
	CreatedAt         time.Time  `bun:",notnull,default:current_timestamp"`
	UpdatedAt         time.Time  `bun:",notnull,default:current_timestamp"`
	CompletedAt       *time.Time `bun:"completed_at"`
	ErrorMessage      *string    `bun:",type:text"`
}

// ChainStateDao maps to the 'chain_state' table.
type ChainStateDao struct {
	bun.BaseModel `bun:"table:chain_state"`
	ChainID       string    `bun:",pk,type:varchar(100)"`
	LastBlock     uint64    `bun:",notnull"`
	LastBlockHash string    `bun:",notnull,type:varchar(255)"` // stores the string offset
	UpdatedAt     time.Time `bun:",notnull,default:current_timestamp"`
}

func toTransferDao(t *relayer.Transfer) *TransferDao {
	return &TransferDao{
		ID:                t.ID,
		Direction:         string(t.Direction),
		Status:            string(t.Status),
		SourceChain:       t.SourceChain,
		DestinationChain:  t.DestinationChain,
		SourceTxHash:      t.SourceTxHash,
		DestinationTxHash: t.DestinationTxHash,
		TokenAddress:      t.TokenAddress,
		Amount:            t.Amount,
		Sender:            t.Sender,
		Recipient:         t.Recipient,
		Nonce:             t.Nonce,
		SourceBlockNumber: t.SourceBlockNumber,
		RetryCount:        t.RetryCount,
		CompletedAt:       t.CompletedAt,
		ErrorMessage:      t.ErrorMessage,
	}
}

func fromTransferDao(d *TransferDao) *relayer.Transfer {
	return &relayer.Transfer{
		ID:                d.ID,
		Direction:         relayer.TransferDirection(d.Direction),
		Status:            relayer.TransferStatus(d.Status),
		SourceChain:       d.SourceChain,
		DestinationChain:  d.DestinationChain,
		SourceTxHash:      d.SourceTxHash,
		DestinationTxHash: d.DestinationTxHash,
		TokenAddress:      d.TokenAddress,
		Amount:            d.Amount,
		Sender:            d.Sender,
		Recipient:         d.Recipient,
		Nonce:             d.Nonce,
		SourceBlockNumber: d.SourceBlockNumber,
		RetryCount:        d.RetryCount,
		CreatedAt:         d.CreatedAt,
		UpdatedAt:         d.UpdatedAt,
		CompletedAt:       d.CompletedAt,
		ErrorMessage:      d.ErrorMessage,
	}
}

func fromChainStateDao(d *ChainStateDao) *relayer.ChainState {
	return &relayer.ChainState{
		ChainID:   d.ChainID,
		LastBlock: d.LastBlock,
		Offset:    d.LastBlockHash,
		UpdatedAt: d.UpdatedAt,
	}
}
