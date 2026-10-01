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
git config remote.upstream.tagOpt --no-tags   # upstream's v2.* tags stay out
git fetch upstream --prune

go build -o vsp ./cmd/vsp
```

Upstream's release tags must not enter this clone: after every sync they sit
closer to `HEAD` than ours, and anything that asks `git describe` — or a stray
`git push --tags` — takes them for this fork's. A clone that already has them
drops every tag origin does not carry with `git fetch origin --prune --prune-tags`.
Never `git push --tags`; push the one tag you mean.

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

**`lint (advisory)` is red on every sync PR — and that is not a finding.**
The job runs with `only-new-issues`, which measures against the PR's diff, and a
sync PR's diff is all of upstream's. Every upstream file counts as "new". The job
is `continue-on-error`; the gates are `build`, `vet` and `test`. To tell upstream's
backlog from something we wrote, take each `file:line` from the job log and match
it against `git diff -U0 upstream/main HEAD`: a hit is ours, no hit is upstream's
and is **not** fixed here (that would diverge in every file and cost the next sync
its conflicts). October 2026: 268 findings, none on a line that differs from
upstream. It cannot be reproduced locally while the installed `golangci-lint` is
built with an older Go than `go.mod` asks for.

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

**As of 2026-10-01:** nothing is open. All four PRs of the September round
(#256, #257, #258, #259) were merged upstream, and the October sync brought them
back. Before opening, the three branch commits were rewritten with the same tree
and parent: the originals carried AI co-author trailers and an AI author, which
this repo's PRs must not.

| PR | Branch | Subject | Status |
|---|---|---|---|
| ~~[#120](https://github.com/oisee/vibing-steampunk/pull/120)~~ | `fix/csrf-head-fallback-and-session-type` | CSRF HEAD→GET fallback, secure-cookie fix, `SAP_SESSION_TYPE` | **closed** 2026-08-31 — see *Superseded by upstream* |
| ~~[#121](https://github.com/oisee/vibing-steampunk/pull/121)~~ | `feat/incl-write-support` | INCL (PROG/I) write support | **merged** upstream (`d8ee78c`), after 131 days open |
| ~~[#126](https://github.com/oisee/vibing-steampunk/pull/126)~~ | `fix/search-type-filter-issue-119` | server-side search type filter | **merged** upstream (`598e37c`), after 123 days open |
| ~~[#164](https://github.com/oisee/vibing-steampunk/pull/164)~~ | `fix/query-top-0-returns-100-rows` | `--top 0` / `all_rows` returns every row | **merged** upstream (`df4a186`) |
| ~~[#256](https://github.com/oisee/vibing-steampunk/pull/256)~~ | `feat/corrnr-at-lock` (`9d720d7`) | corrNr on the LOCK request, variadic, incl. upstream's newer lock paths | **merged** upstream (`558ce8d`); back-fill of `4b80378` + `b615466` + `05f4bd1` |
| ~~[#257](https://github.com/oisee/vibing-steampunk/pull/257)~~ | `fix/redirect-credentials-off-host` (`6066173`) | `CheckRedirect` keeps credentials and CSRF token on the SAP host | **merged** upstream (`f6b9418`); upstream has since added the scheme-downgrade rule (`keepsSAPCredentials`), which this fork now carries |
| ~~[#258](https://github.com/oisee/vibing-steampunk/pull/258)~~ | `fix/retry-request-session-reconcile` (`88df7a3`) | `retryRequest` reads the session back | **merged** upstream (`c1c7cee`); the fork had it as `7e9bce8` |
| ~~#259~~ | — | `vsp update` follows the repository the binary was released from | **merged** upstream (`9408bf1`); the fork shipped it in v3.58.1 as `d492337`, and the October merge produced no diff in `cmd/vsp/update*.go` |

The four branches of the closed round are released: nothing upstream holds them
(`b0f3110`, `59b401b`, `38e8b43`, `2e972de`). Deleted 2026-09-24. The close-if-unanswered dates (2027-04-23, 2027-05-01) are void.

The September-round branches `feat/corrnr-at-lock`, `fix/redirect-credentials-off-host`
and `fix/retry-request-session-reconcile` are released too, now that their PRs are
merged. They may be deleted; nothing upstream holds them. **Not yet done** — that is
a GitHub-side action.

One thing the merges cost us: upstream's copies are the revisions as submitted,
not the revisions on `main`. The September sync therefore brought a second,
older `WriteInclude` and a duplicate `TestLockObject_RejectsLockWithoutHandle`
back into the tree, both of which had to be dropped by hand. The October sync
did it again: the `CheckRedirect` tests, the corrNr helpers and three
`*PassesTransportToLock` tests came back verbatim next to our copies, and our
copies were dropped. Expect that shape whenever one of our PRs lands after we
have kept working on the branch's subject here.

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

### Back-fill — done

`4b80378` (corrNr at LOCK time) and `b615466` (the variadic signature) were
offered as #256 and merged upstream (`558ce8d`). Upstream's `LockObject` is now
variadic too, so a three-argument call from upstream compiles — and **silently
drops the transport**. The October sync found two such sites that arrived with
no corrNr: `WriteMessageClassTexts` and `CreateStructure`. Nothing flags this at
build or merge time. After every sync, list the calls and check each against a
transport in scope:

```bash
grep -rn 'LockObject(' --include=*.go pkg internal cmd | grep -v _test.go
```

Remaining three-argument sites that are intentional: the temp-program cleanup in
`workflows_execute.go`, `UpsertAMCApplication` (no transport parameter) and
`handleDeployZip` (the handler takes none).

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
| [#207](https://github.com/oisee/vibing-steampunk/pull/207) | dme007 | redirect headers, ICMENOSESSION reset | **adopted in part** 2026-09-24 | trace-to-file (`VSP_TRACE_LOG`) taken. Its `CheckRedirect` is the unhardened re-attach our off-host rule replaced, and its `resetCookieJar` in the ICMENOSESSION path builds a bare jar — both kept ours |
| [#209](https://github.com/oisee/vibing-steampunk/pull/209) | dme007 | session-holding proxy contextid guard | **adopted** 2026-09-24 | opt-in (`SAP_PROXY_CONTEXTID_GUARD`); `hasJarCookies` works with our jar, and `retryRequest` got the guard on top of our single session-type decision |
| [#210](https://github.com/oisee/vibing-steampunk/pull/210) | dme007 | pre-auth client proxy wiring | **adopted** 2026-09-24 | `newPreAuthHTTPClient` taken, fed with `newCookieJar` instead of `cookiejar.New` |
| [#203](https://github.com/oisee/vibing-steampunk/pull/203) | oisee | transport choice before the lock | **adopted** 2026-09-24 | `planTransport` runs before the LOCK, which still carries the supplied transport as corrNr |
| [#183](https://github.com/oisee/vibing-steampunk/pull/183) | oisee | single-call lock (#169) | **adopted** 2026-09-24 | `withObjectLock` took no transport; it does now (`05f4bd1`), and upstream's shape test follows until the corrNr back-fill lands |
| [#182](https://github.com/oisee/vibing-steampunk/pull/182) | Augusto42 | write-safety result verification | **adopted** 2026-09-24 | `installer.DeploySource` supersedes our `WriteSourceResult.Deployed` (removed, `8164285`) |
| [#208](https://github.com/oisee/vibing-steampunk/pull/208) | dme007 | transport organizer explicit filters | **adopted** 2026-09-24 | `TransportQuery.normalized` carries our empty-user default |
| [#179](https://github.com/oisee/vibing-steampunk/pull/179) | oisee | CGO-free SQLite | **adopted** 2026-09-24 | ends the local cgo test baseline below |
| [#251](https://github.com/oisee/vibing-steampunk/pull/251) | Viktor Vostrikov | concurrent callers of one client, resettable jar | **adopted in part** 2026-10-01 | its `resettableJar` is the fix for *Known issues* 2 and was taken. Its inner jar is a bare `cookiejar.New`, which drops the `httpCookieJar` Secure-stripping wrapper; ours builds it through `newCookieJar`, and `clearSAPSessionCookies` now delegates to `resetCookieJar` |
| [#229](https://github.com/oisee/vibing-steampunk/pull/229) | Kylin | reload cookie files only for safe session recovery | **adopted** 2026-10-01 | `requireSafeReauth` sits next to our jar reset in the session-expiry path; both kept |
| [#231](https://github.com/oisee/vibing-steampunk/pull/231) | txape10 | compensating unlock at three more sites | **adopted** 2026-10-01 | `failureCleanupContext` is theirs; our `ReleaseLock` and `joinMessage` stay beside it |
| [#217](https://github.com/oisee/vibing-steampunk/pull/217) | Dominik Miescher | proxy context retired after DELETE | **adopted** 2026-10-01 | opt-in with the proxy guard from #209 |
| [#243](https://github.com/oisee/vibing-steampunk/pull/243) | Viktor Vostrikov | DeleteObject gated before the lock | **adopted** 2026-10-01 | `PrepareDelete` mirrors `PrepareSourceUpdate` |
| [#283](https://github.com/oisee/vibing-steampunk/pull/283), [#288](https://github.com/oisee/vibing-steampunk/pull/288) | Alice V. | `--read-only` covers every writing path; one invariant over every tool | **adopted** 2026-10-01 | no fork code in the area. `LockObject` under `--read-only` now refuses every mode but READ, so the "MODIFY" lock the fork's deploy handlers take is refused there too, as intended |
| [#256](https://github.com/oisee/vibing-steampunk/pull/256), [#257](https://github.com/oisee/vibing-steampunk/pull/257), [#258](https://github.com/oisee/vibing-steampunk/pull/258), [#259](https://github.com/oisee/vibing-steampunk/pull/259) | frd1201 | our own four | **merged** upstream | came back as duplicates of our tests; ours were dropped, see *Our open upstream PRs* |

Watched, possible collision: #138 (InstallZADTVSP source deploy). #231, #229,
#251, #217 and #243 landed in the October sync and are decided above; #150 and
#130 are settled by #214 and #213.

---

## Fork-only changes

Deliberately not upstreamed. No PR is owed for these.

| Commit | Subject | Why fork-only |
|---|---|---|
| `d752536` | CHANGELOG for v3.0.0 | our own version line |
| `3f7a90c` | goreleaser release target → `frd1201` | must not point upstream releases at this fork |
| sync 2026-09-24 | `Makefile` `VERSION` uses `git describe --match 'v3.*'` | upstream's v2.* tags are in our history after every sync |
| sync 2026-09-24 | `.github/workflows/release.yml` rewritten for the v3 line | derives `v3.<U>.<P>`, publishes the hand-written CHANGELOG section, pushes only the tag |

---

## Upstream syncs

| Sync | Upstream head | Scope | Notes |
|---|---|---|---|
| `sync/upstream-2026-08` | `9b8789d` (2026-08-27) | 341 commits, 314 files, +52,440 | 13 conflicts. Upstream had independently built several of our fixes, so most resolutions were a choice between two implementations rather than a combination — upstream won wherever the effect was the same. Three defects would have merged in silently: a new upstream file calling the three-arg `LockObject` (broke `go build`), a duplicate jar-reset that discarded the `httpCookieJar` wrapper, and unreachable code that `go vet` rejects. |
| `claude/admiring-ptolemy-r48db4` | `9886d27` (2026-09-24, v2.58.0 + 3) | 110 commits, 217 files, +23,513 | 13 conflicts, 35 hunks. `resetCookieJar` came back a third time, now in the ICMENOSESSION path (#207), and #210 brought a second `cookiejar.New`. Taking "theirs" in the two install loops compiles and counts every object twice. The variadic `LockObject` held — no build break — but five new lock sites arrived without corrNr, which nothing flagged; threaded in `05f4bd1`. |
| `sync/upstream-2026-10` | `9789f00` (2026-10-01, v2.58.0 + 46) | 46 commits, 236 files, +32,305 | 17 conflicts, 36 hunks, about a third of them one pattern: our four-argument `LockObject` calls against upstream's `trPlan.lockCorrNr(...)`, taken theirs, which is nil-safe and equal to ours when no plan exists. Four of our own PRs (#256–#259) came back. Three traps: `resetCookieJar` returned a *fourth* time, now as upstream's `resettableJar` (#251) — good mechanism, bare inner jar, merged by building the inner jar through `newCookieJar`; two lock paths with no corrNr that compiled and merged clean (`WriteMessageClassTexts`, `CreateStructure`); and our own tests arriving verbatim a second time. `TestClearSAPSessionCookies_ReplacesJar` pinned the old "jar reference changes" contract and had to be inverted — the race it protected is the race #251 fixes. |
| `sync/upstream-2026-09` | `8dd2ef8` (2026-09-02) | 50 commits, 48 files, +4,119 | 17 conflicts — more than the August sync on an eighth of the volume, because both trees had spent the week on the same defect. Upstream's issue #91 work supersedes most of our #88 work and was taken whole. Two traps: `resetCookieJar` would have deleted the `httpCookieJar` wrapper (the August trap, renamed), and a new upstream *test* file merged clean and then failed to compile against our four-argument `LockObject` — fixed at the root by `b615466`. Three of our own PRs landed upstream during the window and came back as duplicate definitions. |

**What made this sync survivable** was writing the missing tests *first*. Eleven
of sixteen fork corrections had no test at all, so the merge had no acceptance
criterion until `fork_corrections_test.go` existed in `pkg/adt/` and
`internal/mcp/`. Do that again before the next large sync: a correction with no
test is a correction the merge can delete in silence.

## Known issues — found, fixed since

Both came out of the post-merge review on 2026-08-28. Neither was introduced by
the sync: they predate it here **and** existed in `upstream/main` unchanged.
Both are fixed now (1 by our #258, 2 by upstream #251). Kept as the record of
why the code looks the way it does.

### 1. `retryRequest` does not reconcile the session it just renewed

**Fixed in the fork 2026-09-24** (`7e9bce8`, merged in `2331f97`), offered
upstream as #258 and **merged there** (`c1c7cee`). The analysis below stays as
the record of why.

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

**Fixed 2026-10-01** by upstream #251. The client keeps one `resettableJar` for
its lifetime and `resetCookieJar` empties it under a lock; `clearSAPSessionCookies`
delegates to it. Our merge builds the inner jar through `newCookieJar`, so the
Secure-stripping wrapper survives (`TestSessionRecovery_PreservesSecureStrippingJar`).
One swap is left: a caller-supplied `*http.Client` whose jar is not a
`resettableJar` still gets `client.Jar` assigned. Nothing in this repo builds
one outside tests. The analysis below is the original finding.

`clearSAPSessionCookies` (`pkg/adt/http.go:641`) assigns `hc.Jar` while other
goroutines may be inside `httpClient.Do` reading `c.Jar`. `cmd/vsp/fetchsources.go:93`
fans several goroutines out over one `*adt.Client`, and the MCP server serves
concurrent tool calls on one client too, so the race is reachable — the session
expiry path at `:296` is not covered by `reauthMu`. Detectable under
`go test -race`. A fix needs to decide what the concurrency contract of
`Transport` actually is, which is why it is not a quick patch.

**Rechecked 2026-09-24:** #1 is still open upstream — #209 added the proxy
guard to `retryRequest` and nothing else — and is now offered upstream as its
own PR (see *Our open upstream PRs*). #2 is unchanged; upstream #251 works in
the same area and is on the watch list.

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
| `4b80378`, `b615466`, `05f4bd1` | `9d720d7` | `feat/corrnr-at-lock` — upstream `558ce8d` |
| `b83b4fa` (the `CheckRedirect` part) | `6066173` | `fix/redirect-credentials-off-host` — upstream `f6b9418` |
| `7e9bce8` | `88df7a3` | `fix/retry-request-session-reconcile` — upstream `c1c7cee` |
| `d492337` | — | `vsp update` repository — upstream `9408bf1` |

That happened in the October sync, as predicted here: `lockQueryRecorder`,
`lockHandleXML`, `transportableEditClient`, `assertLockCarried`, the
`*PassesTransportToLock` tests for SetDescription and WriteTextPool, the
`SelfLockPassesTransport` test and both `CheckRedirect` tests existed in
upstream's `lock_corrnr_test.go`, `config_test.go` and `lock_scope_test.go` and
in our `fork_corrections_test.go` files. Ours were dropped. What stays in ours is
what upstream does not have: `TestWriteInclude_PassesTransportToLock` and the
two added in October for `WriteMessageClassTexts` and `CreateStructure`. Those
reuse upstream's helpers, so if upstream renames `lockQueryRecorder` the build
says so.

`6b2cece` (the parked `fork-only/onprem-edit-fixes` branch) was adopted **in
part only**: its corrNr work became `4b80378`, its configurable
`MODIFICATION_SUPPORT` guard was dropped because upstream PR #108 removed the
guard outright. The original commit no longer exists as a reachable SHA — the
branch was deleted after the split.

To re-audit what on `main` is not covered by any upstream PR, see section 9.1 of
the strategy report.

---

## Release

- The fork releases as **`v3.<U>.<P>`**: `U` is the minor of the newest
  upstream release the build contains, `P` counts fork releases on top of it
  (0 for the release that follows a sync). Upstream v2.58.0 → **v3.58.0**; a
  fork-only fix after it → v3.58.1; the next sync that brings v2.60.0 → v3.60.0.
- Not a suffix on upstream's number: `v2.58.0-fork` is a SemVer pre-release,
  sorts *before* v2.58.0, and goreleaser's `prerelease: auto` would mark every
  release as one. If upstream ever tags a 3.x, move to `v4.<U>.<P>` and change
  the `--match` in `Makefile` and `release.yml`.
- **Cutting a release:** the PR that prepares it adds `## [3.<U>.<P>]` to
  `CHANGELOG.md` with *Own changes* and *Adopted from upstream*
  (`git log --no-merges v<last>..HEAD ^upstream/main` lists the former). After
  it is merged, run the *Release* workflow without input: it derives the
  version, refuses a missing CHANGELOG section or an existing tag, pushes the
  tag and nothing else, and goreleaser publishes to `frd1201`.
- Tags are cut from `main` only, and only when the CI job on `main` is green
  **and** an integration run against a real SAP system has passed.

### Local test baseline

`go test ./...` is fully green without a C compiler since upstream #179 moved
the SQLite cache to `modernc.org/sqlite` (adopted 2026-09-24). The
`go-sqlite3 requires cgo` failures this section used to list are gone; any red
test is a real one.

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
