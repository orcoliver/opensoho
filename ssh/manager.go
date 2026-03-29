package ssh

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	defaultSSHPort       = 22
	defaultSSHUser       = "root"
	defaultTimeout       = 10 * time.Second
	keepaliveInterval    = 30 * time.Second
	maxReconnectDelay    = 30 * time.Second
	initialReconnectDelay = 1 * time.Second
)

// Manager manages SSH connections to multiple OpenWrt devices.
type Manager struct {
	devices map[string]*DeviceConn
	mu      sync.RWMutex
	signer  ssh.Signer
	pubKey  string // Public key in authorized_keys format
}

// DeviceConn represents a persistent SSH connection to a single device.
type DeviceConn struct {
	ID       string
	Host     string
	Port     int
	User     string
	client   *ssh.Client
	mu       sync.Mutex
	lastUsed time.Time
}

// NewManager creates a new SSH Manager by loading or generating the server SSH key.
func NewManager(privateKeyPath string) (*Manager, error) {
	signer, pubKey, err := LoadOrGenerateKey(privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize SSH key: %w", err)
	}

	return &Manager{
		devices: make(map[string]*DeviceConn),
		signer:  signer,
		pubKey:  pubKey,
	}, nil
}

// PublicKey returns the server's public key in authorized_keys format.
func (m *Manager) PublicKey() string {
	return m.pubKey
}

// sshClientConfig creates an SSH client config using the server's key.
func (m *Manager) sshClientConfig(user string) *ssh.ClientConfig {
	return &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(m.signer),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // Acceptable for SOHO management
		Timeout:         defaultTimeout,
	}
}

// Connect establishes an SSH connection to a device using the server's key.
// The connection is stored and managed by the Manager.
func (m *Manager) Connect(dc DeviceConn) error {
	config := m.sshClientConfig(dc.User)
	if dc.User == "" {
		config.User = defaultSSHUser
	}
	if dc.Port == 0 {
		dc.Port = defaultSSHPort
	}

	addr := fmt.Sprintf("%s:%d", dc.Host, dc.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh connect to %s failed: %w", addr, err)
	}

	dc.client = client
	dc.lastUsed = time.Now()

	m.mu.Lock()
	m.devices[dc.ID] = &dc
	m.mu.Unlock()

	return nil
}

// ConnectWithPassword connects to a device using password authentication.
// Used during initial device adoption when the server's key isn't yet on the device.
// Returns a temporary client — caller should use InjectPublicKey and then close it.
func (m *Manager) ConnectWithPassword(host string, port int, user, password string) (*ssh.Client, error) {
	if user == "" {
		user = defaultSSHUser
	}
	if port == 0 {
		port = defaultSSHPort
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         defaultTimeout,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh password connect to %s failed: %w", addr, err)
	}

	return client, nil
}

// ConnectNoAuth connects to a device with root and no authentication.
// This is the default state of OpenWrt devices before a password is set.
func (m *Manager) ConnectNoAuth(host string, port int, user string) (*ssh.Client, error) {
	if user == "" {
		user = defaultSSHUser
	}
	if port == 0 {
		port = defaultSSHPort
	}

	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(""), // Empty password
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         defaultTimeout,
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("ssh no-auth connect to %s failed: %w", addr, err)
	}

	return client, nil
}

// InjectPublicKey appends the server's public key to the device's authorized_keys.
// It checks if the key is already present to avoid duplicates.
func (m *Manager) InjectPublicKey(client *ssh.Client) error {
	pubKeyTrimmed := strings.TrimSpace(m.pubKey)

	// Check if key is already present
	checkCmd := fmt.Sprintf("grep -qF '%s' /etc/dropbear/authorized_keys 2>/dev/null && echo EXISTS || echo MISSING", pubKeyTrimmed)
	output, err := executeOnClient(client, checkCmd)
	if err != nil {
		return fmt.Errorf("failed to check authorized_keys: %w", err)
	}

	if strings.TrimSpace(output) == "EXISTS" {
		return nil // Key already present
	}

	// Ensure directory exists and append key
	injectCmd := fmt.Sprintf(
		"mkdir -p /etc/dropbear && echo '%s' >> /etc/dropbear/authorized_keys && chmod 600 /etc/dropbear/authorized_keys",
		pubKeyTrimmed,
	)
	_, err = executeOnClient(client, injectCmd)
	if err != nil {
		return fmt.Errorf("failed to inject public key: %w", err)
	}

	return nil
}

// Execute runs a single command on a managed device and returns its output.
func (m *Manager) Execute(deviceID, command string) (string, error) {
	dc, err := m.getDevice(deviceID)
	if err != nil {
		return "", err
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.client == nil {
		return "", fmt.Errorf("device %s is not connected", deviceID)
	}

	dc.lastUsed = time.Now()
	return executeOnClient(dc.client, command)
}

// ExecuteBatch runs multiple commands sequentially on a managed device.
// Each command is executed in a separate SSH session.
// Returns the output of each command.
func (m *Manager) ExecuteBatch(deviceID string, commands []string) ([]string, error) {
	dc, err := m.getDevice(deviceID)
	if err != nil {
		return nil, err
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	if dc.client == nil {
		return nil, fmt.Errorf("device %s is not connected", deviceID)
	}

	dc.lastUsed = time.Now()
	results := make([]string, len(commands))
	for i, cmd := range commands {
		output, err := executeOnClient(dc.client, cmd)
		if err != nil {
			return results, fmt.Errorf("command %d (%s) failed: %w", i, cmd, err)
		}
		results[i] = output
	}

	return results, nil
}

// ExecuteScript runs multiple commands as a single shell script on a managed device.
// This is more efficient than ExecuteBatch as it uses a single SSH session.
func (m *Manager) ExecuteScript(deviceID string, commands []string) (string, error) {
	script := strings.Join(commands, " && ")
	return m.Execute(deviceID, script)
}

// Ping checks if a device is reachable via SSH.
func (m *Manager) Ping(deviceID string) error {
	_, err := m.Execute(deviceID, "echo ok")
	return err
}

// IsConnected checks if a device has an active SSH connection.
func (m *Manager) IsConnected(deviceID string) bool {
	m.mu.RLock()
	dc, exists := m.devices[deviceID]
	m.mu.RUnlock()

	if !exists || dc.client == nil {
		return false
	}

	// Try a quick operation to verify the connection is alive
	_, _, err := dc.client.SendRequest("keepalive@opensoho", true, nil)
	return err == nil
}

// Disconnect closes the SSH connection to a device and removes it from the manager.
func (m *Manager) Disconnect(deviceID string) {
	m.mu.Lock()
	dc, exists := m.devices[deviceID]
	if exists {
		if dc.client != nil {
			dc.client.Close()
		}
		delete(m.devices, deviceID)
	}
	m.mu.Unlock()
}

// DisconnectAll closes all managed SSH connections.
func (m *Manager) DisconnectAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, dc := range m.devices {
		if dc.client != nil {
			dc.client.Close()
		}
		delete(m.devices, id)
	}
}

// Reconnect attempts to re-establish a dropped SSH connection.
func (m *Manager) Reconnect(deviceID string) error {
	m.mu.RLock()
	dc, exists := m.devices[deviceID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("device %s not found in manager", deviceID)
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Close existing connection if any
	if dc.client != nil {
		dc.client.Close()
		dc.client = nil
	}

	user := dc.User
	if user == "" {
		user = defaultSSHUser
	}

	config := m.sshClientConfig(user)
	addr := fmt.Sprintf("%s:%d", dc.Host, dc.Port)

	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("ssh reconnect to %s failed: %w", addr, err)
	}

	dc.client = client
	dc.lastUsed = time.Now()
	return nil
}

// ProbeSSH attempts a TCP connection to the SSH port to check if it's open.
// Does NOT establish an SSH session — just checks network reachability.
func ProbeSSH(host string, port int, timeout time.Duration) bool {
	if port == 0 {
		port = defaultSSHPort
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// --- internal helpers ---

func (m *Manager) getDevice(deviceID string) (*DeviceConn, error) {
	m.mu.RLock()
	dc, exists := m.devices[deviceID]
	m.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("device %s not found in manager", deviceID)
	}
	return dc, nil
}

func executeOnClient(client *ssh.Client, command string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("failed to create ssh session: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(command)
	if err != nil {
		// Include output in error for debugging (e.g., uci error messages)
		return string(output), fmt.Errorf("command failed: %w (output: %s)", err, strings.TrimSpace(string(output)))
	}

	return string(output), nil
}

// ExecuteOnClientPublic runs a command on a standalone ssh.Client.
// This is used during device adoption when the device is not yet managed.
func ExecuteOnClientPublic(client *ssh.Client, command string) (string, error) {
	return executeOnClient(client, command)
}
