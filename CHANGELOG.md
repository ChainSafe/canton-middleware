# Changelog

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
