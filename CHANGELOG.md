# Changelog

## [0.2.0](https://github.com/thedandano/go-apply/compare/v0.1.8...v0.2.0) (2026-04-22)


### ⚠ BREAKING CHANGES

* **orchestrator:** ApplyConfig gains an Orchestrator field. Existing callers that only set LLM continue to work unchanged; the new Orchestrator field takes precedence for keyword extraction when non-nil.
* **mcpserver:** new MCP tools submit_tailor_t1 and submit_tailor_t2 complete Phase 2 of multi-turn pipeline orchestration.
* **mcpserver:** get_score tool removed; use load_jd → submit_keywords → finalize
* get_score tool removed; use load_jd + submit_keywords instead

### Features

* **augment:** add similarity, source, keyword attrs to retrieval logs (M2) ([#92](https://github.com/thedandano/go-apply/issues/92)) ([19540fb](https://github.com/thedandano/go-apply/commit/19540fba37118ddbe00ae5f8d0a74bbc3ba77fe0))
* **augment:** per-keyword embedding cache with individual hit/miss logs ([61197c2](https://github.com/thedandano/go-apply/commit/61197c201b1310705e358f7a215189da10147997))
* **cli:** add onboard --reset --yes to clear profile ([335995e](https://github.com/thedandano/go-apply/commit/335995e92bd88396578b56ac604488e19aac81f0))
* **cli:** add onboard --reset --yes to clear profile ([3878f03](https://github.com/thedandano/go-apply/commit/3878f0355d38f949c6794e4725ef5acc27c3e29b))
* **cli:** add setup mcp --override and --agent all ([ecc8f22](https://github.com/thedandano/go-apply/commit/ecc8f22b1ee51252304e78306e0277e36a74a8ad))
* **cli:** add setup mcp --override and --agent all ([35c1eb1](https://github.com/thedandano/go-apply/commit/35c1eb129002996201e239effdc599a0c44acb77))
* **logger:** accept stderr level via input struct ([48c0874](https://github.com/thedandano/go-apply/commit/48c087451ce5fb461bfa0fb45c3bf475a8ac459b))
* **logger:** add LOG_FORMAT=json and LOG_LEVEL=debug env var support (M1) ([#91](https://github.com/thedandano/go-apply/issues/91)) ([2fc312d](https://github.com/thedandano/go-apply/commit/2fc312d4721e523e889bdb24b12942c7ab2d5cd3))
* **logger:** add stage banner logs for session and pipeline stages ([0774863](https://github.com/thedandano/go-apply/commit/0774863fdcecd0b769a2dd567c86001c8063e1b3))
* **logger:** human-readable log format + go-apply logs command ([734c8e4](https://github.com/thedandano/go-apply/commit/734c8e47aacb19314d6cc79f272cbf397f133eaf))
* **logger:** human-readable log format + go-apply logs command ([b911a0a](https://github.com/thedandano/go-apply/commit/b911a0a7478ce362d6c9b5c3ff3adebfb7eb0108))
* **logger:** structured payload logging with truncation, redaction, and debug helpers ([e6e644c](https://github.com/thedandano/go-apply/commit/e6e644c6a87f06499d811bfa8c1adfe9a8dbd2e5))
* **mcpserver:** add load_jd, submit_keywords, finalize multi-turn tools ([6a54630](https://github.com/thedandano/go-apply/commit/6a5463043860ff8c5e38b71371521a648475d832))
* **mcpserver:** add session store and envelope types for multi-turn tools ([0b793cc](https://github.com/thedandano/go-apply/commit/0b793cc937007759cc38fa7ac12479f05ffb4ca5))
* **mcpserver:** add session store and envelope types for multi-turn tools ([57495b6](https://github.com/thedandano/go-apply/commit/57495b69a60a3f279f9813a6394f4f2a5eb645d9))
* **mcpserver:** add submit_tailor_t1 and submit_tailor_t2 tools (Phase 2) ([b276b16](https://github.com/thedandano/go-apply/commit/b276b16b3ac93272cca52f4e085cd187d570b941))
* **mcpserver:** land phase 1 multi-turn pipeline (PRs [#65](https://github.com/thedandano/go-apply/issues/65)-[#68](https://github.com/thedandano/go-apply/issues/68)) ([58c5c85](https://github.com/thedandano/go-apply/commit/58c5c856e286864f6a1f6eeaab9476376942986d))
* **mcpserver:** structured response format with T1/T2 score breakdown ([96c26ae](https://github.com/thedandano/go-apply/commit/96c26ae91560e0b174257fdc170eff2ec713be8f))
* **orchestrator:** port.Orchestrator + LLMOrchestrator for CLI/TUI mode (Phase 3) ([#70](https://github.com/thedandano/go-apply/issues/70)) ([8685b5d](https://github.com/thedandano/go-apply/commit/8685b5d51524716da32cba1f0b958c538b3296aa))
* **pipeline:** retrieval at tailoring time + verbose diffs + PII redaction ([#86](https://github.com/thedandano/go-apply/issues/86)) ([cfe6df7](https://github.com/thedandano/go-apply/commit/cfe6df70318c4f72eced65d9c600a56cf2d87ca8))
* remove get_score — multi-turn load_jd/submit_keywords/finalize ([#68](https://github.com/thedandano/go-apply/issues/68)) ([964a19a](https://github.com/thedandano/go-apply/commit/964a19a1de7af83122eab268d292cd6769d383c4))
* **workflow:** add OnboardSummary and finalize session summary ([f9e6e36](https://github.com/thedandano/go-apply/commit/f9e6e36f666f9811211a4c20de79f715d36c50d0))
* **workflow:** add OnboardSummary and finalize session summary ([0f27039](https://github.com/thedandano/go-apply/commit/0f2703995a3c8aefbaa4625700a580080f31c0ab))


### Bug Fixes

* **config:** demote config-loaded log from Info to Debug ([860367f](https://github.com/thedandano/go-apply/commit/860367fd21503ba28a3c83175ce4a77ab1171ce4))
* **logger:** stderrOnly respects StderrLevel; harden test cleanup ([ce6a78e](https://github.com/thedandano/go-apply/commit/ce6a78ee34492bd01a8af70eb4b691ac67535588))
* **logger:** wire slog.SetDefault and cfg.LogLevel ([#72](https://github.com/thedandano/go-apply/issues/72)) ([a5f0bd8](https://github.com/thedandano/go-apply/commit/a5f0bd84aa6d7f74685a9df45471629fb0895b36))
* **logs:** fix goimports grouping in logs_test.go ([d548837](https://github.com/thedandano/go-apply/commit/d548837c438d6f51d4f81562cf5805bc189972a2))
* **mcpserver:** expose profile.onboarded in get_config and skip re-onboarding ([62b23b3](https://github.com/thedandano/go-apply/commit/62b23b33b3a33ce58f73035c2b4f181ef1c84f1f))
* **mcpserver:** expose profile.onboarded in get_config to skip re-onboarding ([792e502](https://github.com/thedandano/go-apply/commit/792e50286e65c21e34bb94ff856db2e0014bc844))
* **mcpserver:** update E2E tests for get_config profile object response ([0352c7b](https://github.com/thedandano/go-apply/commit/0352c7b6405c9d627f2a6a3a874c874567a3f07c))
* **pipeline:** fail fast on keyword extraction failure ([3ba615c](https://github.com/thedandano/go-apply/commit/3ba615c4a28b426bc6241f841e0d698bc2383d29))
* **pipeline:** fail fast on keyword extraction failure ([90c5f98](https://github.com/thedandano/go-apply/commit/90c5f98c0d273283e868be541aff4f4c2bd12ccc))
* **security:** wrap external content in delimiters to prevent prompt injection ([#71](https://github.com/thedandano/go-apply/issues/71)) ([b4a0346](https://github.com/thedandano/go-apply/commit/b4a0346d29fec1643706b104def6e69bd6df28a3))


### Code Refactoring

* Batch 1 simplification pass (L1/T1/C1/R2/TST1) ([#87](https://github.com/thedandano/go-apply/issues/87)) ([c0c527a](https://github.com/thedandano/go-apply/commit/c0c527acfe67d8e06500bb68bf7b176e51866ecc))
* delete augment pipeline (SQLite, vector search, embeddings) ([#98](https://github.com/thedandano/go-apply/issues/98)) ([1ed6532](https://github.com/thedandano/go-apply/commit/1ed6532512d5fc351704b59d4555fb9a9695e31c))
* extract CheckOnboarded + guard CLI run command (M3) ([#93](https://github.com/thedandano/go-apply/issues/93)) ([83960f1](https://github.com/thedandano/go-apply/commit/83960f11d52dc2a32e44d7906b19afbb9f8c1997))
* **logger:** extract magic perms into named consts ([9b8c76f](https://github.com/thedandano/go-apply/commit/9b8c76f1fccb3cc0b7cb3943fcfb212c322764b2))
* **pipeline:** expose AcquireJD and ScoreResumes as public stage methods ([b7e6e57](https://github.com/thedandano/go-apply/commit/b7e6e57414d7812fa86e6f0f736dbb215650ecc1))
* **pipeline:** expose AcquireJD and ScoreResumes as public stage methods ([9f62a90](https://github.com/thedandano/go-apply/commit/9f62a9056caddc2b5e3affbc710cb73303aae6ed))


### Documentation

* **claude:** document squash+rebase merge strategy for linear main history ([640e33a](https://github.com/thedandano/go-apply/commit/640e33a7913be1a115fa2db6ac5a71650618c27c))
* **readme:** logging section, CLI reference audit, and agent compatibility badges ([bb32d49](https://github.com/thedandano/go-apply/commit/bb32d491c5d3b090846f521056ecf7a3b075912e))

## [0.1.8](https://github.com/thedandano/go-apply/compare/v0.1.7...v0.1.8) (2026-04-16)


### Bug Fixes

* **agentconfig:** register claude plugin at ~/.claude/plugins/go-apply/ ([67585c4](https://github.com/thedandano/go-apply/commit/67585c40b8b9bcaf47238f5e3c146b10bf5a3fbc))
* **agentconfig:** register claude plugin at ~/.claude/plugins/go-apply/ ([ff29f05](https://github.com/thedandano/go-apply/commit/ff29f05cfc3128b690b57df1dee821577f3867fc))
* **agentconfig:** register claude plugin at ~/.claude/plugins/go-apply/ ([d4243eb](https://github.com/thedandano/go-apply/commit/d4243eb4b2d54f92eb52b048b2cc3da79092b3c1))
* **bdd:** update get_score param names in BDD steps ([0288063](https://github.com/thedandano/go-apply/commit/0288063c09542468921d07ff3ca0b15757f06018))
* **mcpserver:** address PR review — realistic JD, full onboard, score assertions ([b46feae](https://github.com/thedandano/go-apply/commit/b46feae7e38e6836de98a3827b6908ed1704a885))
* **mcpserver:** clarify Claude orchestration responsibilities in get_score description ([fc8895d](https://github.com/thedandano/go-apply/commit/fc8895d86d2e18004afafdbd504e56b3c5f998d1))
* **mcpserver:** descriptive var names, log branching, blocked-test assertions ([eba8dcc](https://github.com/thedandano/go-apply/commit/eba8dcce5dd3055967346c0efd4acb7cc37c6fc4))
* **mcpserver:** rename tool params, generic log, keyword assertions ([713e904](https://github.com/thedandano/go-apply/commit/713e904795c7acaa1f1593d05948cfeb9c769369))


### Code Refactoring

* **agentconfig:** extract claudePluginsDir const and pluginDir helper ([fe3d3d3](https://github.com/thedandano/go-apply/commit/fe3d3d3cdedab8d34b9c2a319970cc04ba9a97ff))
* **mcpserver:** extract MCP server into internal/mcpserver ([82c32d4](https://github.com/thedandano/go-apply/commit/82c32d4c9dfda728cfbfea0164ba71caf71ccc84))
* **mcpserver:** extract MCP server logic into internal/mcpserver ([bdabc8b](https://github.com/thedandano/go-apply/commit/bdabc8b2df269cf85a6cd06c56ae444658e672d4))
* **mcpserver:** rename test files, gate e2e behind integration tag ([03d11bb](https://github.com/thedandano/go-apply/commit/03d11bb48feab72a4b1d2e07d7f220fcfdde9ce5))
* **mcpserver:** rename test files, gate e2e behind integration tag ([ec67b18](https://github.com/thedandano/go-apply/commit/ec67b18b77a12e97425fa0036a9459b9c07a3b01))
* **model:** extract ParseChannel from cli.resolveChannel ([bdd4fed](https://github.com/thedandano/go-apply/commit/bdd4fedbc8741a9ee6904f41e59e354f593dea25))
* **model:** extract ParseChannel from cli.resolveChannel ([#56](https://github.com/thedandano/go-apply/issues/56)) ([20d65ab](https://github.com/thedandano/go-apply/commit/20d65abd34559d43ec16772bf294446b8f7c322a))

## [0.1.7](https://github.com/thedandano/go-apply/compare/v0.1.6...v0.1.7) (2026-04-16)


### Features

* **mcp:** onboarding middleware for get_score tool ([#53](https://github.com/thedandano/go-apply/issues/53)) ([6e43671](https://github.com/thedandano/go-apply/commit/6e43671517447673e654f5216ef5b2ec6c566e41))


### Bug Fixes

* **logger:** file handler always captures debug logs, stderr stays warn+ ([#55](https://github.com/thedandano/go-apply/issues/55)) ([c964ae6](https://github.com/thedandano/go-apply/commit/c964ae62da7c42997523c6d713e4fac5753fbdaa))

## [0.1.6](https://github.com/thedandano/go-apply/compare/v0.1.5...v0.1.6) (2026-04-16)


### Bug Fixes

* **onboarding:** daily log files, auto-init config, validate embedder before onboard ([#51](https://github.com/thedandano/go-apply/issues/51)) ([5af0e9f](https://github.com/thedandano/go-apply/commit/5af0e9f30a992b8f340000e774206ded6746fc83))

## [0.1.5](https://github.com/thedandano/go-apply/compare/v0.1.4...v0.1.5) (2026-04-16)


### Bug Fixes

* **install:** shasum on macOS, version from build info, CI release_created output ([#49](https://github.com/thedandano/go-apply/issues/49)) ([ffe3dbb](https://github.com/thedandano/go-apply/commit/ffe3dbbe490f327f2bedd33961c3dfe24cb4039e))

## [0.1.4](https://github.com/thedandano/go-apply/compare/v0.1.3...v0.1.4) (2026-04-16)


### Features

* **cli:** add self-update channel ([#47](https://github.com/thedandano/go-apply/issues/47)) ([b79fced](https://github.com/thedandano/go-apply/commit/b79fced1e481a6022ef1714110ebb1960ef563c3))

## [0.1.3](https://github.com/thedandano/go-apply/compare/v0.1.2...v0.1.3) (2026-04-15)


### Documentation

* update README — fix MCP tools, TUI coming soon, add roadmap ([#42](https://github.com/thedandano/go-apply/issues/42)) ([b51af4b](https://github.com/thedandano/go-apply/commit/b51af4b2a466c20388fa3712dcecbcc0613e0807))

## [0.1.2](https://github.com/thedandano/go-apply/compare/v0.1.1...v0.1.2) (2026-04-15)


### Features

* **cli:** validate orchestrator config in CLI mode, guard MCP from irrelevant orchestrator keys ([#41](https://github.com/thedandano/go-apply/issues/41)) ([1ac8f05](https://github.com/thedandano/go-apply/commit/1ac8f05e741b1697109d0bdd3f7c26c801ead57d))

## [0.1.1](https://github.com/thedandano/go-apply/compare/v0.1.0...v0.1.1) (2026-04-15)


### Bug Fixes

* multiple bugs found during first end-to-end run against a real job URL ([#38](https://github.com/thedandano/go-apply/issues/38)) ([069a2e3](https://github.com/thedandano/go-apply/commit/069a2e33dceef8a8b6f2b42e15fca1efe009e365))
