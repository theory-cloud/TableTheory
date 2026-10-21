# Changelog

## Unreleased

### Breaking Changes

* **go:** the Go module path moves to `github.com/theory-cloud/tabletheory/v5` with this major release. Go consumers must
  change their module requirement and every TableTheory import from `github.com/theory-cloud/tabletheory/v4` to
  `github.com/theory-cloud/tabletheory/v5`, then run `go get github.com/theory-cloud/tabletheory/v5@v5.0.0` and
  `go mod tidy`. TypeScript and Python consumers are unaffected by this path move; see `docs/migration/v5.md`.
* **ts:** on the default `TheorydbClient` (`numberUnmarshalMode` `'string'`), typed items now declare DynamoDB `N` fields
  as `string` and `NS` fields as `string[]` instead of `number`/`number[]`. Runtime behavior is unchanged — the
  documented default was already exact-string — so this is a compile-time-only break: consumer code that relied on the
  previous incorrect `number` declaration must handle the decimal strings directly, or construct the client/repository
  in `'number'` mode (`TheorydbClient<'number'>`) to obtain numbers. Semver decision: this public type correction is
  release-major material and must not ship as a patch/minor.
* **contract:** advance the shared model contract to DMS v0.2. High-level updates now distinguish field selection:
  unselected zero-valued `omitempty` fields remain unchanged, while explicitly selected empty `omitempty` fields remove
  the persisted DynamoDB attribute. TypeScript already had this behavior; Go and Python previously stored empty values.
  Existing items remain readable, but consumers relying on empty attributes remaining present must remove `omitempty` or
  use an explicit low-level `SET`. The Go module path is now `github.com/theory-cloud/tabletheory/v4`; the path move and
  its consumer migration are recorded below and in `docs/migration/v4.md`. DMS `M` values use carrier-size emptiness, so
  non-empty maps, objects, Go
  structs, and Python dataclasses remain present even when every contained value is empty. Arrays/lists use length
  semantics, including fixed-length Go arrays, which serialize as `L` on both Create and Update. Go's no-argument
  `Update()` retains sparse zero-value selection so unselected structs and arrays cannot overwrite persisted data.
  Fixed arrays round-trip through stream decoding, batch and transaction reads, and update-builder return values.
  Tagged record fields without `omitempty` remain present in non-JSON `M` carriers, while untagged zero-valued fields
  inside nested struct maps are now omitted; add explicit matching tags when a nested zero must remain present. Go
  transaction writes now use the same top-level `omitempty` predicate, and transactional explicit-empty updates emit
  `REMOVE`. Legacy `gsi:Name:pk` / `gsi:Name:sk` tags remain excluded from updates under exact token parsing. See
  `docs/migration/v3.md`.
* **go:** the Go module path moves to `github.com/theory-cloud/tabletheory/v4` with this major release. Go consumers must
  change their module requirement and every TableTheory import from `github.com/theory-cloud/tabletheory/v3` to
  `github.com/theory-cloud/tabletheory/v4`, then run `go get github.com/theory-cloud/tabletheory/v4@v4.0.0` and
  `go mod tidy`; see `docs/migration/v4.md`.
* **go:** align `ConsistentRead()` on GSI queries with the cross-runtime contract by returning
  `ErrInvalidOperator` instead of silently dropping the flag, including when the Go query optimizer auto-selects a GSI
  from key conditions. Semver decision: this parity repair is release-major material and must not ship as a patch/minor.
* **go:** align newly written DynamoDB shapes for binary and set-tagged fields with TypeScript, Python, and the DMS type
  matrix: `[]byte` writes as `B`, numeric `theorydb:"set"` slices write as `NS`, binary set slices write as `BS`, empty
  set-tagged slices write as `NULL`, and unsupported set element types fail at write time. Legacy shape-driven reads
  remain supported, but filters/conditions over mixed old/new data may need migration. Semver decision: this persisted
  shape convergence is release-major material and must not ship as a patch/minor.
* **go:** replace the ticker-driven memory monitor with on-demand sampling. `MemoryMonitor.Start`,
  `MemoryMonitor.Stop`, `ResourceProtector.StartMemoryMonitoring`, `ResourceProtector.StopMemoryMonitoring`, and
  `ResourceLimits.MemoryCheckInterval` are removed, and `MemoryMonitor.Sample`,
  `ResourceProtector.SampleMemory`, and `ResourceProtector.SetMemoryAlertCallback` replace them. A caller samples
  inside the request whose memory it wants recorded; no goroutine or timer is started, so nothing a sample starts can
  outlive the invocation that asked for it. The removed API had no consumers in this repository, and TypeScript and
  Python never had a counterpart. Semver decision: this removed-API change is release-major material and must not ship
  as a patch/minor.

### Features

* **go:** add `NewWithClient` and a state-backed `pkg/testing/fakedb` consumer test fake
* **contract:** validate the Go state-backed fake against the P0 contract corpus
* **ts:** add a stateful DynamoDB `send()` fake to the public testkit
* **ts:** add model-based transactional updates with explicit field selection; selected empty `omit_empty` attributes
  emit `REMOVE`, while other selected attributes emit `SET`
* **py:** add a stateful DynamoDB testkit fake and top-level re-exports
* add TTL-aware schema provisioning across Go, TypeScript, and Python helpers
* add CDK archival construct for DynamoDB TTL expirations to S3 Glacier lifecycle storage
* **go:** generate `encoding/json.Number` for DMS `decimal_string` attributes so generated decimal fields round-trip
  DynamoDB `N` through the default marshal paths, with no registered converter
* **ts:** add `AutoMigrateOptions.encryption` so `autoMigrate` can encrypt attributes a target model declares
  encrypted, and add the `ErrMigrationEncryptionRequired` and `ErrNumberPrecisionLoss` error codes
* **py:** add `kms_key_arn`, `kms_client`, and `rand_bytes` to `auto_migrate` and export `MigrationEncryptionError`

### Bug Fixes

* **go:** refuse data-copying `AutoMigrateWithOptions` migrations that would write plaintext into a target
  `theorydb:"encrypted"` field, before any table is created or any row is copied, and encrypt plaintext through the
  configured session KMS key when one is present
* **py:** bound derived-key numeric coercion to DynamoDB's exponent range before fixed-point expansion, so a tiny
  input such as `1e100000000` can no longer expand toward an unbounded allocation
* **ts:** aggregate exact-decimal-string `N` values numerically: `sum`/`average`/`min`/`max`/`group by`/`having` no
  longer silently return `0` or order lexicographically. DynamoDB decimal strings are the exact domain: `min`/`max`/
  `group by` order by exact decimal text, and `sum`/`average`/`having` return a number only when every value and the
  result is exactly representable as an IEEE-754 double, otherwise raising `ErrNumberPrecisionLoss`; exponent notation
  outside DynamoDB's `[-130, 125]` adjusted-exponent range is rejected before any fixed-point expansion. Already
  materialized JavaScript numbers (`numberUnmarshalMode: 'number'`) are the lossy domain and keep the historical
  JavaScript-number arithmetic and comparison instead of being re-parsed as exact decimal strings; inputs that mix
  exact decimal strings with JavaScript numbers raise `ErrNumberPrecisionLoss` rather than coercing the string through
  `Number`. Decimal parsing strips mantissa zeros with linear index scans so a long attacker-controlled mantissa cannot
  drive quadratic regular-expression work
* **ts:** retain and compare DMS attribute `format` in normalization, equivalence, and model-to-DMS so format-only
  drift is no longer reported equivalent
* **go:** remove the detached cold-start pre-warm and the multi-account credential-refresh ticker, so no TableTheory
  init path leaves work running after it returns. Lambda freezes the execution environment as soon as the handler
  returns, so a pre-warm started in a goroutine could be frozen mid-flight and resume against an invocation that had
  already completed. `OptimizeForColdStart` keeps its signature and now performs only synchronous, local
  model-metadata work: the removed pre-warm issued `ListTables`, which needed IAM permissions beyond item access and
  was therefore pure waste for consumers that grant only item operations. `MultiAccountDB` refreshes expired partner
  sessions synchronously on the `Partner()` path instead of from a five-minute background ticker.
* **go:** join every worker of a parallel fan-out before returning. `Query.ScanAllSegments` returned on the first
  segment error while the other segment goroutines were still running, and `pkg/protection.SecureBodyReader` returned a
  timeout while its request-body reader was still blocked. Both now stop the remaining work and wait for it on every
  path, so nothing outlives the call that started it.
* **ts:** `mapConcurrent` — the helper behind `Query.scanAllSegments` — now waits for every worker to settle before it
  rejects, aborts the remaining segment scans, and re-throws the original failure instead of one of the abort-induced
  ones.
* **examples:** the payment Lambda webhook and the blog comment notifications are delivered synchronously inside the
  invocation (bounded by a timeout) instead of from a detached goroutine that Lambda could freeze mid-flight.
* **transaction:** refresh library-owned `updated_at` values on Go and TypeScript model-shaped transactional updates,
  matching Python and each runtime's non-transactional update behavior
* **ts:** reject `createdAt` and version fields from model-shaped transactional update selections, and validate
  `updateFn` keys before invoking caller callbacks
* **go:** normalize `theorydb:"json"` values identically in query and explicit transaction updates
* **ci:** mechanically classify every Go `IsEmpty` / `IsSparseUpdateEmpty` call site so predicate drift fails the rubric
* **transaction:** preserve non-nil empty collections in nested Go structs unless `omitempty` is explicit
* **ci:** select direct-PR verifier scripts while a promotion base still contains the retired queue-era policy
* **ci:** retire merge queues and restore direct protected-PR merges with strict live-ref provenance
* **security:** document the expiring AWS CDK bundled `brace-expansion` exception and fail its policy check once AWS
  publishes a fixed bundle
* **deps:** make the CDK Python Lambda a locked `pyproject.toml` project so GitHub dependency graph submissions use one
  consistent uv dependency source
* **go:** pin the repository and example Go toolchains to go1.26.5 for reproducible CI and local security checks
* add first-class legacy DynamORM naming support for uppercase `PK`/`SK` plus camelCase non-key attributes
* align Go, TypeScript, Python, and DMS `json` field semantics around native structured storage plus legacy string compatibility
* harden npm audit allowlist handling so audit service errors fail closed
* **ts:** honor opt-in exact number unmarshalling for update-builder return values and native JSON-number attributes
* update Python lockfile security baseline and remove stale pip-audit exception
* prevent Python Lambda timeout guards from being retried by query and scan helpers
* align Python lifecycle and optimistic-lock writes with the shared P0 contract fixtures
* **go:** round lease expiry up to the whole second at or after `now+duration` in `pkg/lease`, so a sub-second lease cannot be taken over inside the second it was acquired and longer leases are never shortened; the TypeScript and Python lease managers round the same way. Whole-second clocks and durations are unchanged
* **go:** bound `MultiAccountDB.Partner` to the requested partner with exponential failure backoff instead of synchronously refreshing every expired account, so unrelated expired or failing accounts can no longer block a request
* **py:** treat `Table.query_count`/`scan_count` `limit` as a total evaluated-item budget instead of deleting it and scanning the whole remaining partition or table; `limit=None` still pages to exhaustion
* **go/py/ts:** align the stateful fakes with DynamoDB: combined conditions whose outer parentheses do not form a matching pair and missing-attribute comparisons are enforced, `Count` is the matched count and `ScannedCount` the evaluated count, and `Limit` bounds evaluated items before the filter
* **ts:** stop `QueryOptimizer.explain` from reporting an executable index — and suppressing the missing-partition-key error — when a filter condition only resembles an index key
* **go:** classify a condition failure as `ErrVersionConflict` only on case-sensitive whole-token placeholders or attribute names, so `#value`, `#n10`, and case-variant names no longer misclassify a generic conditional failure
* **go:** carry `Config.CredentialsProvider`/`Config.AWSConfigOptions` into the KMS AWS config in `NewSessionWithClient`, and fail early with `ErrEncryptionNotConfigured` when a KMS key ARN has no usable credential source; callers that set `KMSKeyARN` without a `KMSClient` must now supply credentials or an explicit KMS client
* **go/py/ts:** release-state transition helpers now bind the immutable event to the actual row they update: an event whose partition key does not match the actual-state row is rejected before any transaction is submitted, so an event can no longer describe a different release than the one that changed
* **go/py/ts:** deploy-authority provenance validation now includes the optional evidence `digest` in the evidence conflict identity, so identical evidence stays idempotent while the same kind/source/ref with a different digest conflicts; the top-level `provenance.system`/`kind`/`ref` (and `digest` when present) must now align with the evidence entry that earned deploy authority, so unrelated evidence can no longer bless a different top-level artifact
* **go/py/ts:** the release-state examples commit the actual-state update, the immutable event append, and the conditional outbox create in one DynamoDB transaction instead of printing an unused outbox
* **docs:** the canonical vocabulary and runtime docs describe Go GSI tags with the supported indexed form `theorydb:"index:<name>,pk"` / `theorydb:"index:<name>,sk"` instead of the rejected bare `theorydb:"gsi1pk"` / `theorydb:"gsi1sk"`; the generative-artifacts verifier now rejects the bare form and a Go test proves the documented tags register
* **py:** the copyable GSI core-pattern uses Python dataclass field names in `IndexSpec`/`gsi(...)` instead of persisted storage names, and the Python scaffold README no longer claims the removed `theorydb_py` compatibility shim exists
* **docs:** the FaceTheory ISR idempotency recipe no longer documents an invalid transaction that issued both a `ConditionCheck` and a `Delete` against the same lease item; it documents the single conditional lease `Delete` the shipped helper uses
* **examples:** the multi-language TTL archive fails closed when a TTL-expiry `REMOVE` record eligible for archival has no `OldImage`, reporting it as a batch item failure instead of a successful skip, and the archive construct rejects a table whose stream view is knowably not `NEW_AND_OLD_IMAGES`
* **cli:** `generate-ts`/`gen --out` and `tabletheory init` (including `--force`) no longer follow a symlink at the output path or an escaping parent directory, so they cannot truncate or overwrite a file outside the intended root; output is replaced atomically and the victim file is left untouched
* **examples:** the local `contacts`/`notes`/`todo` setup checks DynamoDB Local with an explicit DynamoDB protocol call instead of generic HTTP reachability, so any non-DynamoDB HTTP listener on the local endpoint is refused
* **go:** release `pkg/protection.SecureBodyReader`'s request-concurrency slot before its timeout cleanup instead of after it. A client that declares a body and withholds it leaves a Go HTTP/1 request body whose reader and `Close` both wait on the stalled socket; the timeout path previously closed the body and joined the reader before releasing the slot, so the slot was held until the client disconnected and `MaxConcurrentReq` stalled connections rejected every other request. The slot is now released no later than `MaxRequestTimeout`, ahead of the close and join, which can still block the calling handler (the reader is joined, not abandoned) until the client disconnects or a transport deadline fires; a server- or upstream-level body-read deadline remains the robust fix for live HTTP/1 requests. A regression test drives a real HTTP/1 server and a client that withholds its declared body, alongside a complete-body success control
* **ts:** give each `FaceTheoryIsrMetaStore` one immutable table binding by registering its models on a private fork of a caller-supplied `TheorydbClient` (new `TheorydbClient.fork()`) instead of mutating the shared client's name-keyed registry. Previously, constructing a second store for another table with the same client silently rebound the first store's metadata reads and transaction actions to the second table (`register()` does an unconditional `Map.set`), while its lease operations stayed on the original table — a cross-tenant confused-deputy path. Reads, transactions, and leases now share the store's own table, and the supplied client's registry is left untouched; a shared-client regression test asserts store A reads and transacts only against table A after store B is built
* **go/py:** resolve `write_policy.protected_attributes` against the canonical DynamoDB attribute name first, accepting a Go struct field name or Python field name only when no canonical attribute matches. Both runtimes previously preferred the language field name, so when a canonical attribute name also named a different field — for example a Python field `protected_value` stored as `authority` alongside a field `authority` stored as `description`, or the equivalent PascalCase-naming Go collision — the guard protected the wrong attribute and left the intended one mutable through `Table.update`/`TransactUpdate`/`UpdateBuilder` and their Go equivalents. TypeScript has no separate field-name namespace and was unaffected. Regression tests cover the canonical-name resolution in the model metadata and the transaction protected-field guard

## [6.1.1-rc](https://github.com/theory-cloud/TableTheory/compare/v6.1.0...v6.1.1-rc) (2026-10-21)


### Bug Fixes

* **deps:** bump certifi to 2026.10.19 in publish tooling lock ([a72201f](https://github.com/theory-cloud/TableTheory/commit/a72201fd7e499eb99e6be309b918a88aefc34887))
* **deps:** bump certifi to 2026.10.19 in publish tooling lock ([91ceabb](https://github.com/theory-cloud/TableTheory/commit/91ceabb472a928f54919c9b073ef09ce0f203cbe))
* **examples/blog:** flatten CRLF in untrusted email fields ([fac264e](https://github.com/theory-cloud/TableTheory/commit/fac264e0ac3862353bbbf51578f5c854fe144a4a))
* **examples/blog:** flatten CRLF in untrusted email fields ([7495924](https://github.com/theory-cloud/TableTheory/commit/74959249fb711eaf937039118e46f2c8cefd17b1))
* **go:** move every module to Go 1.27.3 ([2865c9d](https://github.com/theory-cloud/TableTheory/commit/2865c9deafc5151de30bc40c79a4e4f23539efdc))
* **go:** move every module to Go 1.27.3 ([3088a34](https://github.com/theory-cloud/TableTheory/commit/3088a34331a6b9badf35b8737d47d65d90c5f53f))
* **lambda:** deploy on provided.al2023 and judge templates in the gate ([c6464de](https://github.com/theory-cloud/TableTheory/commit/c6464dec6aff1e02bac782efb7af35e8f5b60e89))

## [6.1.0](https://github.com/theory-cloud/TableTheory/compare/v6.1.0-rc...v6.1.0) (2026-10-18)

Stable promotion of `v6.1.0-rc`.


## [6.1.0-rc](https://github.com/theory-cloud/TableTheory/compare/v6.0.0...v6.1.0-rc) (2026-10-18)


### Features

* **ts:** add an explicit discriminator for S-typed JSON fields ([e6b2e05](https://github.com/theory-cloud/TableTheory/commit/e6b2e05d32fc43db758ffc672a6ae73520bff1a1))
* **ts:** let model-shaped transaction updates advance versions ([61a9b0a](https://github.com/theory-cloud/TableTheory/commit/61a9b0a0908dcc682158b29ba91e8af59f4402e0))


### Bug Fixes

* **blog:** build the Lambda bootstrap artifacts ([2a05d2d](https://github.com/theory-cloud/TableTheory/commit/2a05d2d92fc69c14a8b944b6cb87f38c7f003f4d))
* **blog:** declare and handle the REQUEST authorizer contract ([5ddd16d](https://github.com/theory-cloud/TableTheory/commit/5ddd16da44e521266e64e3ced055f753b359dbab))
* **blog:** declare the REQUEST authorizer in the API auth block ([1552641](https://github.com/theory-cloud/TableTheory/commit/1552641c6a809854dbaef0edd0e0905169158dbc))
* **blog:** synthesize the SAM API with a valid authorizer identity ([cffd30d](https://github.com/theory-cloud/TableTheory/commit/cffd30dd6eecc8f91a1582193376b4ccbd68f3db))
* **ci:** gate release workflows to their release branches ([cc2a65c](https://github.com/theory-cloud/TableTheory/commit/cc2a65c8cb6624f023b4437390ea5c5ff464c107))
* **ci:** reject Pages dispatch from non-publication refs ([0bcedb0](https://github.com/theory-cloud/TableTheory/commit/0bcedb045fedf3d157d981a10d352e448cd04a47))
* **contract:** accept the encryption context in the Py runner ([c2b38f9](https://github.com/theory-cloud/TableTheory/commit/c2b38f9e7d1e5be644b1dd15d9d1fa1d001bbe66))
* **encryption:** bind new envelopes to the record identity ([68d2c45](https://github.com/theory-cloud/TableTheory/commit/68d2c45b7f2f63029487c0ed5951425fb0cbb2a7))
* **encryption:** bind v2 envelopes unambiguously in AAD and KMS ([81138de](https://github.com/theory-cloud/TableTheory/commit/81138de9b32c0e8ec259592a08d9d7de158d7438))
* **examples/blog:** authorize from authorizer context, not caller headers ([07834cd](https://github.com/theory-cloud/TableTheory/commit/07834cdb471993040301578db0e84a33e34a7e96))
* **examples/blog:** keep drafts and commenter PII off public routes ([9918e00](https://github.com/theory-cloud/TableTheory/commit/9918e00b9749fecddb96f47789a95232895669bd))
* **examples/cdk-multilang:** require IAM on demo function URLs ([7ac30d0](https://github.com/theory-cloud/TableTheory/commit/7ac30d049f04c56430ffee87f3c7bced4c3b7f99))
* **examples/multi-tenant:** enforce organization roles and project access ([6de4f65](https://github.com/theory-cloud/TableTheory/commit/6de4f6549490e8b5a271a2dfad9a597b3b87628e))
* **examples/payment:** require JWT secret, expiry, and audience ([5c0d790](https://github.com/theory-cloud/TableTheory/commit/5c0d7907ead5710272c56c023e268e0d098d595e))
* **examples/payment:** scope idempotency keys per merchant ([020febf](https://github.com/theory-cloud/TableTheory/commit/020febfeffdb16afcf80ecc22d16aaf77f191125))
* **py:** fail closed on bool/number binding comparisons ([bef6444](https://github.com/theory-cloud/TableTheory/commit/bef6444f696b858b16d81d47a94797997c55cdb1))
* **py:** fail closed when a migration cannot bind an envelope ([c8de2e0](https://github.com/theory-cloud/TableTheory/commit/c8de2e087cdc04537aa14205db2758d8603fc61c))
* **query:** keep case-colliding reserved placeholders distinct ([01bf4a9](https://github.com/theory-cloud/TableTheory/commit/01bf4a9bd4703235b8f5c3e1a8a2527fc0f181de))
* **releasestate:** bind event and outbox rows to the transition ([86a22b5](https://github.com/theory-cloud/TableTheory/commit/86a22b5e069c7a0ecb7ea80ba3dabfba7d3f35db))
* **releasestate:** bind the transition version field to the model role ([2c708f0](https://github.com/theory-cloud/TableTheory/commit/2c708f01afafe794372fbd83848f58fcc92a1ffe))
* **releasestate:** describe binding rows without model registration ([155fbdd](https://github.com/theory-cloud/TableTheory/commit/155fbdd0101705090139092eda9ecc839c56fbf2))
* **releasestate:** reject numeric string identity coercion ([c8f918b](https://github.com/theory-cloud/TableTheory/commit/c8f918b60e91b5102f47f8602f405870e2e5bcd3))
* **releasestate:** resolve binding roles through registry metadata ([eaa6206](https://github.com/theory-cloud/TableTheory/commit/eaa6206e10b575de0515f6241871cb735ce698b9))
* **releasestate:** resolve the version role through registry metadata ([7f6ef1e](https://github.com/theory-cloud/TableTheory/commit/7f6ef1ee5925a8bef4bde5b85f79979c1e063093))
* **schema:** refuse backup names that would delete tables ([6a25556](https://github.com/theory-cloud/TableTheory/commit/6a2555678db7e860f811bb752237c5d6b31054ca))
* **security:** remediate the 2026-10-18 Codex Security packet ([7bf2628](https://github.com/theory-cloud/TableTheory/commit/7bf262819aa1b4dd72ae8ae2414c5fdfe5b1dd5c))
* **security:** wave 2 remediation for the 2026-10-18 Codex Security packet ([5ecdc68](https://github.com/theory-cloud/TableTheory/commit/5ecdc686ff65028faed6673100f9b9035a72ec05))
* **ts:** honor json_string_mode and native parse-mode reads ([1b58e60](https://github.com/theory-cloud/TableTheory/commit/1b58e60c9289422c958f21ec0a183a4ebbaf748d))
* **ts:** keep keyword-shaped placeholders inside their expression clause ([f0cd2d0](https://github.com/theory-cloud/TableTheory/commit/f0cd2d08c5efc95ff5cae43f6b98b4ed995f7130))

## [6.0.0](https://github.com/theory-cloud/TableTheory/compare/v6.0.0-rc...v6.0.0) (2026-10-16)

Stable promotion of `v6.0.0-rc`.


## [6.0.0-rc](https://github.com/theory-cloud/TableTheory/compare/v5.2.0...v6.0.0-rc) (2026-10-16)


### ⚠ BREAKING CHANGES

* **module:** Go consumers must update their module requirement and every TableTheory import from github.com/theory-cloud/tabletheory/v5 to github.com/theory-cloud/tabletheory/v6; see docs/migration/v6.md.
* **lambda:** LambdaDB.WithLambdaTimeout now returns core.DB instead of *LambdaDB. A call that assigned the result back to a *tabletheory.LambdaDB variable, or passed it to a function taking *LambdaDB, no longer compiles. Hold the derived handle as core.DB/core.ExtendedDB, apply the timeout after the LambdaDB-specific calls (PreRegisterModels, WithLambdaTimeoutConfig), or assert the result back with `.(*tabletheory.LambdaDB)`. Stored representation, key encoding, marshalling, and timeout behavior are unchanged.

### Bug Fixes

* **lambda:** return core.DB from LambdaDB.WithLambdaTimeout ([a1e2980](https://github.com/theory-cloud/TableTheory/commit/a1e29809fd8589baaa26749d1c4dffc25850179f))
* **module:** adopt the /v6 Go module path ([a8afe19](https://github.com/theory-cloud/TableTheory/commit/a8afe19440e14a0d78bd241b4c7bbfed98dbd327))

## [5.2.0](https://github.com/theory-cloud/TableTheory/compare/v5.2.0-rc...v5.2.0) (2026-10-14)

Stable promotion of `v5.2.0-rc`.


## [5.2.0-rc](https://github.com/theory-cloud/TableTheory/compare/v5.1.0...v5.2.0-rc) (2026-10-14)


### Features

* complete the v5.1.0 follow-ups, gate the cross-runtime fixtures, consolidate dependabot bumps ([f3ec04b](https://github.com/theory-cloud/TableTheory/commit/f3ec04b0cc2afc344b4318fa15b5b7decac9dd20))
* **errors:** add a safe error surface and opt-in item-image redaction (py) ([fc3f824](https://github.com/theory-cloud/TableTheory/commit/fc3f824dc35ff209915fdb631a2b68135311b697))
* **errors:** add a safe error surface and opt-in item-image redaction (ts) ([747f8df](https://github.com/theory-cloud/TableTheory/commit/747f8dfd7f522fd401fe8c81de0fee95049017d8))


### Bug Fixes

* **ts:** stamp transactWrite put lifecycle fields from the injected clock ([c6bc87c](https://github.com/theory-cloud/TableTheory/commit/c6bc87c42bfccb65a61996d40ed19e345740efca))

## [5.1.0](https://github.com/theory-cloud/TableTheory/compare/v5.1.0-rc...v5.1.0) (2026-10-14)

Stable promotion of `v5.1.0-rc`.


## [5.1.0-rc](https://github.com/theory-cloud/TableTheory/compare/v5.0.0...v5.1.0-rc) (2026-10-14)


### Features

* **errors:** preserve transaction cancellation causes and reasons ([467a865](https://github.com/theory-cloud/TableTheory/commit/467a865a7472afd65adab2a4c34d2b47034c510c))
* **errors:** preserve transaction cancellation causes and reasons across runtimes ([da2e0a1](https://github.com/theory-cloud/TableTheory/commit/da2e0a1815d0b62b3864f0d805ba6736b3435958))
* **marshal:** add item writer-path preflight API surface ([afbd0eb](https://github.com/theory-cloud/TableTheory/commit/afbd0eb95e2160824c9e6be2b7198392dd392211))
* **marshal:** add writer-path item preflight for nil-collection parity ([c4dbecb](https://github.com/theory-cloud/TableTheory/commit/c4dbecb21439d9362a77408e52c36d7acae6244c))
* **marshal:** implement the Go writer-path item preflight ([1c3fa4d](https://github.com/theory-cloud/TableTheory/commit/1c3fa4ddd02aebdeb5d896fc75d8245a7da70399))
* **py:** implement the writer-path item preflight ([aa7e8e5](https://github.com/theory-cloud/TableTheory/commit/aa7e8e58a16db681763feec7cff1ed5d4c6c7078))
* **ts:** implement the writer-path item preflight ([ab43a5f](https://github.com/theory-cloud/TableTheory/commit/ab43a5f72be7f0d4c99eea9d2cd50b375f3db47a))


### Bug Fixes

* **ci:** publish docs from staging pushes again in pages.yml ([bce9495](https://github.com/theory-cloud/TableTheory/commit/bce9495a0f5c30ceb2d386da2d342b8a69417170))
* **ci:** remove the redundant staging release-eligibility gate ([725ce4f](https://github.com/theory-cloud/TableTheory/commit/725ce4f448a38867959d6c13bfbaf02fccce3618))
* **ci:** remove the redundant staging release-eligibility gate ([61dbd08](https://github.com/theory-cloud/TableTheory/commit/61dbd088a5c31f63b45491ef9deef6fba3c1d61d))
* **ci:** revert the pages push trigger to staging ([9e87b72](https://github.com/theory-cloud/TableTheory/commit/9e87b7277acf89e437a6fa69554044d7289b98da))
* **docs:** link the shared conformance table with an absolute URL ([94bffc1](https://github.com/theory-cloud/TableTheory/commit/94bffc1c1dd0b32f7528cc48a1d5236635a8343c))
* **py:** move the item preflight off Table to a module function ([c3c92e2](https://github.com/theory-cloud/TableTheory/commit/c3c92e2ccc120e6ef59a1121bc5a952e15a57829))
* **py:** preserve transaction cancellation reasons and cause ([66ea044](https://github.com/theory-cloud/TableTheory/commit/66ea0445987de9bc359febe22bc89d3a24af3668))
* **ts:** classify transaction cancellations by reason code ([6a45543](https://github.com/theory-cloud/TableTheory/commit/6a45543fd5c1e017b48f625cdefacc8b5ab6049a))


### Reverts

* **changelog:** drop the release-generated changelog edit from this PR ([1dac474](https://github.com/theory-cloud/TableTheory/commit/1dac4749a8cf52653667e5b27b7b8b0909d44483))

## [5.0.0](https://github.com/theory-cloud/TableTheory/compare/v5.0.0-rc...v5.0.0) (2026-10-12)

Stable promotion of `v5.0.0-rc`.


## [5.0.0-rc](https://github.com/theory-cloud/TableTheory/compare/v4.0.2...v5.0.0-rc) (2026-10-12)


### ⚠ BREAKING CHANGES

* **module:** Go consumers must update their module requirement and every TableTheory import from github.com/theory-cloud/tabletheory/v4 to github.com/theory-cloud/tabletheory/v5; see docs/migration/v5.md.
* **ts:** On the default TheorydbClient (numberUnmarshalMode 'string'), typed items now declare DynamoDB N fields as string and NS fields as string[] instead of number/number[]. Consumer code that relied on the previous number declaration must handle the decimal strings directly, or construct the client/repository in 'number' mode (TheorydbClient<'number'>).

### Bug Fixes

* **ci:** bump theorycloud publish awscurl pin to 0.44 ([485eb98](https://github.com/theory-cloud/TableTheory/commit/485eb98a80b2059b14e8822b223207dd0e1c6b29))
* **ci:** bump theorycloud publish awscurl pin to 0.44 for --fail-with-body ([e26a07f](https://github.com/theory-cloud/TableTheory/commit/e26a07f234bb12738e55cf2dbd1fcc89e6f6dda3))
* **ci:** make Lambda runtime discovery comment-aware ([d9fe7f4](https://github.com/theory-cloud/TableTheory/commit/d9fe7f4266d427bb7d5a20c62a69d9b3f0927da9))
* **ci:** remove reusable release-hygiene bootstrap and PR-head verifier bypasses ([520d502](https://github.com/theory-cloud/TableTheory/commit/520d50298436ffd8393e2b791bdadbdd596a9d1a))
* **ci:** run release hygiene from a single head checkout ([92eeb2c](https://github.com/theory-cloud/TableTheory/commit/92eeb2ca85b0964435ec3dd8059320cf6c233323))
* **ci:** run release hygiene from a single head checkout ([cd67bbc](https://github.com/theory-cloud/TableTheory/commit/cd67bbc1f048e293ff579d537c68aed14d38e7bd))
* **ci:** stage release pages in temp files instead of jq arguments ([6ea0e9a](https://github.com/theory-cloud/TableTheory/commit/6ea0e9a389a6f05c3a55e919b831df25200a7cc4))
* **ci:** stage release pages in temp files instead of jq arguments ([6f426e3](https://github.com/theory-cloud/TableTheory/commit/6f426e3c12422df470fbc96ed15c6020849a49ca))
* **cli:** refuse symlinked output paths and unsafe parents ([11e7316](https://github.com/theory-cloud/TableTheory/commit/11e731600873debb711649362c31e2890ea3522d))
* **contract:** fail item_absent closed on unexpected operation errors ([e976ad3](https://github.com/theory-cloud/TableTheory/commit/e976ad33ebb0e27cb4db70d00f8def8318f79a40))
* **count:** bound Python count helpers and keep optimizer diagnostics honest ([b57f54b](https://github.com/theory-cloud/TableTheory/commit/b57f54bceda9ba87966c3284c1fd24d78aeadbca))
* **deps:** bump AWS SDKs to 3.1146.0, eslint, and aws-cdk-lib 2.273.0 ([ed1d5cb](https://github.com/theory-cloud/TableTheory/commit/ed1d5cb9069a037a9447406d94b524cfc50518c7))
* **deps:** bump contract runner client-dynamodb to ^3.1146.0 ([84a4cdb](https://github.com/theory-cloud/TableTheory/commit/84a4cdbc032e3ac5372d928b07854202aadd068a))
* **deps:** bump example SDKs and aws-cdk-lib; retire brace exception ([b9b6e12](https://github.com/theory-cloud/TableTheory/commit/b9b6e122930a805258e5410ae90fa96bceec69da))
* **deps:** bump ts runtime AWS SDKs, eslint, and @types/node ([f97d74d](https://github.com/theory-cloud/TableTheory/commit/f97d74d6a4307c181a2849211c244d9d000dd7d9))
* **dms:** emit json.Number for decimal_string so N round-trips ([b449191](https://github.com/theory-cloud/TableTheory/commit/b449191e4fc07714f22be8d81c60e8585c364105))
* **docs:** document supported Go GSI index tags in the vocabulary ([a48cf27](https://github.com/theory-cloud/TableTheory/commit/a48cf27aaf5a9e23e52361445544ab0580e98ff0))
* **examples:** bind DynamoDB Local to loopback ([5983bd4](https://github.com/theory-cloud/TableTheory/commit/5983bd42d5e0f84dc860b9a162c8915c7f0e71e9))
* **examples:** bound blog SMTP delivery by the caller context ([29ff63d](https://github.com/theory-cloud/TableTheory/commit/29ff63d77a23c8adc23614a331fca4191b514377))
* **examples:** bound pre-auth API-key work, loopback local server ([6b53aa9](https://github.com/theory-cloud/TableTheory/commit/6b53aa916a89d2d0a8e30b03ad11362accaa449a))
* **examples:** fail closed on unarchivable TTL stream records ([9c63c8f](https://github.com/theory-cloud/TableTheory/commit/9c63c8ff9e73fd9b037a83c8d9b12601b3d10733))
* **examples:** verify DynamoDB Local with a protocol check ([ac39545](https://github.com/theory-cloud/TableTheory/commit/ac3954555b70452539b68dcad146934123c21e0b))
* **fakes:** enforce combined conditions and DynamoDB COUNT/Limit semantics ([92d991e](https://github.com/theory-cloud/TableTheory/commit/92d991e17feaafebf5796ac7fa847b91593a7469))
* **guard:** M2 detector and contract-harness integrity (TTSEC2-M2) ([10b1aa4](https://github.com/theory-cloud/TableTheory/commit/10b1aa404e69262cb505b1e4c96602c7f0648ef0))
* **guard:** resolve Python launch bindings at statement position ([ccb40bc](https://github.com/theory-cloud/TableTheory/commit/ccb40bc434c620d74b13bde964ce76da4d99c4a9))
* **guard:** run a labeled transfer's finally in the TS detector ([c7f21b6](https://github.com/theory-cloud/TableTheory/commit/c7f21b69f8de3e29e756769f3453abcd2277c397))
* **guard:** stop awaiting a timer handle from joining it in the TS detector ([b08a7c3](https://github.com/theory-cloud/TableTheory/commit/b08a7c3088c1df12aa0bf10318a79f00109dc536))
* **lease:** round lease expiry up so a lease holds for its full duration ([7c691d3](https://github.com/theory-cloud/TableTheory/commit/7c691d3ff0e2b19688d93fb1333adcf410758875))
* **marshal:** honor anonymous-embed exclusion tags on encode and decode ([c685185](https://github.com/theory-cloud/TableTheory/commit/c685185b9af1113c45314c037862baeaa2566b75))
* **model:** reject present-but-empty attribute-name overrides ([e446b21](https://github.com/theory-cloud/TableTheory/commit/e446b21ceac74315389ed8838a47b861df8fb984))
* **model:** resolve protected attributes canonically ([30c1a74](https://github.com/theory-cloud/TableTheory/commit/30c1a746b95e9cea2cb9d3f09135032fd0d18606))
* **module:** adopt the /v5 Go module path ([946a47e](https://github.com/theory-cloud/TableTheory/commit/946a47e74df1d96f79a9ca3806cb8e9cf7ee5fe7))
* **multiaccount:** bound partner refresh to the requested partner with backoff ([29d1e0e](https://github.com/theory-cloud/TableTheory/commit/29d1e0e7f7d5055c3045ec166c586093800db132))
* **multiaccount:** harden partner refresh failure path ([9f87d5e](https://github.com/theory-cloud/TableTheory/commit/9f87d5e6e6098fcc9daba492a26e440f335f96e4))
* **protection:** free the request slot before body cleanup ([550b251](https://github.com/theory-cloud/TableTheory/commit/550b25156e66049c777963a9189418d06e7252a0))
* **py:** bound derived-key numeric coercion to DynamoDB bounds ([68b1709](https://github.com/theory-cloud/TableTheory/commit/68b170982296b24e0685fb7e0957db53d566a802))
* **py:** make auto_migrate target-encryption aware ([4f83dd4](https://github.com/theory-cloud/TableTheory/commit/4f83dd45dab5e47c07d763ed7620829ef36372c5))
* **py:** return from the bounded count loop to satisfy the type checker ([82cbdb5](https://github.com/theory-cloud/TableTheory/commit/82cbdb55b2a9b1d50e8c6faf4b9dccee93cf5db4))
* **py:** use field names in the GSI pattern and correct scaffold docs ([643c961](https://github.com/theory-cloud/TableTheory/commit/643c961726b9f989e972439d7ad8f92ea040afef))
* **query:** keep a live base context across QueryTimeout changes ([5f4cb39](https://github.com/theory-cloud/TableTheory/commit/5f4cb394bf566b19714f9db0ba972ee4f12d8523))
* **query:** keep cancellation live across QueryTimeout changes ([d3c2fcf](https://github.com/theory-cloud/TableTheory/commit/d3c2fcf2b4d62d0905493115dd3a8df1cde1934d))
* **query:** preserve QueryTimeout deadline across WithCancellation ([f8574d4](https://github.com/theory-cloud/TableTheory/commit/f8574d4b6c822062ee7ec99f680fa967855ac905))
* **query:** report only unprocessed batch requests as failed ([b32c544](https://github.com/theory-cloud/TableTheory/commit/b32c54483095dabd1e2153f29d821249836784a5))
* **query:** tokenize version-conflict classification ([03ffedd](https://github.com/theory-cloud/TableTheory/commit/03ffedd842425b18daa301bf1ec213a44973ecb1))
* **query:** use the deadline zero value instead of a companion flag ([46f6f68](https://github.com/theory-cloud/TableTheory/commit/46f6f68c660f3681094b20c54705eac1d50c93e9))
* **release:** harden release and publishing trust boundaries ([87d7ef5](https://github.com/theory-cloud/TableTheory/commit/87d7ef5e32d05a432b7ca480f6d77dc5747fccfe))
* **release:** harden release and publishing trust boundaries (P-THE-109 M0 / THE-3226–THE-3230) ([73d930e](https://github.com/theory-cloud/TableTheory/commit/73d930eeeb1db388d6336f6549f1f9e8c2539846))
* **releasestate:** bind release-state events and evidence digests ([d716f2c](https://github.com/theory-cloud/TableTheory/commit/d716f2cb46d10d5548cf4ff205fd05fe8825c956))
* **schema:** refuse unsafe encryption in data-copying migrations ([26797a7](https://github.com/theory-cloud/TableTheory/commit/26797a7c99df412e644163ea3daf873c8979c989))
* **scripts:** aggregate coverage by unique source segment ([4b420ca](https://github.com/theory-cloud/TableTheory/commit/4b420ca84bf5f644a8231ade0efd67492bd177a5))
* **scripts:** init generated-key verifier before top-level await ([98a4f78](https://github.com/theory-cloud/TableTheory/commit/98a4f78e47e633986eccd2d0efe9db1d9498abda))
* **scripts:** make npm audit range comparator strict and fail-closed ([88df877](https://github.com/theory-cloud/TableTheory/commit/88df877a2a68a3a1aef6e06f83ae7e5644036202))
* **security:** fail closed in the Pages gate and released baseline ([bc9f98c](https://github.com/theory-cloud/TableTheory/commit/bc9f98cb5c12d541bc89e446df66bd58cf51ba50))
* **security:** fail closed on encrypted json:"-" fields and preserve exact numbers ([4b70a7a](https://github.com/theory-cloud/TableTheory/commit/4b70a7a0641b07be203fc62a5fb66e480dc5a827))
* **security:** harden release and Pages publication trust boundaries ([3e5b6f4](https://github.com/theory-cloud/TableTheory/commit/3e5b6f40c71271b4f753f0a11cf19359babdcf27))
* **security:** harden release and Pages publication trust boundaries ([f80ebc0](https://github.com/theory-cloud/TableTheory/commit/f80ebc0ef8d8ae6c16656943fd84df6eb01b7702))
* **security:** M3 transaction, lease, query and fake semantics ([3295d7e](https://github.com/theory-cloud/TableTheory/commit/3295d7ebff974f9829c09cfe5f9dca667471d549))
* **security:** P-THE-109 M1 data and authorization boundaries (THE-3231–THE-3235) ([3d2dc6b](https://github.com/theory-cloud/TableTheory/commit/3d2dc6b730b44eefe67c7ec85a344a6e0841f8e1))
* **security:** P-THE-109 M2 cross-runtime encryption and exact-number semantics (THE-3236–THE-3240) ([5362fdc](https://github.com/theory-cloud/TableTheory/commit/5362fdceb5d41720a84aac9ee5ff7603ecb2b5f3))
* **security:** preserve registered custom converters on struct and transaction reads ([585db63](https://github.com/theory-cloud/TableTheory/commit/585db63e29f5fb250023e6a443a839519a7ca8b1))
* **security:** reject unsafe DMS names before Go code generation ([ab45ead](https://github.com/theory-cloud/TableTheory/commit/ab45ead051e5673174365e5be5eca73ed35317a2))
* **security:** TableTheory M4 release-state parity, documentation and local safety ([571b7ab](https://github.com/theory-cloud/TableTheory/commit/571b7abf0c8b0563552f02f3dd869284f95b4cbd))
* **security:** TTSEC2-M0 bound slot cleanup, immutable ISR binding, canonical write policy ([c179a5d](https://github.com/theory-cloud/TableTheory/commit/c179a5d4cbdee770eb325fe10641427a9b070537))
* **security:** TTSEC2-M1 runtime result and type integrity (5 findings) ([0bed24d](https://github.com/theory-cloud/TableTheory/commit/0bed24def432d44ca181a0f28056237b7e183fe2))
* **security:** TTSEC2-M4 tooling, example, and local-development safety ([99385da](https://github.com/theory-cloud/TableTheory/commit/99385dafdd29370f947151acb6db053fc4fad85a))
* **security:** verify signed tokens and bind org access in multi-tenant example ([9a743c7](https://github.com/theory-cloud/TableTheory/commit/9a743c7a1a2f8a3eda46b2be84244eaf80804fa0))
* **session:** preserve KMS credentials for injected-client sessions ([fcb44ee](https://github.com/theory-cloud/TableTheory/commit/fcb44ee676e92e48bf129fbc04ba6e168ad9adf2))
* **testkit:** make stateful fake composite keys injective ([581636d](https://github.com/theory-cloud/TableTheory/commit/581636d9b8029de3ec63a80b8005602a9009a615))
* **ts:** aggregate exact decimal-string numbers numerically ([43d6419](https://github.com/theory-cloud/TableTheory/commit/43d6419857c5c2e571995eb8a0ad56e1c8145fee))
* **ts:** align typed numeric reads with the number-unmarshal mode ([1ad13a3](https://github.com/theory-cloud/TableTheory/commit/1ad13a36432c386898a57370cb08b01fba2f0201))
* **ts:** bind ISR store models to a private client ([78c4457](https://github.com/theory-cloud/TableTheory/commit/78c4457be3377dc0329171de132ff202d2a4a10f))
* **ts:** bound aggregate decimals and require exact results ([96c2773](https://github.com/theory-cloud/TableTheory/commit/96c277322215d3ca34a189a6629b23e43195f856))
* **ts:** declare default typed N/NS reads as string ([eccae47](https://github.com/theory-cloud/TableTheory/commit/eccae47ddc2905b63ec65d0b3696f7b5a53bf832))
* **ts:** linear decimal parsing and lossy-number aggregate parity ([5a40a7e](https://github.com/theory-cloud/TableTheory/commit/5a40a7e22ad9bedcf9c40b8bf975f850f94725af))
* **ts:** make autoMigrate target-encryption aware ([4fd0d90](https://github.com/theory-cloud/TableTheory/commit/4fd0d9051a9b56ed3a30ff934a800c56440feee6))
* **ts:** retain and compare DMS attribute format ([16b2d13](https://github.com/theory-cloud/TableTheory/commit/16b2d1372ee9d9a02ca131b4f2d463e74c7aa909))

## [4.0.2](https://github.com/theory-cloud/TableTheory/compare/v4.0.2-rc...v4.0.2) (2026-10-09)

Stable promotion of `v4.0.2-rc`.


## [4.0.2-rc](https://github.com/theory-cloud/TableTheory/compare/v4.0.1...v4.0.2-rc) (2026-10-09)


### Bug Fixes

* **deps:** consolidate eligible Dependabot npm updates within the cooldown ([7446460](https://github.com/theory-cloud/TableTheory/commit/7446460c2ade97f013745954df63300a03e2b8e9))
* **deps:** consolidate eligible Dependabot npm updates within the cooldown ([bca7f63](https://github.com/theory-cloud/TableTheory/commit/bca7f63b580b40093da2207bbbde08e06f98bc4f))
* **deps:** consolidate open Dependabot npm updates within the cooldown ([1ea975b](https://github.com/theory-cloud/TableTheory/commit/1ea975b9b263c5132e7e04a9bd95b8fb82b5597b))
* **deps:** consolidate open Dependabot npm updates within the cooldown ([6a33a88](https://github.com/theory-cloud/TableTheory/commit/6a33a88d1642732187cc383d077c0c3ec56de7bc))
* **deps:** refresh eligible AWS SDK, globals, and cryptography pins ([699bc04](https://github.com/theory-cloud/TableTheory/commit/699bc0435a3c646a600964c1faf0d929a3989e1d))
* **deps:** refresh eligible AWS SDK, globals, and cryptography pins ([6ab3238](https://github.com/theory-cloud/TableTheory/commit/6ab32385b541a47fb4075f5d59a5d0ae0f3c3cae))
* **errors:** expose the empty-update condition as a matchable sentinel ([fdefedc](https://github.com/theory-cloud/TableTheory/commit/fdefedc454433b3914bab36b9f086f962fb2c325))
* **go:** inline reflect.Ptr as reflect.Pointer for Go 1.27 ([ebca0b1](https://github.com/theory-cloud/TableTheory/commit/ebca0b13e3328633f6a27d3f676dd748a41932e1))
* **go:** move every module to Go 1.27.2 ([d8a911b](https://github.com/theory-cloud/TableTheory/commit/d8a911b1d25441995d4414cce5ec5a48594b6a69))
* **go:** move TableTheory to Go 1.27.2 and clear the 2026-10-08 Go advisories ([50799f7](https://github.com/theory-cloud/TableTheory/commit/50799f77a17d20200de8ffb97b5e30ac92347f8b))
* **model:** guard nil models in the registry instead of panicking ([350576e](https://github.com/theory-cloud/TableTheory/commit/350576ede5ef1f90d8d5eb1b0190249c6a9b9854))
* **security:** pin govulncheck to v1.8.0 for Go 1.27.2 analysis ([f60bc6f](https://github.com/theory-cloud/TableTheory/commit/f60bc6f9e335160eeb50e34b0f26e73bf2c5e325))
* **security:** resolve gosec findings reported under Go 1.27 ([c2231ce](https://github.com/theory-cloud/TableTheory/commit/c2231ce65d3b3321354fce998a6d52ed3a1991d8))
* **transaction:** legacy transaction index-path correctness, nil guards, and test hygiene ([4b62278](https://github.com/theory-cloud/TableTheory/commit/4b622782b24ad23dca3ef9cecc602b45445a332f))
* **transaction:** read the legacy delete version through its index path ([51063f1](https://github.com/theory-cloud/TableTheory/commit/51063f1123a7d6e45aa6b60fd0f2e34f48c4daac))
* **transaction:** resolve model fields through metadata index paths ([c2a7b8b](https://github.com/theory-cloud/TableTheory/commit/c2a7b8b841194e46e4f86df1ca920688a99e3d80))

## [4.0.1](https://github.com/theory-cloud/TableTheory/compare/v4.0.1-rc...v4.0.1) (2026-10-04)

Stable promotion of `v4.0.1-rc`.


## [4.0.1-rc](https://github.com/theory-cloud/TableTheory/compare/v4.0.0...v4.0.1-rc) (2026-10-04)


### Bug Fixes

* **py:** update urllib3 2.7.0 -&gt; 2.8.0 (PYSEC-2026-4175/4176/4177) ([0e8e69f](https://github.com/theory-cloud/TableTheory/commit/0e8e69fdf9a7ed9acb8965f47e4eb5de1d4c4c3b))
* **security:** bump brace-expansion to 5.0.12 in ts dev tree ([f2aac4c](https://github.com/theory-cloud/TableTheory/commit/f2aac4ce7a9bb8fdf6a8e06d5e31f75ce6aeeb67))
* **security:** clear the ts brace-expansion advisory and run the cooldown dependency wave ([1206474](https://github.com/theory-cloud/TableTheory/commit/1206474e344e22dc0ad9b415077650e99841302d))
* **security:** govern the AWS-bundled brace-expansion SEC-2 exception ([1d34dca](https://github.com/theory-cloud/TableTheory/commit/1d34dcae730ee25fd6191dd3fdbd437bc2f1ae6e))
* **security:** write one npm audit JSON per retry attempt ([80b0dd3](https://github.com/theory-cloud/TableTheory/commit/80b0dd31c96b34af16927b532db56d22d3c55cbf))

## [4.0.0](https://github.com/theory-cloud/TableTheory/compare/v4.0.0-rc...v4.0.0) (2026-09-28)

Stable promotion of `v4.0.0-rc`.


## [4.0.0-rc](https://github.com/theory-cloud/TableTheory/compare/v3.1.0...v4.0.0-rc) (2026-09-28)


### ⚠ BREAKING CHANGES

* **module:** the Go module path is now github.com/theory-cloud/tabletheory/v4. Go consumers must change their module requirement and every TableTheory import from github.com/theory-cloud/tabletheory/v3 to github.com/theory-cloud/tabletheory/v4, then run `go get github.com/theory-cloud/tabletheory/v4@v4.0.0` and `go mod tidy`. See docs/migration/v4.md.
* **examples:** the template no longer declares BillingFunction, AuditCleanupFunction, or the StripeSecretKey parameter, and deploys on provided.al2023. A deployment that supplied StripeSecretKey must drop it.
* **protection:** MemoryMonitor.Start, MemoryMonitor.Stop, ResourceProtector.StartMemoryMonitoring, ResourceProtector.StopMemoryMonitoring, and ResourceLimits.MemoryCheckInterval are removed and replaced by MemoryMonitor.Sample, ResourceProtector.SampleMemory, and ResourceProtector.SetMemoryAlertCallback.

### Features

* **ci:** guard action pinning, lockfile installs and trigger parity ([b7395f3](https://github.com/theory-cloud/TableTheory/commit/b7395f309dde5f7495a715a2b8a42a57a5aa62d0))
* detached-work guard parity and [#621](https://github.com/theory-cloud/TableTheory/issues/621) review follow-ups ([3088552](https://github.com/theory-cloud/TableTheory/commit/30885522e1eae4b801e104cdca5e6218b8bec529))
* **gov:** enforce report validity and materialization fidelity ([250b1fc](https://github.com/theory-cloud/TableTheory/commit/250b1fceb2da0e1c2bb49db852db43e9146c46ba))
* **multi-tenant:** add the Lambda entrypoint the template declares ([e9f86c6](https://github.com/theory-cloud/TableTheory/commit/e9f86c6bf204753232494cdffec7fc8013fc426a))


### Bug Fixes

* **blog:** bound webhook retry backoff by ctx ([efb58e1](https://github.com/theory-cloud/TableTheory/commit/efb58e1e9dd9b252ce97246e073f55e02134408f))
* **ci:** conform CI to software_repo_gov_infra (R-F1/R-G2/R-G3/R-G5) ([f019a83](https://github.com/theory-cloud/TableTheory/commit/f019a8362637049e6296aaea51b9d05ab21a86cf))
* **ci:** disable install scripts for lockfile installs ([10f0671](https://github.com/theory-cloud/TableTheory/commit/10f0671d94f24c85f6956eaa37ce8186cfdcdc0b))
* **ci:** exercise promotion-path jobs on staging pull requests ([3997c91](https://github.com/theory-cloud/TableTheory/commit/3997c91f9f7a806ec84fe059004b94657398aa0e))
* **ci:** provision the detached-work guard toolchain in unit-cover ([d6a9924](https://github.com/theory-cloud/TableTheory/commit/d6a9924a0ce3f0598e46a887c2856894fccc4159))
* **ci:** provision the pinned scanner toolchain for unit-cover ([995382c](https://github.com/theory-cloud/TableTheory/commit/995382c7bac4e6033183e5822b67dc1286f0b92f))
* **ci:** retry transient upstream failures in dependency scans ([9533889](https://github.com/theory-cloud/TableTheory/commit/953388900808a4464557868432a2da92cbc53735))
* **ci:** run unit-cover on pull requests targeting staging ([e7d1e51](https://github.com/theory-cloud/TableTheory/commit/e7d1e51a2ef5ad989cf15bab576a8ced176f9b13))
* **deps:** bump the pinned GitHub Actions group ([b682a5c](https://github.com/theory-cloud/TableTheory/commit/b682a5caba72b0fa323afe2ded234ccd88d4c44e))
* **deps:** consolidate the open Dependabot Go module groups ([cf06314](https://github.com/theory-cloud/TableTheory/commit/cf063146052aa1a53082ca859a7e037356e85576))
* **deps:** fold the open Dependabot updates into one release-eligible PR ([a086ff0](https://github.com/theory-cloud/TableTheory/commit/a086ff0ce4b1439d8385c78a3f1632c9b4475211))
* **examples:** delete the blog notification async path ([f1a6184](https://github.com/theory-cloud/TableTheory/commit/f1a61843a8f8c05c5fc7f7b9a074b53e7f790d28))
* **examples:** keep Lambda work inside the invocation ([f771685](https://github.com/theory-cloud/TableTheory/commit/f77168599592a780e0cb1b946a1bad15100f8bfe))
* **examples:** make the multi-tenant template deployable and honest ([b6ed7f7](https://github.com/theory-cloud/TableTheory/commit/b6ed7f756d46aa8fdb97d0ae22490ef5bf5ddbdb))
* **examples:** remove the payment example's retry worker ([459bf91](https://github.com/theory-cloud/TableTheory/commit/459bf919d26f2048461842ba0a7585e185d4c75f))
* **gov:** cover every materialized surface with a gitignore rule ([d4e44ee](https://github.com/theory-cloud/TableTheory/commit/d4e44eed71dccdcac8d3fbe23b14737bb6aa2094))
* **guard:** fail the TypeScript scan on a source the parser rejects ([5546368](https://github.com/theory-cloud/TableTheory/commit/554636819dc5dc8bbbcbddd8b9279eb20ce0ec64))
* **guard:** parse a TypeScript labeled break as loop control ([3dbf164](https://github.com/theory-cloud/TableTheory/commit/3dbf164eff8e860ca351a9fb6170d6545cb97957))
* **guard:** recognize detached work beyond one Python function scope ([a832731](https://github.com/theory-cloud/TableTheory/commit/a83273100c882a062f6be408da447b069162b759))
* **guard:** scan TypeScript class field initializers and static blocks ([37da70f](https://github.com/theory-cloud/TableTheory/commit/37da70f221e4d80a51d31dd2776b2e99cd2e0408))
* **lambda:** no work may outlive a Lambda invocation (remove the detached cold-start pre-warm) ([5784b5a](https://github.com/theory-cloud/TableTheory/commit/5784b5a2f54901cbef91eee2c7939b9f188fe96a))
* **lambda:** refresh multi-account sessions on the invocation path ([3673e7b](https://github.com/theory-cloud/TableTheory/commit/3673e7beb051a77632a1a46537bde19f69c45112))
* **lambda:** remove the detached cold-start pre-warm ([71a0aa1](https://github.com/theory-cloud/TableTheory/commit/71a0aa1022cb1e5075b078f0ba4cd5c45ec1ead4))
* **module:** adopt the /v4 Go module path ([467d73d](https://github.com/theory-cloud/TableTheory/commit/467d73d0a405c2879d72eb0e072898680e77fc9f))
* **payment:** state what the export endpoint actually does ([124fa10](https://github.com/theory-cloud/TableTheory/commit/124fa10464e6ebc8141f8e243237593c1097a45f))
* **protection:** join the request-body reader on timeout ([d6da564](https://github.com/theory-cloud/TableTheory/commit/d6da56488d5727594cabf6ddc932e57faa53754c))
* **protection:** keep MemoryMonitor fields fieldalignment-clean ([97ad004](https://github.com/theory-cloud/TableTheory/commit/97ad004c4d68e202cceeb98e6f6910f7b2c151a5))
* **protection:** sample memory on demand instead of a ticker goroutine ([02b0386](https://github.com/theory-cloud/TableTheory/commit/02b038648106f20742c17fc2ee0d2f475db42fe7))
* **py:** import the runtime module with a single style ([f4cb663](https://github.com/theory-cloud/TableTheory/commit/f4cb6634b8c5fbe0fa2c2936021a4216afaab2bf))
* **query:** join every fan-out worker on all paths ([2c58ee2](https://github.com/theory-cloud/TableTheory/commit/2c58ee22401730656e86e890fb0886c374ab92b3))
* **test:** make the memory monitor threshold tests deterministic ([3e6358c](https://github.com/theory-cloud/TableTheory/commit/3e6358c959c55c811b10ff5bcb14f81c77529e2e))
* **test:** make the memory monitor threshold tests deterministic ([2bc8a6e](https://github.com/theory-cloud/TableTheory/commit/2bc8a6e4fd9c4bd17d5ab34a92fb687ba2a180aa))

## [3.1.0](https://github.com/theory-cloud/TableTheory/compare/v3.1.0-rc...v3.1.0) (2026-09-23)

Stable promotion of `v3.1.0-rc`.


## [3.1.0-rc](https://github.com/theory-cloud/TableTheory/compare/v3.0.7...v3.1.0-rc) (2026-09-23)


### Features

* **ci:** read bracket, second-hop, and destructured runtime receivers ([0bd7915](https://github.com/theory-cloud/TableTheory/commit/0bd791593899813c01a2cd2385a758fa9f1b1194))


### Bug Fixes

* **ci:** close the prerelease false pass in the npm engines-floor gate ([daad28c](https://github.com/theory-cloud/TableTheory/commit/daad28c122c60d639f19d78188e207e70e1768f0))
* **ci:** cross-check the audited lockfile set against the npm audit scanner ([4e53e64](https://github.com/theory-cloud/TableTheory/commit/4e53e64e5eaeeda53794e2d702d60864aef7e800))
* **ci:** gate deprecated Lambda runtimes and judge npm lockfile roots ([221dfa1](https://github.com/theory-cloud/TableTheory/commit/221dfa1e8077698ab9e5ab5c93231c3f8d0c11e5))
* **ci:** gate deprecated Lambda runtimes on the CDK surfaces we ship ([593497c](https://github.com/theory-cloud/TableTheory/commit/593497ca45d5f24f31944a1c2c8a0bafadf77a90))
* **ci:** gate npm lockfiles on the Node engines floor ([c260117](https://github.com/theory-cloud/TableTheory/commit/c26011728cdda9f55fb1f8368523743591d6759d))
* **ci:** give the Lambda runtime coverage walk one error boundary ([a94da48](https://github.com/theory-cloud/TableTheory/commit/a94da4815f5668d27b8836e368cf12df513fc8fd))
* **ci:** harden the Lambda runtime deprecation gate (parity port of FaceTheory [#481](https://github.com/theory-cloud/TableTheory/issues/481)) ([1927e35](https://github.com/theory-cloud/TableTheory/commit/1927e3574872fc43b525f53ce72ab2192dc75def))
* **ci:** harden the npm engines-floor matcher and fold in the Dependabot queue ([9690c60](https://github.com/theory-cloud/TableTheory/commit/9690c60c201dcfe37fce12432266eccb55baa757))
* **ci:** judge npm lockfile roots against the Node floor ([63f72e2](https://github.com/theory-cloud/TableTheory/commit/63f72e2407ee49bae28afc2068bc8d5e1cec0c54))
* **ci:** pin the TypeScript Node matrix ([726bf41](https://github.com/theory-cloud/TableTheory/commit/726bf413a38e6aca1817bff2c1ceb408c4ef8281))
* **ci:** recognize Node 22 release verifier policy ([48d1a47](https://github.com/theory-cloud/TableTheory/commit/48d1a476d96ad77d12f2b3a4a1d8432691a8ff21))
* **ci:** recognize Node 22 release verifier policy ([3461384](https://github.com/theory-cloud/TableTheory/commit/3461384aba65bda73b3f83bd8f6a20440ec272c3))
* **ci:** scope the runtime-literal rule to fromString receivers ([2e4beb6](https://github.com/theory-cloud/TableTheory/commit/2e4beb680f63bf009346a1625e9eada4f53f1271))
* **ci:** wire the Lambda runtime gate into SEC-2 and pin its scope ([8d54cf9](https://github.com/theory-cloud/TableTheory/commit/8d54cf92864bfcea51a2f2e0cfadbd095fb284e9))
* **security:** raise the Node floor to 22 ([1f269ac](https://github.com/theory-cloud/TableTheory/commit/1f269ac5f5cbec91a77a04f30084c866d3f8d9f3))
* **security:** raise the Node floor to 22 and gate engines drift ([6ff3891](https://github.com/theory-cloud/TableTheory/commit/6ff38913abb3fa7cf13f6396afa86c0ac9a4672f))
* **security:** remove the stale cdk-multilang brace-expansion exception ([8bda8b6](https://github.com/theory-cloud/TableTheory/commit/8bda8b61e7f564fefe8db6e6d77e24a6e247f673))

## [3.0.7](https://github.com/theory-cloud/TableTheory/compare/v3.0.7-rc...v3.0.7) (2026-09-20)

Stable promotion of `v3.0.7-rc`.


## [3.0.7-rc](https://github.com/theory-cloud/TableTheory/compare/v3.0.6...v3.0.7-rc) (2026-09-20)


### Bug Fixes

* **ci:** harden pages dispatch tag validation and cache surface ([4eb3696](https://github.com/theory-cloud/TableTheory/commit/4eb3696ccc7b9a84f5653a19ab8f1e50983de3a6))
* **ci:** include Python wheel and sdist in release SHA256SUMS ([71028f8](https://github.com/theory-cloud/TableTheory/commit/71028f80d5915762fdbf19ff3233cc6e46d50709))
* **ci:** include Python wheel and sdist in release SHA256SUMS ([872c11b](https://github.com/theory-cloud/TableTheory/commit/872c11b54f3e81b814c728387e590944d79c79e7))
* **ci:** include TS tarball in release SHA256SUMS ([de86a9d](https://github.com/theory-cloud/TableTheory/commit/de86a9da0efd026f288eaed7b6ec51c982c04251))
* **ci:** include TS tarball in release SHA256SUMS ([1a599db](https://github.com/theory-cloud/TableTheory/commit/1a599db3cace4ea7b608e16b57ef8cf669c515ee))
* **ci:** pass repo context to pages deploy dispatch ([4ec1fc4](https://github.com/theory-cloud/TableTheory/commit/4ec1fc4e980cc9737a13ff67196a146bf6e8cee2))
* **ci:** pass repo context to pages deploy dispatch ([d9a1fdf](https://github.com/theory-cloud/TableTheory/commit/d9a1fdfb3bdc531cf4fd0e9bed9e0f7288220a8d))
* **ci:** restore automated Pages deploys stamped from the deployed release ([aaabd3a](https://github.com/theory-cloud/TableTheory/commit/aaabd3a3a6a482b1e43a61b4181de18930b38094))
* **ci:** restore pages deploy correctness after adversarial review ([584c2e0](https://github.com/theory-cloud/TableTheory/commit/584c2e060fd6209fd196a4418c0b6e5add2c8a71))
* **deps:** apply consolidated dependency sweep 2026-09 ([47092ed](https://github.com/theory-cloud/TableTheory/commit/47092ed305821589a3c94b0a17fe9562b4cb7099))
* **deps:** apply dependency wave 2026-08 ([9d3b363](https://github.com/theory-cloud/TableTheory/commit/9d3b36302272257645947fa749cb2d2675ef0c38))
* **deps:** apply dependency wave 2026-08 ([3d8d33b](https://github.com/theory-cloud/TableTheory/commit/3d8d33b38f67f430d8729d63dd14c99626d7cc5f))
* **deps:** bump github-actions group ([137ae42](https://github.com/theory-cloud/TableTheory/commit/137ae42c9b640a5e3e1b97dccd012dd5562c8c7f))
* **deps:** bump go dependency groups (root, contract-runner, examples) ([6d8029a](https://github.com/theory-cloud/TableTheory/commit/6d8029a68d1f4a579cd6997374b8e275c14762b2))
* **deps:** bump python example dependencies ([0027599](https://github.com/theory-cloud/TableTheory/commit/00275996b35bbd02694b6ac8858977c25417a6c2))
* **deps:** bump ts runtime, contract-runner, and npm example groups ([1471b91](https://github.com/theory-cloud/TableTheory/commit/1471b91ffdc4f8ab3e1571a2dc8582ffcd9f7ed1))
* **gov-infra:** treat gov-rubric-report.json as generated untracked evidence ([9e68f0b](https://github.com/theory-cloud/TableTheory/commit/9e68f0b00328638e9775ba288f73410ba9431fe5))
* **gov-infra:** untrack generated rubric report evidence ([c469d63](https://github.com/theory-cloud/TableTheory/commit/c469d638afef7205fe003604c2492f6e8591f1e4))
* **query:** make BatchCreateWithResult report per-item results ([9cc4eb0](https://github.com/theory-cloud/TableTheory/commit/9cc4eb0e7ff74212e80cbd2b9f967c12eea60f1c))
* **query:** make BatchCreateWithResult report per-item results ([83d5503](https://github.com/theory-cloud/TableTheory/commit/83d550365b32caf53801271696c17c24144eb704))
* **query:** report only marshaled items on batch-create chunk write failure ([3af0ce7](https://github.com/theory-cloud/TableTheory/commit/3af0ce7c859a849c00de00eb910c58403139a7f9))
* **query:** resolve data race in parallel batch get ([a691c20](https://github.com/theory-cloud/TableTheory/commit/a691c2094f08b5b6aaf8cad84f2e91b0d3755681))
* **query:** resolve data race in parallel batch get ([d43f010](https://github.com/theory-cloud/TableTheory/commit/d43f010659b5bc9a1d4df0ce00f6d5bf3885d777))
* **query:** synchronize BatchUpdate parallel progress callback delivery ([ec20240](https://github.com/theory-cloud/TableTheory/commit/ec20240b0f17d6971718b13c567b1628a9308cda))
* **query:** synchronize remaining parallel batch error callback delivery ([880f895](https://github.com/theory-cloud/TableTheory/commit/880f895bec46eee9284dff2a812bda76bfe41f6d))

## [3.0.6](https://github.com/theory-cloud/TableTheory/compare/v3.0.6-rc...v3.0.6) (2026-08-21)

Stable promotion of `v3.0.6-rc`.


## [3.0.6-rc](https://github.com/theory-cloud/TableTheory/compare/v3.0.5...v3.0.6-rc) (2026-08-21)


### Bug Fixes

* **gov-infra:** align rubric report provenance ([6e47aab](https://github.com/theory-cloud/TableTheory/commit/6e47aab5375cd65e41677c47c78bcf68a6a880b6))
* **gov-infra:** block git checks outside intended root ([c2b4ab6](https://github.com/theory-cloud/TableTheory/commit/c2b4ab6a5ebf961f72f54ddca01901f1d74d6180))
* **release-notes:** document v3.0.5 Create() conditional flip and migration path ([e3d610c](https://github.com/theory-cloud/TableTheory/commit/e3d610caf9e8b3b524ec5b357d8ceff349785eea))
* **release-notes:** document v3.0.5 Create() migration ([2a00809](https://github.com/theory-cloud/TableTheory/commit/2a0080987b69b3ff23833c987a464d892d459d98))
* **release:** carry curated notes through RC and stable workflows ([9e5d0d4](https://github.com/theory-cloud/TableTheory/commit/9e5d0d416742c7c0fee98972e7fc16bb251acd4b))
* **release:** carry curated release notes from docs/release-notes via release workflow ([6a2300b](https://github.com/theory-cloud/TableTheory/commit/6a2300bbe3ca7a2476a207587951cfa5c41031b2))
* **release:** cover bare RC curated notes ([60a86c3](https://github.com/theory-cloud/TableTheory/commit/60a86c32a9c8fd7a6188bece2861d3fe51654d63))

## [3.0.5](https://github.com/theory-cloud/TableTheory/compare/v3.0.5-rc.1...v3.0.5) (2026-08-18)

Stable promotion of `v3.0.5-rc.1`.

### Behavioral change

> **Go `Create()` is now strict.** As of v3.0.5, ordinary `Create()` always applies `attribute_not_exists` to the
> partition key and returns `ErrConditionFailed` when an item with that key already exists. This prevents the silent
> overwrite reported in [#523](https://github.com/theory-cloud/TableTheory/issues/523) and shipped through
> [#524](https://github.com/theory-cloud/TableTheory/pull/524). Consumers that used `Create()` for overwrite/upsert
> semantics must migrate those call sites to `CreateOrUpdate()`; strict-create call sites require no change. See the
> [migration note](docs/migration/v3.0.5-create.md) and the release-visibility follow-up
> [#552](https://github.com/theory-cloud/TableTheory/issues/552).


## [3.0.5-rc.1](https://github.com/theory-cloud/TableTheory/compare/v3.0.4...v3.0.5-rc.1) (2026-08-18)


### Bug Fixes

* **ci:** make branch-release AGENTS.md assertions materialization-aware ([5fb115e](https://github.com/theory-cloud/TableTheory/commit/5fb115ee78af08e17ffe3afbeab9afa1cdbe91f1))
* **ci:** materialization-aware trusted-base verifier selection ([a404dec](https://github.com/theory-cloud/TableTheory/commit/a404dec4e903df4782bbd8d30a2fed6c5e291f2e))
* materialization hygiene — untrack AGENTS.md + branch-release fidelity patch ([1b8dde2](https://github.com/theory-cloud/TableTheory/commit/1b8dde215cb9b9c525737c7357838dacb693919d))
* Release-As 3.0.5 — single-PR restoration of release-please derivation ([416495f](https://github.com/theory-cloud/TableTheory/commit/416495f35b6ad8a6f04c813f0f50f8e027071242))
* Release-As 3.0.5 — single-PR restoration of release-please derivation ([41cfc18](https://github.com/theory-cloud/TableTheory/commit/41cfc184e2a54070229532ce1c90027340e8353b))
* Release-As 3.0.5-rc.1 — RC-shaped footer for release-please derivation ([295700c](https://github.com/theory-cloud/TableTheory/commit/295700c7e24b67be4f7c384bef100d7effbab94a))
* Release-As 3.0.5-rc.1 — RC-shaped footer for release-please derivation ([41694ab](https://github.com/theory-cloud/TableTheory/commit/41694aba87b60ed0f447440ff3aeba9270ab79be))
* release-please resync trigger ([#530](https://github.com/theory-cloud/TableTheory/issues/530)) ([a1ab302](https://github.com/theory-cloud/TableTheory/commit/a1ab3023554978da884389da79f2cfd4841c9808))
* **release:** honor newest Release-As footer ([6dcfd8d](https://github.com/theory-cloud/TableTheory/commit/6dcfd8d16d81af088da52608155e8f52451ab9da))
* **release:** honor newest Release-As footer ([00a2849](https://github.com/theory-cloud/TableTheory/commit/00a28490e3f21c6e48a22ccf0ff580518f3eb19b))
* **release:** restore release-please baseline ([b7c9812](https://github.com/theory-cloud/TableTheory/commit/b7c9812cf9922b0938bd2a08e30e5bea9d78e8ce))
* **release:** restore release-please baseline for v3.0.5 RC ([12b5247](https://github.com/theory-cloud/TableTheory/commit/12b5247807560979277db3fc815a478683acaf12))
* revert empty staging commit a404dec (net-zero placeholder, no content) ([#527](https://github.com/theory-cloud/TableTheory/issues/527)) ([bfc2141](https://github.com/theory-cloud/TableTheory/commit/bfc2141936229ca31fc8cb1447b13a1ad176682d))
* untrack materialized AGENTS.md ([246a716](https://github.com/theory-cloud/TableTheory/commit/246a716320e8953654eafa453415e89b77290779))

## [3.0.4](https://github.com/theory-cloud/TableTheory/compare/v3.0.4-rc...v3.0.4) (2026-08-07)

Stable promotion of `v3.0.4-rc`.


## [3.0.4-rc](https://github.com/theory-cloud/TableTheory/compare/v3.0.3...v3.0.4-rc) (2026-08-07)


### Bug Fixes

* **ci:** restore supported action and AWS SDK dependencies ([1d980ee](https://github.com/theory-cloud/TableTheory/commit/1d980eea128b1f0289255d5ada6e6191c3daa55d))
* **release:** accept first release candidate titles ([c6ae8cb](https://github.com/theory-cloud/TableTheory/commit/c6ae8cbf9c09d0467e3cb6d79613223dfef0498a))
* **release:** accept first release candidate titles ([0a69b70](https://github.com/theory-cloud/TableTheory/commit/0a69b70f89289c79b773626afe34caab0b5a7ecd))

## [3.0.3](https://github.com/theory-cloud/TableTheory/compare/v3.0.3-rc...v3.0.3) (2026-08-06)

Stable promotion of `v3.0.3-rc`.


## [3.0.3-rc](https://github.com/theory-cloud/TableTheory/compare/v3.0.2...v3.0.3-rc) (2026-08-06)


### Bug Fixes

* **deps:** consolidate dependabot security updates for v3.0.3 ([d07863e](https://github.com/theory-cloud/TableTheory/commit/d07863e0c43817d02a08849e6c1dcb92831c5114))
* **deps:** consolidate dependabot security updates for v3.0.3 ([7953626](https://github.com/theory-cloud/TableTheory/commit/79536260ee9ec7f345d763c30e556bd1482f7e96))
* **governance:** enforce materialization hygiene ([ba5c74d](https://github.com/theory-cloud/TableTheory/commit/ba5c74d4ef244f4eb896606aad580e8e6f207284))
* **validation:** accept bound expression values containing SQL comment tokens ([a82248e](https://github.com/theory-cloud/TableTheory/commit/a82248e280022aa24c2194b2bd05deb6f309e346))

## [3.0.2](https://github.com/theory-cloud/TableTheory/compare/v3.0.2-rc...v3.0.2) (2026-08-01)

Stable promotion of `v3.0.2-rc`.


## [3.0.2-rc](https://github.com/theory-cloud/TableTheory/compare/v3.0.1...v3.0.2-rc) (2026-08-01)


### Bug Fixes

* **errors:** preserve duplicate key sentinel type ([86f45fd](https://github.com/theory-cloud/TableTheory/commit/86f45fd393bcb500693cc8d48901423e13b95e37))
* **model:** align duplicate mapping diagnostics ([9dde04b](https://github.com/theory-cloud/TableTheory/commit/9dde04b5124b37f35862af7cd20628f35dc4d364))
* **model:** preserve duplicate mapping diagnostics ([e42e776](https://github.com/theory-cloud/TableTheory/commit/e42e776c77560a899f4281c60c0c11a53449f535))
* **model:** reject duplicate database attribute mappings ([1acd29a](https://github.com/theory-cloud/TableTheory/commit/1acd29ac03e351bb1018b932ab01f4b5bf9ce438))
* **model:** reject duplicate database attribute mappings ([293cea9](https://github.com/theory-cloud/TableTheory/commit/293cea9e8f88c949b9c16790369d87fe3048e444))
* **query:** support unsigned optimistic lock versions ([db75b15](https://github.com/theory-cloud/TableTheory/commit/db75b15f2ec17de3348d09370f0eef26a39c5025))
* **transaction:** harden version locks and recovery ([93e2d20](https://github.com/theory-cloud/TableTheory/commit/93e2d20b5ca00ca517a729f7260ae811f87038de))
* **transaction:** lock zero versions and poison failed updates ([be4dd64](https://github.com/theory-cloud/TableTheory/commit/be4dd6489a9829bdb287f90aa78e384e6059f902))
* **transaction:** lock zero versions and poison failed updates ([484f9a0](https://github.com/theory-cloud/TableTheory/commit/484f9a0da7d6613451f93233920b200e87b8f71b))
* **transaction:** restore zero-version delete parity ([20ac8bf](https://github.com/theory-cloud/TableTheory/commit/20ac8bf78de9da241ceeba655208d20657576331))

## [3.0.1](https://github.com/theory-cloud/TableTheory/compare/v3.0.1-rc...v3.0.1) (2026-08-01)

Stable promotion of `v3.0.1-rc`.


## [3.0.1-rc](https://github.com/theory-cloud/TableTheory/compare/v3.0.0...v3.0.1-rc) (2026-08-01)


### Bug Fixes

* **deps:** bump aws-cdk-lib to 2.263.0 and retire bundled exception ([87d52bc](https://github.com/theory-cloud/TableTheory/commit/87d52bc1ceec695683e509bfd120e8f66b216b1d))
* **deps:** bump aws-cdk-lib to 2.263.0 and retire bundled exception ([f90f50d](https://github.com/theory-cloud/TableTheory/commit/f90f50dfd1d4a285bee0f24990868c594ab4ad79))
* **go:** align legacy transaction update lifecycle semantics ([560abfc](https://github.com/theory-cloud/TableTheory/commit/560abfc6c317258129dec2443098e47fd977b4b7))
* **go:** align legacy transaction update lifecycle semantics ([0ca2ac3](https://github.com/theory-cloud/TableTheory/commit/0ca2ac307e0966534084d295ad9849b724586565))
* **transaction:** exclude managed legacy update fields ([ab0b065](https://github.com/theory-cloud/TableTheory/commit/ab0b065d84c0ce84e4c26e405269be6e43928aee))
* **transaction:** preserve implicit update compatibility ([be48d17](https://github.com/theory-cloud/TableTheory/commit/be48d170c6db38bd63858cffb54da489b71af103))
* **transaction:** reject empty legacy updates ([61e431d](https://github.com/theory-cloud/TableTheory/commit/61e431d44b8a4ef6294018f8c02e66d05910e042))

## [3.0.0](https://github.com/theory-cloud/TableTheory/compare/v3.0.0-rc...v3.0.0) (2026-07-31)

Stable promotion of `v3.0.0-rc`.


## [3.0.0-rc](https://github.com/theory-cloud/TableTheory/compare/v2.0.6...v3.0.0-rc) (2026-07-31)


### ⚠ BREAKING CHANGES

* Explicitly selected empty omitempty fields now remove their DynamoDB attributes. DMS advances to v0.2, and the Go module path moves to github.com/theory-cloud/tabletheory/v3.

### Features

* adopt DMS v0.2 explicit-empty updates ([1b09947](https://github.com/theory-cloud/TableTheory/commit/1b099475f68dda5816e2b1ce71116cd3da74a7e5))
* **ts:** add transactional model update with DMS empty rule ([8d02e99](https://github.com/theory-cloud/TableTheory/commit/8d02e998b18cb29a867cdf04e459fdbd165c998a))


### Bug Fixes

* **ci:** detect pending major transition verifier support ([e91421d](https://github.com/theory-cloud/TableTheory/commit/e91421da5fb46f18d9542cc75b37a623e91832dd))
* **ci:** detect pending major transition verifier support ([6799e46](https://github.com/theory-cloud/TableTheory/commit/6799e46f6c9a8d3c62a8787c1d366ebbef6f80b1))
* **contract:** close DMS v0.2 parity gaps ([d452b57](https://github.com/theory-cloud/TableTheory/commit/d452b571d6f51b5603200e33c538a44bc5c03f64))
* **contract:** close remaining DMS v0.2 parity gaps ([a8816cc](https://github.com/theory-cloud/TableTheory/commit/a8816cc4745222e8faedb0d0a45196699df85bfe))
* **contract:** eliminate representation-dependent emptiness ([025a464](https://github.com/theory-cloud/TableTheory/commit/025a46495c74b5495fe33a0ab7cc5a6ddd17ff13))
* **contract:** preserve sparse structured updates ([8025999](https://github.com/theory-cloud/TableTheory/commit/8025999b612aac35e36422ed55fb87535a10482b))
* **transaction:** align explicit update parity ([67d462d](https://github.com/theory-cloud/TableTheory/commit/67d462dabb9ac69b1b777e7de29517e3c17ae99e))
* **transaction:** align explicit update parity ([5f74dae](https://github.com/theory-cloud/TableTheory/commit/5f74dae41cc8582de2e09075c407275268e5e3e1))
* **transaction:** align Python version rejection ([3e90b8a](https://github.com/theory-cloud/TableTheory/commit/3e90b8a8bb6117db9fc753e5bd8905b46c9bbdff))
* **transaction:** reject lifecycle field selection ([edea977](https://github.com/theory-cloud/TableTheory/commit/edea9775600ded74e9c2549253d2226e9a105083))

## [2.0.6](https://github.com/theory-cloud/TableTheory/compare/v2.0.6-rc...v2.0.6) (2026-07-30)

Stable promotion of `v2.0.6-rc`.


## [2.0.6-rc](https://github.com/theory-cloud/TableTheory/compare/v2.0.5...v2.0.6-rc) (2026-07-30)


### Bug Fixes

* **ci:** fail fast for unreleasable staging PRs ([db33147](https://github.com/theory-cloud/TableTheory/commit/db33147abf2c76653cc626f41a937b9d52fa5d54))
* **ci:** honor prerelease promotion release driver ([b700911](https://github.com/theory-cloud/TableTheory/commit/b7009115cf0171f3b9e9fccf014e6981a8b7eef7))
* **ci:** honor prerelease promotion release driver ([8be086f](https://github.com/theory-cloud/TableTheory/commit/8be086f56974048113959c60b2913e877b72d6af))

## [2.0.5](https://github.com/theory-cloud/TableTheory/compare/v2.0.5-rc...v2.0.5) (2026-07-24)

Stable promotion of `v2.0.5-rc`.


## [2.0.5-rc](https://github.com/theory-cloud/TableTheory/compare/v2.0.4...v2.0.5-rc) (2026-07-24)


### Bug Fixes

* **transaction:** preserve nested empty collections ([4e887fd](https://github.com/theory-cloud/TableTheory/commit/4e887fda4b6aae37827e372d3894a0c12a3bdbee))
* **transaction:** preserve nested empty collections ([fd2d9c2](https://github.com/theory-cloud/TableTheory/commit/fd2d9c2b3f1fb98b9850e2b3e7283a85663c7c90))

## [2.0.4](https://github.com/theory-cloud/TableTheory/compare/v2.0.4-rc...v2.0.4) (2026-07-22)

Stable promotion of `v2.0.4-rc`.


## [2.0.4-rc](https://github.com/theory-cloud/TableTheory/compare/v2.0.3...v2.0.4-rc) (2026-07-22)


### Bug Fixes

* **ci:** bootstrap direct PR verifiers ([838e5d7](https://github.com/theory-cloud/TableTheory/commit/838e5d7f92fb591472c1ce699fd4c6251f40d4f1))
* **ci:** bootstrap direct PR verifiers ([a3f4fff](https://github.com/theory-cloud/TableTheory/commit/a3f4fffb9c5a57912c145e4f80eca68f8e80e8e6))
* **ci:** restore direct protected PR lane ([e993f70](https://github.com/theory-cloud/TableTheory/commit/e993f70079932c4cd537c94640dc8d3e6597a0ae))
* **deps:** consolidate dependency updates ([f1907d6](https://github.com/theory-cloud/TableTheory/commit/f1907d6e220757ed32beb16f5acfb51968427c5b))
* **deps:** repair dependency security and graph ([33aa8b9](https://github.com/theory-cloud/TableTheory/commit/33aa8b986e8beacd6db7992581cada02a16cc0c3))
* **deps:** repair dependency security and graph ([7f24a9e](https://github.com/theory-cloud/TableTheory/commit/7f24a9e4d206a6682221586adeac9f851a66ce8e))

## [2.0.3](https://github.com/theory-cloud/TableTheory/compare/v2.0.3-rc...v2.0.3) (2026-07-08)

Stable promotion of `v2.0.3-rc`.


## [2.0.3-rc](https://github.com/theory-cloud/TableTheory/compare/v2.0.2...v2.0.3-rc) (2026-07-08)


### Bug Fixes

* **go:** record Go 1.26.5 release driver ([3a9bf51](https://github.com/theory-cloud/TableTheory/commit/3a9bf5193a7a00a46254669fae9be801b14eb8bf))
* **go:** record Go 1.26.5 release driver ([6ca2e11](https://github.com/theory-cloud/TableTheory/commit/6ca2e119be1c22e726b12fb4112b6c79f3ad5ad4))

## [2.0.2](https://github.com/theory-cloud/TableTheory/compare/v2.0.2-rc...v2.0.2) (2026-07-07)

Stable promotion of `v2.0.2-rc`.


## [2.0.2-rc](https://github.com/theory-cloud/TableTheory/compare/v2.0.1...v2.0.2-rc) (2026-07-07)


### Bug Fixes

* **ci:** sign generated stable release pr commits ([c7c2a59](https://github.com/theory-cloud/TableTheory/commit/c7c2a590ab7e6a49db71163c3541c4e3e497c722))
* **go:** make v2 module path consumable ([b768036](https://github.com/theory-cloud/TableTheory/commit/b768036891f3590b0b20127cbb6f4a6394c01a54))
* **go:** make v2 module path consumable ([c6c5981](https://github.com/theory-cloud/TableTheory/commit/c6c59816390eb0219b9c0a63f04bd036a8ab1202))

## [2.0.1](https://github.com/theory-cloud/TableTheory/compare/v2.0.1-rc.2...v2.0.1) (2026-07-07)

Stable promotion of `v2.0.1-rc.2`.


## [2.0.1-rc.2](https://github.com/theory-cloud/TableTheory/compare/v2.0.1-rc.1...v2.0.1-rc.2) (2026-07-07)


### Bug Fixes

* **ci:** generate stable release pr deterministically ([de0daee](https://github.com/theory-cloud/TableTheory/commit/de0daeeff4d1a4beb395fded12296a3d3881581a))
* **ci:** generate stable release pr deterministically ([bd3a49b](https://github.com/theory-cloud/TableTheory/commit/bd3a49b4f67b58d044561be9ec366db8aa69b1aa))
* **ci:** no-op premain during pending stable repair ([4f202f1](https://github.com/theory-cloud/TableTheory/commit/4f202f14a5fc70ffe8dedaadaa97dcebbb187cca))
* **ci:** no-op premain during pending stable repair ([1da1099](https://github.com/theory-cloud/TableTheory/commit/1da109999f0e576e0cd7e9e627ac4bd6cf39a150))
* **ci:** run fuzz smoke with one worker ([0a45db7](https://github.com/theory-cloud/TableTheory/commit/0a45db7b934c2ba37eafda87180aa6b6b7775f4b))

## [2.0.1-rc.1](https://github.com/theory-cloud/TableTheory/compare/v2.0.1-rc...v2.0.1-rc.1) (2026-07-07)


### Bug Fixes

* **ci:** allow premain rc repair into staging ([22e0750](https://github.com/theory-cloud/TableTheory/commit/22e0750e0afb590e21d89427ae91cd7fc1bd24d8))
* **ci:** allow staging rc follow-up repair ([b0aa876](https://github.com/theory-cloud/TableTheory/commit/b0aa876c52cea79e2c570ff6afb14f23cf3bc43b))
* **ci:** derive stable promotion intent ([1d23401](https://github.com/theory-cloud/TableTheory/commit/1d23401a500c420c81d58202b18467ebffae634e))
* **ci:** derive stable promotion intent from premain ([df53986](https://github.com/theory-cloud/TableTheory/commit/df539866d75d807e77f412c6af904000accb725a))
* **ci:** detect resolved supply-chain verifier ([079810a](https://github.com/theory-cloud/TableTheory/commit/079810a06f1e2de11644e6449c3890aa8c85e9e1))
* **ci:** detect resolved supply-chain verifier ([1b154a7](https://github.com/theory-cloud/TableTheory/commit/1b154a7d337a26e1ff478834471851753169a50d))
* **ci:** use resolved promotion verifier ([e389b3b](https://github.com/theory-cloud/TableTheory/commit/e389b3b8d4427c717a2e070c832830e99dffab92))
* **ci:** use resolved promotion verifier ([31d5b2d](https://github.com/theory-cloud/TableTheory/commit/31d5b2dbe523d5a9d063afe42abd2af4cfc72031))

## [2.0.1-rc](https://github.com/theory-cloud/TableTheory/compare/v2.0.0...v2.0.1-rc) (2026-07-06)


### Bug Fixes

* **ci:** add fuzz smoke timeout margin ([31c56e5](https://github.com/theory-cloud/TableTheory/commit/31c56e5d81f6d60f165d4fefb09a0de1b8f28ad8))
* **ci:** cover lower runtime staging gates ([6b11553](https://github.com/theory-cloud/TableTheory/commit/6b115533965c202ba62be2903b26407483a9e2a6))
* **ci:** derive queued main release head ([28ff9a0](https://github.com/theory-cloud/TableTheory/commit/28ff9a0fee3b7379db6c0bbfd75015c7b2b72e44))
* **ci:** harden post-v2 release hygiene gates ([757af07](https://github.com/theory-cloud/TableTheory/commit/757af0787f6c75f03db46f6d4aa61054aeb1d565))
* **ci:** no-op stable release-pr after publish ([e051377](https://github.com/theory-cloud/TableTheory/commit/e0513772cf261d7751c20044beb369135ec9ebac))
* **ci:** no-op stable release-pr after publish ([7c393bb](https://github.com/theory-cloud/TableTheory/commit/7c393bb7fab29c742e8e3eedc5b5ebffab593310))
* **ci:** select v5 verifier for promotions ([3b08dd6](https://github.com/theory-cloud/TableTheory/commit/3b08dd647f2c11a1c3249a256e0bdc996d076efc))
* **ci:** select v5 verifier for promotions ([41342b1](https://github.com/theory-cloud/TableTheory/commit/41342b1516e647d372c59dfd0b7530cdfad69940))
* **ci:** split Python lower-runtime pytest ([8cb0170](https://github.com/theory-cloud/TableTheory/commit/8cb017095c58d7b7b45115e3691f35131b3710c4))
* **contract:** harden harness assertion semantics ([84c7670](https://github.com/theory-cloud/TableTheory/commit/84c7670c4e88cf27808bbfbd3685845e6fc9963a))
* **release:** reject RC-shaped main PR titles ([d38ecc4](https://github.com/theory-cloud/TableTheory/commit/d38ecc4dd84460ba3d2d620c74a8ca690fb52bff))

## [2.0.0](https://github.com/theory-cloud/TableTheory/compare/v2.0.0-rc...v2.0.0) (2026-07-06)


### Bug Fixes

* **ci:** bootstrap main release hygiene for v2 ([49155b5](https://github.com/theory-cloud/TableTheory/commit/49155b5d07b6d2943aa73f67dc96018641e93ab0))
* **ci:** bootstrap main release hygiene for v2 ([a962dfd](https://github.com/theory-cloud/TableTheory/commit/a962dfd03ff004fad1342e83c05366d55a08408b))

## [2.0.0-rc](https://github.com/theory-cloud/TableTheory/compare/v1.10.1...v2.0.0-rc) (2026-07-06)


### ⚠ BREAKING CHANGES

* promote TableTheory product strengthening program
* **py:** the deprecated theorydb_py Python import package is removed in v2; consumers must import tabletheory_py.
* **query:** Go consumers must stop constructing pkg/query.MainExecutor, query.NewExecutor, or pkg/query.DynamoDBAPI directly; use DB.Model/tabletheory.Model flows for execution and query.UnmarshalItem(s) or NewWithClient plus pkg/testing/fakedb for tests.
* **core:** Go callers can no longer pass arbitrary opts ...any values to AutoMigrateWithOptions or CreateTable; use schema.AutoMigrateOption and schema.TableOption values or concrete slices.
* **ts:** TypeScript consumers must import FaceTheory ISR, release-state, and lease helpers from @theory-cloud/tabletheory-ts/facetheory, /release-state, and /lease; the root package now exposes only generic ORM APIs.
* **py:** Python consumers must import tabletheory_py as the canonical runtime package; theorydb_py is no longer the implementation package and remains only as a DeprecationWarning transition shim.
* **ts:** TypeScript now unmarshals DynamoDB N and NS values to canonical decimal strings by default; consumers that intentionally want historical JavaScript Number coercion must configure numberUnmarshalMode: 'number'.
* **core:** Go consumers must replace DB.Transaction, ExtendedDB.TransactionFunc, core.Tx, mocks, and pkg/testing helpers built around the non-atomic callback API with the DynamoDB-backed Transact()/TransactWrite transaction builder.
* Go writes now persist []byte and set-tagged numeric/binary slices as canonical DynamoDB B/NS/BS/NULL shapes and reject unsupported set element types. Legacy shape-driven reads remain, but consumers with raw filters or conditions over old list-shaped data may need backfill.

### Features

* **api:** add typed API surfaces ([b82860c](https://github.com/theory-cloud/TableTheory/commit/b82860c25d4fe4e1336642ab95843f03b5fb779f))
* **ci:** adopt merge queue for protected branches ([4cf17fb](https://github.com/theory-cloud/TableTheory/commit/4cf17fb6d93ce9d3c9a0f245b22f1218287be089))
* **cli:** add DMS validate and codegen ([990c07b](https://github.com/theory-cloud/TableTheory/commit/990c07ba97f52129a155a2ddc21909e7f6701adf))
* **cli:** add init scaffold command ([d625d6e](https://github.com/theory-cloud/TableTheory/commit/d625d6ef26a0d598bae6190dac6e37499d442fce))
* **cli:** generate CDK table constructs from DMS ([d220248](https://github.com/theory-cloud/TableTheory/commit/d220248985a2f88c4968bb8eedb1f88e810eb275))
* **contract:** add cross-runtime interop scenario support ([509b103](https://github.com/theory-cloud/TableTheory/commit/509b103cab64e939241727819a59324e0e3c1942))
* **contract:** add GSI projection P1 scenarios ([b058043](https://github.com/theory-cloud/TableTheory/commit/b0580435dd7cdd6952bb54cfdfbfe78e91175c9f))
* **contract:** add key-contract v0.2 transforms spec ([338d8f3](https://github.com/theory-cloud/TableTheory/commit/338d8f38afd1319007b8e7d90346442c9225e839))
* **contract:** add naming and encryption parity scenarios ([2cf26b6](https://github.com/theory-cloud/TableTheory/commit/2cf26b6338efe6ab60f8531156045080fbf5e147))
* **contract:** add native count P1 scenario ([46ad3ca](https://github.com/theory-cloud/TableTheory/commit/46ad3ca9b1c577bdf29ab666eed17083fcb76a6e))
* **contract:** add optional get P1 scenario ([70f4237](https://github.com/theory-cloud/TableTheory/commit/70f423716efea46238ca582f78d862e583ee92a0))
* **contract:** add pagination cursor scenarios ([c036ba5](https://github.com/theory-cloud/TableTheory/commit/c036ba50f2240bdb55267c093ea6794a55347116))
* **contract:** add query and scan ops to the scenario harness ([f2d7f37](https://github.com/theory-cloud/TableTheory/commit/f2d7f37311a1a5edadf59291e4597af08fd4e0b8))
* **contract:** add query semantics P1 scenarios ([b351bcc](https://github.com/theory-cloud/TableTheory/commit/b351bcc3b710f12b00c5dc13b2888f928e965659))
* **contract:** add transaction and lazy iteration scenarios ([49d7686](https://github.com/theory-cloud/TableTheory/commit/49d76862f1908fe2ae8246629b138332e168f8eb))
* **contract:** add type matrix P0 scenario ([87c852b](https://github.com/theory-cloud/TableTheory/commit/87c852b6cc56ad2b19cabf26ccf0fdd6a8af065b))
* **contract:** add version-conflict error scenario ([fc57f56](https://github.com/theory-cloud/TableTheory/commit/fc57f56c1e756fb590890c0fa095f1bf48ca40b6))
* **contract:** generate runner models from DMS ([6012701](https://github.com/theory-cloud/TableTheory/commit/6012701977aff5fe8b8e8ef91d91982dfd69de76))
* **contract:** implement item_equals and cursor_equals assertions ([af9d827](https://github.com/theory-cloud/TableTheory/commit/af9d827cd9db11651e8f5613962fa46890540e28))
* **contract:** pin exact number precision scenario ([df1855e](https://github.com/theory-cloud/TableTheory/commit/df1855e02c45f04b85d212647e40c6bb6342a759))
* **core:** remove non-atomic Transaction API in favor of Transact() ([5241ba1](https://github.com/theory-cloud/TableTheory/commit/5241ba10eabc3801fbc9ec48e42d7c8ab319b148))
* **core:** require typed schema options ([9d86388](https://github.com/theory-cloud/TableTheory/commit/9d86388032785702ee5f877eb8aab06b0b698ea3))
* **docs:** generate API references from source ([88c7d94](https://github.com/theory-cloud/TableTheory/commit/88c7d9445662bfac78856e199c73db7ef9773369))
* **docs:** ship generative-coding artifacts ([feb423e](https://github.com/theory-cloud/TableTheory/commit/feb423e4a8c5d6e3d13e8c88a12497e04e555dba))
* **examples:** add DynamoDB-Local variant to cdk-multilang ([dcf1867](https://github.com/theory-cloud/TableTheory/commit/dcf18678dbefca5a2ac55ed19a7ec5c63a2d4855))
* **examples:** add one-command Go local quickstart ([35635a9](https://github.com/theory-cloud/TableTheory/commit/35635a9c34770cbb1d69687db3d02f8bb4bfa613))
* **go:** add injectable DynamoDB client constructor ([9f4d17a](https://github.com/theory-cloud/TableTheory/commit/9f4d17ab756f3a66615ca3d8bd275aa20817cb3f))
* **go:** add state-backed DynamoDB fake ([f36cc70](https://github.com/theory-cloud/TableTheory/commit/f36cc709f699bfa1991458a1c565c8d97327c9a0))
* **keycontract:** implement v0.2 transforms across runtimes ([041cc73](https://github.com/theory-cloud/TableTheory/commit/041cc7303b21e23e22df277639ad30b0dff76ca2))
* **make:** add rubric-fast contributor gate ([579b0a7](https://github.com/theory-cloud/TableTheory/commit/579b0a78fdd9903cebddcf2d947ace97936259bb))
* promote TableTheory product strengthening program ([5130b5d](https://github.com/theory-cloud/TableTheory/commit/5130b5dd35e66b05a3ca4ce2020dd8d659007a26))
* **py:** add DMS equivalence gate ([f169d2a](https://github.com/theory-cloud/TableTheory/commit/f169d2a783c26be98acaada344f6c06b4597074a))
* **py:** add native query and scan count ([21b50d2](https://github.com/theory-cloud/TableTheory/commit/21b50d2ae2f13df5a3ab32c872593593c5d94767))
* **py:** add optional get API ([cb4c52d](https://github.com/theory-cloud/TableTheory/commit/cb4c52d305662712929b10aaf57c8c57831711be))
* **py:** add schema migration and transform support ([7ef69c1](https://github.com/theory-cloud/TableTheory/commit/7ef69c17e9b83c76e93174637af972bcd032cc76))
* **py:** add stateful DynamoDB testkit fake ([4616356](https://github.com/theory-cloud/TableTheory/commit/46163564d5bfc9051b988f8c0d914d7f5666a03d))
* **py:** add tabletheory_py canonical import alias ([58723c2](https://github.com/theory-cloud/TableTheory/commit/58723c2e01ad9ec270b2cfdef43f3803f4c9e442))
* **py:** make tabletheory_py the canonical package ([ea60a1a](https://github.com/theory-cloud/TableTheory/commit/ea60a1a4ac0ae80e716b9917e38824218840792a))
* **py:** remove legacy theorydb_py shim ([2aa4a46](https://github.com/theory-cloud/TableTheory/commit/2aa4a4625362f5a69829efe9dbcd64996648ca66))
* **py:** support Python 3.12+ ([e1100ac](https://github.com/theory-cloud/TableTheory/commit/e1100accb49d2c59cfe862de985074ccfa4352fd))
* **query:** add optional first helper in Go ([9b6eec3](https://github.com/theory-cloud/TableTheory/commit/9b6eec30a3de5a3f748d19c60e297fccffd5a819))
* **query:** remove deprecated MainExecutor ([90997b4](https://github.com/theory-cloud/TableTheory/commit/90997b4069809dd41a93cc15143fa9842daf73fa))
* **query:** use native DynamoDB count in Go ([c2467dc](https://github.com/theory-cloud/TableTheory/commit/c2467dc7e47a08c9b0ef03f2ff35f453b5f2359a))
* **release:** publish pip find-links index for Python releases ([fe4ebbb](https://github.com/theory-cloud/TableTheory/commit/fe4ebbbd49fc4c16db6c9a3182285ede3ee9a48b))
* **release:** publish tabletheory CLI as a release asset ([adf1e3d](https://github.com/theory-cloud/TableTheory/commit/adf1e3d16af754438895a26fa06e72509fc70138))
* **ts:** add DMS equivalence gate ([688217c](https://github.com/theory-cloud/TableTheory/commit/688217cd2375479426faf7a4c8b1b1b80ce6bb80))
* **ts:** add domain subpath exports ([eceff7a](https://github.com/theory-cloud/TableTheory/commit/eceff7ac19716b0c89a85205900729c933bf81dd))
* **ts:** add exact number unmarshal mode ([54ecfcc](https://github.com/theory-cloud/TableTheory/commit/54ecfcc3f3d915e5016ecfff135565217b8c2da5))
* **ts:** add native query and scan count ([f6d2030](https://github.com/theory-cloud/TableTheory/commit/f6d2030a05a691e1c38ed1c81f1b66928a8a8ee2))
* **ts:** add optional get API ([66840a0](https://github.com/theory-cloud/TableTheory/commit/66840a07ffb286ed4cd92ef9baf35f5f59ca8d42))
* **ts:** add schema migration and transform support ([6753cc5](https://github.com/theory-cloud/TableTheory/commit/6753cc51972de00da784e81aed90ed5ae62170b6))
* **ts:** add stateful DynamoDB testkit fake ([01c9e06](https://github.com/theory-cloud/TableTheory/commit/01c9e06090239b97f2976739d9d13c15d4d0bfa9))
* **ts:** declare AWS SDK clients as peer dependencies ([0353a2f](https://github.com/theory-cloud/TableTheory/commit/0353a2f0d0fc40b318d27db1536f4fb8c0779e5e))
* **ts:** default to precision-safe number unmarshaling ([7813d7e](https://github.com/theory-cloud/TableTheory/commit/7813d7e193f41319cdd72c9266a75f126a496238))
* **ts:** ground optimizer index selection ([2f3af60](https://github.com/theory-cloud/TableTheory/commit/2f3af609f2dfe0e9cb802dac963ac7eaec0b884c))
* **ts:** move domain helpers to subpath exports ([54d8cc0](https://github.com/theory-cloud/TableTheory/commit/54d8cc09800defb988af9d49ab74ae633c07dfc4))
* **ts:** support Node 20 LTS and CommonJS consumers ([5e039c9](https://github.com/theory-cloud/TableTheory/commit/5e039c91ece867ff1881daf9504adb312b1cc380))


### Bug Fixes

* **ci:** accept unnumbered release-please RC PRs ([e5c26aa](https://github.com/theory-cloud/TableTheory/commit/e5c26aae6e306f6c4eee068b2cdea04f07d9b080))
* **ci:** accept unnumbered release-please RC PRs ([bfc8e52](https://github.com/theory-cloud/TableTheory/commit/bfc8e529af69383619872b6fa48a37e18c689c44))
* **ci:** allow pinned runtime matrices in toolchain check ([7285b0c](https://github.com/theory-cloud/TableTheory/commit/7285b0cb0cef11a6ad4318759c1e15471735c850))
* **ci:** select v2 verifiers for protected promotions ([9e447d4](https://github.com/theory-cloud/TableTheory/commit/9e447d4a15bcc6abd606f94217b38f3a8a75b6a2))
* **ci:** select v2 verifiers for protected promotions ([b8d3cd2](https://github.com/theory-cloud/TableTheory/commit/b8d3cd2c7ccb673a8d3a47a1aa4f0496ebe6cb50))
* **ci:** support premain compatibility checks ([dfdcf97](https://github.com/theory-cloud/TableTheory/commit/dfdcf97f3444f6cb2e8c8aceaa7097e209a9ba7b))
* **ci:** support premain compatibility checks ([5dd6bde](https://github.com/theory-cloud/TableTheory/commit/5dd6bdeeda4dc3735f70947a731e6893f47496f1))
* **cli:** show persisted version in Go scaffold ([ed96d22](https://github.com/theory-cloud/TableTheory/commit/ed96d22d6327a384352a86c8d702ad120f353c4e))
* complete M6 review rework ([1e866c8](https://github.com/theory-cloud/TableTheory/commit/1e866c89999fb054041e83984a59ef3281e81fef))
* **contract:** assert DynamoDB numbers as canonical decimal strings ([e14c485](https://github.com/theory-cloud/TableTheory/commit/e14c485f8f1d088f92b029900f7e03812dd66d9b))
* **contract:** fail closed on interop read assertions ([929a101](https://github.com/theory-cloud/TableTheory/commit/929a101c8fd46de0bb1bb297fb07c8f9ce41ca10))
* **contract:** map encryption error codes in Go driver ([85eeeea](https://github.com/theory-cloud/TableTheory/commit/85eeeea9b46ea6a008f63e6e8e7c7a7745f6b95b))
* **contract:** use consistent reads for raw-item assertions in all runners ([3758b21](https://github.com/theory-cloud/TableTheory/commit/3758b215772744eada1dea44029c73d05ef8d9be))
* **dms:** align naming convention enum ([8e76cdc](https://github.com/theory-cloud/TableTheory/commit/8e76cdcd529baa16a38e42bb45cd0ff9cb3cc18e))
* **dms:** make Python codegen dataclass ordering safe ([0e7f7a4](https://github.com/theory-cloud/TableTheory/commit/0e7f7a41734c7a774359763bedbec2f35e3082e5))
* **go:** classify AWS errors with errors.As ([45efbf3](https://github.com/theory-cloud/TableTheory/commit/45efbf3d1f92ff877a892cf3816e3a8b82267a06))
* **go:** distinguish version conflict errors ([0c83ff1](https://github.com/theory-cloud/TableTheory/commit/0c83ff1a952027eb11b4d6ee4ac66ecf07c62624))
* **go:** satisfy update executor lint ([6718067](https://github.com/theory-cloud/TableTheory/commit/6718067662ea710fa4e4d3452290eb3dddd29b38))
* **gov:** store repo-relative evidence paths in rubric report ([4684391](https://github.com/theory-cloud/TableTheory/commit/46843919e80afdd120deb5be7ac3d9ea55d13c42))
* keep correctness-trap deprecations lint-clean ([5c0052a](https://github.com/theory-cloud/TableTheory/commit/5c0052a8a8590e2e9d673ef7c64ba5bd57a52fc1))
* **lambda:** return cached init error instead of (nil, nil) after failed cold start ([ad0837a](https://github.com/theory-cloud/TableTheory/commit/ad0837af2bf756ed59f8248b0ae78d0d876baf37))
* **lambda:** synchronize lambdaTimeoutBuffer access ([ecd8e40](https://github.com/theory-cloud/TableTheory/commit/ecd8e40dacfd70607ed60f573c9e8d9974efd061))
* **lambda:** synchronize timeout buffer reads ([353edb8](https://github.com/theory-cloud/TableTheory/commit/353edb859273f235365f94e90502cf8cfefed3c7))
* **mocks:** fail assertions instead of panicking on type mismatches ([02bd5ec](https://github.com/theory-cloud/TableTheory/commit/02bd5ec0dae0ac61bd90030670e619620ec89796))
* **model:** improve tag validation diagnostics ([efc8484](https://github.com/theory-cloud/TableTheory/commit/efc8484d563382923760ff1f0a7ee862a2563f8b))
* **py:** distinguish version conflict errors ([cf76ce4](https://github.com/theory-cloud/TableTheory/commit/cf76ce4d96db5426362e836ab339b832b6da6758))
* **py:** keep count helpers within size gate ([96fb8e7](https://github.com/theory-cloud/TableTheory/commit/96fb8e7d0cdbfbb507b622763407b99858085903))
* **py:** keep key contract exports lazy ([961c10f](https://github.com/theory-cloud/TableTheory/commit/961c10ff77d58a46075a8a51d44a19f5723845a8))
* **py:** keep storage type helper typed for build verification ([95a7dde](https://github.com/theory-cloud/TableTheory/commit/95a7ddeb7a9a61a459125d5acffa64154baeb016))
* **py:** preserve legacy shims without wildcards ([b29800a](https://github.com/theory-cloud/TableTheory/commit/b29800af29339d4ecbf33e4207c809213b87dc80))
* **py:** reject unsupported union annotations and unify storage-type resolution ([64c2ad1](https://github.com/theory-cloud/TableTheory/commit/64c2ad1221416d650081f1a048c03404edb24751))
* **query:** keep native count gates green ([9fbac15](https://github.com/theory-cloud/TableTheory/commit/9fbac15eff90809139cd03dc0d648e40d8f09492))
* **release:** accept release-please first RC publishing ([e117879](https://github.com/theory-cloud/TableTheory/commit/e1178795dd6df7130cc6558a875b1ee4848f4701))
* **release:** accept release-please first RC publishing ([0d0873e](https://github.com/theory-cloud/TableTheory/commit/0d0873e9b8d6603d1ef9456053134a7796d7d50b))
* **release:** read branch-sync JSON from git refs ([c7da8e7](https://github.com/theory-cloud/TableTheory/commit/c7da8e7e37a04af6003735d04882ce8c79bb3818))
* **release:** read branch-sync JSON from git refs ([aec7779](https://github.com/theory-cloud/TableTheory/commit/aec7779fee10c21e59c45a9b022cc7cd14440a13))
* **release:** tolerate Python version path transition ([f698ffd](https://github.com/theory-cloud/TableTheory/commit/f698ffd3c1162c2c88b36a1756d4fe33dd1b4440))
* resolve PR export checks ([9c7bee4](https://github.com/theory-cloud/TableTheory/commit/9c7bee47f98869673c112900cb79113e05ce4fed))
* **rubric:** enforce generated model drift gate ([8c1673d](https://github.com/theory-cloud/TableTheory/commit/8c1673deedd16382388e8ad9fbc9060713b8409a))
* **testing:** return explicit error from DefaultDBFactory instead of nil DB ([a3bcb5b](https://github.com/theory-cloud/TableTheory/commit/a3bcb5beeb47fa44cd854294ac0a76b2822f369c))
* **transaction:** document non-atomic DB.Transaction and deprecate in favor of Transact() ([371cb04](https://github.com/theory-cloud/TableTheory/commit/371cb04628d77afc345b8eea4fb82ec22344d04c))
* **ts:** build package exports in unit test ([2a9ebb1](https://github.com/theory-cloud/TableTheory/commit/2a9ebb12109443e02125883e0e913d715c0b2a45))
* **ts:** distinguish version conflict errors ([e509a72](https://github.com/theory-cloud/TableTheory/commit/e509a7299a360d4365e626d79440fffc0a255829))

## [1.10.1](https://github.com/theory-cloud/TableTheory/compare/v1.10.0...v1.10.1) (2026-06-18)


### Bug Fixes

* harden release hygiene and derived key contracts ([ad7fc83](https://github.com/theory-cloud/TableTheory/commit/ad7fc839836b594738abc330f23d613ba564b7d7))
* **release:** allow cycle-state bootstrap repair ([5c32373](https://github.com/theory-cloud/TableTheory/commit/5c32373953d533b798eda265763f487af1276333))
* **release:** allow cycle-state bootstrap repair ([9429dfc](https://github.com/theory-cloud/TableTheory/commit/9429dfce5add1915d471a6910e0e4732f1f0a22d))
* **release:** repair promotion hygiene checks ([74a9eb7](https://github.com/theory-cloud/TableTheory/commit/74a9eb7665aa30f5557db864e418915be815569e))
* **release:** repair promotion hygiene checks ([03f82aa](https://github.com/theory-cloud/TableTheory/commit/03f82aa89dc3ebf0ddfb40294193a22cc6c9ac69))
* **release:** repair promotion hygiene checks ([cfb32d1](https://github.com/theory-cloud/TableTheory/commit/cfb32d12997969e5d3574bb675ef8f1ac26a22c2))
* **release:** scope main hygiene bootstrap ([474f2a9](https://github.com/theory-cloud/TableTheory/commit/474f2a95f663523d05179f04f23b50f28534e21b))
* **release:** validate cycle state target checkout ([02ed9a2](https://github.com/theory-cloud/TableTheory/commit/02ed9a23c2c38c46b65a76b2c75ea9e2fc4361a1))
* **release:** validate cycle state target checkout ([bec4917](https://github.com/theory-cloud/TableTheory/commit/bec4917f45890a7bad86e7724fafcd84c92a726c))
* **security:** clear rubric dependency scans ([a0f757b](https://github.com/theory-cloud/TableTheory/commit/a0f757bbe2c8cb2cd46b2301859ed51f29b46cee))
* **security:** clear TableTheory example dependency alerts ([2916ea5](https://github.com/theory-cloud/TableTheory/commit/2916ea575aa131c7e2192ca4d53c5c6be85e7eec))
* **security:** clear TableTheory example dependency alerts ([f0983b8](https://github.com/theory-cloud/TableTheory/commit/f0983b86873a3671e56d04f2c9d28ddcc7e451d5))
* **security:** recover release cycle for 1.10.1 ([a8e50d0](https://github.com/theory-cloud/TableTheory/commit/a8e50d0e9e733b359a58d37792ed9617456290db))
* **security:** recover TableTheory release cycle for 1.10.1 ([1c595da](https://github.com/theory-cloud/TableTheory/commit/1c595dafb03e765704453399b3e49ea012cce1b3))

## [1.10.1-rc.3](https://github.com/theory-cloud/TableTheory/compare/v1.10.1-rc.2...v1.10.1-rc.3) (2026-06-18)


### Bug Fixes

* **release:** repair promotion hygiene checks ([74a9eb7](https://github.com/theory-cloud/TableTheory/commit/74a9eb7665aa30f5557db864e418915be815569e))
* **release:** repair promotion hygiene checks ([cfb32d1](https://github.com/theory-cloud/TableTheory/commit/cfb32d12997969e5d3574bb675ef8f1ac26a22c2))

## [1.10.1-rc.2](https://github.com/theory-cloud/TableTheory/compare/v1.10.1-rc.1...v1.10.1-rc.2) (2026-06-18)


### Bug Fixes

* **security:** clear TableTheory example dependency alerts ([2916ea5](https://github.com/theory-cloud/TableTheory/commit/2916ea575aa131c7e2192ca4d53c5c6be85e7eec))
* **security:** clear TableTheory example dependency alerts ([f0983b8](https://github.com/theory-cloud/TableTheory/commit/f0983b86873a3671e56d04f2c9d28ddcc7e451d5))

## [1.10.1-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.10.0...v1.10.1-rc.1) (2026-06-18)


### Bug Fixes

* harden release hygiene and derived key contracts ([ad7fc83](https://github.com/theory-cloud/TableTheory/commit/ad7fc839836b594738abc330f23d613ba564b7d7))
* **security:** clear rubric dependency scans ([a0f757b](https://github.com/theory-cloud/TableTheory/commit/a0f757bbe2c8cb2cd46b2301859ed51f29b46cee))
* **security:** recover release cycle for 1.10.1 ([a8e50d0](https://github.com/theory-cloud/TableTheory/commit/a8e50d0e9e733b359a58d37792ed9617456290db))
* **security:** recover TableTheory release cycle for 1.10.1 ([1c595da](https://github.com/theory-cloud/TableTheory/commit/1c595dafb03e765704453399b3e49ea012cce1b3))

## [1.10.0](https://github.com/theory-cloud/TableTheory/compare/v1.9.4...v1.10.0) (2026-06-09)


### Features

* **keycontract:** add derived-key contract parity (THE-2032/THE-2041/THE-2042/THE-2043) ([7e99b12](https://github.com/theory-cloud/TableTheory/commit/7e99b129dc3254aa4d684ed98a0744de10c7055d))
* **keycontract:** define derived-key sidecar contract ([d86b715](https://github.com/theory-cloud/TableTheory/commit/d86b71590b16402cd3d45d9f248ed9d7e9ac0ddb))
* **keycontract:** generate ts key helpers ([4d2071f](https://github.com/theory-cloud/TableTheory/commit/4d2071f1d869a7c5a3c35aaed232ccd166e5ba0c))
* **ts:** add derived-key contract evaluator ([88fbc05](https://github.com/theory-cloud/TableTheory/commit/88fbc050b97c56dad58f6405c7faf56b9346384d))


### Bug Fixes

* **keycontract:** harden cross-runtime key transforms ([4bc99a4](https://github.com/theory-cloud/TableTheory/commit/4bc99a4f0c521da9321638da05f0497dff94ee17))

## [1.10.0-rc](https://github.com/theory-cloud/TableTheory/compare/v1.9.4...v1.10.0-rc) (2026-06-09)


### Features

* **keycontract:** add derived-key contract parity (THE-2032/THE-2041/THE-2042/THE-2043) ([7e99b12](https://github.com/theory-cloud/TableTheory/commit/7e99b129dc3254aa4d684ed98a0744de10c7055d))
* **keycontract:** define derived-key sidecar contract ([d86b715](https://github.com/theory-cloud/TableTheory/commit/d86b71590b16402cd3d45d9f248ed9d7e9ac0ddb))
* **keycontract:** generate ts key helpers ([4d2071f](https://github.com/theory-cloud/TableTheory/commit/4d2071f1d869a7c5a3c35aaed232ccd166e5ba0c))
* **ts:** add derived-key contract evaluator ([88fbc05](https://github.com/theory-cloud/TableTheory/commit/88fbc050b97c56dad58f6405c7faf56b9346384d))


### Bug Fixes

* **keycontract:** harden cross-runtime key transforms ([4bc99a4](https://github.com/theory-cloud/TableTheory/commit/4bc99a4f0c521da9321638da05f0497dff94ee17))

## [1.9.4](https://github.com/theory-cloud/TableTheory/compare/v1.9.3...v1.9.4) (2026-06-03)


### Bug Fixes

* **release:** accept release-please RC version shape ([7c519e7](https://github.com/theory-cloud/TableTheory/commit/7c519e741e9466d5ad2bb54c3aba3bbe352cc627))
* **release:** accept release-please RC version shape ([8b9edc7](https://github.com/theory-cloud/TableTheory/commit/8b9edc7dcdf0bea577a52b9b5b062b9d6b57f2e7))
* **release:** require release creation on gated promotions ([d4ef97c](https://github.com/theory-cloud/TableTheory/commit/d4ef97c81aeefe5fb7cc9110ac0c9f07e37b8e2a))
* **release:** require release creation on gated promotions ([e35acdd](https://github.com/theory-cloud/TableTheory/commit/e35acdd9cf1ce22dc0d7c10df91e0334a276d4d5))

## [1.9.4-rc](https://github.com/theory-cloud/TableTheory/compare/v1.9.3...v1.9.4-rc) (2026-06-03)


### Bug Fixes

* **release:** accept release-please RC version shape ([7c519e7](https://github.com/theory-cloud/TableTheory/commit/7c519e741e9466d5ad2bb54c3aba3bbe352cc627))
* **release:** accept release-please RC version shape ([8b9edc7](https://github.com/theory-cloud/TableTheory/commit/8b9edc7dcdf0bea577a52b9b5b062b9d6b57f2e7))
* **release:** require release creation on gated promotions ([d4ef97c](https://github.com/theory-cloud/TableTheory/commit/d4ef97c81aeefe5fb7cc9110ac0c9f07e37b8e2a))
* **release:** require release creation on gated promotions ([e35acdd](https://github.com/theory-cloud/TableTheory/commit/e35acdd9cf1ce22dc0d7c10df91e0334a276d4d5))

## [1.9.3](https://github.com/theory-cloud/TableTheory/compare/v1.9.1...v1.9.3) (2026-06-03)


### Bug Fixes

* adopt go1.26.4 toolchain to clear Go stdlib CVEs (GO-2026-5037/5038/5039) ([35a6149](https://github.com/theory-cloud/TableTheory/commit/35a61497585ed83d6c140cade71fd63777e95a9c))
* release go1.26.4 toolchain adoption as 1.9.2-rc.1 ([5060eed](https://github.com/theory-cloud/TableTheory/commit/5060eed52ebe1921f94d840676e644a485a0f48b))
* **release:** advance release lane to 1.9.3 ([5c9e9f5](https://github.com/theory-cloud/TableTheory/commit/5c9e9f5778dd43ef84b3bec7d51afb1743346eac))
* **release:** advance release lane to 1.9.3 ([0fffa03](https://github.com/theory-cloud/TableTheory/commit/0fffa03ce70580fd3c7fb7609203784b06471f69))
* **release:** allow pending stable promotion guard ([40c44a5](https://github.com/theory-cloud/TableTheory/commit/40c44a5ac779d4585eda53c8f4f2f6e1fe5a398a))
* **release:** carry CI-driven stable promotion to premain ([09e3c9e](https://github.com/theory-cloud/TableTheory/commit/09e3c9e613790f83a64cfd23c839b6428012b57c))
* **release:** detect exhausted immutable RC state ([f2ba073](https://github.com/theory-cloud/TableTheory/commit/f2ba0739b5b6e48d3afa0921e529b2f98c6d46f5))
* **release:** detect exhausted immutable RC state ([dc593a3](https://github.com/theory-cloud/TableTheory/commit/dc593a3b0f6ac47b4d6e643d6f4defd8309e9027))
* **release:** make stable promotion CI-driven ([325e3be](https://github.com/theory-cloud/TableTheory/commit/325e3be580f56b0d8ddaa55fe7a8d3e9288245fc))
* **release:** make stable promotion CI-driven ([f25efba](https://github.com/theory-cloud/TableTheory/commit/f25efbae55786e4eac8f904bb3d317a28b0f5f34))
* **release:** recover immutable RC publish flow ([bd2e481](https://github.com/theory-cloud/TableTheory/commit/bd2e481be6068d00307331e61849fbb2ba9d9b07))
* **release:** recover release cycle guardrails ([b32693a](https://github.com/theory-cloud/TableTheory/commit/b32693ad01d1c24169fde1bd61edc05d96a81490))
* **release:** recover release cycle guardrails ([df7e4f7](https://github.com/theory-cloud/TableTheory/commit/df7e4f71af65fb990e0ef2a42a77d12e8d5d2bae))
* **release:** sync premain baseline after stable releases ([9a26ea6](https://github.com/theory-cloud/TableTheory/commit/9a26ea63b26a6fee7ac274a97f43c26d349934a7))
* **release:** sync premain baseline after stable releases ([807442d](https://github.com/theory-cloud/TableTheory/commit/807442dc55aa38859e92beef79cb883d9e783bfe))

## [1.9.3-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.9.2-rc.1...v1.9.3-rc.1) (2026-06-03)


### Features

* add FaceTheory ISR MetaStore helper ([0812a68](https://github.com/theory-cloud/TableTheory/commit/0812a68b1203f9179d987dd62eb3529e0332d705))
* add TTL archival lifecycle support ([0f1b88d](https://github.com/theory-cloud/TableTheory/commit/0f1b88d7012ea3436964328a73978ff680137f94))
* add TTL archival lifecycle support ([22ffedc](https://github.com/theory-cloud/TableTheory/commit/22ffedcfccff7f4732eaad50473a89bcffd7815e))
* **dms:** add release-state contract foundation ([ce7d3d3](https://github.com/theory-cloud/TableTheory/commit/ce7d3d3961a36ca4a514faf08e89e1f1b9be4957))
* **dms:** add release-state write policy metadata ([c7862d0](https://github.com/theory-cloud/TableTheory/commit/c7862d0f9462aa49c2a4b2911470f5aaeae9fc28))
* **docs:** add Theory Cloud GitHub Pages site ([8f1b7ec](https://github.com/theory-cloud/TableTheory/commit/8f1b7ec0bc30df2024b2129ce78649e80221591f))
* **docs:** add Theory Cloud GitHub Pages site ([7a3fac9](https://github.com/theory-cloud/TableTheory/commit/7a3fac9138eba9901bc81efaedcd700148d3c118))
* **errors:** add release-state mutation errors ([1ce0a38](https://github.com/theory-cloud/TableTheory/commit/1ce0a38e6aae5e13de21ad7850809ed53a04ca32))
* FaceTheory ISR support (FT-T0..FT-T4) ([66eb65a](https://github.com/theory-cloud/TableTheory/commit/66eb65af9b253946c53f4f88af9dc10668e6d5bd))
* FT-T1 lease helper (Go) ([bb6860a](https://github.com/theory-cloud/TableTheory/commit/bb6860a46380f1227a067dd87ea9fd0d8c4f4f34))
* FT-T1 lease helper (Py) ([71b1c01](https://github.com/theory-cloud/TableTheory/commit/71b1c01ca9d8347e8443627c023d698f4ce4d34b))
* FT-T1 lease helper (TS) ([b04dbb1](https://github.com/theory-cloud/TableTheory/commit/b04dbb17cc72b25d0ba328680170b64fcc37efc6))
* **go:** add LambdaDB timeout configuration ([5b070f1](https://github.com/theory-cloud/TableTheory/commit/5b070f111b95651efab5315a39ca0ead13c87ee1))
* **go:** add opt-in flat encoding for anonymous embeds ([d25d85c](https://github.com/theory-cloud/TableTheory/commit/d25d85cc323a8c2e9c97fa20af24495f133d8c66))
* **go:** enforce release-state write policies ([15eaf11](https://github.com/theory-cloud/TableTheory/commit/15eaf1188c60612101ea0363a81e452f61d76735))
* **go:** enforce release-state write policies ([5d2c433](https://github.com/theory-cloud/TableTheory/commit/5d2c4332c68222948009c5a17469e62ae68e1d37))
* **go:** parse release-state write policy metadata ([c1f912b](https://github.com/theory-cloud/TableTheory/commit/c1f912b237dcfe25a8aa4700899d15ae8bc2277e))
* **mocks:** add transaction builder mock ([ea39672](https://github.com/theory-cloud/TableTheory/commit/ea39672edffd22bf24b1471e244c14b79f06211d))
* **mocks:** add transaction builder mock ([16ab5a5](https://github.com/theory-cloud/TableTheory/commit/16ab5a5d7b22d1087973f96a5c69b2c3a3796c3e))
* **py:** add lambda timeout helper ([d43d776](https://github.com/theory-cloud/TableTheory/commit/d43d776d4e156ce4473a4fe838ad125dec007397))
* **py:** enforce release-state write policies ([0cb8fd4](https://github.com/theory-cloud/TableTheory/commit/0cb8fd47ce53f813db2ba7f7d608c78c94c44371))
* **py:** enforce release-state write policies ([bde01a5](https://github.com/theory-cloud/TableTheory/commit/bde01a56ce8ffa1affda1a3c1bdab3c9df062aff))
* **releasestate:** add release-state helpers ([49bdc14](https://github.com/theory-cloud/TableTheory/commit/49bdc144cb6627d74d28a0e6535df67385e8c676))
* **releasestate:** add transactional transition helpers ([87a0fa6](https://github.com/theory-cloud/TableTheory/commit/87a0fa635ce8c2c7a4be5386ab5af38fcb9fd8f0))
* **releasestate:** validate provenance confidence metadata ([8228344](https://github.com/theory-cloud/TableTheory/commit/82283447200a6c3b96524cd7dd42511642bbf14e))
* **runtime:** add lambda timeout buffer parity ([ce82176](https://github.com/theory-cloud/TableTheory/commit/ce821765944b255bbe9437883c74386d5a5f6eeb))
* TableTheory FaceTheory ISR MetaStore ([f57744d](https://github.com/theory-cloud/TableTheory/commit/f57744d5275744d6399563d29f920bcc045a2f68))
* **ts:** enforce release-state write policies ([85c0ebe](https://github.com/theory-cloud/TableTheory/commit/85c0ebef53bb454d97fffa9d8da77ac975024110))
* **ts:** enforce release-state write policies ([45dba94](https://github.com/theory-cloud/TableTheory/commit/45dba949eb408d815315cc1f0c45f6be76207111))


### Bug Fixes

* add encrypted compat mode for legacy plaintext ([d42d1f5](https://github.com/theory-cloud/TableTheory/commit/d42d1f533e7966ee041829d9bc736d31a419c376))
* add first-class DynamORM naming support ([4ef6e22](https://github.com/theory-cloud/TableTheory/commit/4ef6e225941a93c34aa51f35c0418c8ac51c0499))
* address dependabot alerts 52-55 and pin Go 1.26.2 ([37e0fcb](https://github.com/theory-cloud/TableTheory/commit/37e0fcb982bd6dbf4d996d690b269daa067e4d2e))
* address nested naming review feedback ([2cccc21](https://github.com/theory-cloud/TableTheory/commit/2cccc2194943049f460b540b603db7d9ac6c26e0))
* address security/quality findings ([3b56fb4](https://github.com/theory-cloud/TableTheory/commit/3b56fb4986d2a0e93ced5c682caa2fd401a62087))
* address security/quality findings ([f7adaf7](https://github.com/theory-cloud/TableTheory/commit/f7adaf79d1e7248d3b2654f5c82b33f79cc6e4ac))
* adopt go1.26.4 toolchain to clear Go stdlib CVEs (GO-2026-5037/5038/5039) ([35a6149](https://github.com/theory-cloud/TableTheory/commit/35a61497585ed83d6c140cade71fd63777e95a9c))
* align multilang json field semantics ([4eb0a3b](https://github.com/theory-cloud/TableTheory/commit/4eb0a3bf89a8f12789806d4d1dbea4a6acd515a0))
* bump example cryptography dependency ([452c78f](https://github.com/theory-cloud/TableTheory/commit/452c78f03cdff5cbb2c9ca18b68ead69239fe53a))
* camel case nested dynamorm attributes ([4b24fe3](https://github.com/theory-cloud/TableTheory/commit/4b24fe3c0b016b55edcdd1444fad3b9eb57c6241))
* **ci:** add local theorycloud subtree staging for tabletheory ([0760427](https://github.com/theory-cloud/TableTheory/commit/0760427d60010f404c12443adc19e2ec6367da0f))
* **ci:** add local theorycloud subtree staging for tabletheory ([b7e6913](https://github.com/theory-cloud/TableTheory/commit/b7e6913cfee641cdda0f1975c4d7dd4f1cb054bf))
* **ci:** add theorycloud subtree sync and publish commands ([c205c38](https://github.com/theory-cloud/TableTheory/commit/c205c380aaa179ad654a855512d3f289d2baed03))
* **ci:** add theorycloud subtree sync and publish commands ([3a2c338](https://github.com/theory-cloud/TableTheory/commit/3a2c3381793c99648bd4347e36aea90ca3d6edc4))
* **ci:** align theorycloud publish workflow with KT role contract ([002dfd1](https://github.com/theory-cloud/TableTheory/commit/002dfd1fa23c6c1827022f2af5d7ba36a173e2b6)), closes [#135](https://github.com/theory-cloud/TableTheory/issues/135)
* **ci:** allow stable version alignment on premain promotion ([56984bd](https://github.com/theory-cloud/TableTheory/commit/56984bd4cec3490246b7aeed0b7e17de3268105f))
* **ci:** allow stable version alignment on premain promotion ([4f679a8](https://github.com/theory-cloud/TableTheory/commit/4f679a87f30e9bdb99539a0eeb65d396b2614fc9))
* **ci:** automate theorycloud subtree publishing for tabletheory ([7c47ed2](https://github.com/theory-cloud/TableTheory/commit/7c47ed26cf3e4c72ea67621912a3be8c455460ee))
* **ci:** automate theorycloud subtree publishing for tabletheory ([849c359](https://github.com/theory-cloud/TableTheory/commit/849c35967094a663e00ffc8418f781c8e6726968))
* **ci:** finalize theorycloud publish workflow against KT lab contract ([5f10fc0](https://github.com/theory-cloud/TableTheory/commit/5f10fc0f5271744bbf0f57c21ddaf65cd523129e))
* **ci:** make release assets immutable ([1ef4aca](https://github.com/theory-cloud/TableTheory/commit/1ef4aca7bbd6ef6fffe9a86b9f33b1c0c28e1e97))
* **ci:** make release assets immutable ([e9ad219](https://github.com/theory-cloud/TableTheory/commit/e9ad219f7d806e8faff7422c45ea2b1f066e3904))
* **ci:** make theorycloud publish helper awscurl-compatible ([87945e2](https://github.com/theory-cloud/TableTheory/commit/87945e28d020ff5c516f6c13b4fe6b4e29ff22c5))
* **ci:** publish pages from staging ([6fad410](https://github.com/theory-cloud/TableTheory/commit/6fad410eabcf5305782f019667bddd50a1728f61))
* **ci:** publish pages from staging ([720034c](https://github.com/theory-cloud/TableTheory/commit/720034c9b83989df45c2c1eecb440805d3b0d8fa))
* **ci:** retry git fetch in branch-version-sync ([1b4d855](https://github.com/theory-cloud/TableTheory/commit/1b4d8557fe66c5c333846469369d9e5285cc1232))
* **ci:** unblock dependency scan gates ([addb0e4](https://github.com/theory-cloud/TableTheory/commit/addb0e46088cd066440fff3ddc12747304749896))
* clear rubric failures locally ([2c39a7b](https://github.com/theory-cloud/TableTheory/commit/2c39a7bccbd70b6b8ffc0ee74328442f8eb0cc17))
* clear rubric lint regressions ([f40bb1f](https://github.com/theory-cloud/TableTheory/commit/f40bb1f923320cd03556a6ca7add6f6fe3b526d3))
* complete theorydb json field semantics ([93f36a1](https://github.com/theory-cloud/TableTheory/commit/93f36a179d5b36fc3d1f0c31fe69da0426fe5367))
* **deps:** address dependabot alerts 56 and 57 ([aa73fe8](https://github.com/theory-cloud/TableTheory/commit/aa73fe829a37be4e8c70b3855fdf45bb2def161b))
* **deps:** address dependabot alerts 56 and 57 ([df77c48](https://github.com/theory-cloud/TableTheory/commit/df77c48f831e8cd537c4aa8f26d2055251d86ec0))
* **deps:** patch fast xml builder advisories ([c212509](https://github.com/theory-cloud/TableTheory/commit/c2125093f8f0f30e36cfdfb88865afc0c91e5899))
* **deps:** release dependency security refresh ([bfba142](https://github.com/theory-cloud/TableTheory/commit/bfba14202b5470459ebe907faefc4c38a65db499))
* **deps:** release dependency security refresh ([d459e61](https://github.com/theory-cloud/TableTheory/commit/d459e61c557ebea5fb3f221eafec2c711234d4fc))
* **deps:** resolve npm audit vulnerabilities ([83fadbd](https://github.com/theory-cloud/TableTheory/commit/83fadbd3a7d5e8f3bab0fa85f0da4250bbb1e27a))
* **deps:** update python deps for security ([712cfb5](https://github.com/theory-cloud/TableTheory/commit/712cfb5b08c410d57bbc28bd6664768b2c74b30b))
* **dms:** complete release-state contract plumbing ([4560a6a](https://github.com/theory-cloud/TableTheory/commit/4560a6a03c8ceedbba47c21b4ffe2d18e37c3bbb))
* **docs:** align workflow and transaction docs ([e39b02e](https://github.com/theory-cloud/TableTheory/commit/e39b02e48eb0d13e06971ada2718dcb877deb268))
* **docs:** correct transaction and locking examples ([4af61ab](https://github.com/theory-cloud/TableTheory/commit/4af61abcc849c88c7d6821d70aab7f0f212508f6))
* **docs:** correct wheel-asset name, encryption descriptor, transactions, and update signatures ([1691abc](https://github.com/theory-cloud/TableTheory/commit/1691abcf294783abf9e2ec98450ea70df97ff07a))
* **docs:** narrow ttl and lifecycle claims ([3a4f464](https://github.com/theory-cloud/TableTheory/commit/3a4f4641d280aa1be515cc5d41d3696db76f65f4))
* **docs:** replace internal-doc links in subtree-published pages ([5af9535](https://github.com/theory-cloud/TableTheory/commit/5af9535eafe6b6c52e3af1daffc73b166e5c26fc))
* **docs:** replace invented APIs with the real public surface ([cc45c6e](https://github.com/theory-cloud/TableTheory/commit/cc45c6e7928fc99f933dbfe7e01b60257df4caef))
* **docs:** replace invented APIs with the real public surface ([dbf261a](https://github.com/theory-cloud/TableTheory/commit/dbf261a91279e3ee4e7a9f2e5989737e2b5d2d9a))
* **docs:** use markdown-file relative links across new content ([0b7c6e9](https://github.com/theory-cloud/TableTheory/commit/0b7c6e9d7a817ebebc8cf5541745dc3f17c5df03))
* **expr:** remap grouped filter placeholders on merge ([fd18b2a](https://github.com/theory-cloud/TableTheory/commit/fd18b2a097c26f7fdde4cf9fca216bf5cf8b02e1))
* **go:** add promoted-field plan for anonymous embeds ([0c6edbc](https://github.com/theory-cloud/TableTheory/commit/0c6edbca9ea3525b9693a452f6dd562949f393d0))
* **go:** restore rubric for promoted embed helper parity ([5f7b861](https://github.com/theory-cloud/TableTheory/commit/5f7b86101caf5ed26e1b1bd506dd914b5cfbb4a6))
* improved transaction handling ([30a5d7a](https://github.com/theory-cloud/TableTheory/commit/30a5d7acc371cbcbd38bee1d240e5eab24d49882))
* keep brace-expansion audit finding visible ([1b9d1f6](https://github.com/theory-cloud/TableTheory/commit/1b9d1f6e563e6975f66db147b278cb18c2e5fe82))
* keep brace-expansion audit finding visible (THE-1757) ([27e379c](https://github.com/theory-cloud/TableTheory/commit/27e379c138489b0f47d64239705b5c20b3d5ba7a))
* keep brace-expansion visible without failing SEC-2 ([158e3c5](https://github.com/theory-cloud/TableTheory/commit/158e3c5b72b61668c936451dd56e2de26d329c0f))
* **marshal:** preserve omitempty pointer zero values ([6d1077c](https://github.com/theory-cloud/TableTheory/commit/6d1077cd1d3928ca07a92a5dfcc6ace723da1d8f))
* **marshal:** preserve omitempty pointer zero values ([bf83062](https://github.com/theory-cloud/TableTheory/commit/bf83062d1f84d301b78597c102ceb408209aaf70))
* **marshal:** restore anonymous embed custom hook handling ([2a61893](https://github.com/theory-cloud/TableTheory/commit/2a61893fce94ad3184969b85fa6b88b147f20847))
* **marshal:** restore anonymous embed custom hook handling ([a60ccb4](https://github.com/theory-cloud/TableTheory/commit/a60ccb4272ed287421cc5b246addc0f7c2eb910c))
* **marshal:** share anonymous-embed field traversal across helpers ([384ccdc](https://github.com/theory-cloud/TableTheory/commit/384ccdcaab94cd688df045c0789b12edfe785d51))
* **mocks:** satisfy lint gates ([a9cd117](https://github.com/theory-cloud/TableTheory/commit/a9cd1170fc200489369b76f098635321ed3d81c0))
* **pages:** build docs at custom-domain root ([9c25333](https://github.com/theory-cloud/TableTheory/commit/9c253336aaaff867982c4c1c60edf09505183922))
* **pages:** build docs at custom-domain root ([1cd1e89](https://github.com/theory-cloud/TableTheory/commit/1cd1e89e01df6758ff4ff1af48bbcb1fc8bf7c93))
* **pages:** pin third-party action refs to commit SHAs ([4b2e477](https://github.com/theory-cloud/TableTheory/commit/4b2e4772cc12a47fb2f76eee75c5b8f90845caa1))
* pass SEC-2 dependency scans ([a3e0390](https://github.com/theory-cloud/TableTheory/commit/a3e0390d31d99435fd8c8aa7d48c2aeb7845d977))
* **premain:** restore prerelease version alignment ([9b07cdb](https://github.com/theory-cloud/TableTheory/commit/9b07cdb7df5e69be8012374f742d89252ffde942))
* **py:** execute shared P0 contract fixtures ([b2e72f7](https://github.com/theory-cloud/TableTheory/commit/b2e72f706f4b07eaf99498f70b4451d747ec8acb))
* **py:** reject lifecycle timestamp update tampering ([8448730](https://github.com/theory-cloud/TableTheory/commit/844873044c4f95efabebf1f02034873eefed2ff2))
* **py:** reject lifecycle timestamp update tampering (THE-1750) ([3835d07](https://github.com/theory-cloud/TableTheory/commit/3835d078337484a26974068f3114553491a98439))
* **query:** bind named updates to matched attribute names ([a054b34](https://github.com/theory-cloud/TableTheory/commit/a054b34004b47ace31a18608b87467674411e424))
* **query:** honor promoted anonymous embeds in public unmarshal helpers ([3128cc8](https://github.com/theory-cloud/TableTheory/commit/3128cc898bccbbda809de5dddfb9358f2883ca2e))
* **query:** make encrypted batch retries safe ([f6ec65c](https://github.com/theory-cloud/TableTheory/commit/f6ec65c158401b752c530a4b888ec020390edc41))
* **query:** make encrypted batch retries safe ([e343a29](https://github.com/theory-cloud/TableTheory/commit/e343a2917a7f0121818ebca9aaccbe57109ce0d5))
* **query:** migrate remaining anonymous-embed helper walkers ([7dbdd5f](https://github.com/theory-cloud/TableTheory/commit/7dbdd5f1957b99b9205495cb4370988423ad83d1))
* release go1.26.4 toolchain adoption as 1.9.2-rc.1 ([5060eed](https://github.com/theory-cloud/TableTheory/commit/5060eed52ebe1921f94d840676e644a485a0f48b))
* **release:** advance release lane to 1.9.3 ([5c9e9f5](https://github.com/theory-cloud/TableTheory/commit/5c9e9f5778dd43ef84b3bec7d51afb1743346eac))
* **release:** advance release lane to 1.9.3 ([0fffa03](https://github.com/theory-cloud/TableTheory/commit/0fffa03ce70580fd3c7fb7609203784b06471f69))
* **release:** allow pending stable promotion guard ([40c44a5](https://github.com/theory-cloud/TableTheory/commit/40c44a5ac779d4585eda53c8f4f2f6e1fe5a398a))
* **release:** carry CI-driven stable promotion to premain ([09e3c9e](https://github.com/theory-cloud/TableTheory/commit/09e3c9e613790f83a64cfd23c839b6428012b57c))
* **release:** detect exhausted immutable RC state ([f2ba073](https://github.com/theory-cloud/TableTheory/commit/f2ba0739b5b6e48d3afa0921e529b2f98c6d46f5))
* **release:** detect exhausted immutable RC state ([dc593a3](https://github.com/theory-cloud/TableTheory/commit/dc593a3b0f6ac47b4d6e643d6f4defd8309e9027))
* **release:** keep staging aligned with premain RC line ([ba9f9c6](https://github.com/theory-cloud/TableTheory/commit/ba9f9c697da23b746f5549114e4800331df0ee90))
* **release:** keep staging aligned with premain RC line ([9827164](https://github.com/theory-cloud/TableTheory/commit/9827164cfc907b56ca099afcfa312e40dfd53269))
* **release:** make stable promotion CI-driven ([325e3be](https://github.com/theory-cloud/TableTheory/commit/325e3be580f56b0d8ddaa55fe7a8d3e9288245fc))
* **release:** make stable promotion CI-driven ([f25efba](https://github.com/theory-cloud/TableTheory/commit/f25efbae55786e4eac8f904bb3d317a28b0f5f34))
* **release:** recover immutable RC publish flow ([bd2e481](https://github.com/theory-cloud/TableTheory/commit/bd2e481be6068d00307331e61849fbb2ba9d9b07))
* **release:** recover release cycle guardrails ([b32693a](https://github.com/theory-cloud/TableTheory/commit/b32693ad01d1c24169fde1bd61edc05d96a81490))
* **release:** recover release cycle guardrails ([df7e4f7](https://github.com/theory-cloud/TableTheory/commit/df7e4f71af65fb990e0ef2a42a77d12e8d5d2bae))
* **release:** reset premain manifest baseline ([33cf3bc](https://github.com/theory-cloud/TableTheory/commit/33cf3bce6460db569f017c3127a1606a2414c432))
* **release:** reset premain manifest baseline ([69020e6](https://github.com/theory-cloud/TableTheory/commit/69020e6ab46a9358f0d50347e62a2895a63ff5a2))
* **release:** sync premain baseline after stable releases ([9a26ea6](https://github.com/theory-cloud/TableTheory/commit/9a26ea63b26a6fee7ac274a97f43c26d349934a7))
* **release:** sync premain baseline after stable releases ([807442d](https://github.com/theory-cloud/TableTheory/commit/807442dc55aa38859e92beef79cb883d9e783bfe))
* remediate dependabot dependency alerts ([b9fce7b](https://github.com/theory-cloud/TableTheory/commit/b9fce7ba34b4a0b9a39405bc4020eb678637d9a7))
* remediate dependabot dependency alerts ([9a3dbcb](https://github.com/theory-cloud/TableTheory/commit/9a3dbcb3ec159568d12ea74aed6472d480767f5e))
* remediate rubric dependency scan failures ([0c6b9ee](https://github.com/theory-cloud/TableTheory/commit/0c6b9eec7d38719af075bb09a7908d0a97556ee8))
* remove unreachable json null branch return ([245429b](https://github.com/theory-cloud/TableTheory/commit/245429b8bdc9c3fb307253d44529a53cd1cc683c))
* remove unreachable json null branch return ([aa7902b](https://github.com/theory-cloud/TableTheory/commit/aa7902bfeb560a967408fe038193f8a7e77b5989))
* resolve rubric failures on dynamorm branch ([d2dd8e1](https://github.com/theory-cloud/TableTheory/commit/d2dd8e1da14a7d29eb88a1c53f3bc3eb2591bd9d))
* resolve security alerts in examples ([d986a2f](https://github.com/theory-cloud/TableTheory/commit/d986a2f07aace9a5e8262a3c8a039885885fc6a6))
* **runtime:** apply lambda timeout buffers once ([633cb39](https://github.com/theory-cloud/TableTheory/commit/633cb3923a04449fce837bec97317bb7b02d388e))
* **scripts:** reject symlinks in subtree staging ([91764cd](https://github.com/theory-cloud/TableTheory/commit/91764cd2bba3ef11e7326d4e6302bfd784286e5b))
* **scripts:** reject symlinks in subtree staging ([06747e6](https://github.com/theory-cloud/TableTheory/commit/06747e68a006537a26063fe26b395ac62c952918))
* **security:** bump Go toolchain to go1.25.7 ([033b62c](https://github.com/theory-cloud/TableTheory/commit/033b62cdb9551482020293e4a006e29adb601dac))
* **security:** clear rubric dependency scans ([bbf4481](https://github.com/theory-cloud/TableTheory/commit/bbf44812ff313b534a4d30c6d06cb51573da9b82))
* **security:** harden API key hashing ([2c47b6c](https://github.com/theory-cloud/TableTheory/commit/2c47b6c7dac1084b66448f81dc3d49ce4e4114e0))
* **security:** harden audit and lambda timeout guards ([0e05d23](https://github.com/theory-cloud/TableTheory/commit/0e05d23261b8d6aafa9511c72411d41c42fa5b10))
* **security:** harden protected write policies ([b5e2156](https://github.com/theory-cloud/TableTheory/commit/b5e2156749da4ef6e6de125173c865672811f756))
* **security:** harden protected write policies ([f021d85](https://github.com/theory-cloud/TableTheory/commit/f021d852e9d519e3c9c9f5b83ac14658d285e3bb))
* **security:** upgrade eslint and migrate to flat config ([50b44dc](https://github.com/theory-cloud/TableTheory/commit/50b44dc27e551691e946ea7b1251b25ad8980086))
* support map[string]any model round-trips ([9723c62](https://github.com/theory-cloud/TableTheory/commit/9723c628a0539133c7893a71b802d153fa0f37fd))
* **ts:** raise fast-xml-parser override for audit baseline ([c24b50b](https://github.com/theory-cloud/TableTheory/commit/c24b50bd64c5aad035d6bc6b6d62d051ef104b87))
* **ts:** reject raw transact updates on encrypted models ([f2bcf84](https://github.com/theory-cloud/TableTheory/commit/f2bcf84940b6137c406fbf4048052358210e0c90))
* **types:** decode promoted anonymous embed fields ([c8404d3](https://github.com/theory-cloud/TableTheory/commit/c8404d39e79b2d1eed92c9ad66439a066404c84e))
* unblock legacy naming and lambda KMS config ([897ea3c](https://github.com/theory-cloud/TableTheory/commit/897ea3cd37bff7bc795facc91a3b0451a693d817))

## [1.9.2-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.9.2-rc.1...v1.9.2-rc.1) (2026-06-03)


### Features

* add FaceTheory ISR MetaStore helper ([0812a68](https://github.com/theory-cloud/TableTheory/commit/0812a68b1203f9179d987dd62eb3529e0332d705))
* add TTL archival lifecycle support ([0f1b88d](https://github.com/theory-cloud/TableTheory/commit/0f1b88d7012ea3436964328a73978ff680137f94))
* add TTL archival lifecycle support ([22ffedc](https://github.com/theory-cloud/TableTheory/commit/22ffedcfccff7f4732eaad50473a89bcffd7815e))
* **dms:** add release-state contract foundation ([ce7d3d3](https://github.com/theory-cloud/TableTheory/commit/ce7d3d3961a36ca4a514faf08e89e1f1b9be4957))
* **dms:** add release-state write policy metadata ([c7862d0](https://github.com/theory-cloud/TableTheory/commit/c7862d0f9462aa49c2a4b2911470f5aaeae9fc28))
* **docs:** add Theory Cloud GitHub Pages site ([8f1b7ec](https://github.com/theory-cloud/TableTheory/commit/8f1b7ec0bc30df2024b2129ce78649e80221591f))
* **docs:** add Theory Cloud GitHub Pages site ([7a3fac9](https://github.com/theory-cloud/TableTheory/commit/7a3fac9138eba9901bc81efaedcd700148d3c118))
* **errors:** add release-state mutation errors ([1ce0a38](https://github.com/theory-cloud/TableTheory/commit/1ce0a38e6aae5e13de21ad7850809ed53a04ca32))
* FaceTheory ISR support (FT-T0..FT-T4) ([66eb65a](https://github.com/theory-cloud/TableTheory/commit/66eb65af9b253946c53f4f88af9dc10668e6d5bd))
* FT-T1 lease helper (Go) ([bb6860a](https://github.com/theory-cloud/TableTheory/commit/bb6860a46380f1227a067dd87ea9fd0d8c4f4f34))
* FT-T1 lease helper (Py) ([71b1c01](https://github.com/theory-cloud/TableTheory/commit/71b1c01ca9d8347e8443627c023d698f4ce4d34b))
* FT-T1 lease helper (TS) ([b04dbb1](https://github.com/theory-cloud/TableTheory/commit/b04dbb17cc72b25d0ba328680170b64fcc37efc6))
* **go:** add LambdaDB timeout configuration ([5b070f1](https://github.com/theory-cloud/TableTheory/commit/5b070f111b95651efab5315a39ca0ead13c87ee1))
* **go:** add opt-in flat encoding for anonymous embeds ([d25d85c](https://github.com/theory-cloud/TableTheory/commit/d25d85cc323a8c2e9c97fa20af24495f133d8c66))
* **go:** enforce release-state write policies ([15eaf11](https://github.com/theory-cloud/TableTheory/commit/15eaf1188c60612101ea0363a81e452f61d76735))
* **go:** enforce release-state write policies ([5d2c433](https://github.com/theory-cloud/TableTheory/commit/5d2c4332c68222948009c5a17469e62ae68e1d37))
* **go:** parse release-state write policy metadata ([c1f912b](https://github.com/theory-cloud/TableTheory/commit/c1f912b237dcfe25a8aa4700899d15ae8bc2277e))
* **mocks:** add transaction builder mock ([ea39672](https://github.com/theory-cloud/TableTheory/commit/ea39672edffd22bf24b1471e244c14b79f06211d))
* **mocks:** add transaction builder mock ([16ab5a5](https://github.com/theory-cloud/TableTheory/commit/16ab5a5d7b22d1087973f96a5c69b2c3a3796c3e))
* **py:** add lambda timeout helper ([d43d776](https://github.com/theory-cloud/TableTheory/commit/d43d776d4e156ce4473a4fe838ad125dec007397))
* **py:** enforce release-state write policies ([0cb8fd4](https://github.com/theory-cloud/TableTheory/commit/0cb8fd47ce53f813db2ba7f7d608c78c94c44371))
* **py:** enforce release-state write policies ([bde01a5](https://github.com/theory-cloud/TableTheory/commit/bde01a56ce8ffa1affda1a3c1bdab3c9df062aff))
* **releasestate:** add release-state helpers ([49bdc14](https://github.com/theory-cloud/TableTheory/commit/49bdc144cb6627d74d28a0e6535df67385e8c676))
* **releasestate:** add transactional transition helpers ([87a0fa6](https://github.com/theory-cloud/TableTheory/commit/87a0fa635ce8c2c7a4be5386ab5af38fcb9fd8f0))
* **releasestate:** validate provenance confidence metadata ([8228344](https://github.com/theory-cloud/TableTheory/commit/82283447200a6c3b96524cd7dd42511642bbf14e))
* **runtime:** add lambda timeout buffer parity ([ce82176](https://github.com/theory-cloud/TableTheory/commit/ce821765944b255bbe9437883c74386d5a5f6eeb))
* TableTheory FaceTheory ISR MetaStore ([f57744d](https://github.com/theory-cloud/TableTheory/commit/f57744d5275744d6399563d29f920bcc045a2f68))
* **ts:** enforce release-state write policies ([85c0ebe](https://github.com/theory-cloud/TableTheory/commit/85c0ebef53bb454d97fffa9d8da77ac975024110))
* **ts:** enforce release-state write policies ([45dba94](https://github.com/theory-cloud/TableTheory/commit/45dba949eb408d815315cc1f0c45f6be76207111))


### Bug Fixes

* add encrypted compat mode for legacy plaintext ([d42d1f5](https://github.com/theory-cloud/TableTheory/commit/d42d1f533e7966ee041829d9bc736d31a419c376))
* add first-class DynamORM naming support ([4ef6e22](https://github.com/theory-cloud/TableTheory/commit/4ef6e225941a93c34aa51f35c0418c8ac51c0499))
* address dependabot alerts 52-55 and pin Go 1.26.2 ([37e0fcb](https://github.com/theory-cloud/TableTheory/commit/37e0fcb982bd6dbf4d996d690b269daa067e4d2e))
* address nested naming review feedback ([2cccc21](https://github.com/theory-cloud/TableTheory/commit/2cccc2194943049f460b540b603db7d9ac6c26e0))
* address security/quality findings ([3b56fb4](https://github.com/theory-cloud/TableTheory/commit/3b56fb4986d2a0e93ced5c682caa2fd401a62087))
* address security/quality findings ([f7adaf7](https://github.com/theory-cloud/TableTheory/commit/f7adaf79d1e7248d3b2654f5c82b33f79cc6e4ac))
* adopt go1.26.4 toolchain to clear Go stdlib CVEs (GO-2026-5037/5038/5039) ([35a6149](https://github.com/theory-cloud/TableTheory/commit/35a61497585ed83d6c140cade71fd63777e95a9c))
* align multilang json field semantics ([4eb0a3b](https://github.com/theory-cloud/TableTheory/commit/4eb0a3bf89a8f12789806d4d1dbea4a6acd515a0))
* bump example cryptography dependency ([452c78f](https://github.com/theory-cloud/TableTheory/commit/452c78f03cdff5cbb2c9ca18b68ead69239fe53a))
* camel case nested dynamorm attributes ([4b24fe3](https://github.com/theory-cloud/TableTheory/commit/4b24fe3c0b016b55edcdd1444fad3b9eb57c6241))
* **ci:** add local theorycloud subtree staging for tabletheory ([0760427](https://github.com/theory-cloud/TableTheory/commit/0760427d60010f404c12443adc19e2ec6367da0f))
* **ci:** add local theorycloud subtree staging for tabletheory ([b7e6913](https://github.com/theory-cloud/TableTheory/commit/b7e6913cfee641cdda0f1975c4d7dd4f1cb054bf))
* **ci:** add theorycloud subtree sync and publish commands ([c205c38](https://github.com/theory-cloud/TableTheory/commit/c205c380aaa179ad654a855512d3f289d2baed03))
* **ci:** add theorycloud subtree sync and publish commands ([3a2c338](https://github.com/theory-cloud/TableTheory/commit/3a2c3381793c99648bd4347e36aea90ca3d6edc4))
* **ci:** align theorycloud publish workflow with KT role contract ([002dfd1](https://github.com/theory-cloud/TableTheory/commit/002dfd1fa23c6c1827022f2af5d7ba36a173e2b6)), closes [#135](https://github.com/theory-cloud/TableTheory/issues/135)
* **ci:** allow stable version alignment on premain promotion ([56984bd](https://github.com/theory-cloud/TableTheory/commit/56984bd4cec3490246b7aeed0b7e17de3268105f))
* **ci:** allow stable version alignment on premain promotion ([4f679a8](https://github.com/theory-cloud/TableTheory/commit/4f679a87f30e9bdb99539a0eeb65d396b2614fc9))
* **ci:** automate theorycloud subtree publishing for tabletheory ([7c47ed2](https://github.com/theory-cloud/TableTheory/commit/7c47ed26cf3e4c72ea67621912a3be8c455460ee))
* **ci:** automate theorycloud subtree publishing for tabletheory ([849c359](https://github.com/theory-cloud/TableTheory/commit/849c35967094a663e00ffc8418f781c8e6726968))
* **ci:** finalize theorycloud publish workflow against KT lab contract ([5f10fc0](https://github.com/theory-cloud/TableTheory/commit/5f10fc0f5271744bbf0f57c21ddaf65cd523129e))
* **ci:** make release assets immutable ([1ef4aca](https://github.com/theory-cloud/TableTheory/commit/1ef4aca7bbd6ef6fffe9a86b9f33b1c0c28e1e97))
* **ci:** make release assets immutable ([e9ad219](https://github.com/theory-cloud/TableTheory/commit/e9ad219f7d806e8faff7422c45ea2b1f066e3904))
* **ci:** make theorycloud publish helper awscurl-compatible ([87945e2](https://github.com/theory-cloud/TableTheory/commit/87945e28d020ff5c516f6c13b4fe6b4e29ff22c5))
* **ci:** publish pages from staging ([6fad410](https://github.com/theory-cloud/TableTheory/commit/6fad410eabcf5305782f019667bddd50a1728f61))
* **ci:** publish pages from staging ([720034c](https://github.com/theory-cloud/TableTheory/commit/720034c9b83989df45c2c1eecb440805d3b0d8fa))
* **ci:** retry git fetch in branch-version-sync ([1b4d855](https://github.com/theory-cloud/TableTheory/commit/1b4d8557fe66c5c333846469369d9e5285cc1232))
* **ci:** unblock dependency scan gates ([addb0e4](https://github.com/theory-cloud/TableTheory/commit/addb0e46088cd066440fff3ddc12747304749896))
* clear rubric failures locally ([2c39a7b](https://github.com/theory-cloud/TableTheory/commit/2c39a7bccbd70b6b8ffc0ee74328442f8eb0cc17))
* clear rubric lint regressions ([f40bb1f](https://github.com/theory-cloud/TableTheory/commit/f40bb1f923320cd03556a6ca7add6f6fe3b526d3))
* complete theorydb json field semantics ([93f36a1](https://github.com/theory-cloud/TableTheory/commit/93f36a179d5b36fc3d1f0c31fe69da0426fe5367))
* **deps:** address dependabot alerts 56 and 57 ([aa73fe8](https://github.com/theory-cloud/TableTheory/commit/aa73fe829a37be4e8c70b3855fdf45bb2def161b))
* **deps:** address dependabot alerts 56 and 57 ([df77c48](https://github.com/theory-cloud/TableTheory/commit/df77c48f831e8cd537c4aa8f26d2055251d86ec0))
* **deps:** patch fast xml builder advisories ([c212509](https://github.com/theory-cloud/TableTheory/commit/c2125093f8f0f30e36cfdfb88865afc0c91e5899))
* **deps:** release dependency security refresh ([bfba142](https://github.com/theory-cloud/TableTheory/commit/bfba14202b5470459ebe907faefc4c38a65db499))
* **deps:** release dependency security refresh ([d459e61](https://github.com/theory-cloud/TableTheory/commit/d459e61c557ebea5fb3f221eafec2c711234d4fc))
* **deps:** resolve npm audit vulnerabilities ([83fadbd](https://github.com/theory-cloud/TableTheory/commit/83fadbd3a7d5e8f3bab0fa85f0da4250bbb1e27a))
* **deps:** update python deps for security ([712cfb5](https://github.com/theory-cloud/TableTheory/commit/712cfb5b08c410d57bbc28bd6664768b2c74b30b))
* **dms:** complete release-state contract plumbing ([4560a6a](https://github.com/theory-cloud/TableTheory/commit/4560a6a03c8ceedbba47c21b4ffe2d18e37c3bbb))
* **docs:** align workflow and transaction docs ([e39b02e](https://github.com/theory-cloud/TableTheory/commit/e39b02e48eb0d13e06971ada2718dcb877deb268))
* **docs:** correct transaction and locking examples ([4af61ab](https://github.com/theory-cloud/TableTheory/commit/4af61abcc849c88c7d6821d70aab7f0f212508f6))
* **docs:** correct wheel-asset name, encryption descriptor, transactions, and update signatures ([1691abc](https://github.com/theory-cloud/TableTheory/commit/1691abcf294783abf9e2ec98450ea70df97ff07a))
* **docs:** narrow ttl and lifecycle claims ([3a4f464](https://github.com/theory-cloud/TableTheory/commit/3a4f4641d280aa1be515cc5d41d3696db76f65f4))
* **docs:** replace internal-doc links in subtree-published pages ([5af9535](https://github.com/theory-cloud/TableTheory/commit/5af9535eafe6b6c52e3af1daffc73b166e5c26fc))
* **docs:** replace invented APIs with the real public surface ([cc45c6e](https://github.com/theory-cloud/TableTheory/commit/cc45c6e7928fc99f933dbfe7e01b60257df4caef))
* **docs:** replace invented APIs with the real public surface ([dbf261a](https://github.com/theory-cloud/TableTheory/commit/dbf261a91279e3ee4e7a9f2e5989737e2b5d2d9a))
* **docs:** use markdown-file relative links across new content ([0b7c6e9](https://github.com/theory-cloud/TableTheory/commit/0b7c6e9d7a817ebebc8cf5541745dc3f17c5df03))
* **expr:** remap grouped filter placeholders on merge ([fd18b2a](https://github.com/theory-cloud/TableTheory/commit/fd18b2a097c26f7fdde4cf9fca216bf5cf8b02e1))
* **go:** add promoted-field plan for anonymous embeds ([0c6edbc](https://github.com/theory-cloud/TableTheory/commit/0c6edbca9ea3525b9693a452f6dd562949f393d0))
* **go:** restore rubric for promoted embed helper parity ([5f7b861](https://github.com/theory-cloud/TableTheory/commit/5f7b86101caf5ed26e1b1bd506dd914b5cfbb4a6))
* improved transaction handling ([30a5d7a](https://github.com/theory-cloud/TableTheory/commit/30a5d7acc371cbcbd38bee1d240e5eab24d49882))
* keep brace-expansion audit finding visible ([1b9d1f6](https://github.com/theory-cloud/TableTheory/commit/1b9d1f6e563e6975f66db147b278cb18c2e5fe82))
* keep brace-expansion audit finding visible (THE-1757) ([27e379c](https://github.com/theory-cloud/TableTheory/commit/27e379c138489b0f47d64239705b5c20b3d5ba7a))
* keep brace-expansion visible without failing SEC-2 ([158e3c5](https://github.com/theory-cloud/TableTheory/commit/158e3c5b72b61668c936451dd56e2de26d329c0f))
* **marshal:** preserve omitempty pointer zero values ([6d1077c](https://github.com/theory-cloud/TableTheory/commit/6d1077cd1d3928ca07a92a5dfcc6ace723da1d8f))
* **marshal:** preserve omitempty pointer zero values ([bf83062](https://github.com/theory-cloud/TableTheory/commit/bf83062d1f84d301b78597c102ceb408209aaf70))
* **marshal:** restore anonymous embed custom hook handling ([2a61893](https://github.com/theory-cloud/TableTheory/commit/2a61893fce94ad3184969b85fa6b88b147f20847))
* **marshal:** restore anonymous embed custom hook handling ([a60ccb4](https://github.com/theory-cloud/TableTheory/commit/a60ccb4272ed287421cc5b246addc0f7c2eb910c))
* **marshal:** share anonymous-embed field traversal across helpers ([384ccdc](https://github.com/theory-cloud/TableTheory/commit/384ccdcaab94cd688df045c0789b12edfe785d51))
* **mocks:** satisfy lint gates ([a9cd117](https://github.com/theory-cloud/TableTheory/commit/a9cd1170fc200489369b76f098635321ed3d81c0))
* **pages:** build docs at custom-domain root ([9c25333](https://github.com/theory-cloud/TableTheory/commit/9c253336aaaff867982c4c1c60edf09505183922))
* **pages:** build docs at custom-domain root ([1cd1e89](https://github.com/theory-cloud/TableTheory/commit/1cd1e89e01df6758ff4ff1af48bbcb1fc8bf7c93))
* **pages:** pin third-party action refs to commit SHAs ([4b2e477](https://github.com/theory-cloud/TableTheory/commit/4b2e4772cc12a47fb2f76eee75c5b8f90845caa1))
* pass SEC-2 dependency scans ([a3e0390](https://github.com/theory-cloud/TableTheory/commit/a3e0390d31d99435fd8c8aa7d48c2aeb7845d977))
* **premain:** restore prerelease version alignment ([9b07cdb](https://github.com/theory-cloud/TableTheory/commit/9b07cdb7df5e69be8012374f742d89252ffde942))
* **py:** execute shared P0 contract fixtures ([b2e72f7](https://github.com/theory-cloud/TableTheory/commit/b2e72f706f4b07eaf99498f70b4451d747ec8acb))
* **py:** reject lifecycle timestamp update tampering ([8448730](https://github.com/theory-cloud/TableTheory/commit/844873044c4f95efabebf1f02034873eefed2ff2))
* **py:** reject lifecycle timestamp update tampering (THE-1750) ([3835d07](https://github.com/theory-cloud/TableTheory/commit/3835d078337484a26974068f3114553491a98439))
* **query:** bind named updates to matched attribute names ([a054b34](https://github.com/theory-cloud/TableTheory/commit/a054b34004b47ace31a18608b87467674411e424))
* **query:** honor promoted anonymous embeds in public unmarshal helpers ([3128cc8](https://github.com/theory-cloud/TableTheory/commit/3128cc898bccbbda809de5dddfb9358f2883ca2e))
* **query:** make encrypted batch retries safe ([f6ec65c](https://github.com/theory-cloud/TableTheory/commit/f6ec65c158401b752c530a4b888ec020390edc41))
* **query:** make encrypted batch retries safe ([e343a29](https://github.com/theory-cloud/TableTheory/commit/e343a2917a7f0121818ebca9aaccbe57109ce0d5))
* **query:** migrate remaining anonymous-embed helper walkers ([7dbdd5f](https://github.com/theory-cloud/TableTheory/commit/7dbdd5f1957b99b9205495cb4370988423ad83d1))
* release go1.26.4 toolchain adoption as 1.9.2-rc.1 ([5060eed](https://github.com/theory-cloud/TableTheory/commit/5060eed52ebe1921f94d840676e644a485a0f48b))
* **release:** allow pending stable promotion guard ([40c44a5](https://github.com/theory-cloud/TableTheory/commit/40c44a5ac779d4585eda53c8f4f2f6e1fe5a398a))
* **release:** detect exhausted immutable RC state ([f2ba073](https://github.com/theory-cloud/TableTheory/commit/f2ba0739b5b6e48d3afa0921e529b2f98c6d46f5))
* **release:** detect exhausted immutable RC state ([dc593a3](https://github.com/theory-cloud/TableTheory/commit/dc593a3b0f6ac47b4d6e643d6f4defd8309e9027))
* **release:** keep staging aligned with premain RC line ([ba9f9c6](https://github.com/theory-cloud/TableTheory/commit/ba9f9c697da23b746f5549114e4800331df0ee90))
* **release:** keep staging aligned with premain RC line ([9827164](https://github.com/theory-cloud/TableTheory/commit/9827164cfc907b56ca099afcfa312e40dfd53269))
* **release:** recover immutable RC publish flow ([bd2e481](https://github.com/theory-cloud/TableTheory/commit/bd2e481be6068d00307331e61849fbb2ba9d9b07))
* **release:** recover release cycle guardrails ([b32693a](https://github.com/theory-cloud/TableTheory/commit/b32693ad01d1c24169fde1bd61edc05d96a81490))
* **release:** recover release cycle guardrails ([df7e4f7](https://github.com/theory-cloud/TableTheory/commit/df7e4f71af65fb990e0ef2a42a77d12e8d5d2bae))
* **release:** reset premain manifest baseline ([33cf3bc](https://github.com/theory-cloud/TableTheory/commit/33cf3bce6460db569f017c3127a1606a2414c432))
* **release:** reset premain manifest baseline ([69020e6](https://github.com/theory-cloud/TableTheory/commit/69020e6ab46a9358f0d50347e62a2895a63ff5a2))
* **release:** sync premain baseline after stable releases ([9a26ea6](https://github.com/theory-cloud/TableTheory/commit/9a26ea63b26a6fee7ac274a97f43c26d349934a7))
* **release:** sync premain baseline after stable releases ([807442d](https://github.com/theory-cloud/TableTheory/commit/807442dc55aa38859e92beef79cb883d9e783bfe))
* remediate dependabot dependency alerts ([b9fce7b](https://github.com/theory-cloud/TableTheory/commit/b9fce7ba34b4a0b9a39405bc4020eb678637d9a7))
* remediate dependabot dependency alerts ([9a3dbcb](https://github.com/theory-cloud/TableTheory/commit/9a3dbcb3ec159568d12ea74aed6472d480767f5e))
* remediate rubric dependency scan failures ([0c6b9ee](https://github.com/theory-cloud/TableTheory/commit/0c6b9eec7d38719af075bb09a7908d0a97556ee8))
* remove unreachable json null branch return ([245429b](https://github.com/theory-cloud/TableTheory/commit/245429b8bdc9c3fb307253d44529a53cd1cc683c))
* remove unreachable json null branch return ([aa7902b](https://github.com/theory-cloud/TableTheory/commit/aa7902bfeb560a967408fe038193f8a7e77b5989))
* resolve rubric failures on dynamorm branch ([d2dd8e1](https://github.com/theory-cloud/TableTheory/commit/d2dd8e1da14a7d29eb88a1c53f3bc3eb2591bd9d))
* resolve security alerts in examples ([d986a2f](https://github.com/theory-cloud/TableTheory/commit/d986a2f07aace9a5e8262a3c8a039885885fc6a6))
* **runtime:** apply lambda timeout buffers once ([633cb39](https://github.com/theory-cloud/TableTheory/commit/633cb3923a04449fce837bec97317bb7b02d388e))
* **scripts:** reject symlinks in subtree staging ([91764cd](https://github.com/theory-cloud/TableTheory/commit/91764cd2bba3ef11e7326d4e6302bfd784286e5b))
* **scripts:** reject symlinks in subtree staging ([06747e6](https://github.com/theory-cloud/TableTheory/commit/06747e68a006537a26063fe26b395ac62c952918))
* **security:** bump Go toolchain to go1.25.7 ([033b62c](https://github.com/theory-cloud/TableTheory/commit/033b62cdb9551482020293e4a006e29adb601dac))
* **security:** clear rubric dependency scans ([bbf4481](https://github.com/theory-cloud/TableTheory/commit/bbf44812ff313b534a4d30c6d06cb51573da9b82))
* **security:** harden API key hashing ([2c47b6c](https://github.com/theory-cloud/TableTheory/commit/2c47b6c7dac1084b66448f81dc3d49ce4e4114e0))
* **security:** harden audit and lambda timeout guards ([0e05d23](https://github.com/theory-cloud/TableTheory/commit/0e05d23261b8d6aafa9511c72411d41c42fa5b10))
* **security:** harden protected write policies ([b5e2156](https://github.com/theory-cloud/TableTheory/commit/b5e2156749da4ef6e6de125173c865672811f756))
* **security:** harden protected write policies ([f021d85](https://github.com/theory-cloud/TableTheory/commit/f021d852e9d519e3c9c9f5b83ac14658d285e3bb))
* **security:** upgrade eslint and migrate to flat config ([50b44dc](https://github.com/theory-cloud/TableTheory/commit/50b44dc27e551691e946ea7b1251b25ad8980086))
* support map[string]any model round-trips ([9723c62](https://github.com/theory-cloud/TableTheory/commit/9723c628a0539133c7893a71b802d153fa0f37fd))
* **ts:** raise fast-xml-parser override for audit baseline ([c24b50b](https://github.com/theory-cloud/TableTheory/commit/c24b50bd64c5aad035d6bc6b6d62d051ef104b87))
* **ts:** reject raw transact updates on encrypted models ([f2bcf84](https://github.com/theory-cloud/TableTheory/commit/f2bcf84940b6137c406fbf4048052358210e0c90))
* **types:** decode promoted anonymous embed fields ([c8404d3](https://github.com/theory-cloud/TableTheory/commit/c8404d39e79b2d1eed92c9ad66439a066404c84e))
* unblock legacy naming and lambda KMS config ([897ea3c](https://github.com/theory-cloud/TableTheory/commit/897ea3cd37bff7bc795facc91a3b0451a693d817))

## [1.9.2-rc.1](https://github.com/theory-cloud/TableTheory/compare/v1.9.1...v1.9.2-rc.1) (2026-06-03)


### Bug Fixes

* adopt go1.26.4 toolchain to clear Go stdlib CVEs (GO-2026-5037/5038/5039) ([35a6149](https://github.com/theory-cloud/TableTheory/commit/35a61497585ed83d6c140cade71fd63777e95a9c))
* release go1.26.4 toolchain adoption as 1.9.2-rc.1 ([5060eed](https://github.com/theory-cloud/TableTheory/commit/5060eed52ebe1921f94d840676e644a485a0f48b))
* **release:** allow pending stable promotion guard ([40c44a5](https://github.com/theory-cloud/TableTheory/commit/40c44a5ac779d4585eda53c8f4f2f6e1fe5a398a))
* **release:** recover release cycle guardrails ([b32693a](https://github.com/theory-cloud/TableTheory/commit/b32693ad01d1c24169fde1bd61edc05d96a81490))
* **release:** recover release cycle guardrails ([df7e4f7](https://github.com/theory-cloud/TableTheory/commit/df7e4f71af65fb990e0ef2a42a77d12e8d5d2bae))
* **release:** sync premain baseline after stable releases ([9a26ea6](https://github.com/theory-cloud/TableTheory/commit/9a26ea63b26a6fee7ac274a97f43c26d349934a7))
* **release:** sync premain baseline after stable releases ([807442d](https://github.com/theory-cloud/TableTheory/commit/807442dc55aa38859e92beef79cb883d9e783bfe))

## [1.9.1](https://github.com/theory-cloud/TableTheory/compare/v1.9.0...v1.9.1) (2026-05-30)


### Bug Fixes

* **ci:** publish pages from staging ([6fad410](https://github.com/theory-cloud/TableTheory/commit/6fad410eabcf5305782f019667bddd50a1728f61))
* keep brace-expansion audit finding visible ([1b9d1f6](https://github.com/theory-cloud/TableTheory/commit/1b9d1f6e563e6975f66db147b278cb18c2e5fe82))
* keep brace-expansion audit finding visible (THE-1757) ([27e379c](https://github.com/theory-cloud/TableTheory/commit/27e379c138489b0f47d64239705b5c20b3d5ba7a))
* keep brace-expansion visible without failing SEC-2 ([158e3c5](https://github.com/theory-cloud/TableTheory/commit/158e3c5b72b61668c936451dd56e2de26d329c0f))
* **pages:** build docs at custom-domain root ([9c25333](https://github.com/theory-cloud/TableTheory/commit/9c253336aaaff867982c4c1c60edf09505183922))
* **pages:** build docs at custom-domain root ([1cd1e89](https://github.com/theory-cloud/TableTheory/commit/1cd1e89e01df6758ff4ff1af48bbcb1fc8bf7c93))
* **py:** reject lifecycle timestamp update tampering ([8448730](https://github.com/theory-cloud/TableTheory/commit/844873044c4f95efabebf1f02034873eefed2ff2))
* **py:** reject lifecycle timestamp update tampering (THE-1750) ([3835d07](https://github.com/theory-cloud/TableTheory/commit/3835d078337484a26974068f3114553491a98439))

## [1.9.0-rc.2](https://github.com/theory-cloud/TableTheory/compare/v1.9.0-rc.1...v1.9.0-rc.2) (2026-05-30)


### Bug Fixes

* **ci:** publish pages from staging ([6fad410](https://github.com/theory-cloud/TableTheory/commit/6fad410eabcf5305782f019667bddd50a1728f61))
* **ci:** publish pages from staging ([720034c](https://github.com/theory-cloud/TableTheory/commit/720034c9b83989df45c2c1eecb440805d3b0d8fa))
* keep brace-expansion audit finding visible ([1b9d1f6](https://github.com/theory-cloud/TableTheory/commit/1b9d1f6e563e6975f66db147b278cb18c2e5fe82))
* keep brace-expansion audit finding visible (THE-1757) ([27e379c](https://github.com/theory-cloud/TableTheory/commit/27e379c138489b0f47d64239705b5c20b3d5ba7a))
* keep brace-expansion visible without failing SEC-2 ([158e3c5](https://github.com/theory-cloud/TableTheory/commit/158e3c5b72b61668c936451dd56e2de26d329c0f))
* **pages:** build docs at custom-domain root ([9c25333](https://github.com/theory-cloud/TableTheory/commit/9c253336aaaff867982c4c1c60edf09505183922))
* **pages:** build docs at custom-domain root ([1cd1e89](https://github.com/theory-cloud/TableTheory/commit/1cd1e89e01df6758ff4ff1af48bbcb1fc8bf7c93))
* **py:** reject lifecycle timestamp update tampering ([8448730](https://github.com/theory-cloud/TableTheory/commit/844873044c4f95efabebf1f02034873eefed2ff2))
* **py:** reject lifecycle timestamp update tampering (THE-1750) ([3835d07](https://github.com/theory-cloud/TableTheory/commit/3835d078337484a26974068f3114553491a98439))

## [1.9.0](https://github.com/theory-cloud/TableTheory/compare/v1.8.4...v1.9.0) (2026-05-28)


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
