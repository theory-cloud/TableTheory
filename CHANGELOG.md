# Changelog

## Unreleased

### Breaking Changes

* **go:** align `ConsistentRead()` on GSI queries with the cross-runtime contract by returning
  `ErrInvalidOperator` instead of silently dropping the flag, including when the Go query optimizer auto-selects a GSI
  from key conditions. Semver decision: this parity repair is release-major material and must not ship as a patch/minor.
* **go:** align newly written DynamoDB shapes for binary and set-tagged fields with TypeScript, Python, and the DMS type
  matrix: `[]byte` writes as `B`, numeric `theorydb:"set"` slices write as `NS`, binary set slices write as `BS`, empty
  set-tagged slices write as `NULL`, and unsupported set element types fail at write time. Legacy shape-driven reads
  remain supported, but filters/conditions over mixed old/new data may need migration. Semver decision: this persisted
  shape convergence is release-major material and must not ship as a patch/minor.

### Features

* **go:** add `NewWithClient` and a state-backed `pkg/testing/fakedb` consumer test fake
* **contract:** validate the Go state-backed fake against the P0 contract corpus
* **ts:** add a stateful DynamoDB `send()` fake to the public testkit
* **py:** add a stateful DynamoDB testkit fake and top-level re-exports
* add TTL-aware schema provisioning across Go, TypeScript, and Python helpers
* add CDK archival construct for DynamoDB TTL expirations to S3 Glacier lifecycle storage

### Bug Fixes

* add first-class legacy DynamORM naming support for uppercase `PK`/`SK` plus camelCase non-key attributes
* align Go, TypeScript, Python, and DMS `json` field semantics around native structured storage plus legacy string compatibility
* harden npm audit allowlist handling so audit service errors fail closed
* **ts:** honor opt-in exact number unmarshalling for update-builder return values and native JSON-number attributes
* update Python lockfile security baseline and remove stale pip-audit exception
* prevent Python Lambda timeout guards from being retried by query and scan helpers
* align Python lifecycle and optimistic-lock writes with the shared P0 contract fixtures

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
