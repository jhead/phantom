// Package compat is the end-to-end compatibility harness.
//
// BLACK-BOX RULE: this package must not import phantom's implementation
// packages (internal/proto, internal/proxy, internal/clientmap). It knows only
// the built binary, its command-line flags, and UDP. Importing internal/corpus
// is permitted - that package is fixture data and the XFAIL registry, with no
// phantom protocol logic in it. TestBlackBoxRuleHolds in e2e_test.go enforces
// this mechanically rather than trusting anyone to remember.
//
// See DESIGN.md.
package compat

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jhead/phantom/internal/corpus"
)

// Timeouts. Generous by default because CI runners are slow and UDP is lossy;
// override with PHANTOM_TEST_TIMEOUT_SCALE for very slow machines.
var (
	ReadyTimeout   = 20 * time.Second
	PingTimeout    = 2 * time.Second
	ShutdownGrace  = 5 * time.Second
	pollInterval   = 150 * time.Millisecond
	pingServerPort = 19132
)

func init() {
	if s := os.Getenv("PHANTOM_TEST_TIMEOUT_SCALE"); s != "" {
		if scale, err := strconv.ParseFloat(s, 64); err == nil && scale > 0 {
			ReadyTimeout = time.Duration(float64(ReadyTimeout) * scale)
			PingTimeout = time.Duration(float64(PingTimeout) * scale)
			ShutdownGrace = time.Duration(float64(ShutdownGrace) * scale)
		}
	}
}

var (
	buildOnce sync.Once
	binPath   string
	buildErr  error
)

// BuildPhantom compiles the phantom binary once per test run and returns its
// path. Tests drive the real shipped artifact, not an in-process ProxyServer:
// Start() blocks, Close() panics if called before bind, and serverID is a
// package global - so "two instances must advertise different ids" is simply
// not observable in-process.
func BuildPhantom(t *testing.T) string {
	t.Helper()

	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "phantom-compat-*")
		if err != nil {
			buildErr = err
			return
		}
		out := filepath.Join(dir, "phantom")

		root, err := repoRoot()
		if err != nil {
			buildErr = err
			return
		}

		cmd := exec.Command("go", "build", "-o", out, "./cmd")
		cmd.Dir = root
		if combined, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("building phantom: %v\n%s", err, combined)
			return
		}
		binPath = out
	})

	if buildErr != nil {
		t.Fatalf("%v", buildErr)
	}
	return binPath
}

// repoRoot walks up from this file's package directory to the module root.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("could not locate go.mod above %s", dir)
}

// RequirePingPort skips the test unless UDP :19132 is free.
//
// phantom binds :19132 unconditionally, and because it sets SO_REUSEPORT a
// second binder does not fail - the kernel load-balances datagrams between
// them, so two concurrent cases would silently steal each other's packets and
// produce baffling flakes. Skipping loudly beats that. In CI each shard runs on
// its own runner, so the port is always free there.
func RequirePingPort(t *testing.T) {
	t.Helper()

	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: pingServerPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Skipf("UDP :%d is in use (%v) - phantom binds it unconditionally, so this "+
			"test cannot run here. Stop any running phantom or Minecraft instance.",
			pingServerPort, err)
	}
	_ = conn.Close()
}

// Opts configures a phantom subprocess.
type Opts struct {
	RemoteServer string        // -server (required)
	BindPort     int           // -bind_port; 0 picks a free one
	IdleTimeout  time.Duration // -timeout
	RemovePorts  bool          // -remove_ports
	Workers      int           // -workers
	Debug        bool          // -debug
}

// Phantom is a running phantom subprocess.
type Phantom struct {
	BindPort int
	Addr     string

	cmd *exec.Cmd
	out *lockedBuffer
	t   *testing.T
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Start launches phantom and blocks until it answers a ping, then registers
// cleanup. Readiness is established by BEHAVIOUR - we ping until we get a pong
// - rather than by scraping logs or sleeping a fixed interval.
func Start(t *testing.T, opts Opts) *Phantom {
	t.Helper()

	bin := BuildPhantom(t)

	port := opts.BindPort
	if port == 0 {
		port = freeUDPPort(t)
	}

	// -bind_port is always passed explicitly: phantom's default of 0 makes it
	// choose a random port, which the test would have no way to discover.
	args := []string{
		"-server", opts.RemoteServer,
		"-bind", "127.0.0.1",
		"-bind_port", strconv.Itoa(port),
	}
	if opts.IdleTimeout > 0 {
		args = append(args, "-timeout", strconv.Itoa(int(opts.IdleTimeout.Seconds())))
	}
	if opts.RemovePorts {
		args = append(args, "-remove_ports")
	}
	if opts.Workers > 0 {
		args = append(args, "-workers", strconv.Itoa(opts.Workers))
	}
	if opts.Debug {
		args = append(args, "-debug")
	}

	out := &lockedBuffer{}
	cmd := exec.Command(bin, args...)
	cmd.Stdout = out
	cmd.Stderr = out

	if err := cmd.Start(); err != nil {
		t.Fatalf("starting phantom: %v", err)
	}

	p := &Phantom{
		BindPort: port,
		Addr:     fmt.Sprintf("127.0.0.1:%d", port),
		cmd:      cmd,
		out:      out,
		t:        t,
	}
	t.Cleanup(p.Close)

	if err := p.waitReady(); err != nil {
		t.Fatalf("phantom did not become ready: %v\n--- phantom output ---\n%s", err, out.String())
	}
	return p
}

// waitReady pings until phantom proxies a pong back.
func (p *Phantom) waitReady() error {
	deadline := time.Now().Add(ReadyTimeout)
	var lastErr error

	for time.Now().Before(deadline) {
		if p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited() {
			return fmt.Errorf("phantom exited early with %v", p.cmd.ProcessState)
		}
		if _, err := p.Ping(); err == nil {
			return nil
		} else {
			lastErr = err
		}
		time.Sleep(pollInterval)
	}
	return fmt.Errorf("no pong within %v (last error: %v)", ReadyTimeout, lastErr)
}

// Ping sends an Unconnected Ping to phantom and returns the pong.
func (p *Phantom) Ping() ([]byte, error) {
	return p.PingWith(corpus.PingID, DefaultPingTime())
}

// PingWith sends a ping with a specific packet id and ping time, which is how
// the 0x02 and ping-time-echo invariants are exercised.
func (p *Phantom) PingWith(packetID byte, pingTime []byte) ([]byte, error) {
	c, err := NewClient(p.Addr)
	if err != nil {
		return nil, err
	}
	defer c.Close()
	return c.Ping(packetID, pingTime)
}

// Output returns everything phantom has logged so far.
func (p *Phantom) Output() string { return p.out.String() }

// Close terminates phantom, escalating to SIGKILL if it will not exit.
func (p *Phantom) Close() {
	if p.cmd.Process == nil {
		return
	}
	_ = p.cmd.Process.Signal(os.Interrupt)

	done := make(chan struct{})
	go func() {
		_ = p.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(ShutdownGrace):
		_ = p.cmd.Process.Kill()
		<-done
	}
}

// Client is a minimal RakNet-speaking UDP client.
type Client struct {
	conn *net.UDPConn
}

// NewClient dials a UDP socket at addr.
func NewClient(addr string) (*Client, error) {
	raddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp4", nil, raddr)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

// DefaultPingTime is an arbitrary fixed ping time; servers must echo it back.
func DefaultPingTime() []byte {
	return []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77}
}

// BuildPing assembles an Unconnected Ping datagram.
func BuildPing(packetID byte, pingTime []byte) []byte {
	out := make([]byte, 0, 33)
	out = append(out, packetID)
	out = append(out, pingTime...)
	out = append(out, corpus.Magic...)
	guid := make([]byte, 8)
	binary.BigEndian.PutUint64(guid, 0x8877665544332211)
	return append(out, guid...)
}

// Ping sends an offline ping and waits for a reply.
func (c *Client) Ping(packetID byte, pingTime []byte) ([]byte, error) {
	if err := c.Send(BuildPing(packetID, pingTime)); err != nil {
		return nil, err
	}
	return c.Recv(PingTimeout)
}

// Send writes a datagram.
func (c *Client) Send(data []byte) error {
	_, err := c.conn.Write(data)
	return err
}

// Recv waits for a datagram.
func (c *Client) Recv(timeout time.Duration) ([]byte, error) {
	if err := c.conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	n, err := c.conn.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// LocalAddr is the client's source address.
func (c *Client) LocalAddr() net.Addr { return c.conn.LocalAddr() }

// Close releases the socket.
func (c *Client) Close() error { return c.conn.Close() }

// freeUDPPort asks the kernel for an unused UDP port.
func freeUDPPort(t *testing.T) int {
	t.Helper()

	addr, err := net.ResolveUDPAddr("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		t.Fatalf("finding a free port: %v", err)
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).Port
}
