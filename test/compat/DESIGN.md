# Cross-version compatibility harness — design

Status: **implemented** — see §11 for what changed during the build and §12 for
what is verified vs. unverified.
Date: 2026-07-25 (rev 3)

## 1. Problem

phantom sits between a Bedrock client (console) and a remote Bedrock server. It is
mostly a transparent UDP relay, but it *parses and rewrites* one packet type: the
RakNet Unconnected Pong (`0x1c`), whose payload is a semicolon-delimited MOTD string
whose shape changes as Mojang ships new protocol versions.

Today we have no way to answer "does phantom still work on MC 1.2x?" without a
console, a real server, and a human. The failure mode is silent and version-gated:
phantom keeps proxying, but a new client either won't list the server or shows
garbage.

**Goal:** an automated, CI-resident harness that validates phantom against N
Minecraft protocol versions, and fails loudly when a new version breaks an invariant.

## 2. What actually varies across versions

Only four things can plausibly break phantom. The matrix targets these and nothing
else — a naive full cross-product of every knob is combinatorial waste.

| Dim | What varies | Range | phantom code at risk |
|-----|-------------|-------|----------------------|
| **D1** | Pong MOTD field count / semantics | ~40 MC versions; 12 fields historically, 13+ on modern servers | `proto.readPong`/`writePong` (fixed 12-field struct → **extra fields silently dropped**) |
| **D2** | RakNet protocol version in `OpenConnectionRequest1` | 7–11 | pure passthrough — assert it *stays* passthrough |
| **D3** | Negotiated MTU / max datagram size | ≤1464 payload | `maxMTU = 1472` read buffer; truncation risk |
| **D4** | Client ping behavior (`0x01` vs `0x02`, ping-time echo) | 2 packet IDs | offline-pong path only matches `0x01`; pong never echoes ping time |

**Design consequence:** D1 gets the *broad sweep* across all N versions. D2/D3/D4 are
orthogonal to version and get a small fixed scenario set at one pinned version. Keeps
the matrix linear (N + k) instead of exponential (N × k).

## 3. Two tiers, two testing philosophies

The tiers are not just fast/slow — they are **different kinds of test**, and the
boundary is enforced by what each is allowed to import.

### T0 — White-box unit tests (pure Go, no network)

Imports `internal/proto` and `internal/proxy` and calls them **directly**. Replays a
committed corpus of **real pong payloads captured per protocol version** through
parse → rewrite → assert bytes.

- Runtime **< 1s**. No network, no ports, no Node, no subprocess.
- Scales to all ~40 versions for free. This is the backbone of "N versions".
- Full white-box freedom: construct a `ProxyServer` with whatever internal state a
  case needs, call unexported functions, assert on struct fields not just bytes.

**Package placement is forced by Go, not chosen.** `rewriteUnconnectedPong` is an
unexported method on `*ProxyServer`, so its tests must be in-package:

```
internal/proto/corpus_test.go     # package proto  — parse/build round-trip
internal/proto/fuzz_test.go       # package proto  — FuzzReadUnconnectedPing
internal/proxy/rewrite_test.go    # package proxy  — rewrite goldens (unexported method)
test/compat/corpus/               # shared data only, no Go
```

The **fuzz target** is a natural fit here and cheap: `ReadUnconnectedPing` is a parser
eating untrusted network bytes, seeded from the version corpus. Short run in CI
(`-fuzztime=30s`), longer nightly.

### T1 — Black-box end-to-end (real binary, real UDP)

Knows **only** the built `phantom` binary, its CLI flags, and UDP sockets. It gets
**zero imports from `internal/`** — that rule is what keeps it honest. If T1 needs a
Go type from phantom to make an assertion, the assertion belongs in T0.

Two upstream flavors:
- **Go fake server** (`test/compat/fakeserver`) — a standalone RakNet upstream with
  byte-exact control over the pong. Emits malformed, truncated, oversized, zero-field
  and 30-field pongs. Used for adversarial cases and D2/D3/D4.
- **`bedrock-protocol` server/client** (Node, pinned) — a *real* stack. Its `ping()`
  parses phantom's rewritten pong with a genuine third-party parser, so a malformed
  rewrite fails loudly instead of round-tripping through our own buggy parser. Used
  for full handshake + login + spawn.

- Runtime target **< 3 min** wall-clock across all shards.
- Version sampling: oldest supported, latest, latest-of-each-minor ≈ 8–10 versions.

## 4. Invariant → tier assignment

Given upstream pong bytes `U` and the bytes `P` phantom emits:

| # | Invariant | Tier | Status |
|---|-----------|------|--------|
| 1 | `Edition`, `MOTD`, `ProtocolVersion`, `Version`, `Players`, `MaxPlayers`, `SubMOTD`, `GameType` byte-identical `U`→`P` | T0 + T1 | ✅ |
| 2 | **Trailing fields beyond the known 12 preserved** | T0 | ❌ XFAIL (D1) |
| 3 | RakNet magic exactly 16 bytes, unmodified | T0 | ✅ |
| 4a | `ServerID` replaced with phantom's instance ID | T0 | ✅ |
| 4b | `ServerID` stable across pings; **differs between two running instances** | **T1 only** — `serverID` is a package global, unobservable in-process | ✅ |
| 5 | `Port4`/`Port6` = bound port; both empty under `-remove_ports` | T0 (logic) + T1 (flag wiring) | ✅ |
| 6 | `PingTime` echoes the *client's* ping time | T0 (build preserves) + T1 (offline path stamps) | ❌ XFAIL (D4) |
| 7 | Binary `ServerGUID` (bytes 9–16) non-zero and per-instance | T0 + T1 (uniqueness) | ❌ XFAIL |
| 8 | RakNet handshake completes for proto 8/9/10/11 | T1 | ✅ |
| 9 | `bedrock-protocol` client reaches `spawn` through phantom | T1 | ✅ |
| 10 | Payload integrity both directions at 1, 576, 1400, 1464, 1472, 1500 bytes — byte-identical or cleanly dropped, never silently truncated | T1 | ✅ |
| 11 | Two concurrent clients never receive each other's datagrams | T1 | ✅ |
| 12 | Idle client reaped after `-timeout` | T1 (slow) | ✅ |
| 13 | Upstream down → offline pong; upstream back → proxying resumes, no restart | T1 (slow) | ✅ |
| 14 | Client sending `0x02` gets an offline pong | T1 | ❌ XFAIL (D4) |
| 15 | Malformed/truncated/30-field pongs never panic and never produce output the reference parser rejects | T0 (fuzz) + T1 (fake server) | ⚠️ partly |

Note how cleanly the split falls out: **every T0 invariant is about bytes and pure
functions; every T1-only invariant is about process identity, sockets, or time.**
That's the boundary test — if a proposed case doesn't fit that description, it's in
the wrong tier.

## 5. Version matrix as data

`test/compat/matrix.json` is the single source of truth, committed and reviewable:

```json
[
  { "mc": "1.16.201", "protocol": 422, "raknet": 10, "tier1": true, "shard": 0, "fields": 12 },
  { "mc": "1.21.0",   "protocol": 685, "raknet": 11, "tier1": true, "shard": 2, "fields": 13 },
  { "mc": "1.26.30",  "protocol": 860, "raknet": 11, "tier1": true, "shard": 3, "fields": 13 }
]
```

Two generators keep it honest:
- `make corpus` — starts a `bedrock-protocol` server per entry, captures the raw pong,
  writes `corpus/*.bin` + `*.golden.json`. Dev-time/nightly only; **CI never generates
  the corpus, it only replays committed bytes.**
- **Nightly matrix-drift job** — diffs `matrix.json` against `bedrock-protocol`'s
  supported-version list; opens an issue when Mojang ships a version we don't cover.
  This is what makes "N versions" stay current without human vigilance.

## 6. CI structure

GitHub Actions matrices, used where they earn their keep:

**`ci.yml` — every push/PR**

| Job | Matrix | Why |
|-----|--------|-----|
| `unit` | — | `go vet` (fails today on the unkeyed struct literal), `go test -race ./...` |
| `t0` | `os: [ubuntu, macos, windows]` | Corpus + fuzz. Go-only, no deps, seconds. OS matrix is nearly free and catches endian/path/CRLF surprises. |
| `t1` | `shard: [0,1,2,3]` | Build binary + `npm ci` + run that shard's versions serially |
| `cross-compile` | — | All `make build` targets still link |

**Why shard T1 rather than one job per version:** each job pays ~40–60s of
checkout/toolchain/`npm ci` overhead for a few seconds of test. Per-version jobs would
spend 90% of runner minutes on setup. Four shards balance that against parallelism and
keep the PR check list readable.

**The shard matrix also solves port isolation.** phantom hard-binds `:19132`
unconditionally (`reuse.ListenPacket("udp4", ":19132")`), and `SO_REUSEPORT` means two
instances *load-balance each other's packets* rather than failing cleanly — so two
concurrent T1 cases on one machine would silently corrupt each other. Each shard gets
its own runner, hence its own network stack. **Within** a shard, cases run serially.
No network namespaces, no `unshare`, no root. Locally, `go test` runs serially and
preflights `:19132`, **skipping with a clear message** if it's busy — a dev running
Minecraft or phantom shouldn't see mysterious red.

**`nightly.yml` — non-blocking**
- Full ~40-version T1 sweep (vs. the ~10 sampled per-push)
- Extended fuzzing (`-fuzztime=10m`)
- Matrix-drift check

## 7. Harness mechanics

**Subprocess, not in-process.** T1 runs the built binary. `Start()` blocks, `Close()`
nil-panics on early shutdown, and `serverID` is a package global — so invariant 4b is
*impossible* in-process. Bonus: we test the shipped artifact and its flag parsing.

**Readiness without sleeps.** Never `sleep`. Poll by *behavior*: retry the ping until
a pong arrives or a deadline expires. No log-scraping, no fixed delays. Every timeout
is a named, overridable constant.

**Always pass `-bind_port` explicitly** — `-bind_port 0` randomizes it and the harness
would have nothing to talk to.

**Slow cases are tagged.** Idle-reap needs `-timeout 1` plus the 5s sweep ≈ 7s;
offline/online recovery similar. Behind a `slow` build tag: included in CI, excluded
from default `make test`.

**Known-failing tests.** Invariants 2, 6, 7, 14 fail on `master` today — they are the
open protocol bugs in `TODO.md`. Landing this harness must not turn CI permanently
red. So `known_failures.json` maps test ID → TODO item, XFAIL-marked. CI **fails if an
XFAIL unexpectedly passes**, forcing the marker deleted in the same PR that fixes the
bug. The registry doubles as a live bug-status dashboard.

## 8. Layout

```
internal/proto/corpus_test.go      # T0: parse/build round-trip      (package proto)
internal/proto/fuzz_test.go        # T0: FuzzReadUnconnectedPing     (package proto)
internal/proxy/rewrite_test.go     # T0: rewrite goldens             (package proxy)

test/compat/
  DESIGN.md            # this file
  matrix.json          # version matrix (source of truth)
  known_failures.json  # XFAIL registry → TODO.md items
  corpus/              # captured pongs + goldens (shared T0 data, no Go)
  harness.go           # T1: process lifecycle, readiness polling, preflight
  e2e_test.go          # T1                         (build tag: e2e)
  slow_test.go         # T1 slow cases              (build tag: e2e,slow)
  fakeserver/          # T1: standalone scriptable RakNet upstream
  node/                # pinned bedrock-protocol; ping.js / connect.js / serve.js

.github/workflows/ci.yml
.github/workflows/nightly.yml
```

Node stays *thin*: three single-purpose CLIs that print JSON on stdout. Go owns the
matrix, process lifecycle, and every assertion — `make test` remains the one entry
point and Go contributors never edit JS.

## 9. Phasing

| Phase | Deliverable | Value |
|-------|-------------|-------|
| **0** | `ci.yml`: `go vet`, `go test -race`, cross-compile | Closes "No CI" in TODO.md; catches the known `go vet` failure |
| **1** | T0: corpus + goldens + fuzz + `matrix.json` | Broad N-version coverage, ~1s, zero deps |
| **2** | `harness.go` + `fakeserver` + T1 core (1, 4b, 5–7, 10, 15) | Real UDP, real binary, no Node yet |
| **3** | Node `bedrock-protocol` integration (8, 9) + shard matrix | Real third-party client stack validates our rewrite |
| **4** | `nightly.yml`: full sweep, extended fuzz, matrix drift | Stays current without humans |

Phases 0–1 alone would have caught the dropped-trailing-field bug, need no Node, and
land independently. Recommend starting there.

## 10. Explicit non-goals

- **Real BDS testing — dropped.** Considered as a third tier; rejected for now. ~150MB
  downloads, EULA, linux/amd64 only, and Mojang has broken old-version CDN URLs before.
  Revisit only if T0/T1 prove insufficient at catching real drift.
- Not a performance or load benchmark.
- Not testing real consoles (Xbox/PS), and **not testing LAN broadcast discovery** to
  `255.255.255.255`. This is a genuine gap, not a non-issue: broadcast *is* phantom's
  real discovery path on console. Covering it needs a `dummy0` interface (Linux, root).
  Deferred deliberately.
- Not testing Xbox Live auth — offline mode only.
- Not asserting game-layer packet semantics; phantom is opaque above RakNet and the
  harness treats it that way.

## 11. What changed during implementation

Eight deviations from rev 2, each with its reason.

1. **Corpus lives in `internal/corpus/`, not `test/compat/corpus/`.** Both T0 test
   packages need it, and `go:embed` cannot reach outside its own package directory.
   Embedding removes all relative-path resolution from the tests.

2. **Corpus is consolidated JSON, not per-version `.bin` + `.golden.json`.** 51
   versions would have meant 102 near-identical files. One `captured.json` shows
   exactly which version's bytes changed in a diff.

3. **No golden output bytes.** Storing phantom's current output as "expected" would
   enshrine the very bugs the harness exists to find. T0 asserts the field-level
   *invariants* from §4 instead. Strictly better, and it made the XFAIL registry
   meaningful.

4. **The black-box rule is "no *implementation* imports".** T1 may import
   `internal/corpus` (fixture data and the XFAIL registry, zero phantom protocol
   logic); it may not import `internal/proto`, `internal/proxy` or
   `internal/clientmap`. `TestBlackBoxRuleHolds` enforces this with `go list -deps`
   rather than trusting anyone to remember.

5. **Invariant 9 is "reaches `join`", not "reaches `spawn`".** A client only spawns
   once the server sends `start_game` and chunk data, which `bedrock-protocol`'s
   `createServer` does not do on its own. Writing a mini game server would test
   game-layer semantics that are an explicit non-goal, across 8 versions, while
   making phantom bugs indistinguishable from gaps in our fake server. Reaching
   `join` already proves the full RakNet handshake *and* login completed through
   phantom — which is the entire surface phantom actually relays.

6. **A `node` build tag, separate from `e2e`.** The rest of T1 runs on machines with
   no Node toolchain.

7. **`go 1.12` → `go 1.21` in go.mod.** `go:embed` needs ≥1.16 and native fuzzing
   needs ≥1.18. Not optional. `cmd/phantom.go`'s unkeyed struct literal was also
   fixed, because Phase 0 CI runs `go vet` and it failed on it.

8. **The captured corpus has a real limitation, recorded in the file itself.**
   `bedrock-protocol` emits a **constant 13-field pong across all 51 versions** —
   only the protocol number and version string vary. So per-version capture is a
   drift *anchor*, not a source of shape diversity. That diversity is what
   `synthetic.json` is for (10-, 12-, 13-, 14- and 15-field shapes, plus hostile
   content and malformed frames). Worth knowing before anyone assumes 51 captured
   versions means 51 distinct shapes tested.

## 12. Verification status

Run locally on darwin/arm64:

| Tier | Result |
|------|--------|
| T0 corpus + rewrite (`go test ./internal/...`) | **passing**, ~0.3s, 51 versions swept |
| T0 fuzz, 25s | **passing**, 11.4M execs, no crashers |
| T1 e2e + slow (`-tags='e2e slow'`) | **passing**, ~30s |
| T1 `ping` through phantom, all 8 matrix versions | **passing** — a real third-party parser accepts phantom's rewritten pong and sees phantom's port |
| T1 full `connect` session | **not verified locally** — see below |
| CI workflows | **not verified** — never executed; no runner available here |

**The `connect` session tests have never actually run.** `bedrock-protocol`'s native
RakNet addon crashes on darwin/arm64 (`trace/BPT trap`); it answers pings through a
separate JS path, which is why the ping tier works. The tests are written with a
**control connection**: they first connect straight to the upstream with phantom out
of the path. If that fails, the environment cannot support the test and it *skips*;
only if the direct connection succeeds and the proxied one fails is phantom blamed.
So they are safe to land — they cannot produce a false accusation — but their first
real execution will be on Linux CI, and they should be treated as unproven until a
green run exists.

Eight invariants currently XFAIL against real captured bytes, all registered in
`internal/corpus/data/known_failures.json` against their `TODO.md` entries.
