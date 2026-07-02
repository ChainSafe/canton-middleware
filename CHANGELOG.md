# Changelog

## [0.8.0](https://github.com/ChainSafe/canton-middleware/compare/v0.7.0...v0.8.0) (2026-07-02)


### Features

* **api:** transfer history + outgoing/expired endpoints ([#331](https://github.com/ChainSafe/canton-middleware/issues/331)) ([1cd3f5f](https://github.com/ChainSafe/canton-middleware/commit/1cd3f5f0c6c97563b4194e388410b5cf2aed6879))
* **api:** transfer to external party via party id ([#324](https://github.com/ChainSafe/canton-middleware/issues/324)) ([a0268a0](https://github.com/ChainSafe/canton-middleware/commit/a0268a012c0e728a8d6a1a05ffe7dc4741de456c))
* configurable transfer validity ([#334](https://github.com/ChainSafe/canton-middleware/issues/334)) ([2115077](https://github.com/ChainSafe/canton-middleware/commit/211507756121e6bdcf671742a6be0f4b217a484e))
* **indexer:** generalized transfers table ([#330](https://github.com/ChainSafe/canton-middleware/issues/330)) ([ccb5d76](https://github.com/ChainSafe/canton-middleware/commit/ccb5d76922d3ab946d563f588ac7806327eafb51))
* **relayer:** persist eth scan progress ([#344](https://github.com/ChainSafe/canton-middleware/issues/344)) ([123a84a](https://github.com/ChainSafe/canton-middleware/commit/123a84a433664f6fd58fa3a545b51fe7b89c568d))


### Bug Fixes

* **relayer:** reconnect Canton stream on EOF ([#343](https://github.com/ChainSafe/canton-middleware/issues/343)) ([b20e2dc](https://github.com/ChainSafe/canton-middleware/commit/b20e2dc1690ede0fe0c5394332452cf494699a30))

## [0.7.0](https://github.com/ChainSafe/canton-middleware/compare/v0.6.0...v0.7.0) (2026-06-22)


### Features

* verify and log TokenTransferEvent in ProcessDepositAndMint ([#325](https://github.com/ChainSafe/canton-middleware/issues/325)) ([d94e679](https://github.com/ChainSafe/canton-middleware/commit/d94e679cfffd4c2e665fbf9bf342fdaf8ca194b2))

## [0.6.0](https://github.com/ChainSafe/canton-middleware/compare/v0.5.3...v0.6.0) (2026-06-17)


### Features

* admin whitelist API ([#313](https://github.com/ChainSafe/canton-middleware/issues/313)) ([a798ad4](https://github.com/ChainSafe/canton-middleware/commit/a798ad4b0b48f5b08a951e527e309bbcfed66c09))
* gate eth_sendRawTransaction behind the whitelist ([#305](https://github.com/ChainSafe/canton-middleware/issues/305)) ([e23ed70](https://github.com/ChainSafe/canton-middleware/commit/e23ed703388cbae8bfe235cb4e403763352f49e4))

## [0.5.3](https://github.com/ChainSafe/canton-middleware/compare/v0.5.2...v0.5.3) (2026-06-12)


### Bug Fixes

* **ethrpc:** persist failed transaction receipts as status=0 ([#309](https://github.com/ChainSafe/canton-middleware/issues/309)) ([6dfca79](https://github.com/ChainSafe/canton-middleware/commit/6dfca796f52b8865b444626900898c9e0e5ea971))

## [0.5.2](https://github.com/ChainSafe/canton-middleware/compare/v0.5.1...v0.5.2) (2026-06-09)


### Bug Fixes

* **relayer:** chunk eth log queries and honor configured start block ([#306](https://github.com/ChainSafe/canton-middleware/issues/306)) ([b0dc75f](https://github.com/ChainSafe/canton-middleware/commit/b0dc75f7180faec0f777f7d514ff6094bff84624))

## [0.5.1](https://github.com/ChainSafe/canton-middleware/compare/v0.5.0...v0.5.1) (2026-06-08)


### Bug Fixes

* **ethrpc:** report zero native balance and gas ([#304](https://github.com/ChainSafe/canton-middleware/issues/304)) ([6d552df](https://github.com/ChainSafe/canton-middleware/commit/6d552df23cfb0e93d9830c4e4d4ef1ab59920201))
* run all server goroutines under one errgroup ([#303](https://github.com/ChainSafe/canton-middleware/issues/303)) ([16ffa3e](https://github.com/ChainSafe/canton-middleware/commit/16ffa3e67b57909dd1de80e14b4d3e5b880b20f2))
* serve /health at the configured health_check_url ([#301](https://github.com/ChainSafe/canton-middleware/issues/301)) ([9a5ccbd](https://github.com/ChainSafe/canton-middleware/commit/9a5ccbd023c37b9fb013e68b5936f74a085f981f))
* stop running balance reconciler in the API server ([#300](https://github.com/ChainSafe/canton-middleware/issues/300)) ([f9de530](https://github.com/ChainSafe/canton-middleware/commit/f9de53035004de3c17b9e445052fad83f6fd4379))

## [0.5.0](https://github.com/ChainSafe/canton-middleware/compare/v0.4.2...v0.5.0) (2026-06-08)


### Features

* api and indexer instrumented with metrics ([#221](https://github.com/ChainSafe/canton-middleware/issues/221)) ([6dd9d7d](https://github.com/ChainSafe/canton-middleware/commit/6dd9d7d208d26380913b186cb05cdfef593596f5))
* **ci:** add release-please workflow with reusable docker build ([#288](https://github.com/ChainSafe/canton-middleware/issues/288)) ([1f95bf2](https://github.com/ChainSafe/canton-middleware/commit/1f95bf2434bdb8c7b086ad641c4248233e32b912))
* **ci:** use GraphQL createCommitOnBranch for signed commits ([#286](https://github.com/ChainSafe/canton-middleware/issues/286)) ([38ad918](https://github.com/ChainSafe/canton-middleware/commit/38ad9183624a0d643e703946f1910dc6ca2479ae))
* **metric:** instrument submitter ([#295](https://github.com/ChainSafe/canton-middleware/issues/295)) ([57602c1](https://github.com/ChainSafe/canton-middleware/commit/57602c1b4214bf8f620980763d9416674b883a54))
* **metrics:** eth rpc instrumented ([#297](https://github.com/ChainSafe/canton-middleware/issues/297)) ([9a0044e](https://github.com/ChainSafe/canton-middleware/commit/9a0044e4af2ed25c661b316f6df4ebfa0acde0dd))
* **metrics:** instrument accept worker ([#299](https://github.com/ChainSafe/canton-middleware/issues/299)) ([661d817](https://github.com/ChainSafe/canton-middleware/commit/661d81707ff1f5b12e826df8fa577db587357930))


### Bug Fixes

* **relayer:** minimize eth_getLogs requests limit ([#289](https://github.com/ChainSafe/canton-middleware/issues/289)) ([3731441](https://github.com/ChainSafe/canton-middleware/commit/373144155c6a2bba691de8d61132be5a47d8dad4))
