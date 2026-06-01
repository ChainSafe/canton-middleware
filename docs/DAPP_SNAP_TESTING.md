# Testing with the Wayfinder dApp and MetaMask Snap

This guide covers the steps to run the Canton Middleware locally and connect it to the dApp and MetaMask Snap 
for end-to-end testing.

> **Full testing guide:** Complete setup instructions for the Snap and dApp side are maintained in the Canton Snap repository:
> [github.com/ChainSafe/canton-snap — Testing with Middleware](https://github.com/ChainSafe/canton-snap/blob/main/docs/testing-with-middleware.md)

---

## 1. Start the Middleware

```bash
make docker-up
```

This builds and starts the full local stack: Canton node, API server, indexer, relayer, Anvil, and all supporting services. Wait for all containers to report healthy before proceeding.

---

## 2. Whitelist an Address

The API server requires addresses to be whitelisted before registration. Run the interactive utility and enter the EVM address you want to allow:

```bash
go run scripts/utils/whitelist.go
```

To whitelist non-interactively:

```bash
go run scripts/utils/whitelist.go -address 0xYourAddress -note "tester"
```

---

## 3. Fund an Address

Once the address is registered through the dApp (or Snap), mint DEMO and PROMPT tokens to it:

```bash
go run scripts/utils/fund-wallet.go
```

The script connects to the running stack automatically, prompts for the EVM address and amount, and mints to the corresponding Canton party.

---

## 4. Verify on the dApp

With tokens minted, open the dApp and connect MetaMask using the local network:

| Setting | Value |
|---------|-------|
| RPC URL | `http://localhost:8081/eth` |
| Chain ID | `31337` |

Balances and transfers should be visible immediately. Use the Snap to sign Canton transfers and confirm the balance updates reflect on both the dApp and the indexer.

---

## MetaMask Configuration

| Network Name | RPC URL | Chain ID | Currency Symbol |
|---|---|---|---|
| Canton Local | `http://localhost:8081/eth` | 31337 | ETH |
