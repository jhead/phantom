//go:build e2e && node

package compat

// End-to-end tests driving a real client stack through phantom.
//
// The fake upstream in fakeserver/ gives byte-exact control but is, by
// construction, only as correct as our own understanding of RakNet. These tests
// close that gap by putting a genuine third-party implementation on both ends:
// bedrock-protocol serves, and bedrock-protocol pings and connects. Its parser
// judges phantom's rewritten pong, so a malformed rewrite fails loudly instead
// of round-tripping through the same misunderstanding twice.
//
// Run with: go test -tags='e2e node' ./test/compat/...
//
// The `node` tag is separate from `e2e` so the rest of the suite runs on machines with
// no Node toolchain.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Matrix mirrors matrix.json.
type Matrix struct {
	Shards   int             `json:"shards"`
	Versions []MatrixVersion `json:"versions"`
}

// MatrixVersion is one Minecraft version the e2e suite exercises.
type MatrixVersion struct {
	MC       string `json:"mc"`
	Protocol int    `json:"protocol"`
	RakNet   int    `json:"raknet"`
	Shard    int    `json:"shard"`
	Note     string `json:"note"`
}

// LoadMatrix reads the committed e2e version matrix, honouring PHANTOM_SHARD so
// CI can split it across runners.
func LoadMatrix(t *testing.T) Matrix {
	t.Helper()

	root, err := repoRoot()
	if err != nil {
		t.Fatalf("locating repo root: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "test", "compat", "matrix.json"))
	if err != nil {
		t.Fatalf("reading matrix.json: %v", err)
	}

	var m Matrix
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("parsing matrix.json: %v", err)
	}

	if s := os.Getenv("PHANTOM_SHARD"); s != "" {
		shard, err := strconv.Atoi(s)
		if err != nil {
			t.Fatalf("PHANTOM_SHARD=%q is not a number", s)
		}
		var kept []MatrixVersion
		for _, v := range m.Versions {
			if v.Shard == shard {
				kept = append(kept, v)
			}
		}
		m.Versions = kept
		t.Logf("shard %d: %d version(s)", shard, len(kept))
	}

	if len(m.Versions) == 0 {
		t.Skip("no versions selected for this shard")
	}
	return m
}

// nodeDir is test/compat/node.
func nodeDir(t *testing.T) string {
	t.Helper()
	root, err := repoRoot()
	if err != nil {
		t.Fatalf("locating repo root: %v", err)
	}
	return filepath.Join(root, "test", "compat", "node")
}

// requireNode skips unless the Node toolchain and installed deps are present.
func requireNode(t *testing.T) {
	t.Helper()

	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	if _, err := os.Stat(filepath.Join(nodeDir(t), "node_modules")); err != nil {
		t.Skipf("test/compat/node/node_modules is missing - run `npm ci` in %s", nodeDir(t))
	}
}

// bedrockServer is a bedrock-protocol server subprocess.
type bedrockServer struct {
	Port int
	Addr string
	cmd  *exec.Cmd
	out  *lockedBuffer
}

// startBedrockServer boots a real server for the given version. Readiness is
// confirmed by pinging it until it answers, not by trusting its ready line.
func startBedrockServer(t *testing.T, version string) *bedrockServer {
	t.Helper()

	port := freeUDPPort(t)
	out := &lockedBuffer{}

	cmd := exec.Command("node", "serve.js",
		"--port", strconv.Itoa(port),
		"--version", version)
	cmd.Dir = nodeDir(t)
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting bedrock server: %v", err)
	}

	s := &bedrockServer{
		Port: port,
		Addr: fmt.Sprintf("127.0.0.1:%d", port),
		cmd:  cmd,
		out:  out,
	}
	t.Cleanup(s.Close)

	deadline := time.Now().Add(ReadyTimeout)
	for time.Now().Before(deadline) {
		c, err := NewClient(s.Addr)
		if err == nil {
			_, perr := c.Ping(0x01, DefaultPingTime())
			_ = c.Close()
			if perr == nil {
				return s
			}
		}
		time.Sleep(pollInterval)
	}

	t.Fatalf("bedrock-protocol server (%s) never answered a ping\n--- output ---\n%s",
		version, out.String())
	return nil
}

func (s *bedrockServer) Close() {
	if s.cmd.Process == nil {
		return
	}
	_ = s.cmd.Process.Kill()
	_ = s.cmd.Wait()
}

// pingResult is ping.js's JSON output.
type pingResult struct {
	OK    bool   `json:"ok"`
	Error string `json:"error"`
	Ad    struct {
		MOTD          string      `json:"motd"`
		LevelName     string      `json:"levelName"`
		PlayersOnline int         `json:"playersOnline"`
		PlayersMax    int         `json:"playersMax"`
		Gamemode      string      `json:"gamemode"`
		ServerID      string      `json:"serverId"`
		PortV4        int         `json:"portV4"`
		PortV6        int         `json:"portV6"`
		Protocol      interface{} `json:"protocol"`
		Version       string      `json:"version"`
	} `json:"advertisement"`
}

// connectResult is connect.js's JSON output.
type connectResult struct {
	OK      bool   `json:"ok"`
	Joined  bool   `json:"joined"`
	Stage   string `json:"stage"`
	Error   string `json:"error"`
	Version string `json:"version"`
}

// runNode executes one of the thin Node CLIs and decodes its JSON line.
func runNode(t *testing.T, timeout time.Duration, target interface{}, args ...string) error {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", args...)
	cmd.Dir = nodeDir(t)

	out, err := cmd.Output()
	line := lastJSONLine(string(out))
	if line == "" {
		return fmt.Errorf("no JSON from `node %s`: %v", strings.Join(args, " "), err)
	}
	if jerr := json.Unmarshal([]byte(line), target); jerr != nil {
		return fmt.Errorf("undecodable output %q: %v", line, jerr)
	}
	return nil
}

// lastJSONLine picks the final JSON object from stdout, ignoring any noise the
// native RakNet addon prints.
func lastJSONLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if strings.HasPrefix(l, "{") && strings.HasSuffix(l, "}") {
			return l
		}
	}
	return ""
}

// TestRealClientPingsThroughPhantom is the core cross-version assertion: for
// every version in the matrix, a real bedrock-protocol client must be able to
// PARSE the pong phantom rewrote, and must see the upstream's identity with
// phantom's port substituted.
func TestRealClientPingsThroughPhantom(t *testing.T) {
	requireNode(t)
	RequirePingPort(t)

	m := LoadMatrix(t)

	for _, v := range m.Versions {
		t.Run(v.MC, func(t *testing.T) {
			srv := startBedrockServer(t, v.MC)
			p := Start(t, Opts{RemoteServer: srv.Addr})

			var res pingResult
			if err := runNode(t, 30*time.Second, &res,
				"ping.js", "--host", "127.0.0.1", "--port", strconv.Itoa(p.BindPort)); err != nil {
				t.Fatalf("ping.js: %v", err)
			}

			if !res.OK {
				t.Fatalf("a real client could not parse phantom's pong: %s", res.Error)
			}

			// The advertised version must be the upstream's, untouched.
			if res.Ad.Version != v.MC {
				t.Errorf("client saw version %q, want the upstream's %q",
					res.Ad.Version, v.MC)
			}
			if got := fmt.Sprintf("%v", res.Ad.Protocol); got != strconv.Itoa(v.Protocol) {
				t.Errorf("client saw protocol %q, want %d", got, v.Protocol)
			}

			// The advertised port must be phantom's, not the upstream's - that
			// substitution is the entire reason phantom rewrites the pong.
			if res.Ad.PortV4 != p.BindPort {
				t.Errorf("client saw port %d, want phantom's bind port %d "+
					"(upstream is on %d)", res.Ad.PortV4, p.BindPort, srv.Port)
			}
			if res.Ad.ServerID == "" {
				t.Error("client saw an empty server id")
			}
		})
	}
}

// TestRealClientSessionThroughPhantom drives a full RakNet handshake and login
// through phantom - the opaque path phantom relays without understanding.
//
// It first opens a CONTROL connection straight to the upstream. That baseline
// is what makes the result trustworthy: if the direct connection fails, the
// client stack cannot run here at all and the test skips; only if direct
// succeeds and the proxied one fails is phantom actually at fault. Without the
// control, a broken RakNet backend would look exactly like a phantom bug.
func TestRealClientSessionThroughPhantom(t *testing.T) {
	requireNode(t)
	RequirePingPort(t)

	m := LoadMatrix(t)

	for _, v := range m.Versions {
		t.Run(v.MC, func(t *testing.T) {
			srv := startBedrockServer(t, v.MC)

			// Control: straight to the upstream, phantom not involved.
			var direct connectResult
			if err := runNode(t, 60*time.Second, &direct,
				"connect.js", "--host", "127.0.0.1",
				"--port", strconv.Itoa(srv.Port), "--version", v.MC); err != nil {
				t.Skipf("control connection could not run (%v); "+
					"the bedrock-protocol client stack is unusable in this "+
					"environment, so a proxied failure would be unattributable", err)
			}
			if !direct.OK {
				t.Skipf("control connection FAILED without phantom in the path "+
					"(stage %q: %s).\nThis is an environment or bedrock-protocol "+
					"problem, not a phantom one - skipping rather than reporting a "+
					"false phantom failure.", direct.Stage, direct.Error)
			}

			// The control worked, so anything that fails now is phantom's doing.
			p := Start(t, Opts{RemoteServer: srv.Addr})

			var proxied connectResult
			if err := runNode(t, 60*time.Second, &proxied,
				"connect.js", "--host", "127.0.0.1",
				"--port", strconv.Itoa(p.BindPort), "--version", v.MC); err != nil {
				t.Fatalf("connect.js through phantom: %v", err)
			}

			if !proxied.OK {
				t.Errorf("a real client reached %q directly but only %q through phantom: %s\n"+
					"--- phantom output ---\n%s",
					direct.Stage, proxied.Stage, proxied.Error, p.Output())
			}
		})
	}
}
