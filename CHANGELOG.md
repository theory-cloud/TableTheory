# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.3.0-rc.2](https://github.com/theory-cloud/tabletheory/compare/v1.2.1-rc.2...v1.3.0-rc.2) (2026-01-18)


### Features

* **hgm:** add HGM infra rubric + M0/M1 artifacts ([061d4ca](https://github.com/theory-cloud/tabletheory/commit/061d4ca9f19bfdd109520eaf3bbf21631bfa2cfa))
* **hgm:** bring rubric to legacy parity ([11ebc67](https://github.com/theory-cloud/tabletheory/commit/11ebc67dac14a0b0dfb0f5362f50c2d23f5d8a92))
* **hgm:** implement COM-6 logging/ops standards gate ([fad15ce](https://github.com/theory-cloud/tabletheory/commit/fad15ceca34f60bbf3e02bf234e8490bd1d634e3))
* **hgm:** implement MAI-2 maintainability roadmap gate ([83340a7](https://github.com/theory-cloud/tabletheory/commit/83340a71d603425e0db6c721f2c93eddd9d9b0ec))
* **hgm:** sunset legacy rubric runner via HGM parity ([1fbc828](https://github.com/theory-cloud/tabletheory/commit/1fbc82807311eb4e7cc34a230f709ccb80c8861b))


### Bug Fixes

* **hgm:** unblock SEC-1 by verifying tool pins via go metadata ([30ed3a7](https://github.com/theory-cloud/tabletheory/commit/30ed3a790b5b4cac0fdce90903681ac307f5ae7c))

## [1.2.1-rc.2](https://github.com/theory-cloud/tabletheory/compare/v1.2.1-rc.1...v1.2.1-rc.2) (2026-01-17)


### Bug Fixes

* **release:** sync premain stable manifest with main ([c5eed29](https://github.com/theory-cloud/tabletheory/commit/c5eed2988d88ae9e68d13db7a4417f3cd11ad7b6))

## [1.2.1-rc.1](https://github.com/theory-cloud/tabletheory/compare/v1.2.1-rc...v1.2.1-rc.1) (2026-01-17)


### Bug Fixes

* **release:** package ts/py assets reliably ([9554dc5](https://github.com/theory-cloud/tabletheory/commit/9554dc5fe85344669b8a7bcdc7f27a8b5751118a))
* **release:** repair prerelease asset packaging ([6e96be8](https://github.com/theory-cloud/tabletheory/commit/6e96be8307dd56be17a80cff56eaff008d2ea840))

## [1.2.1-rc](https://github.com/theory-cloud/tabletheory/compare/v1.2.0...v1.2.1-rc) (2026-01-17)


### Bug Fixes

* **release:** prevent premain version drift ([d0fea25](https://github.com/theory-cloud/tabletheory/commit/d0fea25ef49fd58f0c488f071071b8a82845dae6))
* **release:** resync premain after v1.2.0 ([216b17a](https://github.com/theory-cloud/tabletheory/commit/216b17a7e3ab970915f5d09d9fe99f4152e401e4))
* **security:** resolve CodeQL alert + toolchain/deps ([9159519](https://github.com/theory-cloud/tabletheory/commit/9159519ff22490377ea406ad5fa6e2cafe5b203d))
* **security:** resolve CodeQL alert and toolchain drift ([8687fa7](https://github.com/theory-cloud/tabletheory/commit/8687fa73185681be0509997fbcf3422926ac016f))

## [1.2.0](https://github.com/theory-cloud/tabletheory/compare/v1.1.0...v1.2.0) (2026-01-17)


### Features

* **cdk:** add multilang demo stack (VP-5) ([d68bf52](https://github.com/theory-cloud/tabletheory/commit/d68bf52d7a42f180a53834dfe97a403a4507058b))
* **cdk:** exercise enc/tx/batch in demo ([0b05d82](https://github.com/theory-cloud/tabletheory/commit/0b05d828669fab2346d5281b8c70c5d8b70ae95b))
* **conversion:** add json + custom converters (FC-6) ([6142c70](https://github.com/theory-cloud/tabletheory/commit/6142c70f9fa9d2ba91283b66206d09a67a96c312))
* **dms:** implement DMS-first workflow (FC-0) ([38c1b72](https://github.com/theory-cloud/tabletheory/commit/38c1b7264943f513686100ad63bee50e1eec3b24))
* **py:** batch + transactions (PY-4) ([12aca8d](https://github.com/theory-cloud/tabletheory/commit/12aca8d712d52dcb0733ce50409eaddef7a17f35))
* **py:** core CRUD operations (PY-2) ([9065f92](https://github.com/theory-cloud/tabletheory/commit/9065f924023c48cd2c77bcab06da7728b0600638))
* **py:** docs + examples (PY-7) ([b4ddcad](https://github.com/theory-cloud/tabletheory/commit/b4ddcadee977192a8be05a94384828277723ccb5))
* **py:** encrypted semantics (PY-6) ([7fe02dc](https://github.com/theory-cloud/tabletheory/commit/7fe02dc0c30bab60b7470481e6015cea57c6f076))
* **py:** model definition API (PY-1) ([c8e9077](https://github.com/theory-cloud/tabletheory/commit/c8e90772904ac8c69e0b121b7a9da2703820fc1a))
* **py:** query/scan operations (PY-3) ([3023af3](https://github.com/theory-cloud/tabletheory/commit/3023af37d0d43b1244ab109c370bb56577fbf6cb))
* **py:** scaffold tabletheory-py (PY-0) ([46221b8](https://github.com/theory-cloud/tabletheory/commit/46221b878f5772d1ce2174f5d785035bb28dd225))
* **py:** streams + events unmarshalling (PY-5) ([12e8d13](https://github.com/theory-cloud/tabletheory/commit/12e8d13091dff7ea50ee6f1747947d3cc2c7768e))
* **query:** aggregates + optimizer (FC-8) ([e7f79dc](https://github.com/theory-cloud/tabletheory/commit/e7f79dc54a1ac54ce33be51c8e1a8126c206d79c))
* **query:** filters, retries, parallel scan (FC-2) ([e56466e](https://github.com/theory-cloud/tabletheory/commit/e56466eb9db266920a677e9930a2669ecc887ef2))
* **runtime:** lambda + multi-account helpers (FC-4) ([3dee078](https://github.com/theory-cloud/tabletheory/commit/3dee078242df9fbcb0fbedc63c4b296d67332e47))
* **schema:** add TS/Py table helpers (FC-1) ([31365c8](https://github.com/theory-cloud/tabletheory/commit/31365c8b41cd9a63ad6408578427ab52bb0ede05))
* **security:** add validation + protection helpers (FC-5) ([c64b04b](https://github.com/theory-cloud/tabletheory/commit/c64b04b8699dfb244eba8c5fbb60b135e4610d9b))
* **testkit:** add python testkit helpers (FC-7) ([b2b5b0e](https://github.com/theory-cloud/tabletheory/commit/b2b5b0eca064974072428b9bb6324cae2e6b8aa9))
* **testkit:** public mocks + inject now/rand (VP-4) ([105de2e](https://github.com/theory-cloud/tabletheory/commit/105de2e729f78629594898eb5be87dc9d3dfc2dc))
* **ts:** add KMS encryption provider ([c104bc2](https://github.com/theory-cloud/tabletheory/commit/c104bc280c6fa3ecc323981360241e49ca3546ec))
* **update:** add UpdateBuilder parity (FC-3) ([8594a5d](https://github.com/theory-cloud/tabletheory/commit/8594a5d0b6af5bf6943fa58393b0b0753690ede4))


### Bug Fixes

* **cdk:** track multilang demo entrypoint ([94b715f](https://github.com/theory-cloud/tabletheory/commit/94b715f7d8e49884b278b4e179714c525416c1c0))
* **py:** align encrypted payload encoding ([db0b8ae](https://github.com/theory-cloud/tabletheory/commit/db0b8aec304a2b3a72af8f8bce857d2d7b433668))

## [1.1.0-rc.5](https://github.com/theory-cloud/tabletheory/compare/v1.1.0-rc.4...v1.1.0-rc.5) (2026-01-17)


### Bug Fixes

* **security:** resolve CodeQL alert + toolchain/deps ([9159519](https://github.com/theory-cloud/tabletheory/commit/9159519ff22490377ea406ad5fa6e2cafe5b203d))
* **security:** resolve CodeQL alert and toolchain drift ([8687fa7](https://github.com/theory-cloud/tabletheory/commit/8687fa73185681be0509997fbcf3422926ac016f))

## [1.1.0-rc.4](https://github.com/theory-cloud/tabletheory/compare/v1.1.0-rc.3...v1.1.0-rc.4) (2026-01-17)


### Features

* **cdk:** add multilang demo stack (VP-5) ([d68bf52](https://github.com/theory-cloud/tabletheory/commit/d68bf52d7a42f180a53834dfe97a403a4507058b))
* **cdk:** exercise enc/tx/batch in demo ([0b05d82](https://github.com/theory-cloud/tabletheory/commit/0b05d828669fab2346d5281b8c70c5d8b70ae95b))
* **conversion:** add json + custom converters (FC-6) ([6142c70](https://github.com/theory-cloud/tabletheory/commit/6142c70f9fa9d2ba91283b66206d09a67a96c312))
* **dms:** implement DMS-first workflow (FC-0) ([38c1b72](https://github.com/theory-cloud/tabletheory/commit/38c1b7264943f513686100ad63bee50e1eec3b24))
* **py:** batch + transactions (PY-4) ([12aca8d](https://github.com/theory-cloud/tabletheory/commit/12aca8d712d52dcb0733ce50409eaddef7a17f35))
* **py:** core CRUD operations (PY-2) ([9065f92](https://github.com/theory-cloud/tabletheory/commit/9065f924023c48cd2c77bcab06da7728b0600638))
* **py:** docs + examples (PY-7) ([b4ddcad](https://github.com/theory-cloud/tabletheory/commit/b4ddcadee977192a8be05a94384828277723ccb5))
* **py:** encrypted semantics (PY-6) ([7fe02dc](https://github.com/theory-cloud/tabletheory/commit/7fe02dc0c30bab60b7470481e6015cea57c6f076))
* **py:** model definition API (PY-1) ([c8e9077](https://github.com/theory-cloud/tabletheory/commit/c8e90772904ac8c69e0b121b7a9da2703820fc1a))
* **py:** query/scan operations (PY-3) ([3023af3](https://github.com/theory-cloud/tabletheory/commit/3023af37d0d43b1244ab109c370bb56577fbf6cb))
* **py:** scaffold tabletheory-py (PY-0) ([46221b8](https://github.com/theory-cloud/tabletheory/commit/46221b878f5772d1ce2174f5d785035bb28dd225))
* **py:** streams + events unmarshalling (PY-5) ([12e8d13](https://github.com/theory-cloud/tabletheory/commit/12e8d13091dff7ea50ee6f1747947d3cc2c7768e))
* **query:** aggregates + optimizer (FC-8) ([e7f79dc](https://github.com/theory-cloud/tabletheory/commit/e7f79dc54a1ac54ce33be51c8e1a8126c206d79c))
* **query:** filters, retries, parallel scan (FC-2) ([e56466e](https://github.com/theory-cloud/tabletheory/commit/e56466eb9db266920a677e9930a2669ecc887ef2))
* **runtime:** lambda + multi-account helpers (FC-4) ([3dee078](https://github.com/theory-cloud/tabletheory/commit/3dee078242df9fbcb0fbedc63c4b296d67332e47))
* **schema:** add TS/Py table helpers (FC-1) ([31365c8](https://github.com/theory-cloud/tabletheory/commit/31365c8b41cd9a63ad6408578427ab52bb0ede05))
* **security:** add validation + protection helpers (FC-5) ([c64b04b](https://github.com/theory-cloud/tabletheory/commit/c64b04b8699dfb244eba8c5fbb60b135e4610d9b))
* **testkit:** add python testkit helpers (FC-7) ([b2b5b0e](https://github.com/theory-cloud/tabletheory/commit/b2b5b0eca064974072428b9bb6324cae2e6b8aa9))
* **testkit:** public mocks + inject now/rand (VP-4) ([105de2e](https://github.com/theory-cloud/tabletheory/commit/105de2e729f78629594898eb5be87dc9d3dfc2dc))
* **ts:** add KMS encryption provider ([c104bc2](https://github.com/theory-cloud/tabletheory/commit/c104bc280c6fa3ecc323981360241e49ca3546ec))
* **update:** add UpdateBuilder parity (FC-3) ([8594a5d](https://github.com/theory-cloud/tabletheory/commit/8594a5d0b6af5bf6943fa58393b0b0753690ede4))


### Bug Fixes

* **cdk:** track multilang demo entrypoint ([94b715f](https://github.com/theory-cloud/tabletheory/commit/94b715f7d8e49884b278b4e179714c525416c1c0))
* **py:** align encrypted payload encoding ([db0b8ae](https://github.com/theory-cloud/tabletheory/commit/db0b8aec304a2b3a72af8f8bce857d2d7b433668))
* **rubric:** handle prerelease alignment on main ([20cb88d](https://github.com/theory-cloud/tabletheory/commit/20cb88db243817c872d915170df6b13e4e36034f))

## [1.1.0](https://github.com/theory-cloud/tabletheory/compare/v1.0.37...v1.1.0) (2026-01-16)


### Features

* decrypt encrypted tag fields on read ([af82905](https://github.com/theory-cloud/tabletheory/commit/af82905cce303088cc0a2481cfad9c11d5e5e42c))
* fail closed on encrypted fields without KMSKeyARN ([6b88a8d](https://github.com/theory-cloud/tabletheory/commit/6b88a8d74cb463ee9c4ae1ed9a96bb883cd28be2))
* implement encrypted tag write-time encryption ([301c0e0](https://github.com/theory-cloud/tabletheory/commit/301c0e01a17b45e8246466fb1bd2393d21ef091d))
* make marshaling safe by default ([24cf465](https://github.com/theory-cloud/tabletheory/commit/24cf465021cacac366edd75446b95a0f519dfbff))
* **rubric:** enforce TypeScript quality gates ([808c275](https://github.com/theory-cloud/tabletheory/commit/808c275f189f669a42778e41203ed718723341ee))
* **ts:** TS-0 tooling and package skeleton ([6614aa9](https://github.com/theory-cloud/tabletheory/commit/6614aa93b37d56ffd6f57bcd1699203faf09f7b5))
* **ts:** TS-1 model schema and validation ([d7932f9](https://github.com/theory-cloud/tabletheory/commit/d7932f9678b1889a550ce9dd900c8e3b6b6f9c46))
* **ts:** TS-2 CRUD operations (P0) ([da0f25b](https://github.com/theory-cloud/tabletheory/commit/da0f25bb1e4d646b31fde18089a7d10fc3b357ac))
* **ts:** TS-3 query, scan, and cursor ([dafa93c](https://github.com/theory-cloud/tabletheory/commit/dafa93cf42b64143ec6398f4eecfd21ee7220bea))
* **ts:** TS-4 batch and transactions (P2) ([dc3df67](https://github.com/theory-cloud/tabletheory/commit/dc3df67ba23141f80241bf71cad27c41288b27fe))
* **ts:** TS-5 streams unmarshalling ([46fead1](https://github.com/theory-cloud/tabletheory/commit/46fead1e673d3b0113b38acf04bd8db7aced46d5))
* **ts:** TS-6 encrypted field semantics ([b16e03c](https://github.com/theory-cloud/tabletheory/commit/b16e03c63f4376388e80cd3204af9560047f8e34))
* **ts:** TS-7 docs and examples ([1fa4ecd](https://github.com/theory-cloud/tabletheory/commit/1fa4ecd44621ec3f4370e69ee70cf6f00c25155f))
* TypeScript TableTheory (Phase 1) + DMS v0.1 draft ([62513d4](https://github.com/theory-cloud/tabletheory/commit/62513d41e0fd1fdd2fc98ce068a5d40978faa263))


### Bug Fixes

* **ci:** install golangci-lint v2.5.0 ([d9b270f](https://github.com/theory-cloud/tabletheory/commit/d9b270fa1d66dc34dd2a7b1e9346043becdaebd8))
* **ci:** install ripgrep for rubric scripts ([807df03](https://github.com/theory-cloud/tabletheory/commit/807df0392d6a60b38861d40bd7e1b8f3898a7e81))
* **ci:** pin golangci-lint to v1.64.8 ([e5cecd5](https://github.com/theory-cloud/tabletheory/commit/e5cecd56c8563e1bd45e47560547a5375cafcd58))
* **ci:** pin release-please action by SHA ([2d6a7dc](https://github.com/theory-cloud/tabletheory/commit/2d6a7dcdd4cd77c178526915ce2d32d339ff6629))
* **ci:** pin release-please action SHA ([719d667](https://github.com/theory-cloud/tabletheory/commit/719d66793575bbf331895c82d01f0bc547a9737b))
* consistent omitempty behavior for Update() ([aa86849](https://github.com/theory-cloud/tabletheory/commit/aa868496c39bdd8610b99dd6a6f4d62b16b9de49))
* **encryption:** harden encrypted tag semantics ([2d591a0](https://github.com/theory-cloud/tabletheory/commit/2d591a0ad5a4dc08778e62e62352745c7f849553))
* enforce network hygiene defaults ([a2dcb8b](https://github.com/theory-cloud/tabletheory/commit/a2dcb8bb35ec981df869ab2b5202b4e1776b28f0))
* ensure BatchGetBuilder uses model metadata ([4171c33](https://github.com/theory-cloud/tabletheory/commit/4171c3304524a66102e093b820d88b0a9e59683e))
* **expr:** harden list index update expressions ([95d22b7](https://github.com/theory-cloud/tabletheory/commit/95d22b7235807da81b26c0212467c6f5b2d99ae8))
* handle version fields across numeric types ([aacb69d](https://github.com/theory-cloud/tabletheory/commit/aacb69d0d7a9fa0ba749a3bb7e11da223f72d4df))
* only skip zero values in nested struct marshaling when omitempty is set ([f0d31ac](https://github.com/theory-cloud/tabletheory/commit/f0d31acf1cddb066d67c7713470d652bb260d194))
* only skip zero values in nested struct marshaling when omitempty… ([86a3b43](https://github.com/theory-cloud/tabletheory/commit/86a3b430190dc6852a4d2c9f9d2e73087c441ed1))
* **query:** align UnmarshalItem with TableTheory tags ([b2d580b](https://github.com/theory-cloud/tabletheory/commit/b2d580b9b0a94961c0fde67a9d5b4d1a90a82f71))
* **query:** correct ScanAllSegments destination type ([685ddc3](https://github.com/theory-cloud/tabletheory/commit/685ddc3f1d4eda52c7d78cd3c77efaa80fc2684e))
* **release:** include TypeScript in release-please versioning ([7367964](https://github.com/theory-cloud/tabletheory/commit/7367964aebeb88f87d02c5e21c68792a7d86bb66))
* **release:** include TypeScript in release-please versioning ([19791d8](https://github.com/theory-cloud/tabletheory/commit/19791d89e7728741908b6c31a4b66f0f3a3209a2))
* remove panics from expression builder ([dfd5445](https://github.com/theory-cloud/tabletheory/commit/dfd5445ba537190d82e86b1dfa5eea439f773db5))
* respect omitempty for empty collections in Update ([9fb7f1f](https://github.com/theory-cloud/tabletheory/commit/9fb7f1f8ce8cbd9999271b7c2183628a8dbd6fff))
* **rubric:** make rubric green ([ef074bc](https://github.com/theory-cloud/tabletheory/commit/ef074bca8cbca23498dcf4a3dcf8127785ff28a4))
* **testing:** correct getTypeString for AnythingOfType ([7104519](https://github.com/theory-cloud/tabletheory/commit/710451944e823d09e02c9b441f892acd71999fcc))

## [1.1.0-rc.3](https://github.com/theory-cloud/tabletheory/compare/v1.1.0-rc.2...v1.1.0-rc.3) (2026-01-16)


### Bug Fixes

* **release:** include TypeScript in release-please versioning ([7367964](https://github.com/theory-cloud/tabletheory/commit/7367964aebeb88f87d02c5e21c68792a7d86bb66))
* **release:** include TypeScript in release-please versioning ([19791d8](https://github.com/theory-cloud/tabletheory/commit/19791d89e7728741908b6c31a4b66f0f3a3209a2))

## [1.1.0-rc.2](https://github.com/theory-cloud/tabletheory/compare/v1.1.0-rc.1...v1.1.0-rc.2) (2026-01-16)


### Features

* **rubric:** enforce TypeScript quality gates ([808c275](https://github.com/theory-cloud/tabletheory/commit/808c275f189f669a42778e41203ed718723341ee))
* **ts:** TS-0 tooling and package skeleton ([6614aa9](https://github.com/theory-cloud/tabletheory/commit/6614aa93b37d56ffd6f57bcd1699203faf09f7b5))
* **ts:** TS-1 model schema and validation ([d7932f9](https://github.com/theory-cloud/tabletheory/commit/d7932f9678b1889a550ce9dd900c8e3b6b6f9c46))
* **ts:** TS-2 CRUD operations (P0) ([da0f25b](https://github.com/theory-cloud/tabletheory/commit/da0f25bb1e4d646b31fde18089a7d10fc3b357ac))
* **ts:** TS-3 query, scan, and cursor ([dafa93c](https://github.com/theory-cloud/tabletheory/commit/dafa93cf42b64143ec6398f4eecfd21ee7220bea))
* **ts:** TS-4 batch and transactions (P2) ([dc3df67](https://github.com/theory-cloud/tabletheory/commit/dc3df67ba23141f80241bf71cad27c41288b27fe))
* **ts:** TS-5 streams unmarshalling ([46fead1](https://github.com/theory-cloud/tabletheory/commit/46fead1e673d3b0113b38acf04bd8db7aced46d5))
* **ts:** TS-6 encrypted field semantics ([b16e03c](https://github.com/theory-cloud/tabletheory/commit/b16e03c63f4376388e80cd3204af9560047f8e34))
* **ts:** TS-7 docs and examples ([1fa4ecd](https://github.com/theory-cloud/tabletheory/commit/1fa4ecd44621ec3f4370e69ee70cf6f00c25155f))
* TypeScript TableTheory (Phase 1) + DMS v0.1 draft ([62513d4](https://github.com/theory-cloud/tabletheory/commit/62513d41e0fd1fdd2fc98ce068a5d40978faa263))

## [1.1.0-rc.1](https://github.com/theory-cloud/tabletheory/compare/v1.1.0-rc...v1.1.0-rc.1) (2026-01-11)


### Bug Fixes

* **ci:** pin release-please action by SHA ([2d6a7dc](https://github.com/theory-cloud/tabletheory/commit/2d6a7dcdd4cd77c178526915ce2d32d339ff6629))
* **ci:** pin release-please action SHA ([719d667](https://github.com/theory-cloud/tabletheory/commit/719d66793575bbf331895c82d01f0bc547a9737b))

## [1.1.0-rc](https://github.com/theory-cloud/tabletheory/compare/v1.0.37...v1.1.0-rc) (2026-01-11)


### Features

* decrypt encrypted tag fields on read ([af82905](https://github.com/theory-cloud/tabletheory/commit/af82905cce303088cc0a2481cfad9c11d5e5e42c))
* fail closed on encrypted fields without KMSKeyARN ([6b88a8d](https://github.com/theory-cloud/tabletheory/commit/6b88a8d74cb463ee9c4ae1ed9a96bb883cd28be2))
* implement encrypted tag write-time encryption ([301c0e0](https://github.com/theory-cloud/tabletheory/commit/301c0e01a17b45e8246466fb1bd2393d21ef091d))
* make marshaling safe by default ([24cf465](https://github.com/theory-cloud/tabletheory/commit/24cf465021cacac366edd75446b95a0f519dfbff))


### Bug Fixes

* **ci:** install golangci-lint v2.5.0 ([d9b270f](https://github.com/theory-cloud/tabletheory/commit/d9b270fa1d66dc34dd2a7b1e9346043becdaebd8))
* **ci:** install ripgrep for rubric scripts ([807df03](https://github.com/theory-cloud/tabletheory/commit/807df0392d6a60b38861d40bd7e1b8f3898a7e81))
* **ci:** pin golangci-lint to v1.64.8 ([e5cecd5](https://github.com/theory-cloud/tabletheory/commit/e5cecd56c8563e1bd45e47560547a5375cafcd58))
* consistent omitempty behavior for Update() ([aa86849](https://github.com/theory-cloud/tabletheory/commit/aa868496c39bdd8610b99dd6a6f4d62b16b9de49))
* **encryption:** harden encrypted tag semantics ([2d591a0](https://github.com/theory-cloud/tabletheory/commit/2d591a0ad5a4dc08778e62e62352745c7f849553))
* enforce network hygiene defaults ([a2dcb8b](https://github.com/theory-cloud/tabletheory/commit/a2dcb8bb35ec981df869ab2b5202b4e1776b28f0))
* ensure BatchGetBuilder uses model metadata ([4171c33](https://github.com/theory-cloud/tabletheory/commit/4171c3304524a66102e093b820d88b0a9e59683e))
* **expr:** harden list index update expressions ([95d22b7](https://github.com/theory-cloud/tabletheory/commit/95d22b7235807da81b26c0212467c6f5b2d99ae8))
* handle version fields across numeric types ([aacb69d](https://github.com/theory-cloud/tabletheory/commit/aacb69d0d7a9fa0ba749a3bb7e11da223f72d4df))
* only skip zero values in nested struct marshaling when omitempty is set ([f0d31ac](https://github.com/theory-cloud/tabletheory/commit/f0d31acf1cddb066d67c7713470d652bb260d194))
* only skip zero values in nested struct marshaling when omitempty… ([86a3b43](https://github.com/theory-cloud/tabletheory/commit/86a3b430190dc6852a4d2c9f9d2e73087c441ed1))
* **query:** align UnmarshalItem with TableTheory tags ([b2d580b](https://github.com/theory-cloud/tabletheory/commit/b2d580b9b0a94961c0fde67a9d5b4d1a90a82f71))
* **query:** correct ScanAllSegments destination type ([685ddc3](https://github.com/theory-cloud/tabletheory/commit/685ddc3f1d4eda52c7d78cd3c77efaa80fc2684e))
* remove panics from expression builder ([dfd5445](https://github.com/theory-cloud/tabletheory/commit/dfd5445ba537190d82e86b1dfa5eea439f773db5))
* respect omitempty for empty collections in Update ([9fb7f1f](https://github.com/theory-cloud/tabletheory/commit/9fb7f1f8ce8cbd9999271b7c2183628a8dbd6fff))
* **rubric:** make rubric green ([ef074bc](https://github.com/theory-cloud/tabletheory/commit/ef074bca8cbca23498dcf4a3dcf8127785ff28a4))
* **testing:** correct getTypeString for AnythingOfType ([7104519](https://github.com/theory-cloud/tabletheory/commit/710451944e823d09e02c9b441f892acd71999fcc))

## [Unreleased]

- **[CRITICAL]** Resolved expression placeholder collisions in `UpdateBuilder` when combining update expressions with query conditions.
  - Implemented `ResetConditions()` in expression builder and shared builder context to prevent placeholder overlaps.
- **[CRITICAL]** Fixed hardcoded `Version` field name in optimistic locking.
  - `ConditionVersion()` now dynamically retrieves the version field name from model metadata via `VersionFieldName()`, allowing custom version field names.

## [1.0.37] - 2025-11-11

### Added
- First-class conditional write helpers on `core.Query`: `IfNotExists()`, `IfExists()`, `WithCondition()`, and `WithConditionExpression()` make it trivial to express DynamoDB condition checks without dropping to the raw SDK.
- Documentation now includes canonical examples for conditional creates, updates, and deletes along with guidance on handling `ErrConditionFailed`.
- `docs/whats-new.md` plus new `examples/feature_spotlight.go` snippets illustrate conditional helpers, the fluent transaction builder, and the BatchGet builder with custom retry policies.
- Fluent transaction builder via `db.Transact()` plus the `core.TransactionBuilder` interface, including a context-aware `TransactWrite` helper, per-operation condition helpers (`tabletheory.Condition`, `tabletheory.IfNotExists`, etc.), and detailed `TransactionError` reporting with automatic retries for transient cancellation reasons.
- Retry-aware batch read API: `BatchGetWithOptions`, `BatchGetBuilder`, and the new `tabletheory.NewKeyPair` helper support automatic chunking, exponential backoff with jitter, progress callbacks, and bounded parallelism.

### Changed
- Create/Update/Delete paths in both the high-level `theorydb` package and the modular `pkg/query` builder now share a common expression compiler, allowing query-level conditions and advanced expressions to flow through every write operation.
- `pkg/query` executors translate DynamoDB `ConditionalCheckFailedException` responses into `customerrors.ErrConditionFailed`, enabling consistent conflict handling via `errors.Is`.
- `BatchExecutor.ExecuteBatchGet` now returns the raw DynamoDB items after retrying `UnprocessedKeys`, and top-level `BatchGet` delegates to the shared chunking engine to preserve ordering guarantees.

### Fixed
- `db.Model(...).Create()` no longer injects an implicit `attribute_not_exists` guard; callers opt in via `IfNotExists()` just like `pkg/query`, preserving the documented overwrite semantics.
- Passing `WithRetry(nil)` (or a `BatchGetOptions` with `RetryPolicy: nil`) now disables BatchGet retries as intended, instead of silently substituting the default retry policy.

## [1.0.36] - 2025-11-09

### Fixed
- Removed verbose debug logging from `Model.Update()` and the custom converter lookup so production logs stay clean without changing behavior.

## [1.0.35] - 2025-10-31

### Fixed
- Nested structs flagged with `theorydb:"json"` now apply the active naming convention (camelCase or snake_case) before honoring explicit `json` tags, keeping attribute names consistent at every level.

## [1.0.34] - 2025-10-29

### Fixed
- **[CRITICAL]** Custom converters now properly invoked during `Update()` operations
  - Security validation was rejecting custom struct types before converter check
  - Fixed by checking for custom converters BEFORE security validation
  - Custom types with registered converters now bypass security validation (converters handle their own validation)
  - Removed silent NULL fallbacks - validation/conversion failures now panic with clear error messages
- Field name validation in `Update()` - unknown field names now return clear error messages instead of silently skipping

## [1.0.33] - 2025-10-28

### Added
- Support for legacy snake_case naming convention alongside default camelCase:
  - New `naming:snake_case` struct tag to opt-in to snake_case attribute names
  - Automatic conversion of Go field names to snake_case (e.g., `FirstName` → `first_name`)
  - Smart acronym handling in snake_case conversion (e.g., `UserID` → `user_id`, `URLValue` → `url_value`)
  - Per-model naming convention detection and validation
  - Both naming conventions can coexist in the same application
  - Integration tests demonstrating mixed convention usage
- `OrCondition` method to UpdateBuilder for OR logic in conditional expressions:
  - Enables complex business rules like rate limiting with privilege checks
  - Supports mixing AND/OR conditions with left-to-right evaluation
  - Works with all condition types including attribute existence checks
  - Particularly useful for scenarios like "allow if under limit OR premium user OR whitelisted"
- Full implementation of core DynamoDB operations that were previously stubs:
  - `ExecuteQuery` and `ExecuteScan` with complete pagination, filtering, and projection support
  - `ExecuteQueryWithPagination` and `ExecuteScanWithPagination` for paginated results with metadata
  - `ExecuteBatchGet` and `ExecuteBatchWrite` with automatic retry logic for unprocessed items
  - Helper functions for unmarshaling DynamoDB items to Go structs
- Core API methods to the Query interface:
  - `BatchDelete` - Delete multiple items by their keys with support for various key formats
  - `BatchWrite` - Mixed batch operations supporting both puts and deletes in a single request
  - `BatchUpdateWithOptions` - Batch update operations with customizable options
- Fully functional `UpdateBuilder` implementation with fluent API:
  - Support for Set, Add, Remove operations
  - List manipulation methods (AppendToList, PrependToList, RemoveFromListAt, SetListElement)
  - Conditional update support with ConditionExists, ConditionNotExists, ConditionVersion
  - ReturnValues option support
- `CreateOrUpdate()` method for upsert operations - creates a new item or overwrites an existing one
- Improved error messages for `Create()` when attempting to create an item with duplicate keys

### Changed
- `UpdateBuilder()` method now returns a functional builder instead of nil
- Improved error messages to follow Go conventions (lowercase)

### Fixed
- **[CRITICAL BUG FIX]** Custom type converters registered via `RegisterTypeConverter()` are now properly invoked during `Update()` operations
  - Previously, custom converters only worked during `Create()` operations but were silently ignored during `Update()`, causing incorrect data storage (NULL values or nested struct representations instead of custom format)
  - The expression builder now receives and uses the converter lookup, ensuring consistent behavior across all operations
  - This fix affects: `Update()`, `UpdateBuilder()`, filter conditions, and all query/scan operations
  - **Breaking Change Impact**: None - this only fixes broken functionality
  - **Migration**: Code using custom converters with `Update()` will now work correctly without changes
  - Added comprehensive test suite (`theorydb_custom_converter_update_test.go`) to prevent regression
- Circular dependencies between core and query packages
- Interface signature mismatches for `BatchUpdateWithOptions` across packages
- Missing mock implementations for `BatchWrite` and `BatchUpdateWithOptions` in test helpers
- Stress test compilation error by properly creating DynamoDB client from config
- Batch operations test to use the correct interface signatures
- All staticcheck warnings including:
  - Removed unused types (`executor`, `metadataAdapter`, `filter`)
  - Fixed error string capitalization
  - Removed unnecessary blank identifier assignments
- Unmarshal error when using `All()` with slice of pointers (e.g., `[]*Model`)
- UpdateBuilder overlapping document paths error when using multiple `SetIfNotExists` operations

### Removed
- Unused `executor` type and methods from theorydb.go (functionality exists elsewhere)
- Unused `metadataAdapter` type and methods 
- Unused `filter` struct definition

## [1.0.9] - 2025-01-02

### Added
- Significant performance improvements achieving near-parity with AWS SDK
- Comprehensive documentation updates

### Changed
- Primary key recognition now properly uses `GetItem` for single lookups instead of `Query`
- API refinements for consistency:
  - `Where()` consistently uses 3 parameters: `(field, operator, value)`
  - Replaced `Find()` with `All()` for retrieving multiple results
  - `First()` now requires destination parameter

### Fixed
- Fixed primary key recognition for DynamoDB attribute names vs Go field names
- Resolved index query compilation issues
- Corrected field mapping for models with custom attribute names
- Memory usage reduced by 77% (from 179KB to 42KB per operation)
- Allocations reduced by 77% (from 2,416 to 566 per operation)

### Performance
- Single lookup operations: ~5x faster (from 2.5ms to 0.52ms)
- Now only 1.01x slower than raw AWS SDK (essentially negligible)

## [1.0.3] - 2024-12-20

## [1.0.2] - 2024-01-XX

### Added
- Pre-built mock implementations in `pkg/mocks` package
  - `MockDB` - implements `core.DB` interface
  - `MockQuery` - implements all 26+ methods of `core.Query` interface
  - `MockUpdateBuilder` - implements `core.UpdateBuilder` interface
- Comprehensive mocking documentation and examples
- Interface segregation proposal for future improvements

### Fixed
- Teams no longer need to implement all Query interface methods manually for testing
- Eliminates "trial and error" discovery of missing mock methods

## [1.0.1] - 2025-06-10

### Added
- Interface-based design for improved testability
  - New `core.DB` interface for basic operations
  - New `core.ExtendedDB` interface for full functionality
  - `NewBasic()` function that returns `core.DB` for simpler use cases
- Comprehensive testing documentation and examples
- Mock implementation examples for unit testing
- Runtime type checking for interface methods accepting `any` type

### Changed
- **BREAKING**: `tabletheory.New()` now returns `core.ExtendedDB` interface instead of `*tabletheory.DB`
- All methods that accept specific option types now accept `...any` with runtime validation
- Updated all examples and tests to use interfaces
- Improved separation between core operations and schema management

### Fixed
- Lambda.go now properly handles interface types
- Transaction callbacks properly use type assertions
- All test helper functions updated to return interfaces

### Migration Guide
See [Release Notes v1.0.1](docs/releases/v1.0.1-interface-improvements.md) for detailed migration instructions.

## [0.1.1] - 2025-06-10

### Added
- Lambda-native optimizations with 11ms cold starts (91% faster than standard SDK)
- Type-safe ORM interface for DynamoDB operations
- Multi-account support with automatic credential management
- Smart query optimization and automatic index selection
- Comprehensive struct tag system for model configuration
- Built-in support for transactions and batch operations
- Automatic connection pooling and retry logic
- Expression builder for complex queries
- Schema migration and validation tools
- Comprehensive test suite with 85%+ coverage

### Changed
- Restructured documentation into organized categories
- Improved error handling with context-aware messages
- Enhanced performance monitoring and metrics

### Fixed
- Connection reuse in Lambda environments
- Memory optimization for large batch operations
- Proper handling of DynamoDB limits

## [0.1.0] - 2025-06-10

### Added
- Initial release of TableTheory
- Basic CRUD operations
- Query and scan functionality
- Transaction support
- Batch operations
- Index management
- Expression builder
- Basic documentation

[Unreleased]: https://github.com/theory-cloud/tabletheory/compare/v1.0.36...HEAD
[1.0.36]: https://github.com/theory-cloud/tabletheory/compare/v1.0.35...v1.0.36
[1.0.35]: https://github.com/theory-cloud/tabletheory/compare/v1.0.34...v1.0.35
[1.0.9]: https://github.com/theory-cloud/tabletheory/compare/v1.0.3...v1.0.9
[1.0.3]: https://github.com/theory-cloud/tabletheory/compare/v1.0.2...v1.0.3
[1.0.2]: https://github.com/theory-cloud/tabletheory/compare/v1.0.1...v1.0.2
[1.0.1]: https://github.com/theory-cloud/tabletheory/compare/v0.1.1...v1.0.1
[0.1.1]: https://github.com/theory-cloud/tabletheory/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/theory-cloud/tabletheory/releases/tag/v0.1.0
