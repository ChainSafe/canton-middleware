# M2: MainNet RPC transcript

Every supported read method, called against the public MainNet endpoint
`https://middleware-api-prod1.02.chainsafe.dev/eth` on 24 November 2026: 25 calls, no errors. Each entry shows
the time of the call, the request and the full response.

The transaction and blocks are from the [recorded transfer](./m2#recorded-transfer-on-the-current-release)
(block `0x25`). The block responses show the [synthetic-block limitation](./m2#known-limitation-synthetic-blocks):
no transactions and a timestamp derived from the block number.

### `net_version`

2026-11-24 13:46:29 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "net_version", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "1155111102"
}
```

### `net_listening`

2026-11-24 13:46:30 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "net_listening", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": true
}
```

### `net_peerCount`

2026-11-24 13:46:31 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "net_peerCount", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x0"
}
```

### `web3_clientVersion`

2026-11-24 13:46:32 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "web3_clientVersion", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "canton-middleware/v0.10.0-13b07ac230726a4ff8d73b4f39ced49310d1c383"
}
```

### `web3_sha3`

2026-11-24 13:46:32 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "web3_sha3", "params": ["0x68656c6c6f"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x1c8aff950685c2ed4bc3174f3472287b56d9517b9c948127319a09a7a36deac8"
}
```

### `eth_chainId`

2026-11-24 13:46:33 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_chainId", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x44d998be"
}
```

### `eth_blockNumber`

2026-11-24 13:46:34 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_blockNumber", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0xe31cd"
}
```

### `eth_syncing`

2026-11-24 13:46:35 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_syncing", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": false
}
```

### `eth_gasPrice`

2026-11-24 13:46:36 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_gasPrice", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x0"
}
```

### `eth_maxPriorityFeePerGas`

2026-11-24 13:46:37 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_maxPriorityFeePerGas", "params": []}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x0"
}
```

### `eth_getBalance`

2026-11-24 13:46:37 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_getBalance", "params": ["0xb90d8162a3a1a2d760699f1f08c5f0cdae1808f9", "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x0"
}
```

### `eth_getTransactionCount`

2026-11-24 13:46:38 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_getTransactionCount", "params": ["0xb90d8162a3a1a2d760699f1f08c5f0cdae1808f9", "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x13"
}
```

### `eth_getCode`

2026-11-24 13:46:39 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_getCode", "params": ["0xc1A0000000000000000000000000000000000001", "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x6080"
}
```

### `eth_estimateGas`

2026-11-24 13:46:40 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_estimateGas", "params": [{"from": "0xb90d8162a3a1a2d760699f1f08c5f0cdae1808f9", "to": "0xc1A0000000000000000000000000000000000001", "data": "0xa9059cbb000000000000000000000000dc379b8bacad1f1df38470626c3350096cf2aef20000000000000000000000000000000000000000000000000000000000000001"}]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x5208"
}
```

### `eth_call name()`

2026-11-24 13:46:41 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_call", "params": [{"to": "0xc1A0000000000000000000000000000000000001", "data": "0x06fdde03"}, "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x0000000000000000000000000000000000000000000000000000000000000020000000000000000000000000000000000000000000000000000000000000000855534420436f696e000000000000000000000000000000000000000000000000"
}
```

### `eth_call symbol()`

2026-11-24 13:46:42 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_call", "params": [{"to": "0xc1A0000000000000000000000000000000000001", "data": "0x95d89b41"}, "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000055553444378000000000000000000000000000000000000000000000000000000"
}
```

### `eth_call decimals()`

2026-11-24 13:46:42 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_call", "params": [{"to": "0xc1A0000000000000000000000000000000000001", "data": "0x313ce567"}, "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x0000000000000000000000000000000000000000000000000000000000000006"
}
```

### `eth_call totalSupply() on DEMO`

2026-11-24 13:46:43 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_call", "params": [{"to": "0xde30000000000000000000000000000000000001", "data": "0x18160ddd"}, "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x00000000000000000000000000000000000000000000003635c9adc5dea00000"
}
```

### `eth_call balanceOf(sender)`

2026-11-24 13:46:44 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_call", "params": [{"to": "0xc1A0000000000000000000000000000000000001", "data": "0x70a08231000000000000000000000000b90d8162a3a1a2d760699f1f08c5f0cdae1808f9"}, "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x00000000000000000000000000000000000000000000000000000000001ca534"
}
```

### `eth_call allowance(owner, spender)`

2026-11-24 13:46:45 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_call", "params": [{"to": "0xc1A0000000000000000000000000000000000001", "data": "0xdd62ed3e000000000000000000000000b90d8162a3a1a2d760699f1f08c5f0cdae1808f9000000000000000000000000dc379b8bacad1f1df38470626c3350096cf2aef2"}, "latest"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x0000000000000000000000000000000000000000000000000000000000000000"
}
```

### `eth_getTransactionByHash`

2026-11-24 13:46:46 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_getTransactionByHash", "params": ["0x0ef8e36bfa026cd9848efc694585de41a40d7054dd17019070fd2b2df35a9afb"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "hash": "0x0ef8e36bfa026cd9848efc694585de41a40d7054dd17019070fd2b2df35a9afb",
    "nonce": "0x0",
    "blockHash": "0x7b2d41c55afb29f625b856dfde73b68b18a7726c27f20c6d886d025ced6d0027",
    "blockNumber": "0x25",
    "transactionIndex": "0x0",
    "from": "0xdc379b8bacad1f1df38470626c3350096cf2aef2",
    "to": "0xc1a0000000000000000000000000000000000001",
    "value": "0x0",
    "gasPrice": "0x0",
    "gas": "0x5208",
    "input": "0xa9059cbb000000000000000000000000b90d8162a3a1a2d760699f1f08c5f0cdae1808f900000000000000000000000000000000000000000000000000000000000186a0",
    "v": null,
    "r": null,
    "s": null,
    "type": "0x2",
    "chainId": "0x44d998be"
  }
}
```

### `eth_getTransactionReceipt`

2026-11-24 13:46:47 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_getTransactionReceipt", "params": ["0x0ef8e36bfa026cd9848efc694585de41a40d7054dd17019070fd2b2df35a9afb"]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "transactionHash": "0x0ef8e36bfa026cd9848efc694585de41a40d7054dd17019070fd2b2df35a9afb",
    "transactionIndex": "0x0",
    "blockHash": "0x7b2d41c55afb29f625b856dfde73b68b18a7726c27f20c6d886d025ced6d0027",
    "blockNumber": "0x25",
    "from": "0xdc379b8bacad1f1df38470626c3350096cf2aef2",
    "to": "0xc1a0000000000000000000000000000000000001",
    "cumulativeGasUsed": "0x5208",
    "gasUsed": "0x5208",
    "contractAddress": null,
    "logs": [
      {
        "address": "0xc1a0000000000000000000000000000000000001",
        "topics": [
          "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
          "0x000000000000000000000000dc379b8bacad1f1df38470626c3350096cf2aef2",
          "0x000000000000000000000000b90d8162a3a1a2d760699f1f08c5f0cdae1808f9"
        ],
        "data": "0x00000000000000000000000000000000000000000000000000000000000186a0",
        "blockNumber": "0x25",
        "transactionHash": "0x0ef8e36bfa026cd9848efc694585de41a40d7054dd17019070fd2b2df35a9afb",
        "transactionIndex": "0x0",
        "blockHash": "0x7b2d41c55afb29f625b856dfde73b68b18a7726c27f20c6d886d025ced6d0027",
        "blockTimestamp": "0x6aeb438d",
        "logIndex": "0x0",
        "removed": false
      }
    ],
    "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000000000000000000000000000008200000800000000200000000000000000000000080000000000000000000000000000000000000000000000000000410000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000002000000000000000000000000000000000000000000000000000000000000000000000000000000000000000400000000000000020004000000000000",
    "status": "0x1",
    "effectiveGasPrice": "0x0",
    "type": "0x2",
    "cantonUpdateId": "1220ad025502905c72a4294fe1f4b4eaf8d2d3e26d6581ac70f293986b7115811aa2"
  }
}
```

### `eth_getLogs`

2026-11-24 13:46:48 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_getLogs", "params": [{"fromBlock": "0x25", "toBlock": "0x25", "address": "0xc1A0000000000000000000000000000000000001", "topics": ["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"]}]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": [
    {
      "address": "0xc1a0000000000000000000000000000000000001",
      "topics": [
        "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef",
        "0x000000000000000000000000dc379b8bacad1f1df38470626c3350096cf2aef2",
        "0x000000000000000000000000b90d8162a3a1a2d760699f1f08c5f0cdae1808f9"
      ],
      "data": "0x00000000000000000000000000000000000000000000000000000000000186a0",
      "blockNumber": "0x25",
      "transactionHash": "0x0ef8e36bfa026cd9848efc694585de41a40d7054dd17019070fd2b2df35a9afb",
      "transactionIndex": "0x0",
      "blockHash": "0x7b2d41c55afb29f625b856dfde73b68b18a7726c27f20c6d886d025ced6d0027",
      "blockTimestamp": "0x6aeb438d",
      "logIndex": "0x0",
      "removed": false
    }
  ]
}
```

### `eth_getBlockByNumber`

2026-11-24 13:46:48 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_getBlockByNumber", "params": ["0x25", false]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "number": "0x25",
    "hash": "0x7b2d41c55afb29f625b856dfde73b68b18a7726c27f20c6d886d025ced6d0027",
    "parentHash": "0xd8b94fda1234a9865701902ee9aca89d22850107014d32f28d11485efade63f2",
    "nonce": "0x0000000000000000",
    "sha3Uncles": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
    "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
    "transactionsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
    "stateRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
    "receiptsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
    "miner": "0x0000000000000000000000000000000000000000",
    "difficulty": "0x0",
    "totalDifficulty": "0x0",
    "extraData": "0x",
    "size": "0x0",
    "gasLimit": "0x5208",
    "gasUsed": "0x0",
    "timestamp": "0x1bc",
    "transactions": [],
    "uncles": [],
    "baseFeePerGas": "0x0"
  }
}
```

### `eth_getBlockByHash`

2026-11-24 13:46:49 UTC

```json
{"jsonrpc": "2.0", "id": 1, "method": "eth_getBlockByHash", "params": ["0x7b2d41c55afb29f625b856dfde73b68b18a7726c27f20c6d886d025ced6d0027", false]}
```

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "number": "0x25",
    "hash": "0x7b2d41c55afb29f625b856dfde73b68b18a7726c27f20c6d886d025ced6d0027",
    "parentHash": "0xd8b94fda1234a9865701902ee9aca89d22850107014d32f28d11485efade63f2",
    "nonce": "0x0000000000000000",
    "sha3Uncles": "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
    "logsBloom": "0x00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000",
    "transactionsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
    "stateRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
    "receiptsRoot": "0x56e81f171bcc55a6ff8345e692c0f86e5b48e01b996cadc001622fb5e363b421",
    "miner": "0x0000000000000000000000000000000000000000",
    "difficulty": "0x0",
    "totalDifficulty": "0x0",
    "extraData": "0x",
    "size": "0x0",
    "gasLimit": "0x5208",
    "gasUsed": "0x0",
    "timestamp": "0x1bc",
    "transactions": [],
    "uncles": [],
    "baseFeePerGas": "0x0"
  }
}
```
