package store

import (
	"time"

	"github.com/chainsafe/canton-middleware/pkg/ethrpc"

	"github.com/uptrace/bun"
)

// EvmTransactionDao maps to the evm_transactions table.
type EvmTransactionDao struct {
	bun.BaseModel `bun:"table:evm_transactions"`
	TxHash        []byte    `bun:"tx_hash,pk,notnull,type:bytea"`
	FromAddress   string    `bun:"from_address,notnull,type:text"`
	ToAddress     string    `bun:"to_address,notnull,type:text"`
	Nonce         int64     `bun:"nonce,notnull"`
	Input         []byte    `bun:"input,notnull,type:bytea"`
	ValueWei      string    `bun:"value_wei,notnull,default:'0',type:text"`
	Status        int16     `bun:"status,notnull,default:1,type:smallint"`
	BlockNumber   int64     `bun:"block_number,notnull"`
	BlockHash     []byte    `bun:"block_hash,notnull,type:bytea"`
	TxIndex       int       `bun:"tx_index,notnull,default:0"`
	GasUsed       int64     `bun:"gas_used,notnull,default:21000"`
	ErrorMessage  *string   `bun:"error_message,type:text"`
	CreatedAt     time.Time `bun:"created_at,notnull,default:current_timestamp"`
}

func toEvmTransactionDao(tx *ethrpc.EvmTransaction) *EvmTransactionDao {
	if tx == nil {
		return nil
	}

	return &EvmTransactionDao{
		TxHash:       tx.TxHash,
		FromAddress:  tx.FromAddress,
		ToAddress:    tx.ToAddress,
		Nonce:        tx.Nonce,
		Input:        tx.Input,
		ValueWei:     tx.ValueWei,
		Status:       tx.Status,
		BlockNumber:  tx.BlockNumber,
		BlockHash:    tx.BlockHash,
		TxIndex:      tx.TxIndex,
		GasUsed:      tx.GasUsed,
		ErrorMessage: stringPtrOrNil(tx.ErrorMessage),
	}
}

func fromEvmTransactionDao(dao *EvmTransactionDao) *ethrpc.EvmTransaction {
	if dao == nil {
		return nil
	}

	tx := &ethrpc.EvmTransaction{
		TxHash:      dao.TxHash,
		FromAddress: dao.FromAddress,
		ToAddress:   dao.ToAddress,
		Nonce:       dao.Nonce,
		Input:       dao.Input,
		ValueWei:    dao.ValueWei,
		Status:      dao.Status,
		BlockNumber: dao.BlockNumber,
		BlockHash:   dao.BlockHash,
		TxIndex:     dao.TxIndex,
		GasUsed:     dao.GasUsed,
	}
	if dao.ErrorMessage != nil {
		tx.ErrorMessage = *dao.ErrorMessage
	}

	return tx
}

// EvmMetaDao maps to the evm_meta table.
type EvmMetaDao struct {
	bun.BaseModel `bun:"table:evm_meta"`
	Key           string `bun:",pk,notnull,type:text"`
	Value         string `bun:",notnull,type:text"`
}

// EvmLogDao maps to the evm_logs table.
type EvmLogDao struct {
	bun.BaseModel `bun:"table:evm_logs"`
	TxHash        []byte  `bun:"tx_hash,pk,notnull,type:bytea"`
	LogIndex      int     `bun:"log_index,pk,notnull"`
	Address       []byte  `bun:"address,notnull,type:bytea"`
	Topic0        *[]byte `bun:"topic0,type:bytea"`
	Topic1        *[]byte `bun:"topic1,type:bytea"`
	Topic2        *[]byte `bun:"topic2,type:bytea"`
	Topic3        *[]byte `bun:"topic3,type:bytea"`
	Data          *[]byte `bun:"data,type:bytea"`
	BlockNumber   int64   `bun:"block_number,notnull"`
	BlockHash     []byte  `bun:"block_hash,notnull,type:bytea"`
	TxIndex       int     `bun:"tx_index,notnull,default:0"`
	Removed       bool    `bun:"removed,notnull,default:false"`
}

func toEvmLogDao(log *ethrpc.EvmLog) *EvmLogDao {
	if log == nil {
		return nil
	}

	return &EvmLogDao{
		TxHash:      log.TxHash,
		LogIndex:    log.LogIndex,
		Address:     log.Address,
		Topic0:      topicAt(log.Topics, 0),
		Topic1:      topicAt(log.Topics, 1),
		Topic2:      topicAt(log.Topics, 2),
		Topic3:      topicAt(log.Topics, 3),
		Data:        bytesPtr(log.Data),
		BlockNumber: log.BlockNumber,
		BlockHash:   log.BlockHash,
		TxIndex:     log.TxIndex,
		Removed:     log.Removed,
	}
}

func fromEvmLogDao(dao *EvmLogDao) *ethrpc.EvmLog {
	if dao == nil {
		return nil
	}

	log := &ethrpc.EvmLog{
		TxHash:      dao.TxHash,
		LogIndex:    dao.LogIndex,
		Address:     dao.Address,
		Data:        derefBytes(dao.Data),
		BlockNumber: dao.BlockNumber,
		BlockHash:   dao.BlockHash,
		TxIndex:     dao.TxIndex,
		Removed:     dao.Removed,
	}

	if dao.Topic0 != nil && len(*dao.Topic0) > 0 {
		log.Topics = append(log.Topics, *dao.Topic0)
	}
	if dao.Topic1 != nil && len(*dao.Topic1) > 0 {
		log.Topics = append(log.Topics, *dao.Topic1)
	}
	if dao.Topic2 != nil && len(*dao.Topic2) > 0 {
		log.Topics = append(log.Topics, *dao.Topic2)
	}
	if dao.Topic3 != nil && len(*dao.Topic3) > 0 {
		log.Topics = append(log.Topics, *dao.Topic3)
	}

	return log
}

func topicAt(topics [][]byte, idx int) *[]byte {
	if idx < 0 || idx >= len(topics) {
		return nil
	}
	return bytesPtr(topics[idx])
}

func bytesPtr(data []byte) *[]byte {
	if data == nil {
		return nil
	}
	out := make([]byte, len(data))
	copy(out, data)
	return &out
}

func derefBytes(data *[]byte) []byte {
	if data == nil {
		return nil
	}
	out := make([]byte, len(*data))
	copy(out, *data)
	return out
}

func stringPtrOrNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
