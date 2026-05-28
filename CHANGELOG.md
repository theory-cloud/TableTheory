# Changelog

## Unreleased

### Features

* add TTL-aware schema provisioning across Go, TypeScript, and Python helpers
* add CDK archival construct for DynamoDB TTL expirations to S3 Glacier lifecycle storage

### Bug Fixes

* add first-class legacy DynamORM naming support for uppercase `PK`/`SK` plus camelCase non-key attributes
* align Go, TypeScript, Python, and DMS `json` field semantics around native structured storage plus legacy string compatibility
* harden npm audit allowlist handling so audit service errors fail closed
* update Python lockfile security baseline and remove stale pip-audit exception
* prevent Python Lambda timeout guards from being retried by query and scan helpers
* align Python lifecycle and optimistic-lock writes with the shared P0 contract fixtures

## [1.9.0-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.8.3-rc.1...v1.9.0-rc.1) (2026-05-28)


### Features

* **docs:** add Theory Cloud GitHub Pages site ([8f1b7ec](https://github.com/theory-cloud/TableTheory/commit/8f1b7ec0bc30df2024b2129ce78649e80221591f))
* **docs:** add Theory Cloud GitHub Pages site ([7a3fac9](https://github.com/theory-cloud/TableTheory/commit/7a3fac9138eba9901bc81efaedcd700148d3c118))


### Bug Fixes

* **docs:** align workflow and transaction docs ([e39b02e](https://github.com/theory-cloud/TableTheory/commit/e39b02e48eb0d13e06971ada2718dcb877deb268))
* **docs:** correct transaction and locking examples ([4af61ab](https://github.com/theory-cloud/TableTheory/commit/4af61abcc849c88c7d6821d70aab7f0f212508f6))
* **docs:** correct wheel-asset name, encryption descriptor, transactions, and update signatures ([1691abc](https://github.com/theory-cloud/TableTheory/commit/1691abcf294783abf9e2ec98450ea70df97ff07a))
* **docs:** narrow ttl and lifecycle claims ([3a4f464](https://github.com/theory-cloud/TableTheory/commit/3a4f4641d280aa1be515cc5d41d3696db76f65f4))
* **docs:** replace internal-doc links in subtree-published pages ([5af9535](https://github.com/theory-cloud/TableTheory/commit/5af9535eafe6b6c52e3af1daffc73b166e5c26fc))
* **docs:** replace invented APIs with the real public surface ([cc45c6e](https://github.com/theory-cloud/TableTheory/commit/cc45c6e7928fc99f933dbfe7e01b60257df4caef))
* **docs:** replace invented APIs with the real public surface ([dbf261a](https://github.com/theory-cloud/TableTheory/commit/dbf261a91279e3ee4e7a9f2e5989737e2b5d2d9a))
* **docs:** use markdown-file relative links across new content ([0b7c6e9](https://github.com/theory-cloud/TableTheory/commit/0b7c6e9d7a817ebebc8cf5541745dc3f17c5df03))
* **pages:** pin third-party action refs to commit SHAs ([4b2e477](https://github.com/theory-cloud/TableTheory/commit/4b2e4772cc12a47fb2f76eee75c5b8f90845caa1))
* **py:** execute shared P0 contract fixtures ([b2e72f7](https://github.com/theory-cloud/TableTheory/commit/b2e72f706f4b07eaf99498f70b4451d747ec8acb))

## [1.8.4](https://github.com/theory-cloud/TableTheory/compare/v1.8.3...v1.8.4) (2026-05-26)


### Bug Fixes

* **deps:** release dependency security refresh ([bfba142](https://github.com/theory-cloud/TableTheory/commit/bfba14202b5470459ebe907faefc4c38a65db499))
* **deps:** release dependency security refresh ([d459e61](https://github.com/theory-cloud/TableTheory/commit/d459e61c557ebea5fb3f221eafec2c711234d4fc))

## [1.8.3-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.8.3-rc...v1.8.3-rc.1) (2026-05-26)


### Bug Fixes

* **deps:** release dependency security refresh ([bfba142](https://github.com/theory-cloud/TableTheory/commit/bfba14202b5470459ebe907faefc4c38a65db499))
* **deps:** release dependency security refresh ([d459e61](https://github.com/theory-cloud/TableTheory/commit/d459e61c557ebea5fb3f221eafec2c711234d4fc))

## [1.8.3](https://github.com/theory-cloud/TableTheory/compare/v1.8.2...v1.8.3) (2026-05-15)


### Bug Fixes

* **security:** harden audit and lambda timeout guards ([0e05d23](https://github.com/theory-cloud/TableTheory/commit/0e05d23261b8d6aafa9511c72411d41c42fa5b10))

## [1.8.3-rc](https://github.com/theory-cloud/TableTheory/compare/v1.8.2...v1.8.3-rc) (2026-05-14)


### Bug Fixes

* **security:** harden audit and lambda timeout guards ([0e05d23](https://github.com/theory-cloud/TableTheory/commit/0e05d23261b8d6aafa9511c72411d41c42fa5b10))

## [1.8.2](https://github.com/theory-cloud/TableTheory/compare/v1.8.1...v1.8.2) (2026-05-10)


### Bug Fixes

* **deps:** patch fast xml builder advisories ([c212509](https://github.com/theory-cloud/TableTheory/commit/c2125093f8f0f30e36cfdfb88865afc0c91e5899))
* **query:** make encrypted batch retries safe ([f6ec65c](https://github.com/theory-cloud/TableTheory/commit/f6ec65c158401b752c530a4b888ec020390edc41))
* **query:** make encrypted batch retries safe ([e343a29](https://github.com/theory-cloud/TableTheory/commit/e343a2917a7f0121818ebca9aaccbe57109ce0d5))
* **security:** clear rubric dependency scans ([bbf4481](https://github.com/theory-cloud/TableTheory/commit/bbf44812ff313b534a4d30c6d06cb51573da9b82))

## [1.8.0-rc.2](https://github.com/theory-cloud/TableTheory/compare/v1.8.0-rc.1...v1.8.0-rc.2) (2026-05-10)


### Bug Fixes

* **deps:** patch fast xml builder advisories ([c212509](https://github.com/theory-cloud/TableTheory/commit/c2125093f8f0f30e36cfdfb88865afc0c91e5899))
* **query:** make encrypted batch retries safe ([f6ec65c](https://github.com/theory-cloud/TableTheory/commit/f6ec65c158401b752c530a4b888ec020390edc41))
* **query:** make encrypted batch retries safe ([e343a29](https://github.com/theory-cloud/TableTheory/commit/e343a2917a7f0121818ebca9aaccbe57109ce0d5))
* **security:** clear rubric dependency scans ([bbf4481](https://github.com/theory-cloud/TableTheory/commit/bbf44812ff313b534a4d30c6d06cb51573da9b82))

## [1.8.1](https://github.com/theory-cloud/TableTheory/compare/v1.8.0...v1.8.1) (2026-05-04)


### Bug Fixes

* **marshal:** preserve omitempty pointer zero values ([6d1077c](https://github.com/theory-cloud/TableTheory/commit/6d1077cd1d3928ca07a92a5dfcc6ace723da1d8f))
* **marshal:** preserve omitempty pointer zero values ([bf83062](https://github.com/theory-cloud/TableTheory/commit/bf83062d1f84d301b78597c102ceb408209aaf70))

## [1.8.0-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.8.0-rc...v1.8.0-rc.1) (2026-05-04)


### Bug Fixes

* **marshal:** preserve omitempty pointer zero values ([6d1077c](https://github.com/theory-cloud/TableTheory/commit/6d1077cd1d3928ca07a92a5dfcc6ace723da1d8f))
* **marshal:** preserve omitempty pointer zero values ([bf83062](https://github.com/theory-cloud/TableTheory/commit/bf83062d1f84d301b78597c102ceb408209aaf70))

## [1.8.0](https://github.com/theory-cloud/TableTheory/compare/v1.7.1...v1.8.0) (2026-04-30)


### Features

* **go:** add LambdaDB timeout configuration ([5b070f1](https://github.com/theory-cloud/TableTheory/commit/5b070f111b95651efab5315a39ca0ead13c87ee1))
* **py:** add lambda timeout helper ([d43d776](https://github.com/theory-cloud/TableTheory/commit/d43d776d4e156ce4473a4fe838ad125dec007397))
* **runtime:** add lambda timeout buffer parity ([ce82176](https://github.com/theory-cloud/TableTheory/commit/ce821765944b255bbe9437883c74386d5a5f6eeb))


### Bug Fixes

* **runtime:** apply lambda timeout buffers once ([633cb39](https://github.com/theory-cloud/TableTheory/commit/633cb3923a04449fce837bec97317bb7b02d388e))

## [1.8.0-rc](https://github.com/theory-cloud/TableTheory/compare/v1.7.1-rc...v1.8.0-rc) (2026-04-30)


### Features

* **go:** add LambdaDB timeout configuration ([5b070f1](https://github.com/theory-cloud/TableTheory/commit/5b070f111b95651efab5315a39ca0ead13c87ee1))
* **py:** add lambda timeout helper ([d43d776](https://github.com/theory-cloud/TableTheory/commit/d43d776d4e156ce4473a4fe838ad125dec007397))
* **runtime:** add lambda timeout buffer parity ([ce82176](https://github.com/theory-cloud/TableTheory/commit/ce821765944b255bbe9437883c74386d5a5f6eeb))


### Bug Fixes

* **runtime:** apply lambda timeout buffers once ([633cb39](https://github.com/theory-cloud/TableTheory/commit/633cb3923a04449fce837bec97317bb7b02d388e))

## [1.7.1](https://github.com/theory-cloud/TableTheory/compare/v1.7.0...v1.7.1) (2026-04-27)


### Bug Fixes

* **security:** harden protected write policies ([b5e2156](https://github.com/theory-cloud/TableTheory/commit/b5e2156749da4ef6e6de125173c865672811f756))
* **security:** harden protected write policies ([f021d85](https://github.com/theory-cloud/TableTheory/commit/f021d852e9d519e3c9c9f5b83ac14658d285e3bb))

## [1.7.1-rc](https://github.com/theory-cloud/TableTheory/compare/v1.7.0...v1.7.1-rc) (2026-04-27)


### Bug Fixes

* **security:** harden protected write policies ([b5e2156](https://github.com/theory-cloud/TableTheory/commit/b5e2156749da4ef6e6de125173c865672811f756))
* **security:** harden protected write policies ([f021d85](https://github.com/theory-cloud/TableTheory/commit/f021d852e9d519e3c9c9f5b83ac14658d285e3bb))

## [1.7.0](https://github.com/theory-cloud/TableTheory/compare/v1.6.1...v1.7.0) (2026-04-25)


### Features

* **dms:** add release-state contract foundation ([ce7d3d3](https://github.com/theory-cloud/TableTheory/commit/ce7d3d3961a36ca4a514faf08e89e1f1b9be4957))
* **dms:** add release-state write policy metadata ([c7862d0](https://github.com/theory-cloud/TableTheory/commit/c7862d0f9462aa49c2a4b2911470f5aaeae9fc28))
* **errors:** add release-state mutation errors ([1ce0a38](https://github.com/theory-cloud/TableTheory/commit/1ce0a38e6aae5e13de21ad7850809ed53a04ca32))
* **go:** enforce release-state write policies ([15eaf11](https://github.com/theory-cloud/TableTheory/commit/15eaf1188c60612101ea0363a81e452f61d76735))
* **go:** enforce release-state write policies ([5d2c433](https://github.com/theory-cloud/TableTheory/commit/5d2c4332c68222948009c5a17469e62ae68e1d37))
* **go:** parse release-state write policy metadata ([c1f912b](https://github.com/theory-cloud/TableTheory/commit/c1f912b237dcfe25a8aa4700899d15ae8bc2277e))
* **py:** enforce release-state write policies ([0cb8fd4](https://github.com/theory-cloud/TableTheory/commit/0cb8fd47ce53f813db2ba7f7d608c78c94c44371))
* **py:** enforce release-state write policies ([bde01a5](https://github.com/theory-cloud/TableTheory/commit/bde01a56ce8ffa1affda1a3c1bdab3c9df062aff))
* **releasestate:** add release-state helpers ([49bdc14](https://github.com/theory-cloud/TableTheory/commit/49bdc144cb6627d74d28a0e6535df67385e8c676))
* **releasestate:** add transactional transition helpers ([87a0fa6](https://github.com/theory-cloud/TableTheory/commit/87a0fa635ce8c2c7a4be5386ab5af38fcb9fd8f0))
* **releasestate:** validate provenance confidence metadata ([8228344](https://github.com/theory-cloud/TableTheory/commit/82283447200a6c3b96524cd7dd42511642bbf14e))
* **ts:** enforce release-state write policies ([85c0ebe](https://github.com/theory-cloud/TableTheory/commit/85c0ebef53bb454d97fffa9d8da77ac975024110))
* **ts:** enforce release-state write policies ([45dba94](https://github.com/theory-cloud/TableTheory/commit/45dba949eb408d815315cc1f0c45f6be76207111))


### Bug Fixes

* **deps:** address dependabot alerts 56 and 57 ([aa73fe8](https://github.com/theory-cloud/TableTheory/commit/aa73fe829a37be4e8c70b3855fdf45bb2def161b))
* **deps:** address dependabot alerts 56 and 57 ([df77c48](https://github.com/theory-cloud/TableTheory/commit/df77c48f831e8cd537c4aa8f26d2055251d86ec0))
* **dms:** complete release-state contract plumbing ([4560a6a](https://github.com/theory-cloud/TableTheory/commit/4560a6a03c8ceedbba47c21b4ffe2d18e37c3bbb))

## [1.6.1](https://github.com/theory-cloud/TableTheory/compare/v1.6.0...v1.6.1) (2026-04-23)


### Bug Fixes

* **expr:** remap grouped filter placeholders on merge ([fd18b2a](https://github.com/theory-cloud/TableTheory/commit/fd18b2a097c26f7fdde4cf9fca216bf5cf8b02e1))
* **marshal:** restore anonymous embed custom hook handling ([2a61893](https://github.com/theory-cloud/TableTheory/commit/2a61893fce94ad3184969b85fa6b88b147f20847))
* **marshal:** restore anonymous embed custom hook handling ([a60ccb4](https://github.com/theory-cloud/TableTheory/commit/a60ccb4272ed287421cc5b246addc0f7c2eb910c))
* **query:** bind named updates to matched attribute names ([a054b34](https://github.com/theory-cloud/TableTheory/commit/a054b34004b47ace31a18608b87467674411e424))
* **scripts:** reject symlinks in subtree staging ([91764cd](https://github.com/theory-cloud/TableTheory/commit/91764cd2bba3ef11e7326d4e6302bfd784286e5b))
* **scripts:** reject symlinks in subtree staging ([06747e6](https://github.com/theory-cloud/TableTheory/commit/06747e68a006537a26063fe26b395ac62c952918))
* **ts:** raise fast-xml-parser override for audit baseline ([c24b50b](https://github.com/theory-cloud/TableTheory/commit/c24b50bd64c5aad035d6bc6b6d62d051ef104b87))
* **ts:** reject raw transact updates on encrypted models ([f2bcf84](https://github.com/theory-cloud/TableTheory/commit/f2bcf84940b6137c406fbf4048052358210e0c90))

## [1.6.0-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.6.0-rc...v1.6.0-rc.1) (2026-04-23)


### Bug Fixes

* **expr:** remap grouped filter placeholders on merge ([fd18b2a](https://github.com/theory-cloud/TableTheory/commit/fd18b2a097c26f7fdde4cf9fca216bf5cf8b02e1))
* **marshal:** restore anonymous embed custom hook handling ([2a61893](https://github.com/theory-cloud/TableTheory/commit/2a61893fce94ad3184969b85fa6b88b147f20847))
* **marshal:** restore anonymous embed custom hook handling ([a60ccb4](https://github.com/theory-cloud/TableTheory/commit/a60ccb4272ed287421cc5b246addc0f7c2eb910c))
* **query:** bind named updates to matched attribute names ([a054b34](https://github.com/theory-cloud/TableTheory/commit/a054b34004b47ace31a18608b87467674411e424))
* **scripts:** reject symlinks in subtree staging ([91764cd](https://github.com/theory-cloud/TableTheory/commit/91764cd2bba3ef11e7326d4e6302bfd784286e5b))
* **scripts:** reject symlinks in subtree staging ([06747e6](https://github.com/theory-cloud/TableTheory/commit/06747e68a006537a26063fe26b395ac62c952918))
* **ts:** raise fast-xml-parser override for audit baseline ([c24b50b](https://github.com/theory-cloud/TableTheory/commit/c24b50bd64c5aad035d6bc6b6d62d051ef104b87))
* **ts:** reject raw transact updates on encrypted models ([f2bcf84](https://github.com/theory-cloud/TableTheory/commit/f2bcf84940b6137c406fbf4048052358210e0c90))

## [1.6.0](https://github.com/theory-cloud/TableTheory/compare/v1.5.5...v1.6.0) (2026-04-18)


### Features

* **go:** add opt-in flat encoding for anonymous embeds ([d25d85c](https://github.com/theory-cloud/TableTheory/commit/d25d85cc323a8c2e9c97fa20af24495f133d8c66))


### Bug Fixes

* **go:** add promoted-field plan for anonymous embeds ([0c6edbc](https://github.com/theory-cloud/TableTheory/commit/0c6edbca9ea3525b9693a452f6dd562949f393d0))
* **go:** restore rubric for promoted embed helper parity ([5f7b861](https://github.com/theory-cloud/TableTheory/commit/5f7b86101caf5ed26e1b1bd506dd914b5cfbb4a6))
* **marshal:** share anonymous-embed field traversal across helpers ([384ccdc](https://github.com/theory-cloud/TableTheory/commit/384ccdcaab94cd688df045c0789b12edfe785d51))
* **query:** honor promoted anonymous embeds in public unmarshal helpers ([3128cc8](https://github.com/theory-cloud/TableTheory/commit/3128cc898bccbbda809de5dddfb9358f2883ca2e))
* **query:** migrate remaining anonymous-embed helper walkers ([7dbdd5f](https://github.com/theory-cloud/TableTheory/commit/7dbdd5f1957b99b9205495cb4370988423ad83d1))
* **types:** decode promoted anonymous embed fields ([c8404d3](https://github.com/theory-cloud/TableTheory/commit/c8404d39e79b2d1eed92c9ad66439a066404c84e))

## [1.6.0-rc](https://github.com/theory-cloud/TableTheory/compare/v1.5.5-rc...v1.6.0-rc) (2026-04-18)


### Features

* **go:** add opt-in flat encoding for anonymous embeds ([d25d85c](https://github.com/theory-cloud/TableTheory/commit/d25d85cc323a8c2e9c97fa20af24495f133d8c66))


### Bug Fixes

* **go:** add promoted-field plan for anonymous embeds ([0c6edbc](https://github.com/theory-cloud/TableTheory/commit/0c6edbca9ea3525b9693a452f6dd562949f393d0))
* **go:** restore rubric for promoted embed helper parity ([5f7b861](https://github.com/theory-cloud/TableTheory/commit/5f7b86101caf5ed26e1b1bd506dd914b5cfbb4a6))
* **marshal:** share anonymous-embed field traversal across helpers ([384ccdc](https://github.com/theory-cloud/TableTheory/commit/384ccdcaab94cd688df045c0789b12edfe785d51))
* **query:** honor promoted anonymous embeds in public unmarshal helpers ([3128cc8](https://github.com/theory-cloud/TableTheory/commit/3128cc898bccbbda809de5dddfb9358f2883ca2e))
* **query:** migrate remaining anonymous-embed helper walkers ([7dbdd5f](https://github.com/theory-cloud/TableTheory/commit/7dbdd5f1957b99b9205495cb4370988423ad83d1))
* **types:** decode promoted anonymous embed fields ([c8404d3](https://github.com/theory-cloud/TableTheory/commit/c8404d39e79b2d1eed92c9ad66439a066404c84e))

## [1.5.5-rc](https://github.com/theory-cloud/TableTheory/compare/v1.5.4...v1.5.5-rc) (2026-04-14)


### Bug Fixes

* **ci:** add local theorycloud subtree staging for tabletheory ([0760427](https://github.com/theory-cloud/TableTheory/commit/0760427d60010f404c12443adc19e2ec6367da0f))
* **ci:** add local theorycloud subtree staging for tabletheory ([b7e6913](https://github.com/theory-cloud/TableTheory/commit/b7e6913cfee641cdda0f1975c4d7dd4f1cb054bf))
* **ci:** add theorycloud subtree sync and publish commands ([c205c38](https://github.com/theory-cloud/TableTheory/commit/c205c380aaa179ad654a855512d3f289d2baed03))
* **ci:** add theorycloud subtree sync and publish commands ([3a2c338](https://github.com/theory-cloud/TableTheory/commit/3a2c3381793c99648bd4347e36aea90ca3d6edc4))
* **ci:** align theorycloud publish workflow with KT role contract ([002dfd1](https://github.com/theory-cloud/TableTheory/commit/002dfd1fa23c6c1827022f2af5d7ba36a173e2b6)), closes [#135](https://github.com/theory-cloud/TableTheory/issues/135)
* **ci:** automate theorycloud subtree publishing for tabletheory ([7c47ed2](https://github.com/theory-cloud/TableTheory/commit/7c47ed26cf3e4c72ea67621912a3be8c455460ee))
* **ci:** automate theorycloud subtree publishing for tabletheory ([849c359](https://github.com/theory-cloud/TableTheory/commit/849c35967094a663e00ffc8418f781c8e6726968))
* **ci:** finalize theorycloud publish workflow against KT lab contract ([5f10fc0](https://github.com/theory-cloud/TableTheory/commit/5f10fc0f5271744bbf0f57c21ddaf65cd523129e))
* **ci:** make theorycloud publish helper awscurl-compatible ([87945e2](https://github.com/theory-cloud/TableTheory/commit/87945e28d020ff5c516f6c13b4fe6b4e29ff22c5))

## [1.5.4](https://github.com/theory-cloud/TableTheory/compare/v1.5.3...v1.5.4) (2026-04-13)


### Bug Fixes

* align multilang json field semantics ([4eb0a3b](https://github.com/theory-cloud/TableTheory/commit/4eb0a3bf89a8f12789806d4d1dbea4a6acd515a0))
* clear rubric failures locally ([2c39a7b](https://github.com/theory-cloud/TableTheory/commit/2c39a7bccbd70b6b8ffc0ee74328442f8eb0cc17))
* complete theorydb json field semantics ([93f36a1](https://github.com/theory-cloud/TableTheory/commit/93f36a179d5b36fc3d1f0c31fe69da0426fe5367))
* remove unreachable json null branch return ([245429b](https://github.com/theory-cloud/TableTheory/commit/245429b8bdc9c3fb307253d44529a53cd1cc683c))
* remove unreachable json null branch return ([aa7902b](https://github.com/theory-cloud/TableTheory/commit/aa7902bfeb560a967408fe038193f8a7e77b5989))
* support map[string]any model round-trips ([9723c62](https://github.com/theory-cloud/TableTheory/commit/9723c628a0539133c7893a71b802d153fa0f37fd))

## [1.5.3](https://github.com/theory-cloud/TableTheory/compare/v1.5.2...v1.5.3) (2026-04-09)


### Bug Fixes

* address dependabot alerts 52-55 and pin Go 1.26.2 ([37e0fcb](https://github.com/theory-cloud/TableTheory/commit/37e0fcb982bd6dbf4d996d690b269daa067e4d2e))

## [1.5.2](https://github.com/theory-cloud/TableTheory/compare/v1.5.1...v1.5.2) (2026-04-03)


### Bug Fixes

* add first-class DynamORM naming support ([4ef6e22](https://github.com/theory-cloud/TableTheory/commit/4ef6e225941a93c34aa51f35c0418c8ac51c0499))
* address nested naming review feedback ([2cccc21](https://github.com/theory-cloud/TableTheory/commit/2cccc2194943049f460b540b603db7d9ac6c26e0))
* bump example cryptography dependency ([452c78f](https://github.com/theory-cloud/TableTheory/commit/452c78f03cdff5cbb2c9ca18b68ead69239fe53a))
* camel case nested dynamorm attributes ([4b24fe3](https://github.com/theory-cloud/TableTheory/commit/4b24fe3c0b016b55edcdd1444fad3b9eb57c6241))
* clear rubric lint regressions ([f40bb1f](https://github.com/theory-cloud/TableTheory/commit/f40bb1f923320cd03556a6ca7add6f6fe3b526d3))
* resolve rubric failures on dynamorm branch ([d2dd8e1](https://github.com/theory-cloud/TableTheory/commit/d2dd8e1da14a7d29eb88a1c53f3bc3eb2591bd9d))
* resolve security alerts in examples ([d986a2f](https://github.com/theory-cloud/TableTheory/commit/d986a2f07aace9a5e8262a3c8a039885885fc6a6))

## [1.5.1](https://github.com/theory-cloud/TableTheory/compare/v1.5.0...v1.5.1) (2026-03-28)


### Bug Fixes

* **ci:** unblock dependency scan gates ([addb0e4](https://github.com/theory-cloud/TableTheory/commit/addb0e46088cd066440fff3ddc12747304749896))
* remediate dependabot dependency alerts ([b9fce7b](https://github.com/theory-cloud/TableTheory/commit/b9fce7ba34b4a0b9a39405bc4020eb678637d9a7))
* remediate dependabot dependency alerts ([9a3dbcb](https://github.com/theory-cloud/TableTheory/commit/9a3dbcb3ec159568d12ea74aed6472d480767f5e))

## [1.5.0](https://github.com/theory-cloud/TableTheory/compare/v1.4.2...v1.5.0) (2026-03-23)


### Features

* add TTL archival lifecycle support ([0f1b88d](https://github.com/theory-cloud/TableTheory/commit/0f1b88d7012ea3436964328a73978ff680137f94))
* add TTL archival lifecycle support ([22ffedc](https://github.com/theory-cloud/TableTheory/commit/22ffedcfccff7f4732eaad50473a89bcffd7815e))


### Bug Fixes

* remediate rubric dependency scan failures ([0c6b9ee](https://github.com/theory-cloud/TableTheory/commit/0c6b9eec7d38719af075bb09a7908d0a97556ee8))

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
