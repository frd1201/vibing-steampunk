# FORK.md — operating this fork

`frd1201/vibing-steampunk` is a **downstream distribution** of
[`oisee/vibing-steampunk`](https://github.com/oisee/vibing-steampunk): own
release line, own pace, but permanently mergeable with upstream. Fixes that are
useful to everyone still go up as pull requests.

Rationale and decisions live in
[`reports/2026-08-03-001-fork-strategy.md`](reports/2026-08-03-001-fork-strategy.md).
**This file is the operational short reference** — the one to keep open.

---

## Setup

```bash
git clone https://github.com/frd1201/vibing-steampunk.git
cd vibing-steampunk
git remote add upstream https://github.com/oisee/vibing-steampunk.git
git fetch upstream --prune

go build -o vsp ./cmd/vsp
```

`go install github.com/frd1201/vibing-steampunk/cmd/vsp@latest` does **not**
work by design: `go.mod` deliberately keeps the upstream module path
`github.com/oisee/vibing-steampunk` so that upstream merges stay conflict-free.
Build from this repo, or use the release artifacts.

---

## The two rules

**1. Anything upstream-worthy branches off `upstream/main`, never off `main`.**
Otherwise the pull request drags this fork's commits along and becomes noise for
the upstream maintainer.

**2. Never cherry-pick, always merge.** A cherry-pick produces an identical
commit under a different SHA, after which `git branch --merged` and
`git log upstream/main..main` no longer tell the truth about what has already
been submitted upstream. The single exception is back-filling a PR for work
that is already on `main` — see below.

---

## Branches

| Prefix | Purpose | Branches off | Merges into |
|---|---|---|---|
| `main` | integration branch, source of all tags | — | — |
| `feat/*`, `fix/*` | own work, upstream-worthy | `upstream/main` | PR upstream **and** merge into `main` |
| `fork-only/*` | deliberately not upstream-worthy | `main` | `main` only |
| `upstream-pr/<n>` | adopting someone else's upstream PR | that PR's head | `main` |
| `sync/upstream-YYYY-MM` | catch branch for an upstream merge | `main` | `main` |
| `probe/pr-<n>` | throwaway trial merge | `main` | deleted |

**Never delete a `feat/*` or `fix/*` branch while its upstream PR is open** —
GitHub auto-closes a pull request when its head branch disappears. Merging the
fork-internal PR does *not* release the branch; only the upstream PR does.
`fork-only/*` and `upstream-pr/*` branches have no upstream PR and are deleted
as soon as they are merged.

When merging any PR on GitHub, always pick **"Create a merge commit"**.
"Squash and merge" is a cherry-pick in disguise and breaks rule 2.

---

## Monthly upstream check

`upstream/main` is a **fetch snapshot**, not a live mirror — it shows whatever
the last `git fetch upstream` pulled down. That is the whole reason this check
exists. Two minutes, once a month, or whenever GitHub reports activity upstream:

```bash
git fetch upstream --prune
git log --oneline main..upstream/main        # empty? done, nothing to do.
```

If it is not empty, merge through a catch branch rather than straight onto
`main`:

```bash
git switch -c sync/upstream-$(date +%Y-%m) main
git merge upstream/main
go build ./... && go test ./...              # gate: must be green
git switch main && git merge --no-ff sync/upstream-$(date +%Y-%m)
git branch -d sync/upstream-$(date +%Y-%m)
```

---

## Workflow A — your own change

One question decides the branch type: *does this solve a problem every user has,
and is it free of site- or customer-specific detail?*

### Yes — upstream-worthy: one branch, two PRs

```bash
git fetch upstream
git switch -c feat/<topic> upstream/main       # not off main!
# ... develop, test ...
git push -u origin feat/<topic>

# PR 1 — into our own main. Runs the CI gate before the merge.
gh pr create --repo frd1201/vibing-steampunk --base main

# PR 2 — upstream. Same head branch, different base repo.
gh pr create --repo oisee/vibing-steampunk --base main
```

One head branch serves both PRs; GitHub allows this because the bases differ.
Merge PR 1 with **"Create a merge commit"** — never "Squash and merge", which
is a cherry-pick in disguise and breaks rule 2.

**Do not wait for the upstream merge.** The change goes into `main` as soon as
PR 1 is green, so it is available here. Upstream may take months, or never.

**Do not delete the branch when GitHub offers to after merging PR 1.** That
closes PR 2. Only the upstream PR governs a branch's lifetime — the
fork-internal PR does not hold it.

Review fixes requested upstream are pushed to the same branch: they show up in
PR 2 automatically, and come back into `main` through **another**
`git merge --no-ff feat/<topic>`. Never by copying the commit across.

### No — fork-only: one branch, one PR

```bash
git switch -c fork-only/<topic> main
# ... develop, test ...
git push -u origin fork-only/<topic>
gh pr create --repo frd1201/vibing-steampunk --base main
```

No upstream PR — site-specific work has no business there. Delete the branch
right after the merge; nothing holds it.

Then add a row to *Fork-only changes* below.

---

## Workflow B — adopt an upstream PR

The trigger is always a **concrete problem here** that already has a fix
upstream. Not "that PR looks useful".

```bash
gh pr checkout <n> --repo oisee/vibing-steampunk --branch upstream-pr/<n>

git diff upstream/main...upstream-pr/<n>       # 1. read the diff

git switch -c probe/pr-<n> main                # 2. trial merge, judge conflicts
git merge upstream-pr/<n>

go build ./... && go test ./...                # 3. gate
go test -tags=integration -v ./pkg/adt/        #    against a real system

git switch main                                # 4. adopt, with provenance
git merge --no-ff upstream-pr/<n> \
  -m "Merge upstream PR #<n> (<author>) — <summary>"
git branch -D probe/pr-<n>
```

Then: CHANGELOG entry under *Adopted from upstream*, and a row in the
*Upstream PR decisions* table below.

**If the PR touches code we already changed, decide before merging:** either our
version wins (record it as rejected, with the reason), or theirs wins (roll ours
back, close our PR with a pointer), or both are partly right (new `feat/*`
branch off `upstream/main` combining them, offered upstream as a new PR).

---

## Back-filling a PR

If something upstream-worthy ends up on `main` without a PR, this is the only
place a cherry-pick is correct:

```bash
git switch -c feat/<topic> upstream/main
git cherry-pick <sha>
go build ./... && go test ./...
git push -u origin feat/<topic>
gh pr create --repo oisee/vibing-steampunk --base main
```

The cost is that the change now exists under two SHAs. Rule 1 exists precisely
to avoid ever needing this.

---

## Our open upstream PRs

Keep these branches alive until the PR is closed.

**Nothing is open upstream as of 2026-09-02.** All three closed within four days
of the August rebase, which is the fact this table now exists to record.

| PR | Branch | Subject | Status |
|---|---|---|---|
| ~~[#120](https://github.com/oisee/vibing-steampunk/pull/120)~~ | `fix/csrf-head-fallback-and-session-type` | CSRF HEAD→GET fallback, secure-cookie fix, `SAP_SESSION_TYPE` | **closed** 2026-08-31 — see *Superseded by upstream* |
| ~~[#121](https://github.com/oisee/vibing-steampunk/pull/121)~~ | `feat/incl-write-support` | INCL (PROG/I) write support | **merged** upstream (`d8ee78c`), after 131 days open |
| ~~[#126](https://github.com/oisee/vibing-steampunk/pull/126)~~ | `fix/search-type-filter-issue-119` | server-side search type filter | **merged** upstream (`598e37c`), after 123 days open |
| ~~[#164](https://github.com/oisee/vibing-steampunk/pull/164)~~ | `fix/query-top-0-returns-100-rows` | `--top 0` / `all_rows` returns every row | **merged** upstream (`df4a186`) |

Both `feat/*` branches are now released: nothing upstream holds them, so they can
be deleted. The close-if-unanswered dates (2027-04-23, 2027-05-01) are void.

One thing the merges cost us: upstream's copies are the revisions as submitted,
not the revisions on `main`. The September sync therefore brought a second,
older `WriteInclude` and a duplicate `TestLockObject_RejectsLockWithoutHandle`
back into the tree, both of which had to be dropped by hand. Expect that shape
whenever one of our PRs lands after we have kept working on the branch's subject
here.

### Superseded by upstream

**#120 — closed 2026-08-31.** Upstream solved the same problem independently and better,
and part of our version is now known to be wrong.

- `6b136b7` shipped the HEAD→GET fallback with a 401 **and** 403 short-circuit
  — the same shape as ours.
- `ff32cd7` then removed the 403 half as a bug: some systems refuse the HEAD and
  answer the GET perfectly well, so short-circuiting there reintroduces exactly
  the unusability the fallback exists to prevent.
- Our follow-up commit `886a9b2` adds that 403 short-circuit. It is dated after
  upstream's revert but was written without sight of it.

Upstream's `fetchCSRFTokenWithReauth` / `probeCSRFToken` also carries SSO-redirect
detection and reauth-on-tokenless-200, which ours never had. The sync takes
upstream's version whole.

What was genuinely ours in #120 and is **not** upstream: the `httpCookieJar`
Secure-stripping wrapper and the `SAP_SESSION_TYPE` env var. Both survive in
`main` and are now pinned by `pkg/adt/fork_corrections_test.go` and
`internal/mcp/fork_corrections_test.go`. If #120 is reopened in any form, it
should carry only those two.

**#121 and #126 stay valid.** Upstream's include work (`4dff03f`) is a *read*
fallback only and never touches the write path; and `SearchObjectByType` /
`CanonicalObjectType` do not exist upstream at all. Both branches were rebased
onto `upstream/main` (`9b8789d`) on 2026-08-31 — each had one textual conflict
(upstream had independently added the same maxResults+1 over-fetch trick for
truncation detection that #126 collided with; #121 collided with upstream's
own widening of the WriteSource type switch to `FUNC`/`MSAG`/`TABL`). Both are
`MERGEABLE`/`CLEAN` again after the push.

**Review dates:** the module-path trigger is **void as written** (checked
2026-08-28). It rested on upstream's six-month code-commit clock started
2026-04-15; upstream has since shipped 341 commits in the week to 2026-08-27,
so "six months without a code commit" is not going to fire. The other two
triggers in that clause — our PRs being rejected, or a deliberate hard fork —
still stand and are the ones to watch.
Close-if-unanswered dates: #121 on **2027-04-23**, #126 on **2027-05-01**.
#120 is closed, see below.

### Pending back-fill

`4b80378` (corrNr at LOCK time) is **upstream-worthy** — it follows the SAP ADT
API spec and helps anyone editing objects in transportable packages. It was
developed on a fork-only branch before this operating model existed, so no PR
exists.

The September sync raised the price of not doing it. `LockObject` had four
parameters here and three upstream, so every upstream call site is written
three-argument and each one arrives as a build break at the next merge — twice
now, and the September occurrence (`pkg/adt/session_affinity_test.go`) merged
without a conflict at all before failing to compile. `b615466` makes `corrNr`
variadic, which makes both spellings valid and takes the recurrence to zero.

**Back-fill `b615466` together with `4b80378`.** The variadic signature is the
part upstream can accept without changing a single call site of their own, which
makes it the version worth offering. Branch off `upstream/main`, cherry-pick
both, open a PR.

---

## Upstream PR decisions

Every adopted or rejected upstream PR gets a row, with the reason. Nothing is
adopted yet.

| PR | Author | Subject | Decision | Reason |
|---|---|---|---|---|
| [#108](https://github.com/oisee/vibing-steampunk/pull/108) | dme007 | deploy session ordering, MODIFICATION_SUPPORT | **adopted** 2026-08-03 (`2d4fa5f`) | `1bc5804` shows SAP's `IF_ADT_LOCK_RESULT` documents `NoModification` as `CO_MOD_SUPPORT_NOT_NEEDED`, so the guard from `22517d4` was a false positive on customer-namespace objects. Also brings redirect header preservation and `ICMENOSESSION` recovery, which we lacked. Conflicts: comment-only in `workflows_deploy.go`; in `http.go` both sides kept (our CSRF `HEAD`→`GET` fallback from #120 plus their trace helpers and `clearSAPSessionCookies`); their redirect handling is `CheckRedirect` in `pkg/adt/config.go`, not `http.go`. |
| [#125](https://github.com/oisee/vibing-steampunk/pull/125) | dme007 | skip redundant mutation gate after lock | **superseded** by #108 | same subject area; #108 covers it |
| [#139](https://github.com/oisee/vibing-steampunk/pull/139) | enricoandreoli | program includes as source-bearing objects | **moot** | our #121 landed upstream first (`d8ee78c`) |
| [#145](https://github.com/oisee/vibing-steampunk/pull/145) | zooloo303 | reuse an object's open transport instead of 409 | **adopted** 2026-09-02 (`3bbf200`) | merged upstream as `8dd2ef8`; `resolveWriteTransport` re-runs `checkTransportableEdit` on the discovered request, so auto-reuse cannot bypass `--allow-transportable-edits` or `--allowed-transports`. Interacts cleanly with corrNr-at-LOCK: a supplied transport makes the reuse a no-op, an empty one lets `LockResult.CorrNr` supply it |
| [#149](https://github.com/oisee/vibing-steampunk/pull/149) | lin2qwer1-cloud | SRVB read routing, `binding_category` docs | **adopted** 2026-09-02 (`3bbf200`) | SRVB was advertised and dropped by the switch; the docs had 0/1 inverted (0=UI, 1=A2X) |
| [#106](https://github.com/oisee/vibing-steampunk/pull/106) | dme007 | install: propagate Description, `PackageExists` probe | **adopted** 2026-09-02 (`3bbf200`) | `GetPackage` cannot tell "no package" from "empty package" |
| [#128](https://github.com/oisee/vibing-steampunk/pull/128) | andreasmuenster | client for browser auth | **adopted** 2026-09-02 (`3bbf200`) | arrived carrying the revert of our #120 content — see *Superseded by upstream* |
| [#173](https://github.com/oisee/vibing-steampunk/pull/173) | oisee | transport listing rebuilt around the tree | **adopted** 2026-09-02 (`3bbf200`) | no fork code in the area |
| [#174](https://github.com/oisee/vibing-steampunk/pull/174) | oisee | activation parser | **adopted** 2026-09-02 (`3bbf200`) | supersedes our own fix for the same defect: theirs merges the wrapped and root shapes for messages, entries **and** properties, ours only for messages |
| [#167](https://github.com/oisee/vibing-steampunk/pull/167) | oisee | issue #91 session affinity | **adopted** 2026-09-02 (`3bbf200`) | supersedes most of our #88 work — see the sync row below |

Watched, no collision known: #150 (ActivateMultiple), #148 (activation
parsing), #138 (InstallZADTVSP source deploy), #130 (ENHO read), #107
(WebSocket proxy).

---

## Fork-only changes

Deliberately not upstreamed. No PR is owed for these.

| Commit | Subject | Why fork-only |
|---|---|---|
| `d752536` | CHANGELOG for v3.0.0 | our own version line |
| `3f7a90c` | goreleaser release target → `frd1201` | must not point upstream releases at this fork |

---

## Upstream syncs

| Sync | Upstream head | Scope | Notes |
|---|---|---|---|
| `sync/upstream-2026-08` | `9b8789d` (2026-08-27) | 341 commits, 314 files, +52,440 | 13 conflicts. Upstream had independently built several of our fixes, so most resolutions were a choice between two implementations rather than a combination — upstream won wherever the effect was the same. Three defects would have merged in silently: a new upstream file calling the three-arg `LockObject` (broke `go build`), a duplicate jar-reset that discarded the `httpCookieJar` wrapper, and unreachable code that `go vet` rejects. |
| `sync/upstream-2026-09` | `8dd2ef8` (2026-09-02) | 50 commits, 48 files, +4,119 | 17 conflicts — more than the August sync on an eighth of the volume, because both trees had spent the week on the same defect. Upstream's issue #91 work supersedes most of our #88 work and was taken whole. Two traps: `resetCookieJar` would have deleted the `httpCookieJar` wrapper (the August trap, renamed), and a new upstream *test* file merged clean and then failed to compile against our four-argument `LockObject` — fixed at the root by `b615466`. Three of our own PRs landed upstream during the window and came back as duplicate definitions. |

**What made this sync survivable** was writing the missing tests *first*. Eleven
of sixteen fork corrections had no test at all, so the merge had no acceptance
criterion until `fork_corrections_test.go` existed in `pkg/adt/` and
`internal/mcp/`. Do that again before the next large sync: a correction with no
test is a correction the merge can delete in silence.

## Known issues — found, not fixed

Both came out of the post-merge review on 2026-08-28. Neither was introduced by
the sync: they predate it here **and** exist in `upstream/main` unchanged, so
each is a candidate for an upstream PR rather than a fork-only patch. Recorded
here so the next person does not have to rediscover them.

### 1. `retryRequest` does not reconcile the session it just renewed

`pkg/adt/http.go:331`. `Request()` reads three things back off every response:
`adoptServerCookies` (`:238`), the CSRF token (`:265`) and the session id
(`:270`). `retryRequest` reads back **none** of them.

That matters because of *when* it runs. Its four callers (`:249`, `:260`,
`:296`, `:317`) are the CSRF-refresh-on-403 path, the session-expiry recovery
and the SSO re-auth — precisely the moments SAP issues a fresh
`SAP_SESSIONID`. After the retry the jar holds the new session id while
`config.Cookies` still holds the dead one; the next `addCookies` sends both
under the same name, the server honours one of them, and the cached CSRF token
belongs to the other. The caller is told its token is invalid.

Upstream's own commit for `adoptServerCookies` — `b9c22f3` *"take the session
id SAP reissues, instead of sending two"* — describes exactly this failure. The
fix was added to `Request()` and not to `retryRequest`, so the gap between the
two paths grew by one step at the sync; the omission itself is older than that.

**Shape of the fix:** have `retryRequest` do what `Request()` does after
reading the body — `adoptServerCookies(resp)`, then store a returned CSRF token
and session id. Small and testable with `httptest`: serve a retry response
carrying a new `SAP_SESSIONID` and a new `X-CSRF-Token`, then assert the next
request sends one session id and the new token. While there, note that
`retryRequest` re-sets the session-type header itself right after
`setDefaultHeaders` has already done it — a second copy of that logic which
does not know about `SessionKeep`.

**Why no PR yet:** deliberately parked 2026-08-28. It is upstream-worthy (rule
1 — branch off `upstream/main`, not `main`), so when it is picked up it should
go through Workflow A rather than being patched here first.

**Rechecked 2026-09-02, after the September sync: still open.** Upstream's
issue #91 work went through `Request()` and the CSRF probe and left
`retryRequest` untouched, so the gap between the two paths is exactly where it
was. With #121, #126 and #164 all merged, this is now the strongest candidate
for the next upstream PR — behind the `b615466` + `4b80378` back-fill, which
has a concrete recurring cost attached to it.

### 2. Unsynchronised jar swap on a shared `http.Client`

`clearSAPSessionCookies` (`pkg/adt/http.go:641`) assigns `hc.Jar` while other
goroutines may be inside `httpClient.Do` reading `c.Jar`. `cmd/vsp/fetchsources.go:93`
fans several goroutines out over one `*adt.Client`, and the MCP server serves
concurrent tool calls on one client too, so the race is reachable — the session
expiry path at `:296` is not covered by `reauthMu`. Detectable under
`go test -race`. A fix needs to decide what the concurrency contract of
`Transport` actually is, which is why it is not a quick patch.

**Rechecked 2026-09-02: unchanged, and deliberately so.** The September sync
kept `clearSAPSessionCookies` over upstream's `resetCookieJar`, so the race
comes with it. Upstream's version has the same race and additionally drops the
Secure-stripping jar, so taking theirs would have traded a known race for a
known regression.

---

## Known SHA-tracking gaps

Content is correct in all cases; only git's ability to match commits is lost.
Relevant when syncing after upstream merges #120 or #121.

| On `main` | Duplicate of | On branch |
|---|---|---|
| `a47b225` | `2ea6004` | `feat/incl-write-support` |
| `886a9b2` | `59b401b` | `fix/csrf-head-fallback-and-session-type` |

`6b2cece` (the parked `fork-only/onprem-edit-fixes` branch) was adopted **in
part only**: its corrNr work became `4b80378`, its configurable
`MODIFICATION_SUPPORT` guard was dropped because upstream PR #108 removed the
guard outright. The original commit no longer exists as a reachable SHA — the
branch was deleted after the split.

To re-audit what on `main` is not covered by any upstream PR, see section 9.1 of
the strategy report.

---

## Release

- The fork owns the **3.x** version band; upstream is on 2.x. If upstream ever
  tags a 3.x, switch to `v3.4.0-fork.1`.
- Tags are cut from `main` only, and only when the CI job on `main` is green
  **and** an integration run against a real SAP system has passed.

### Local test baseline

`go test ./...` is **not** fully green on a Windows dev box without a C
compiler. Measured 2026-08-03 on `main` at `4deea3b`, Go 1.26.5:

| | |
|---|---|
| packages `ok` | 14 |
| packages without tests | 4 |
| packages failing | 2 — `cmd/vsp`, `pkg/cache` |
| failing tests | 7, all `go-sqlite3 requires cgo to work` |

`CGO_ENABLED` defaults to `0` when no C compiler is on `PATH`, which stubs out
`go-sqlite3`. This is an environment limitation, not a defect. Local gate is
therefore **"no new failures beyond these 7"**; the CI job on `ubuntu-latest`
runs with cgo enabled and is the authoritative gate. Install MinGW/MSYS2 if you
want the sqlite tests locally.
- CHANGELOG keeps two sections per release: *Own changes* and *Adopted from
  upstream* (with PR number and author).

## Module path — review trigger

`go.mod` stays on `github.com/oisee/vibing-steampunk`. Move it to a fork-owned
path in a single commit if any of these happens:

- ~~upstream goes **six months without a code commit** — the clock started
  2026-04-15, so **check on 2026-10-15**~~ — **void, checked 2026-08-28.**
  Upstream shipped 341 commits in the week to 2026-08-27. Dormancy is not the
  scenario to plan for; keeping up with an active upstream is; or
- ~~our upstream PRs are rejected~~ — **void, checked 2026-09-02.** #121, #126
  and #164 were all merged upstream between 2026-09-01 and 2026-09-02; #120 was
  closed as superseded, not rejected. Every clause of this trigger that rested
  on upstream being unresponsive has now been disproved by upstream. What is
  left is the third one; or
- we deliberately decide to hard-fork.

Cost of the move: 104 files (73 of them Go), plus a permanent merge tax on every
upstream sync — and that tax is now measurably higher than when this was
written, since upstream is moving fast enough for the fork to need real syncs.
