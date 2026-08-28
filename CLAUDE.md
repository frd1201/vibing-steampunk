# CLAUDE.md

**vsp** — Go-native MCP server and CLI for SAP ABAP Development Tools (ADT).

> **Doc intent:** CLAUDE.md = dev context. README.md = user onboarding. FORK.md = fork operations. reports/ = research/history. contexts/ = session handoff.

> **This is a fork.** `frd1201/vibing-steampunk`, downstream of `oisee/vibing-steampunk`.
> Before branching, merging, or touching `go.mod` / `.goreleaser.yml`, read [FORK.md](FORK.md).
> Two rules that are easy to break by accident: upstream-worthy work branches off
> `upstream/main` (not `main`), and never cherry-pick — always merge.

> **Facts below verified against the tree on 2026-08-28.** Counts drift as code
> lands. If a number here disagrees with the code, the code is right — fix the
> number rather than working around it.

---

## Current Priorities

### 1. Graph Engine (`pkg/graph/`) — Largely built, adapters incomplete
The package is well past prototype: 44 files, 192 test functions, wired into
both the CLI and the MCP analysis router.

**Built:** core types (`graph.go`), parser dep extraction (`builder_parser.go`),
SQL builders for CROSS and WBCROSSGT (`builder_sql.go`), config and transport
builders, boundary/crossing/effects/scope analysis, and a broad query layer —
impact, health, API surface, rename, slim, signature, class sections, config
where-used, transport boundaries, usage examples. Export formats: DOT, PlantUML,
GraphML.

**Still open:**
- **D010INC adapter** — `SourceD010INC` and `EdgeLoads` exist in `graph.go`, but no builder does.
- **ADT adapters** — `SourceADTCallGraph`, `SourceADTWhereUsed`, `SourceADTCDSDeps` are declared constants only.
- **Unification** — `cmd/vsp/cli_deps.go`, `cmd/vsp/cli_extra.go` and `pkg/ctxcomp/analyzer.go` still carry their own dependency logic alongside `pkg/graph/`.

Design: [002](reports/2026-04-05-002-graph-engine-design.md), [003](reports/2026-04-05-003-graph-engine-alignment-for-claude.md).
Later refinements: [006 knowledge MVP](reports/2026-04-05-006-graph-knowledge-mvp-design.md),
[007 enrichment signals](reports/2026-04-05-007-graph-enrichment-signals-proposal.md),
[2026-04-08-001 boundary direction](reports/2026-04-08-001-boundary-crossing-direction-proposal.md).

### 2. GUI Debugger — Strategic
Plan: MCP debug sessions → DAP → Web UI. ADT REST API mapped from `CL_TPDA_ADT_RES_APP`.
Design: [001](reports/2026-04-05-001-gui-debugger-design.md)

### 3. Fork operations
See [FORK.md](FORK.md) for the live list. Two items with dates attached:
- **2026-10-15** — module-path review trigger (upstream's six-month code-commit clock).
- **Pending back-fill** — `4b80378` (corrNr at LOCK time) is upstream-worthy but has no PR.

### 4. Issue references
Issue numbers in the reports (#88 lock handle, #55 RunReport in APC, #46/#45 sync
script, #2 GUI debugger) refer to **`oisee/vibing-steampunk`** — this fork has no
issue tracker of its own. Their current state is not mirrored here; check upstream
before treating any of them as open.

---

## Build & Test

```bash
go build -o vsp ./cmd/vsp              # Build
go test ./...                           # Unit tests
go test -tags=integration -v ./pkg/adt/ # Integration (needs SAP)
make build                              # Current platform → ./build/
make build-all                          # 3 common platforms (linux-amd64, darwin-arm64, windows-amd64)
make build-all-all                      # All 9 platforms (linux ×4, darwin ×2, windows ×3)
```

`go test ./...` is fully green on Linux with cgo available (verified 2026-08-28,
16 packages ok). On a box without a C compiler, `go-sqlite3` stubs out and
`cmd/vsp` + `pkg/cache` fail with `requires cgo to work` — environment limit, not
a defect. See FORK.md → *Local test baseline*.

Key flags: `--mode focused|expert|hyperfocused`, `--read-only`, `--allowed-packages "Z*"`, `--disabled-groups 5THD`

---

## Codebase

```
cmd/vsp/              CLI entry + 41 top-level commands (68 incl. subcommands)
internal/
  mcp/
    handlers_*.go       Domain handlers (read, edit, debug, graph, ...) — 37 files
    tools_register.go   Registration + mode logic (153 tools)
    tools_focused.go    Focused mode whitelist (102 tools)
    tools_groups.go     11 disableable groups (5/U, T, H, D, C, G, GC, R, I, N, X)
    tools_aliases.go    Tool alias registration
    handlers_universal.go  Hyperfocused single-tool (SAP)
  lsp/                LSP server (jsonrpc, server, types)
pkg/
  adt/                ADT client (HTTP, CSRF, sessions, all SAP ops)
  graph/              Dependency graph engine (see Priorities)
  ctxcomp/            Context compression (dep resolution for read)
  abaplint/           ABAP lexer + parser (76 statement types, 13 lint rules, 8 on by default)
  dsl/                Fluent API, YAML workflows, batch ops
  cache/              In-memory + SQLite (needs cgo)
  config/             Configuration loading
  scripting/          Lua engine
  jseval/             JS evaluation
  ts2abap/, ts2go/    TypeScript transpilation (research)
  llvm2abap/          LLVM→ABAP (research)
  wasmcomp/           WASM→ABAP (research)
embedded/             Embedded ZADT_VSP ABAP sources + deps ZIPs
```

| Task | Files |
|------|-------|
| Add MCP tool | `tools_register.go` + `handlers_*.go` + `tools_focused.go` (+ `tools_groups.go` if groupable) |
| Add ADT operation | `pkg/adt/client.go`, `crud.go`, `devtools.go`, `codeintel.go` |
| Add graph feature | `pkg/graph/` |
| Add lint rule | `pkg/abaplint/rules.go` + wire into `defaultRules()` in `lint.go` |
| Add ABAP statement type | `pkg/abaplint/matcher.go` (`register()`) |
| Add integration test | `pkg/adt/integration_test.go` |
| Fix MCP/docs/config | `README.md`, `docs/cli-agents/*`, `handlers_universal.go` |

---

## Adding a New MCP Tool

1. Handler in `handlers_*.go`:
```go
func (s *Server) handleX(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
    name, _ := req.GetArguments()["name"].(string)
    result, err := s.adtClient.Method(ctx, name)
    if err != nil { return newToolResultError(err.Error()), nil }
    return mcp.NewToolResultText(format(result)), nil
}
```
2. Register in `tools_register.go` with `shouldRegister("X")`
3. Route in the matching router — analysis tools go through `routeAnalysisAction` in `handlers_analysis.go`
4. Add to `tools_focused.go` if needed in focused mode
5. Add to a group in `tools_groups.go` if it should be disableable via `--disabled-groups`

**A new lint rule is not live until it is in `defaultRules()`.** Five of the 13
implemented rules (`select_star`, `hardcoded_credentials`, `catch_cx_root`,
`commit_in_loop`, `dynamic_call_no_try`) exist but are not wired into the default
set — check before assuming a rule runs.

---

## Common Issues

1. **CSRF errors** — auto-refreshed in `http.go`
2. **Lock conflicts** — edit handler does auto lock/unlock
3. **Session issues** — some CRUD/debugger flows are session-sensitive; verify stateful/stateless before changing transport or auth logic
4. **Auth** — use basic OR cookies, not both
5. **ZADT_VSP** — WebSocket debug/RFC/RunReport require it installed on SAP
6. **cgo** — `pkg/cache` SQLite tests need a C compiler; without one they fail by design

## Security

Never commit `.env`, `cookies.txt`, `.mcp.json`, or local agent/MCP config files (all in `.gitignore`).

### Sanitize policy for tracked docs, tests, and examples

The public repo must not contain concrete identifiers that tie code or
docs to a live SAP system, a real user, or a customer's ABAP namespace.
Anything that does belongs under `.local/` (gitignored) and never in
`contexts/`, `reports/`, `docs/`, or any tracked test fixture.

**Never in tracked files:**
- Real SAP usernames — use `TESTUSER`
- Real hostnames or IPs — use `dev.example.local`, `prodsys-a.example`, `trialsys.example`
- System aliases that name a live box — use `devsys`, `devsys-adt`, `prodsys-a`, `prodsys-b`
- Live transport numbers (`DEVK[0-9]+`, `R[0-9]{2}K[0-9]+`, `D[0-9]{2}K[0-9]+`) — use `TR-EXAMPLE`
- Live change request IDs — use `CR-EXAMPLE`
- Customer ABAP namespaces from real projects — use synthetic `ZDEMO_*`, `ZCL_DEMO_*`, `ZIF_DEMO_*`, `$ZDEMO`
- Customer transport attribute names — use `Z_CR_ATTR`
- Real passwords, API keys, bearer tokens (obvious, but stated)
- Real person names tied to private systems (OSS attribution for upstream libraries is fine — "user X on private host Y" is not)

**Always OK in tracked files:**
- `$ZHIRTEST*`, `ZCL_HIRT*`, `ZCUSTOM_DEVELOPMENT` — pre-agreed synthetic fixtures
- Public GitHub handles that are already in the Go module path
- Upstream OSS attribution for library authors

**Operational scratch goes under `.local/`** — session notes, live CR
dumps, bug repros with real identifiers, debugging transcripts. The
`.local/` dir is gitignored. If you need to reference it from a
tracked doc, redact first.

**Before every commit that touches `reports/`, `contexts/`, `docs/`,
or test fixtures:** scan the staged diff for the identifier families
above. The detection signature (concrete literal list of past-leaked
strings) lives at `.local/scripts/check-identifiers.sh` and is
gitignored on purpose — the signature itself would otherwise be the
leak it is trying to prevent. Structural patterns safe to commit:

```bash
git diff --cached | grep -nE \
  '\b[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b|' \
  '\b[A-Z][0-9]{2}K[0-9]{6}\b|' \
  '\bDEVK[0-9]{6,}\b'
```

That catches IPv4 literals and SAP transport IDs without hardcoding
a specific customer's values. Pair it with the private signature
file for the names-based families (usernames, hostnames, ABAP object
prefixes). If either matches, move the content under `.local/` and
replace the tracked version with a synthetic placeholder. Rule of
thumb: "would a stranger reading this file be able to identify the
customer, the system, or a live account?" If yes, redact.

## Conventions

Reports: `reports/YYYY-MM-DD-NNN-title.md`. SAP objects: `ZADT_<nn>_<name>`, `ZCL_ADT_<name>`, packages `$ZADT*`.

---

## Areas Requiring Care

| Area | Risk | Notes |
|------|------|-------|
| `pkg/graph/` | Adapters incomplete | Parser + CROSS/WBCROSSGT/config/transport builders exist; D010INC and ADT adapters are declared constants with no builder |
| Dep logic duplication | Three implementations | `cli_deps.go`, `cli_extra.go`, `ctxcomp/analyzer.go` each predate `pkg/graph/`; changing one does not change the others |
| `pkg/abaplint/lint.go` | Silent no-op | 5 of 13 rules are not in `defaultRules()` and never run |
| `handlers_debugger.go` | WebSocket-only | REST breakpoints 403 on newer SAP; use ZADT_VSP. `handlers_debugger_legacy.go` holds the old path |
| `handlers_amdp.go` | Experimental | Session works, breakpoints unreliable |
| `pkg/adt/ui5.go` | Read-only | Write needs `/UI5/CL_REPOSITORY_LOAD` |
| `pkg/llvm2abap/`, `pkg/wasmcomp/`, `pkg/ts2abap/`, `pkg/ts2go/` | Research | Not production; don't treat as stable |
| `pkg/adt/debugger.go` (REST) | Deprecated | Prefer `websocket_debug.go` |
| `pkg/cache/` | cgo-dependent | SQLite backend needs a C compiler |
| `docs/cli-agents/*` | Config drift | Codex TOML format may differ from Claude/Gemini JSON docs |
