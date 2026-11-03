# Get started with MetaMask

This walks through connecting a stock MetaMask wallet to the middleware, registering, and sending a CIP-56
token. No Snap or custom client is needed. For the full API surface see the
[API guide](/middleware/API_DOCUMENTATION).

## 1. Add the network

In MetaMask, open **Networks → Add a custom network**:

| Field | Canton MainNet | Canton DevNet |
|---|---|---|
| RPC URL | `https://middleware-api-prod1.02.chainsafe.dev/eth` | `https://middleware-api-dev1.01.chainsafe.dev/eth` |
| Chain ID | `1155111102` (`0x44d998be`) | `1115511101` (`0x427d593d`) |
| Currency symbol | `CANTON` | `CANTON` |

The currency symbol is only a label. The native balance is always zero and gas price is zero, so no native
token is needed to send. Chain IDs are deployment configuration; confirm them with:

```bash
curl -s -X POST https://middleware-api-prod1.02.chainsafe.dev/eth \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_chainId","params":[]}'
```

## 2. Register

Open the reference dApp for the network ([MainNet](https://evm-middleware.chainsafe.io),
[DevNet](https://dapp-dev1.01.chainsafe.dev)), connect MetaMask and sign the registration message
(EIP-191). This allocates a Canton party for your address. Registration is limited to allowlisted
addresses; contact ChainSafe to be added.

## 3. Import tokens

Add each token as a custom token at its address:

| Network | Token | Address | Decimals |
|---|---|---|---|
| MainNet | USDCx | `0xc1A0000000000000000000000000000000000001` | 6 |
| MainNet | DEMO | `0xde30000000000000000000000000000000000001` | 18 |
| DevNet | USDCx | `0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48` | 6 |
| DevNet | DEMO | `0xde30000000000000000000000000000000000001` | 18 |
| DevNet | PROMPT | `0x90cb4f9eF6d682F4338f0E360B9C079fbb32048e` | 18 |

The live list is served at `GET /tokens` on each API host.

## 4. Send a transfer

Use MetaMask's own **Send** screen: choose the token, paste the recipient's address, enter an amount and
confirm. The recipient must also be registered.

::: warning Pick the right network
If both networks are added, MetaMask's asset picker can list two "USD Coin (USDCx)" entries, one per
network. Check that the confirmation screen shows the network you intend before confirming.
:::

MetaMask lists only transactions sent from an account. Incoming transfers show as a balance change, not in
the recipient's Activity tab.

## 5. Check the result

The transfer runs on Canton asynchronously; MetaMask polls for the receipt. You can check it yourself:

```bash
curl -s -X POST https://middleware-api-prod1.02.chainsafe.dev/eth \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"eth_getTransactionReceipt","params":["<tx hash>"]}'
```

- `status: 0x1` means the transfer settled on Canton; `0x0` means it failed, with the cause in
  `revertReason`.
- `cantonUpdateId` is the Canton update that settled the transfer (from release v0.10.0).
- The receipt's `Transfer` log is also returned by `eth_getLogs` for the token address.
