package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gossh "golang.org/x/crypto/ssh"
)

// startTestSSHServer spins up a minimal SSH server on a random port for testing.
// Returns the address (host:port) and a cleanup function.
func startTestSSHServer(t *testing.T) (string, func()) {
	t.Helper()

	// Generate a host key for the test server
	_, hostPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	hostSigner, err := gossh.NewSignerFromKey(hostPriv)
	require.NoError(t, err)

	config := &gossh.ServerConfig{
		NoClientAuth: true, // Accept unauthenticated connections
		// Also accept password auth (for ConnectWithPassword tests)
		PasswordCallback: func(c gossh.ConnMetadata, pass []byte) (*gossh.Permissions, error) {
			return nil, nil // Accept any password
		},
		// Also accept public key auth (for Connect tests)
		PublicKeyCallback: func(c gossh.ConnMetadata, pubKey gossh.PublicKey) (*gossh.Permissions, error) {
			return nil, nil // Accept any key
		},
	}
	config.AddHostKey(hostSigner)

	// Listen on a random port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return // listener closed
			}
			go handleTestConnection(conn, config)
		}
	}()

	cleanup := func() {
		listener.Close()
		<-done
	}

	return listener.Addr().String(), cleanup
}

func handleTestConnection(conn net.Conn, config *gossh.ServerConfig) {
	defer conn.Close()

	sshConn, chans, reqs, err := gossh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	defer sshConn.Close()

	// Handle global requests (keepalives, etc.)
	go func() {
		for req := range reqs {
			if req.WantReply {
				req.Reply(true, nil)
			}
		}
	}()

	// Handle channels
	for newChannel := range chans {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(gossh.UnknownChannelType, "unsupported")
			continue
		}

		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}

		go func() {
			defer channel.Close()
			for req := range requests {
				switch req.Type {
				case "exec":
					// Parse command from the request
					if len(req.Payload) < 4 {
						req.Reply(false, nil)
						return
					}
					cmdLen := int(req.Payload[0])<<24 | int(req.Payload[1])<<16 | int(req.Payload[2])<<8 | int(req.Payload[3])
					if len(req.Payload) < 4+cmdLen {
						req.Reply(false, nil)
						return
					}
					cmd := string(req.Payload[4 : 4+cmdLen])

					req.Reply(true, nil)

					// Simulate command responses
					switch {
					case cmd == "echo ok":
						fmt.Fprint(channel, "ok\n")
					case cmd == "echo hello && echo world":
						fmt.Fprint(channel, "hello\nworld\n")
					case strings.HasPrefix(cmd, "grep -qF"):
						fmt.Fprint(channel, "MISSING\n")
					case strings.HasPrefix(cmd, "mkdir -p"):
						// Success, no output
					case cmd == "cat /etc/openwrt_release":
						fmt.Fprint(channel, "DISTRIB_ID='OpenWrt'\nDISTRIB_RELEASE='23.05.3'\n")
					default:
						fmt.Fprintf(channel, "mock: %s\n", cmd)
					}

					channel.SendRequest("exit-status", false, []byte{0, 0, 0, 0})
					return // Close channel (sends EOF) so CombinedOutput returns
				default:
					if req.WantReply {
						req.Reply(false, nil)
					}
				}
			}
		}()
	}
}

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := tmpDir + "/test_key"

	mgr, err := NewManager(keyPath)
	require.NoError(t, err)
	assert.NotNil(t, mgr)
	assert.NotEmpty(t, mgr.PublicKey())
	assert.True(t, strings.HasPrefix(mgr.PublicKey(), "ssh-ed25519 "))
}

func TestNewManager_LoadExistingKey(t *testing.T) {
	tmpDir := t.TempDir()
	keyPath := tmpDir + "/test_key"

	// Create manager (generates key)
	mgr1, err := NewManager(keyPath)
	require.NoError(t, err)
	pub1 := mgr1.PublicKey()

	// Create another manager (loads same key)
	mgr2, err := NewManager(keyPath)
	require.NoError(t, err)
	pub2 := mgr2.PublicKey()

	assert.Equal(t, pub1, pub2, "loading the same key file should produce the same public key")
}

func TestConnect_NoAuth(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	// The test server accepts any client, so ConnectNoAuth should work
	client, err := mgr.ConnectNoAuth(host, port, "root")
	require.NoError(t, err)
	assert.NotNil(t, client)
	defer client.Close()

	// Execute a command on the temporary client
	output, err := executeOnClient(client, "echo ok")
	assert.NoError(t, err)
	assert.Equal(t, "ok\n", output)
}

func TestExecute(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	// Register device via ConnectNoAuth then add to manager manually
	client, err := mgr.ConnectNoAuth(host, port, "root")
	require.NoError(t, err)

	mgr.mu.Lock()
	mgr.devices["test-device"] = &DeviceConn{
		ID:       "test-device",
		Host:     host,
		Port:     port,
		User:     "root",
		client:   client,
		lastUsed: time.Now(),
	}
	mgr.mu.Unlock()

	// Execute single command
	output, err := mgr.Execute("test-device", "echo ok")
	assert.NoError(t, err)
	assert.Equal(t, "ok\n", output)
}

func TestExecuteBatch(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	client, err := mgr.ConnectNoAuth(host, port, "root")
	require.NoError(t, err)

	mgr.mu.Lock()
	mgr.devices["test-device"] = &DeviceConn{
		ID:     "test-device",
		Host:   host,
		Port:   port,
		User:   "root",
		client: client,
	}
	mgr.mu.Unlock()

	results, err := mgr.ExecuteBatch("test-device", []string{"echo ok", "echo ok"})
	assert.NoError(t, err)
	assert.Equal(t, 2, len(results))
	assert.Equal(t, "ok\n", results[0])
	assert.Equal(t, "ok\n", results[1])
}

func TestExecuteScript(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	client, err := mgr.ConnectNoAuth(host, port, "root")
	require.NoError(t, err)

	mgr.mu.Lock()
	mgr.devices["test-device"] = &DeviceConn{
		ID:     "test-device",
		Host:   host,
		Port:   port,
		User:   "root",
		client: client,
	}
	mgr.mu.Unlock()

	output, err := mgr.ExecuteScript("test-device", []string{"echo hello", "echo world"})
	assert.NoError(t, err)
	assert.Equal(t, "hello\nworld\n", output)
}

func TestInjectPublicKey(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	client, err := mgr.ConnectNoAuth(host, port, "root")
	require.NoError(t, err)
	defer client.Close()

	// Should not error (mock server accepts any command)
	err = mgr.InjectPublicKey(client)
	assert.NoError(t, err)
}

func TestPing(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	client, err := mgr.ConnectNoAuth(host, port, "root")
	require.NoError(t, err)

	mgr.mu.Lock()
	mgr.devices["test-device"] = &DeviceConn{
		ID:     "test-device",
		Host:   host,
		Port:   port,
		User:   "root",
		client: client,
	}
	mgr.mu.Unlock()

	err = mgr.Ping("test-device")
	assert.NoError(t, err)
}

func TestExecute_UnknownDevice(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	_, err = mgr.Execute("nonexistent", "echo ok")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDisconnect(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	client, err := mgr.ConnectNoAuth(host, port, "root")
	require.NoError(t, err)

	mgr.mu.Lock()
	mgr.devices["test-device"] = &DeviceConn{
		ID:     "test-device",
		Host:   host,
		Port:   port,
		User:   "root",
		client: client,
	}
	mgr.mu.Unlock()

	// Verify connected
	err = mgr.Ping("test-device")
	assert.NoError(t, err)

	// Disconnect
	mgr.Disconnect("test-device")

	// Should no longer be found
	_, err = mgr.Execute("test-device", "echo ok")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestProbeSSH(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	// Probe should succeed against the test server
	assert.True(t, ProbeSSH(host, port, 2*time.Second))

	// Probe should fail against a closed port
	assert.False(t, ProbeSSH("127.0.0.1", 59999, 500*time.Millisecond))
}

func TestDisconnectAll(t *testing.T) {
	addr, cleanup := startTestSSHServer(t)
	defer cleanup()

	parts := strings.SplitN(addr, ":", 2)
	host := parts[0]
	port := 0
	fmt.Sscanf(parts[1], "%d", &port)

	tmpDir := t.TempDir()
	mgr, err := NewManager(tmpDir + "/test_key")
	require.NoError(t, err)

	// Add two devices
	for _, id := range []string{"dev1", "dev2"} {
		client, err := mgr.ConnectNoAuth(host, port, "root")
		require.NoError(t, err)

		mgr.mu.Lock()
		mgr.devices[id] = &DeviceConn{
			ID: id, Host: host, Port: port, User: "root", client: client,
		}
		mgr.mu.Unlock()
	}

	mgr.DisconnectAll()

	mgr.mu.RLock()
	assert.Equal(t, 0, len(mgr.devices))
	mgr.mu.RUnlock()
}
