# Upstream sync 2026-09 — two opinions about the same defect

Done 2026-09-02. `upstream/main` at `8dd2ef8`, merged as `3bbf200`, with
`b615466` in front of it. Every number here was measured — a trial merge in a
throwaway worktree first, then the real one.

The operational rules are in [FORK.md](../FORK.md). This file is what this
particular sync taught, which is mostly one thing: **the fork and upstream are
now working on the same code at the same time, and that is a different problem
from catching up.**

---

## Shape

| | August sync | This one |
|---|---|---|
| Commits | 341 | **50** |
| Files | 314 | **48** |
| Insertions | +52,440 | **+4,119** |
| Conflicts | 13 | **17** |
| Elapsed | four months | **six days** |

An eighth of the volume and *more* conflicts. That inversion is the finding.
When upstream was a backlog, most of a sync was new files landing in empty
space. Now almost every conflicted hunk is two texts of the same fix, and the
work is judgement rather than volume.

## Our own PRs came home, and brought older copies of themselves

Upstream merged #121 (INCL write), #126 (search type filter) and #164
(`--top 0`) between 1 and 2 September. Good news, and a mechanical hazard: what
upstream merged is the revision **as submitted**, not the revision on `main`
here, because work continued on the subject after the PR was opened.

So the merge re-introduced:

- a second `WriteInclude` in `pkg/adt/workflows.go`, at the pre-`48fcf5a`
  revision — Lock before SyntaxCheck, which is the order the fix exists to
  correct;
- a second `WriteClassResult` type;
- a second `TestLockObject_RejectsLockWithoutHandle`, testing the same thing
  with different fixtures.

None of that is upstream's fault and none of it is detectable from the conflict
list — the duplicate type and the duplicate test compile fine until Go
complains about the redeclaration, and the older `WriteInclude` would have
compiled *silently* had the names differed. Expect this shape every time one of
our PRs lands after we have kept working on its subject. Diff the two copies
before choosing; do not assume the incoming one is newer.

## Upstream's issue #91 is a better answer than our issue #88

Both trees fixed "a stateless request inside a lock window kills the handle".
Ours made the offending requests stateful. Upstream's keeps them out of the
window:

- `withMutationPackageChecked` / `gateAndMark` mark **one object** as
  package-checked, so `checkMutation` skips only the networked lookup, only for
  that object. Steps 1 and 3 — `--read-only`, `--disallowed-ops`,
  `--allow-transportable-edits` — are pure config predicates and always run.
  Upstream refused PR #125 for skipping the whole gate, which is the difference
  between a session fix and an authorisation bypass. Worth reading before
  touching it.
- `releaseLockAfterFailure` runs the compensating UNLOCK on
  `context.WithoutCancel(ctx)` with its own 30s deadline. Every compensating
  unlock in this package, ours included, reused the caller's context — so a
  mutation that failed **because** its context was cancelled never sent the
  unlock at all. A client timeout inside a lock window was a guaranteed ENQUEUE
  leak. That one is worth knowing independently of the merge.

Taken whole. Our narrower version is superseded.

## Four resolutions where the obvious side loses something

**`resetCookieJar` is the August trap under a new name.** Upstream's new
function builds a bare `cookiejar.New(nil)`. Taking it deletes this fork's
`httpCookieJar` Secure-stripping wrapper and leaves `t.jar` pointing at a dead
jar — over plain HTTP, SAP's `Secure`-flagged session cookies get dropped
again. Kept `clearSAPSessionCookies`.
`TestSessionRecovery_PreservesSecureStrippingJar` is the guard, and it was run
before anything else in `http.go` was touched.

**`fetchCSRFTokenFor` is upstream's, its predicate is ours.** Upstream's probe
tests `stateful || t.config.SessionType == SessionStateful`. That is complete
for upstream and short for us: this fork also has `SessionKeep`, whose entire
point is going stateful once a session exists. A bare `== SessionStateful`
would leave the probe unmarked in exactly the configuration a user reaches for
when they are already fighting lock-handle errors. The merged version keeps
`sessionTypeIsStateful()`.

**`workflows_fileio.go` needed both sides.** Ours carried the `buildSourceURL`
fix — the rename PUT was sending `text/plain` ABAP to the object resource
instead of `.../source/main`; theirs carried `releaseLockAfterFailure` and the
double-unlock fix. Taking either side whole silently loses the other.

**`DeployFromFile` keeps our ordering.** Upstream moved the syntax-error early
return below the lock, which takes an ENQUEUE only to release it unused. Ours
returns first.

## `LockObject` — the recurrence, closed

`LockObject` had four parameters here and three upstream since April. Seven
upstream call sites in this sync use the three-argument form. Six sat inside
conflict markers. **One did not**: `pkg/adt/session_affinity_test.go` is a new
file, merged clean, and failed to compile.

That is the second occurrence — the August sync recorded the same failure in a
different file — and it would have recurred on every sync while the signatures
differed. `b615466` makes `corrNr` variadic: upstream's spelling compiles, ours
keeps sending corrNr on the LOCK request where the ADT API expects it, and the
recurrence goes to zero.

The variadic form is also what makes the back-fill offerable — upstream can take
it without touching a single call site of their own. FORK.md's *Pending
back-fill* now names `b615466` alongside `4b80378` for that reason.

## Gate

`go build ./...`, `go vet ./...`, `go vet -tags integration ./...` and
`go test ./...` all green, cgo included (this box has a C compiler, so the
`pkg/cache` and `cmd/vsp` sqlite tests actually ran). 26 fork-correction cases
pass. `docs_parity_test.go` — upstream's new test that reads README.md and
asserts every published tool count matches what the server registers — passes,
because the README hunk took upstream's corrected 147/102 figures.

**Not run: `go test -tags=integration ./pkg/adt/`.** No SAP system in this
environment. That gate is still owed before `main`, and it is not a formality
here: the session-affinity marker, the detached compensating unlock and the
#144 transport reuse all change what goes over the wire inside a lock window,
and no `httptest` mock proves an ENQUEUE survives a cancelled mutation.

## What is still open

- **`retryRequest` reconciles nothing it renews** (FORK.md, known issue 1).
  Rechecked: upstream's #91 work went through `Request()` and the CSRF probe and
  left `retryRequest` alone. Unchanged, and now the strongest candidate for the
  next upstream PR after the back-fill.
- **The unsynchronised jar swap** (known issue 2). Kept, knowingly: upstream's
  version has the same race *and* drops the Secure-stripping jar.
- **The back-fill**: `b615466` + `4b80378`, off `upstream/main`, per Workflow A.
- **Upstream's own open items** on #91, recorded in their CLAUDE.md and now in
  ours: the keep-alive ticker (on by default, 5m) and the MCP
  cross-tool-call window are still unguarded lock windows.
