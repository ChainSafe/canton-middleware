# Canton-Ethereum Bridge & ERC-20 API Server

This document describes the Canton-Ethereum Bridge and ERC-20 API Server, and how they deliver the objectives of [CIP-0086: ERC-20 Middleware and Distributed Indexer for Canton Network](https://github.com/global-synchronizer-foundation/cips/commit/9a646bce15ec273bf728f18d18ba685f30f015ad).


## The Canton-Ethereum Bridge

The **Canton-Ethereum Bridge** is a bi-directional relay system that enables tokenized assets to move between EVM-compatible chains (like Ethereum) and the Canton Network. On the EVM side, a `CantonBridge` smart contract holds locked tokens and emits events when users deposit (lock) or withdraw (unlock) assets. On the Canton side, **Daml smart contracts** (following the CIP56 token standard) manage holdings with Canton's native privacy and atomicity guarantees.

A **Relayer** service continuously monitors both chains:

- **Deposits (EVM → Canton):** Detects `DepositToCanton` events, resolves the recipient's Canton party via fingerprint mapping, and mints CIP56 tokens to that party
- **Withdrawals (Canton → EVM):** Detects `WithdrawalEvent` contracts on Canton, burns the user's tokens, and releases the corresponding ERC-20 tokens from the bridge contract on EVM


## The ERC-20 API Server

The **ERC-20 API Server** provides a JSON-RPC interface that exposes Canton token holdings using familiar ERC-20 semantics. Users authenticate by signing messages with their EVM private keys (EIP-191), enabling interaction via existing Web3 wallets like MetaMask without requiring Canton-specific tooling. The server handles:

- **User onboarding** (`user_register`) - Links EVM addresses to Canton parties via fingerprint mappings
- **Balance queries** (`erc20_balanceOf`) - Returns cached balances for low-latency reads
- **Transfers** (`erc20_transfer`) - Executes Canton-side token transfers between registered users
- **Metadata** (`erc20_name`, `erc20_symbol`, `erc20_totalSupply`, `erc20_decimals`) - Standard ERC-20 queries

A PostgreSQL cache maintains synchronized balance data through real time updates from the relayer and periodic reconciliation against Canton's ledger state.


## Delivering CIP-0086: ERC-20 Middleware and Distributed Indexer for Canton Network

This solution directly implements the objectives of CIP-0086:

### ✅ ERC-20 Middleware

- Provides a standard JSON-RPC interface (`erc20_*` methods) compatible with existing Web3 tooling
- Enables EVM wallet authentication users interact with Canton using their existing Ethereum keys
- Abstracts Canton's Daml contract model behind familiar ERC-20 semantics

### ✅ Distributed Indexer

- PostgreSQL cache stores aggregated balance and supply data for fast queries
- Real-time synchronization via relayer event processing (deposits/withdrawals)
- Periodic reconciliation ensures cache consistency with Canton's authoritative ledger state

### ✅ Cross-Chain Interoperability

- Bi-directional bridge enables assets to flow between EVM chains and Canton
- Fingerprint-based identity mapping links EVM addresses to Canton parties
- Atomic deposit/withdrawal flows with onchain event correlation


## Preserving Canton Privacy Principles: Issuer-Centric Architecture

A critical design choice we made is that **all operations occur within the participant node's scope**. Specifically, within the **issuer's (bridge operator's) visibility boundary**. This preserves Canton's core privacy model.

### How It Works

- The **Bridge Issuer** is the only Canton party that sees all bridge related contracts (FingerprintMappings, PendingDeposits, WithdrawalEvents, CIP56Holdings)
- Individual **User Parties** are allocated by the issuer and only have observer rights on their own holdings
- The relayer and API server operate **as the issuer**, interacting with Canton through the issuer's participant node

### Why This Matters

| Traditional Blockchain | Canton Issuer-Centric Model |
|------------------------|----------------------------|
| All transactions visible to all nodes | Holdings only visible to owner + issuer |
| Global state = privacy leak | Data partitioned by participant visibility |
| Public indexers see everything | Indexer scoped to issuer's contracts only |

### Privacy Guarantees Preserved

1. **No global visibility** - User balances and transfer activity are NOT broadcast network-wide
2. **Issuer acts as custodian** - Only the bridge operator's node needs to process/index this data
3. **User parties are isolated** - Users cannot see each other's holdings; only the issuer can aggregate
4. **Regulatory compliance** - The issuer can implement KYC/AML controls within their scope without exposing data to other Canton participants

This architecture means the "distributed indexer" from CIP-0086 operates within a **single trust boundary** (the issuer's participant node), rather than requiring network wide indexing that would violate Canton's privacy by design principles. Users get the convenience of ERC-20 style queries while the issuer maintains full control over data visibility and is exactly as Canton intends for institutional grade token infrastructure.


## Business Case: Multi-Token Bridge Operator

A single participant node (issuer) can onboard **multiple EVM token bridge connections**, enabling users within that issuer's scope to hold and interact with multiple bridged assets simultaneously.

### Multi-Token Architecture

```
                    ┌─────────────────────────────────────────────────┐
                    │           Issuer's Participant Node             │
                    │                                                 │
   EVM Side         │    Canton Side                                  │
  ┌─────────┐       │   ┌──────────────────────────────────────────┐  │
  │ PROMPT  │◄──────┼──►│  CIP56Manager (PROMPT)                   │  │
  │ Bridge  │       │   │  └─► User1 Holdings                      │  │
  └─────────┘       │   │  └─► User2 Holdings                      │  │
                    │   ├──────────────────────────────────────────┤  │
  ┌─────────┐       │   │  CIP56Manager (USDC)                     │  │
  │  USDC   │◄──────┼──►│  └─► User1 Holdings                      │  │
  │ Bridge  │       │   │  └─► User2 Holdings                      │  │
  └─────────┘       │   ├──────────────────────────────────────────┤  │
                    │   │  CIP56Manager (WETH)                     │  │
  ┌─────────┐       │   │  └─► User1 Holdings                      │  │
  │  WETH   │◄──────┼──►│  └─► User3 Holdings                      │  │
  │ Bridge  │       │   └──────────────────────────────────────────┘  │
  └─────────┘       │                                                 │
                    │   ┌─────────────┐    ┌─────────────────────┐    │
                    │   │  Relayer    │    │   API Server        │    │
                    │   │ (all tokens)│    │  (multi-token RPC)  │    │
                    │   └─────────────┘    └─────────────────────┘    │
                    └─────────────────────────────────────────────────┘
```

### Capabilities

| Capability | Description |
|------------|-------------|
| **Multi-asset portfolios** | Users can hold PROMPT, USDC, WETH, and other bridged tokens simultaneously in their Canton party |
| **Cross-token transfers** | Users can send different token types to different recipients in separate transactions |
| **Unified identity** | Single fingerprint/party maps to all token holdings. One registration covers all bridged assets |
| **Atomic multi-token operations** | Canton's Daml can compose transactions across token types (e.g., swap USDC for PROMPT atomically) |
| **Single API endpoint** | Users query balances and execute transfers through one API server supporting all bridged tokens |

### Privacy Scope Preserved

All operations remain **within the issuer's visibility boundary**:

- The issuer sees all tokens, all users, all transfers
- Each user sees only their own holdings across all token types
- **Other Canton participants see nothing**. There is no cross-issuer data leakage

### Use Cases

This architecture positions a single issuer as an **institutional custodian** or **multi-asset bridge operator**:

| Operator Type | Value Proposition |
|---------------|-------------------|
| **Crypto Exchange** | Offer Canton-based custody with ERC-20 compatibility for multiple assets, enabling private OTC trading |
| **Asset Manager** | Manage client portfolios across bridged tokens with Canton's privacy guarantees |
| **Payment Processor** | Support multiple stablecoins (USDC, USDT, DAI) for merchant settlements on Canton |
| **DeFi Protocol** | Bridge liquidity pools to Canton for privacy-preserving yield strategies |

Users are onboarded once and gain access to an entire ecosystem of bridged EVM tokens, all with Canton's privacy guarantees intact.


## References

- [CIP-0086: ERC-20 Middleware and Distributed Indexer for Canton Network](https://github.com/global-synchronizer-foundation/cips/commit/9a646bce15ec273bf728f18d18ba685f30f015ad)
- [Global Synchronizer Foundation CIPs Repository](https://github.com/global-synchronizer-foundation/cips)

