# CIP-0086 Architecture Diagram

## Mermaid Diagram (render at mermaid.live or in GitHub)

```mermaid
flowchart TB
    subgraph User["User Layer"]
        MM["🦊 MetaMask Wallet"]
    end

    subgraph Middleware["CIP-0086 Middleware"]
        API["ERC-20 API Server<br/>localhost:8081/eth<br/><i>JSON-RPC Interface</i>"]
        REL["Relayer<br/><i>Bridge Events Processor</i>"]
        DB[("PostgreSQL<br/>Distributed Indexer<br/><i>Balance Cache</i>")]
    end

    subgraph Canton["Canton Network (DevNet)"]
        CL["Canton Ledger API<br/><i>gRPC + OAuth2</i>"]
        subgraph DAML["DAML Smart Contracts"]
            FM["FingerprintMapping<br/><i>EVM ↔ Canton Identity</i>"]
            H["CIP56Holding<br/><i>Token Balances</i>"]
            NTC["NativeTokenConfig<br/><i>DEMO Token</i>"]
            BC["WayfinderBridgeConfig<br/><i>PROMPT Token</i>"]
            EV["Events<br/><i>Mint/Burn/Transfer</i>"]
        end
    end

    subgraph Ethereum["Ethereum (Sepolia)"]
        SC["Bridge Contract<br/>0x363D...d75"]
        PT["PROMPT Token<br/>0x90cb...048e"]
    end

    MM -->|"eth_sendTransaction<br/>eth_call"| API
    API -->|"Read/Write"| DB
    API -->|"Exercise Choices<br/>(Transfer, Mint)"| CL
    CL --> FM
    CL --> H
    CL --> NTC
    CL --> BC
    CL --> EV
    
    REL -->|"Watch Bridge Events"| SC
    REL -->|"Mint on Deposit<br/>Burn on Withdraw"| CL
    REL -->|"Sync State"| DB
    
    SC -.->|"depositToCanton()"| PT

    style MM fill:#f5a623,stroke:#333,color:#000
    style API fill:#4a90d9,stroke:#333,color:#fff
    style REL fill:#4a90d9,stroke:#333,color:#fff
    style DB fill:#50c878,stroke:#333,color:#000
    style CL fill:#9b59b6,stroke:#333,color:#fff
    style DAML fill:#e8e8e8,stroke:#666
    style Canton fill:#f0e6ff,stroke:#9b59b6
    style Ethereum fill:#fff3e0,stroke:#f5a623
    style Middleware fill:#e3f2fd,stroke:#4a90d9
```

---

## ASCII Version (for terminals/simple display)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           CIP-0086 ARCHITECTURE                             │
└─────────────────────────────────────────────────────────────────────────────┘

    ┌──────────────────┐
    │  🦊 MetaMask     │
    │  (Any EVM Wallet)│
    └────────┬─────────┘
             │ eth_sendTransaction
             │ eth_call, eth_getBalance
             ▼
┌────────────────────────────────────────────────────────────────────────────┐
│                         CIP-0086 MIDDLEWARE                                 │
│  ┌─────────────────────────┐      ┌─────────────────────────┐              │
│  │   ERC-20 API Server     │      │       Relayer           │              │
│  │   localhost:8081/eth    │      │   Bridge Event Watcher  │              │
│  │                         │      │                         │              │
│  │  • JSON-RPC Interface   │      │  • Sepolia → Canton     │              │
│  │  • Transfer Execution   │      │  • Deposit/Withdraw     │              │
│  │  • Balance Queries      │      │  • Event Sync           │              │
│  └───────────┬─────────────┘      └───────────┬─────────────┘              │
│              │                                │                             │
│              └──────────┬─────────────────────┘                             │
│                         │                                                   │
│              ┌──────────▼──────────┐                                        │
│              │     PostgreSQL      │                                        │
│              │  Distributed Indexer│                                        │
│              │  (Balance Cache)    │                                        │
│              └──────────┬──────────┘                                        │
└─────────────────────────┼──────────────────────────────────────────────────┘
                          │ gRPC + OAuth2
                          ▼
┌────────────────────────────────────────────────────────────────────────────┐
│                        CANTON NETWORK (DevNet)                              │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │                      DAML Smart Contracts                            │   │
│  │                                                                      │   │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐   │   │
│  │  │ FingerprintMapping│  │  CIP56Holding   │  │     Events       │   │   │
│  │  │                  │  │                  │  │                  │   │   │
│  │  │ EVM Address ←→   │  │ Token Balances   │  │ MintEvent        │   │   │
│  │  │ Canton Party     │  │ (DEMO & PROMPT)  │  │ BurnEvent        │   │   │
│  │  └──────────────────┘  └──────────────────┘  │ TransferEvent    │   │   │
│  │                                              │ BridgeMintEvent  │   │   │
│  │  ┌──────────────────┐  ┌──────────────────┐  │ BridgeBurnEvent  │   │   │
│  │  │NativeTokenConfig │  │WayfinderBridge   │  └──────────────────┘   │   │
│  │  │                  │  │    Config        │                         │   │
│  │  │ DEMO Token       │  │ PROMPT Token     │                         │   │
│  │  │ (Native Canton)  │  │ (Bridged ERC-20) │                         │   │
│  │  └──────────────────┘  └──────────────────┘                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
└────────────────────────────────────────────────────────────────────────────┘
                          ▲
                          │ Bridge Events
                          │
┌─────────────────────────┴──────────────────────────────────────────────────┐
│                        ETHEREUM (Sepolia Testnet)                           │
│                                                                             │
│  ┌──────────────────────────┐    ┌──────────────────────────┐              │
│  │    Bridge Contract       │    │    PROMPT Token          │              │
│  │  0x363Dd0b55bf74D5b...   │◄───│  0x90cb4f9eF6d682F...    │              │
│  │                          │    │                          │              │
│  │  • depositToCanton()     │    │  • ERC-20 Standard       │              │
│  │  • withdrawToEthereum()  │    │  • 18 decimals           │              │
│  └──────────────────────────┘    └──────────────────────────┘              │
└────────────────────────────────────────────────────────────────────────────┘
```

---

## Data Flow Summary

### Transfer Flow (MetaMask → Canton)
```
1. User initiates transfer in MetaMask
2. MetaMask sends eth_sendTransaction to API Server
3. API Server validates & looks up FingerprintMapping
4. API Server exercises IssuerTransfer on Canton
5. Canton creates TransferEvent + updates Holdings
6. API Server updates PostgreSQL cache
7. MetaMask shows confirmed transaction
```

### Bridge Deposit Flow (Ethereum → Canton)
```
1. User deposits PROMPT to Bridge Contract on Sepolia
2. Relayer watches for Deposit events
3. Relayer calls BridgeMint on Canton
4. Canton creates BridgeMintEvent + CIP56Holding
5. PostgreSQL cache updated
6. User sees PROMPT balance in MetaMask
```

---

## Key Components

| Component | Purpose | Port/Endpoint |
|-----------|---------|---------------|
| API Server | ERC-20 JSON-RPC interface | `localhost:8081/eth` |
| Relayer | Bridge event processor | (background service) |
| PostgreSQL | Balance cache / indexer | `localhost:5432` |
| Canton Ledger | DAML contract execution | `canton-ledger-api-grpc-dev1.chainsafe.dev:80` |
| Sepolia Bridge | Cross-chain deposits | `0x363Dd0b55bf74D5b494B064AA8E8c2Ef5eD58d75` |

---

## Token Addresses

| Token | Type | Address |
|-------|------|---------|
| DEMO | Native Canton | `0xDE30000000000000000000000000000000000001` |
| PROMPT | Bridged ERC-20 | `0x90cb4f9eF6d682F4338f0E360B9C079fbb32048e` |
