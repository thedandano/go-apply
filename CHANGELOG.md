# Changelog

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
