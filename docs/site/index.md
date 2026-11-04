---
layout: home

hero:
  name: Canton EVM Middleware
  text: CIP-56 tokens from MetaMask and any EVM tool
  tagline: An Ethereum JSON-RPC facade, indexer and Ethereum ↔ Canton bridge for Canton Network. Apache-2.0.
  actions:
    - theme: brand
      text: Get started
      link: /get-started
    - theme: alt
      text: Architecture
      link: /middleware/ARCHITECTURE
    - theme: alt
      text: Grant verification
      link: /grant/

features:
  - title: Ethereum JSON-RPC
    details: Unmodified MetaMask and ethers.js read balances and send ERC-20 transfers; each call becomes a CIP-56 operation on Canton.
    link: /middleware/API_DOCUMENTATION
  - title: Daml contracts
    details: CIP-56 token, bridge core and Wayfinder bridge packages, with the shared fingerprint authorization module.
    link: /contracts/README
  - title: Security and privacy
    details: Identity mapping, authorization and data access under Canton's privacy model.
    link: /middleware/SECURITY_AND_PRIVACY_MODEL
---

## Live endpoints

| Network | JSON-RPC (`/eth`) and REST API | Reference dApp |
|---|---|---|
| Canton MainNet | `https://middleware-api-prod1.02.chainsafe.dev` | [evm-middleware.chainsafe.io](https://evm-middleware.chainsafe.io) |
| Canton DevNet | `https://middleware-api-dev1.01.chainsafe.dev` | [dapp-dev1.01.chainsafe.dev](https://dapp-dev1.01.chainsafe.dev) |

## Repositories

| Repository | Contents |
|---|---|
| [canton-middleware](https://github.com/ChainSafe/canton-middleware) | API server (JSON-RPC, REST), indexer, relayer, Canton SDK, deployment templates |
| [canton-erc20](https://github.com/ChainSafe/canton-erc20) | Daml packages: `common`, `cip56-token`, `bridge-core`, `bridge-wayfinder` |
| [canton-wayfinder](https://github.com/ChainSafe/canton-wayfinder) | Ethereum bridge contracts (Foundry) |
| [canton-snap](https://github.com/ChainSafe/canton-snap) | MetaMask Snap for non-custodial signing, and the reference dApp |

canton-middleware pins canton-erc20 and canton-wayfinder as submodules, so the contract docs on this site match
the commits the middleware release was built against.
