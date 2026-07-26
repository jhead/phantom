# Cross-version compatibility test strategy

phantom sits between a Minecraft Bedrock client and a remote Bedrock server. It is
mostly a transparent UDP relay, but it parses and rewrites one packet type: the
RakNet Unconnected Pong (0x1c), whose payload is a semicolon-delimited MOTD string
whose shape changes as Mojang ships new protocol versions.

That makes compatibility failures silent and version-gated. phantom keeps proxying,
but a client on a newer version either does not list the server or displays wrong
information.

## Goals

- Validate phantom against every Minecraft protocol version the tooling supports.
- Run unattended in CI on every push, with no external services and no credentials.
- Fail loudly and specifically when a protocol version breaks an invariant.
- Stay compatible with every new Minecraft version.

## Test types

Tests are split into two types: unit tests that import phantom's packages, and end to
end tests that may use only the compiled binary.

### Unit tests

Unit tests replay a corpus of pong payloads through the parser and the rewriter,
asserting on the resulting bytes and fields.

- Run in well under a second, with no network, no ports, no subprocess and no Node.
- Cover all 51 supported protocol versions on every push and on every OS.
- Can construct a ProxyServer with whatever internal state a case needs and call
  unexported functions.

They live in two packages because `rewriteUnconnectedPong` is an unexported method on
`*ProxyServer` and Go cannot reach it from an external test package. Parser tests are
in `internal/proto`, rewrite tests in `internal/proxy`.

`ReadUnconnectedPing` parses untrusted bytes straight off a UDP socket, so there is
also a fuzz target. It is seeded from the corpus, so the fuzzer starts from realistic
structure rather than discovering the RakNet header from scratch.

### End to end tests

These run the compiled phantom binary as a subprocess and drive it over real UDP
sockets. Two kinds of upstream stand behind it:

- A scriptable fake server in `test/compat/fakeserver`, giving byte-exact control over
  the pong. It emits malformed, truncated, oversized, zero-field and 30-field pongs,
  none of which a real server can be made to produce.
- A real bedrock-protocol server, paired with a real bedrock-protocol client. Its
  parser judges phantom's rewritten pong, so a malformed rewrite fails against an
  independent implementation instead of round-tripping through phantom's own parser
  and hiding the same misunderstanding twice.

Running the binary rather than an in-process ProxyServer is forced by three things.
`Start()` blocks, `Close()` panics if called before a successful bind, and `serverID`
is a package global, which makes "two instances must advertise different identities"
unobservable from inside a single process. It also exercises the artifact that ships,
including its flag parsing.

The bedrock-protocol side is driven by Node scripts, each a single-purpose CLI that
prints one line of JSON. Go owns the version matrix, process lifecycle and every
assertion, so `make test` remains the single entry point and Go contributors do not
need to edit JavaScript.

### Boundary

End to end tests must not import `internal/proto`, `internal/proxy` or
`internal/clientmap`. They may import `internal/corpus`, which holds fixture data and
the known-failure registry and contains no phantom protocol logic.
`TestBlackBoxRuleHolds` enforces this with `go list -deps` rather than relying on
convention.

Unit invariants are about bytes and pure functions. End to end invariants are about
process identity, sockets and time. An assertion that needs phantom's internals
belongs in a unit test.

## Invariants

Given the pong an upstream server sends and the pong phantom emits in response:

### Field preservation

1. Edition, MOTD, ProtocolVersion, Version, Players, MaxPlayers, SubMOTD and GameType
   pass through byte-identical. Covered by both types.
2. Fields beyond the twelve phantom models are preserved. Real servers send thirteen.
   Covered by unit tests.
3. The 16-byte RakNet magic is unmodified. Covered by unit tests.

### Rewriting

4. The ServerID field carries phantom's instance identity, is stable across repeated
   pings, and differs between two running instances. The multi-instance half is end to
   end only, since `serverID` is a package global.
5. Port4 and Port6 carry phantom's bound port, and both are empty under
   `-remove_ports`. A server that advertises no ports keeps advertising none. Unit
   tests cover the logic, end to end tests the flag wiring.
6. The pong echoes the ping time the client sent. Unit tests cover the proxied path,
   end to end tests the offline path.
7. The binary ServerGUID at bytes 9 to 16 is non-zero and per-instance. RakNet
   identifies servers by this value, not by the MOTD string field. Covered by both
   types.

### Session behavior

8. The RakNet handshake completes through phantom for RakNet protocol versions 8
   through 11.
9. A real bedrock-protocol client completes the RakNet handshake and the login
   sequence through phantom.
10. Payloads survive the round trip byte-identically at 1, 576, 1400, 1464 and 1472
    bytes, or are dropped cleanly, never truncated silently.
11. Two concurrent clients never receive each other's datagrams.
12. An idle client is reaped after the configured timeout, and a later packet from
    that client still proxies.
13. When the upstream stops responding phantom answers with its offline pong, and
    when the upstream returns phantom resumes proxying without a restart.
14. A client pinging with 0x02 receives an offline pong while the upstream is down, as
    it does with 0x01.

All of these are covered by end to end tests.

### Robustness

15. Truncated, oversized, zero-field and 30-field pongs never panic phantom and never
    produce output an independent parser rejects. Covered by the fuzz target and by
    end to end tests against the fake server.

## Test corpus

The corpus lives in `internal/corpus/data` and is embedded into the binary, so tests
resolve no paths at runtime.

### Captured corpus

`captured.json` holds real pong bytes recorded from a bedrock-protocol server, one
entry per supported Minecraft version. `make corpus` regenerates it, and the result is
committed, so a change in any version's wire bytes shows up as a reviewable diff. CI
replays the committed bytes and never captures new ones.

bedrock-protocol uses a single advertisement serializer, so the field count is
constant across every version it emits and only the protocol number and version
string vary. The captured corpus is therefore a drift anchor and a source of realistic
bytes, not a source of shape diversity.

### Synthetic corpus

`synthetic.json` supplies the shape diversity the captured corpus cannot: 10, 12, 13,
14 and 15 field pongs, empty fields, section-sign colour codes, multi-byte UTF-8, a
raw semicolon inside the MOTD, an oversized MOTD, and 30 fields. It also supplies
malformed frames: empty packets, truncation at several offsets, a length prefix that
exceeds the body, a zeroed magic, and a ping packet ID on an otherwise valid pong.

Fixtures that test a semantic defect are built as complete, well-formed frames with
exactly one thing wrong. A hand-written truncated fixture would trip the length check
first and prove nothing about the defect it claims to test.

## Version matrix and sharding

`test/compat/matrix.json` lists the versions the end to end tests exercise and assigns
each to a CI shard. Unit tests sweep every supported version because it is cheap. End
to end tests boot a real client and server per version, so they sample the oldest
supported version, the newest, and the newest of each minor line in between.

Sharding provides isolation as well as parallelism. phantom binds UDP port 19132
unconditionally, and because it sets SO_REUSEPORT a second binder does not fail. The
kernel load-balances datagrams between the two, so two concurrent cases on one host
silently consume each other's packets. One shard per CI runner gives each its own
network stack. Cases run serially within a shard.

Locally the suite runs serially and checks port 19132 first, skipping with an
explanatory message if it is busy, so a developer running Minecraft or phantom does
not see confusing failures.

## Known failure registry

`internal/corpus/data/known_failures.json` records invariants phantom does not
satisfy, each with a reason and a reference to the tracking entry in TODO.md. Four
outcomes:

- Unregistered and passing: silent success.
- Unregistered and failing: an ordinary test failure.
- Registered and failing: reported as XFAIL, and the build stays green.
- Registered and passing: reported as XPASS, and the build fails.

The last case is the point of the mechanism. Fixing a protocol bug breaks the build
until its entry is deleted, so the registry cannot decay into a list of things that
were fixed long ago. It doubles as an accurate inventory of open protocol bugs.

Assertions return an error rather than calling `t.Errorf` directly, so a failure can
be captured and reinterpreted instead of being recorded immediately.

## Harness mechanics

Readiness is established by behavior. The harness pings until it receives a pong or a
deadline expires. It does not scrape logs and it does not sleep for fixed intervals.

`-bind_port` is always passed explicitly, because phantom's default of 0 selects a
random port that the test would have no way to discover.

Timeouts are named constants and scale with the `PHANTOM_TEST_TIMEOUT_SCALE`
environment variable. Cases that confirm a reply will not arrive use a much shorter
budget than readiness cases, since there is nothing left to wait for once phantom is
known to be up.

Cases that wait on phantom's real timers, such as the 5 second idle sweep, sit behind
a `slow` build tag. CI includes them and `make test` does not. Cases that need a real
client stack sit behind a `node` build tag, so the rest of the suite runs on machines
with no Node toolchain.

Session tests open a control connection directly to the upstream before testing the
proxied path. If the direct connection fails, the client stack cannot run in that
environment and the test skips. Only a direct success followed by a proxied failure
attributes the problem to phantom. Without that control, an unrelated problem in the
client stack would be indistinguishable from a phantom bug.

## Continuous integration

The per-push pipeline runs four jobs:

- `vet` runs gofmt, `go vet` and `go test -race`, pinned to the Go version declared in
  go.mod so accidental use of a newer language feature is caught.
- `unit` runs the corpus tests across Linux, macOS and Windows, plus a short fuzzing
  pass. It uses a current Go toolchain rather than the declared minimum, because
  recent macOS releases reject binaries without an LC_UUID load command and Go's
  internal linker only began emitting one in 1.26.
- `e2e` runs the end to end tests across four shards, one per runner.
- `cross-compile` builds every release target.

The nightly pipeline runs the full unsampled version sweep, a long fuzzing pass, and a
drift check comparing the versions bedrock-protocol supports against the versions this
repository tests. The drift check is what keeps coverage current as Minecraft ships
new releases.

## Out of scope

- Performance and load testing.
- Real console clients, and LAN broadcast discovery to 255.255.255.255. Broadcast is
  phantom's real discovery path on console, so this is a genuine gap. Covering it
  needs a dummy network interface and root.
- Xbox Live authentication. All tests run in offline mode.
- Game-layer packet semantics. phantom is opaque above RakNet and the tests treat it
  that way. Session tests assert that login completes, not what the server sends
  afterwards.
- Testing against Mojang's Bedrock Dedicated Server, which would add large downloads,
  an EULA, a single supported architecture, and dependence on version-specific
  download URLs that have broken in the past.
