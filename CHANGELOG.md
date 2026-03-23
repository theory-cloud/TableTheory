# Changelog

## Unreleased

### Features

* add TTL-aware schema provisioning across Go, TypeScript, and Python helpers
* add CDK archival construct for DynamoDB TTL expirations to S3 Glacier lifecycle storage

## [1.5.0-rc](https://github.com/theory-cloud/TableTheory/compare/v1.4.2...v1.5.0-rc) (2026-03-23)


### Features

* add TTL archival lifecycle support ([0f1b88d](https://github.com/theory-cloud/TableTheory/commit/0f1b88d7012ea3436964328a73978ff680137f94))
* add TTL archival lifecycle support ([22ffedc](https://github.com/theory-cloud/TableTheory/commit/22ffedcfccff7f4732eaad50473a89bcffd7815e))


### Bug Fixes

* remediate rubric dependency scan failures ([0c6b9ee](https://github.com/theory-cloud/TableTheory/commit/0c6b9eec7d38719af075bb09a7908d0a97556ee8))

## [1.4.2](https://github.com/theory-cloud/TableTheory/compare/v1.4.1...v1.4.2) (2026-03-05)


### Bug Fixes

* add encrypted compat mode for legacy plaintext ([d42d1f5](https://github.com/theory-cloud/TableTheory/commit/d42d1f533e7966ee041829d9bc736d31a419c376))
* **ci:** allow stable version alignment on premain promotion ([56984bd](https://github.com/theory-cloud/TableTheory/commit/56984bd4cec3490246b7aeed0b7e17de3268105f))
* **ci:** allow stable version alignment on premain promotion ([4f679a8](https://github.com/theory-cloud/TableTheory/commit/4f679a87f30e9bdb99539a0eeb65d396b2614fc9))

## [1.4.1](https://github.com/theory-cloud/TableTheory/compare/v1.4.0...v1.4.1) (2026-02-24)


### Bug Fixes

* unblock legacy naming and lambda KMS config ([897ea3c](https://github.com/theory-cloud/TableTheory/commit/897ea3cd37bff7bc795facc91a3b0451a693d817))

## [1.4.1-rc](https://github.com/theory-cloud/TableTheory/compare/v1.4.0...v1.4.1-rc) (2026-02-24)


### Bug Fixes

* unblock legacy naming and lambda KMS config ([897ea3c](https://github.com/theory-cloud/TableTheory/commit/897ea3cd37bff7bc795facc91a3b0451a693d817))

## [1.4.0](https://github.com/theory-cloud/TableTheory/compare/v1.3.0...v1.4.0) (2026-02-14)


### Features

* add FaceTheory ISR MetaStore helper ([0812a68](https://github.com/theory-cloud/TableTheory/commit/0812a68b1203f9179d987dd62eb3529e0332d705))
* TableTheory FaceTheory ISR MetaStore ([f57744d](https://github.com/theory-cloud/TableTheory/commit/f57744d5275744d6399563d29f920bcc045a2f68))


### Bug Fixes

* **deps:** resolve npm audit vulnerabilities ([83fadbd](https://github.com/theory-cloud/TableTheory/commit/83fadbd3a7d5e8f3bab0fa85f0da4250bbb1e27a))
* **deps:** update python deps for security ([712cfb5](https://github.com/theory-cloud/TableTheory/commit/712cfb5b08c410d57bbc28bd6664768b2c74b30b))
* pass SEC-2 dependency scans ([a3e0390](https://github.com/theory-cloud/TableTheory/commit/a3e0390d31d99435fd8c8aa7d48c2aeb7845d977))
* **release:** reset premain manifest baseline ([33cf3bc](https://github.com/theory-cloud/TableTheory/commit/33cf3bce6460db569f017c3127a1606a2414c432))
* **release:** reset premain manifest baseline ([69020e6](https://github.com/theory-cloud/TableTheory/commit/69020e6ab46a9358f0d50347e62a2895a63ff5a2))
* **security:** bump Go toolchain to go1.25.7 ([033b62c](https://github.com/theory-cloud/TableTheory/commit/033b62cdb9551482020293e4a006e29adb601dac))

## [1.4.0-rc](https://github.com/theory-cloud/TableTheory/compare/v1.3.0...v1.4.0-rc) (2026-02-14)


### Features

* add FaceTheory ISR MetaStore helper ([0812a68](https://github.com/theory-cloud/TableTheory/commit/0812a68b1203f9179d987dd62eb3529e0332d705))
* TableTheory FaceTheory ISR MetaStore ([f57744d](https://github.com/theory-cloud/TableTheory/commit/f57744d5275744d6399563d29f920bcc045a2f68))


### Bug Fixes

* **deps:** resolve npm audit vulnerabilities ([83fadbd](https://github.com/theory-cloud/TableTheory/commit/83fadbd3a7d5e8f3bab0fa85f0da4250bbb1e27a))
* **deps:** update python deps for security ([712cfb5](https://github.com/theory-cloud/TableTheory/commit/712cfb5b08c410d57bbc28bd6664768b2c74b30b))
* pass SEC-2 dependency scans ([a3e0390](https://github.com/theory-cloud/TableTheory/commit/a3e0390d31d99435fd8c8aa7d48c2aeb7845d977))
* **release:** reset premain manifest baseline ([33cf3bc](https://github.com/theory-cloud/TableTheory/commit/33cf3bce6460db569f017c3127a1606a2414c432))
* **release:** reset premain manifest baseline ([69020e6](https://github.com/theory-cloud/TableTheory/commit/69020e6ab46a9358f0d50347e62a2895a63ff5a2))
* **security:** bump Go toolchain to go1.25.7 ([033b62c](https://github.com/theory-cloud/TableTheory/commit/033b62cdb9551482020293e4a006e29adb601dac))

## [1.3.0](https://github.com/theory-cloud/TableTheory/compare/v1.2.1...v1.3.0) (2026-01-29)


### Bug Fixes

* **release:** keep staging aligned with premain RC line ([ba9f9c6](https://github.com/theory-cloud/TableTheory/commit/ba9f9c697da23b746f5549114e4800331df0ee90))
* **release:** keep staging aligned with premain RC line ([9827164](https://github.com/theory-cloud/TableTheory/commit/9827164cfc907b56ca099afcfa312e40dfd53269))
* **security:** upgrade eslint and migrate to flat config ([50b44dc](https://github.com/theory-cloud/TableTheory/commit/50b44dc27e551691e946ea7b1251b25ad8980086))

## [1.3.0-rc.2](https://github.com/theory-cloud/TableTheory/compare/v1.3.0-rc.1...v1.3.0-rc.2) (2026-01-29)


### Bug Fixes

* **release:** keep staging aligned with premain RC line ([ba9f9c6](https://github.com/theory-cloud/TableTheory/commit/ba9f9c697da23b746f5549114e4800331df0ee90))
* **release:** keep staging aligned with premain RC line ([9827164](https://github.com/theory-cloud/TableTheory/commit/9827164cfc907b56ca099afcfa312e40dfd53269))
* **security:** upgrade eslint and migrate to flat config ([50b44dc](https://github.com/theory-cloud/TableTheory/commit/50b44dc27e551691e946ea7b1251b25ad8980086))

## [1.2.1](https://github.com/theory-cloud/TableTheory/compare/v1.2.0...v1.2.1) (2026-01-23)


### Bug Fixes

* improved transaction handling ([30a5d7a](https://github.com/theory-cloud/TableTheory/commit/30a5d7acc371cbcbd38bee1d240e5eab24d49882))

## [1.3.0-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.3.0-rc...v1.3.0-rc.1) (2026-01-23)


### Features

* FaceTheory ISR support (FT-T0..FT-T4) ([66eb65a](https://github.com/theory-cloud/TableTheory/commit/66eb65af9b253946c53f4f88af9dc10668e6d5bd))
* FT-T1 lease helper (Go) ([bb6860a](https://github.com/theory-cloud/TableTheory/commit/bb6860a46380f1227a067dd87ea9fd0d8c4f4f34))
* FT-T1 lease helper (Py) ([71b1c01](https://github.com/theory-cloud/TableTheory/commit/71b1c01ca9d8347e8443627c023d698f4ce4d34b))
* FT-T1 lease helper (TS) ([b04dbb1](https://github.com/theory-cloud/TableTheory/commit/b04dbb17cc72b25d0ba328680170b64fcc37efc6))
* **mocks:** add transaction builder mock ([ea39672](https://github.com/theory-cloud/TableTheory/commit/ea39672edffd22bf24b1471e244c14b79f06211d))
* **mocks:** add transaction builder mock ([16ab5a5](https://github.com/theory-cloud/TableTheory/commit/16ab5a5d7b22d1087973f96a5c69b2c3a3796c3e))


### Bug Fixes

* address security/quality findings ([3b56fb4](https://github.com/theory-cloud/TableTheory/commit/3b56fb4986d2a0e93ced5c682caa2fd401a62087))
* address security/quality findings ([f7adaf7](https://github.com/theory-cloud/TableTheory/commit/f7adaf79d1e7248d3b2654f5c82b33f79cc6e4ac))
* **ci:** make release assets immutable ([1ef4aca](https://github.com/theory-cloud/TableTheory/commit/1ef4aca7bbd6ef6fffe9a86b9f33b1c0c28e1e97))
* **ci:** make release assets immutable ([e9ad219](https://github.com/theory-cloud/TableTheory/commit/e9ad219f7d806e8faff7422c45ea2b1f066e3904))
* **ci:** retry git fetch in branch-version-sync ([1b4d855](https://github.com/theory-cloud/TableTheory/commit/1b4d8557fe66c5c333846469369d9e5285cc1232))
* improved transaction handling ([30a5d7a](https://github.com/theory-cloud/TableTheory/commit/30a5d7acc371cbcbd38bee1d240e5eab24d49882))
* **mocks:** satisfy lint gates ([a9cd117](https://github.com/theory-cloud/TableTheory/commit/a9cd1170fc200489369b76f098635321ed3d81c0))
* **premain:** restore prerelease version alignment ([9b07cdb](https://github.com/theory-cloud/TableTheory/commit/9b07cdb7df5e69be8012374f742d89252ffde942))
* **security:** harden API key hashing ([2c47b6c](https://github.com/theory-cloud/TableTheory/commit/2c47b6c7dac1084b66448f81dc3d49ce4e4114e0))

## [1.2.1](https://github.com/theory-cloud/TableTheory/compare/v1.2.0...v1.2.1) (2026-01-23)


### Bug Fixes

* improved transaction handling ([30a5d7a](https://github.com/theory-cloud/TableTheory/commit/30a5d7acc371cbcbd38bee1d240e5eab24d49882))

## [1.2.0](https://github.com/theory-cloud/TableTheory/compare/v1.1.5...v1.2.0) (2026-01-22)


### Features

* **mocks:** add transaction builder mock ([ea39672](https://github.com/theory-cloud/TableTheory/commit/ea39672edffd22bf24b1471e244c14b79f06211d))
* **mocks:** add transaction builder mock ([16ab5a5](https://github.com/theory-cloud/TableTheory/commit/16ab5a5d7b22d1087973f96a5c69b2c3a3796c3e))


### Bug Fixes

* **ci:** retry git fetch in branch-version-sync ([1b4d855](https://github.com/theory-cloud/TableTheory/commit/1b4d8557fe66c5c333846469369d9e5285cc1232))
* **mocks:** satisfy lint gates ([a9cd117](https://github.com/theory-cloud/TableTheory/commit/a9cd1170fc200489369b76f098635321ed3d81c0))
* **premain:** restore prerelease version alignment ([9b07cdb](https://github.com/theory-cloud/TableTheory/commit/9b07cdb7df5e69be8012374f742d89252ffde942))

## [1.2.0-rc](https://github.com/theory-cloud/TableTheory/compare/v1.1.5...v1.2.0-rc) (2026-01-22)


### Features

* **mocks:** add transaction builder mock ([ea39672](https://github.com/theory-cloud/TableTheory/commit/ea39672edffd22bf24b1471e244c14b79f06211d))
* **mocks:** add transaction builder mock ([16ab5a5](https://github.com/theory-cloud/TableTheory/commit/16ab5a5d7b22d1087973f96a5c69b2c3a3796c3e))


### Bug Fixes

* **ci:** retry git fetch in branch-version-sync ([1b4d855](https://github.com/theory-cloud/TableTheory/commit/1b4d8557fe66c5c333846469369d9e5285cc1232))
* **mocks:** satisfy lint gates ([a9cd117](https://github.com/theory-cloud/TableTheory/commit/a9cd1170fc200489369b76f098635321ed3d81c0))
* **premain:** restore prerelease version alignment ([9b07cdb](https://github.com/theory-cloud/TableTheory/commit/9b07cdb7df5e69be8012374f742d89252ffde942))

## [1.1.5](https://github.com/theory-cloud/TableTheory/compare/v1.1.4...v1.1.5) (2026-01-20)


### Bug Fixes

* address security/quality findings ([3b56fb4](https://github.com/theory-cloud/TableTheory/commit/3b56fb4986d2a0e93ced5c682caa2fd401a62087))
* address security/quality findings ([f7adaf7](https://github.com/theory-cloud/TableTheory/commit/f7adaf79d1e7248d3b2654f5c82b33f79cc6e4ac))

## [1.1.5-rc](https://github.com/theory-cloud/TableTheory/compare/v1.1.4...v1.1.5-rc) (2026-01-20)


### Bug Fixes

* address security/quality findings ([3b56fb4](https://github.com/theory-cloud/TableTheory/commit/3b56fb4986d2a0e93ced5c682caa2fd401a62087))
* address security/quality findings ([f7adaf7](https://github.com/theory-cloud/TableTheory/commit/f7adaf79d1e7248d3b2654f5c82b33f79cc6e4ac))

## [1.1.4](https://github.com/theory-cloud/TableTheory/compare/v1.1.3...v1.1.4) (2026-01-19)


### Bug Fixes

* **ci:** make release assets immutable ([1ef4aca](https://github.com/theory-cloud/TableTheory/commit/1ef4aca7bbd6ef6fffe9a86b9f33b1c0c28e1e97))
* **ci:** make release assets immutable ([e9ad219](https://github.com/theory-cloud/TableTheory/commit/e9ad219f7d806e8faff7422c45ea2b1f066e3904))

## [1.1.4-rc](https://github.com/theory-cloud/TableTheory/compare/v1.1.3...v1.1.4-rc) (2026-01-19)


### Bug Fixes

* **ci:** make release assets immutable ([1ef4aca](https://github.com/theory-cloud/TableTheory/commit/1ef4aca7bbd6ef6fffe9a86b9f33b1c0c28e1e97))
* **ci:** make release assets immutable ([e9ad219](https://github.com/theory-cloud/TableTheory/commit/e9ad219f7d806e8faff7422c45ea2b1f066e3904))

## [1.1.3](https://github.com/theory-cloud/TableTheory/compare/v1.1.2...v1.1.3) (2026-01-19)


### Bug Fixes

* **security:** harden API key hashing ([2c47b6c](https://github.com/theory-cloud/TableTheory/commit/2c47b6c7dac1084b66448f81dc3d49ce4e4114e0))

## [1.1.3-rc](https://github.com/theory-cloud/TableTheory/compare/v1.1.2...v1.1.3-rc) (2026-01-19)


### Bug Fixes

* **security:** harden API key hashing ([2c47b6c](https://github.com/theory-cloud/TableTheory/commit/2c47b6c7dac1084b66448f81dc3d49ce4e4114e0))

## [1.1.2](https://github.com/theory-cloud/TableTheory/compare/v1.1.1...v1.1.2) (2026-01-19)


### Bug Fixes

* **ci:** add release PR workflow ([07a5cc8](https://github.com/theory-cloud/TableTheory/commit/07a5cc8d12195ba8f702ecbe21b8d9c57b9efa9a))
* **ci:** align versions for v1.1.1 ([bb74851](https://github.com/theory-cloud/TableTheory/commit/bb74851ae8871c2cf6b8e6906b66f15669a2f928))
* **ci:** allow manual asset upload for existing tag ([0f8ea9e](https://github.com/theory-cloud/TableTheory/commit/0f8ea9eeae26035670adfe5f8c4d469694ccda12))
* **ci:** make release dispatch input compatible ([cd0c245](https://github.com/theory-cloud/TableTheory/commit/cd0c245d6987bdbbc1b15754abe4f1f1337dd9cf))

## 1.0.0 (2026-01-19)


### Bug Fixes

* **ci:** address ruff and coverage ([f38a930](https://github.com/theory-cloud/TableTheory/commit/f38a9308b7397314168f68a5ee99ff9d2695d2c0))
* **ci:** bootstrap release-please for v1.0.0 ([fbfa057](https://github.com/theory-cloud/TableTheory/commit/fbfa057aa208b3074d25bfde3da22915eaa7802d))
* **ci:** make release-pr workflows valid ([69ef5da](https://github.com/theory-cloud/TableTheory/commit/69ef5da01717ccaf0a0d852396d11888117e1bb4))
* **ci:** prevent release-please PR loops ([85bcb20](https://github.com/theory-cloud/TableTheory/commit/85bcb2031a9bb66bdea22ce8a92b1c82bb021bb3))
* **ci:** stop release-please release PR loops ([6ed99ab](https://github.com/theory-cloud/TableTheory/commit/6ed99abb4c58b9cdb40da6851941d9b2884cbdc8))
* **ci:** support immutable releases ([#4](https://github.com/theory-cloud/TableTheory/issues/4)) ([372fc1e](https://github.com/theory-cloud/TableTheory/commit/372fc1e5d3b1ce036525270425f8a69123019d1f))
* **release:** bootstrap v1.0.0 ([bfec532](https://github.com/theory-cloud/TableTheory/commit/bfec532f54596504954eaf1a09dd747f5b4397a4))

## Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
