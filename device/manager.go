// Package device handles device lifecycle: adoption, discovery, and health checks.
package device

import (
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/rubenbe/opensoho/monitoring"
	sshpkg "github.com/rubenbe/opensoho/ssh"
)

// AdoptionResult contains the result of a device adoption attempt.
type AdoptionResult struct {
	DeviceID  string `json:"device_id"`
	Host      string `json:"host"`
	Model     string `json:"model"`
	Version   string `json:"version"`
	MAC       string `json:"mac"`
	Hostname  string `json:"hostname"`
	Adopted   bool   `json:"adopted"`
	Error     string `json:"error,omitempty"`
}

// DiscoveredDevice represents a device found during network discovery.
type DiscoveredDevice struct {
	IP        string `json:"ip"`
	MAC       string `json:"mac,omitempty"`
	IsOpenWrt bool   `json:"is_openwrt"`
	Version   string `json:"version,omitempty"`
	Model     string `json:"model,omitempty"`
	Hostname  string `json:"hostname,omitempty"`
	SSHOpen   bool   `json:"ssh_open"`
}

// Manager handles device lifecycle operations.
type Manager struct {
	SSH *sshpkg.Manager
}

// NewManager creates a new device Manager.
func NewManager(sshManager *sshpkg.Manager) *Manager {
	return &Manager{SSH: sshManager}
}

// Adopt performs the full device adoption flow:
//  1. Connect via password or no-auth (default OpenWrt)
//  2. Verify it's an OpenWrt device
//  3. Collect device info (model, MAC, version)
//  4. Inject the server's SSH public key
//  5. Close the temporary connection
//  6. Reconnect using key-based auth
//
// Returns device info on success.
func (m *Manager) Adopt(host string, port int, password string) (*AdoptionResult, error) {
	result := &AdoptionResult{Host: host}

	// Step 1: Connect (password or no-auth)
	var client interface{ Close() error }
	var execFunc func(cmd string) (string, error)

	if password == "" {
		// Default OpenWrt: root with no password
		c, err := m.SSH.ConnectNoAuth(host, port, "root")
		if err != nil {
			result.Error = fmt.Sprintf("connection failed: %v", err)
			return result, fmt.Errorf("adopt: connect to %s failed: %w", host, err)
		}
		client = c
		execFunc = func(cmd string) (string, error) {
			return sshpkg.ExecuteOnClientPublic(c, cmd)
		}
	} else {
		c, err := m.SSH.ConnectWithPassword(host, port, "root", password)
		if err != nil {
			result.Error = fmt.Sprintf("connection failed: %v", err)
			return result, fmt.Errorf("adopt: connect to %s failed: %w", host, err)
		}
		client = c
		execFunc = func(cmd string) (string, error) {
			return sshpkg.ExecuteOnClientPublic(c, cmd)
		}
	}
	defer client.Close()

	// Step 2: Verify OpenWrt
	releaseOutput, err := execFunc("cat /etc/openwrt_release 2>/dev/null || echo ''")
	if err != nil {
		result.Error = "failed to read openwrt_release"
		return result, fmt.Errorf("adopt: not an OpenWrt device at %s: %w", host, err)
	}

	isOpenWrt, version := monitoring.ParseOpenWrtRelease(releaseOutput)
	if !isOpenWrt {
		result.Error = "not an OpenWrt device"
		return result, fmt.Errorf("adopt: %s is not an OpenWrt device", host)
	}
	result.Version = version

	// Step 3: Collect device info
	boardOutput, err := execFunc("ubus call system board 2>/dev/null || echo '{}'")
	if err == nil {
		info, err := monitoring.ParseBoardInfo(boardOutput)
		if err == nil {
			result.Model = info.Model
			result.Hostname = info.Hostname
		}
	}

	macOutput, err := execFunc("cat /sys/class/net/br-lan/address 2>/dev/null || cat /sys/class/net/eth0/address 2>/dev/null || echo ''")
	if err == nil {
		result.MAC = monitoring.ParseMACAddress(macOutput)
	}

	// Step 4: Inject SSH public key
	// We need the ssh.Client type for InjectPublicKey
	if password == "" {
		c, _ := m.SSH.ConnectNoAuth(host, port, "root")
		if c != nil {
			err = m.SSH.InjectPublicKey(c)
			c.Close()
		}
	} else {
		c, _ := m.SSH.ConnectWithPassword(host, port, "root", password)
		if c != nil {
			err = m.SSH.InjectPublicKey(c)
			c.Close()
		}
	}
	if err != nil {
		result.Error = fmt.Sprintf("key injection failed: %v", err)
		return result, fmt.Errorf("adopt: key injection failed on %s: %w", host, err)
	}

	// Step 5 & 6: Connect using key-based auth and register in manager
	deviceID := result.MAC
	if deviceID == "" {
		deviceID = host // Fallback to IP
	}
	result.DeviceID = deviceID

	err = m.SSH.Connect(sshpkg.DeviceConn{
		ID:   deviceID,
		Host: host,
		Port: port,
		User: "root",
	})
	if err != nil {
		result.Error = fmt.Sprintf("key-based reconnect failed: %v", err)
		return result, fmt.Errorf("adopt: key-based connect to %s failed: %w", host, err)
	}

	result.Adopted = true
	log.Printf("[device] adopted %s (%s) at %s — %s %s", result.Hostname, result.MAC, host, result.Model, result.Version)
	return result, nil
}

// Discover scans a network range for potential OpenWrt devices.
// cidr should be like "192.168.1.0/24".
// It probes SSH on each reachable host and tries to identify OpenWrt.
func (m *Manager) Discover(cidr string, timeout time.Duration) ([]DiscoveredDevice, error) {
	hosts, err := expandCIDR(cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid CIDR %s: %w", cidr, err)
	}

	if timeout == 0 {
		timeout = 2 * time.Second
	}

	log.Printf("[discovery] scanning %d hosts in %s", len(hosts), cidr)

	results := make([]DiscoveredDevice, 0)
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrency to avoid overwhelming the network
	sem := make(chan struct{}, 50)

	for _, host := range hosts {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			device := probeHost(m.SSH, ip, timeout)
			if device != nil {
				mu.Lock()
				results = append(results, *device)
				mu.Unlock()
			}
		}(host)
	}

	wg.Wait()
	log.Printf("[discovery] found %d devices in %s", len(results), cidr)
	return results, nil
}

// probeHost checks if a host has SSH open and tries to identify it as OpenWrt.
func probeHost(sshMgr *sshpkg.Manager, ip string, timeout time.Duration) *DiscoveredDevice {
	// Quick TCP probe first
	if !sshpkg.ProbeSSH(ip, 22, timeout) {
		return nil
	}

	device := &DiscoveredDevice{
		IP:      ip,
		SSHOpen: true,
	}

	// Try to connect and identify
	client, err := sshMgr.ConnectNoAuth(ip, 22, "root")
	if err != nil {
		// SSH is open but we can't connect (has password, etc.)
		return device
	}
	defer client.Close()

	// Check if it's OpenWrt
	output, err := sshpkg.ExecuteOnClientPublic(client, "cat /etc/openwrt_release 2>/dev/null || echo ''")
	if err != nil {
		return device
	}

	isOpenWrt, version := monitoring.ParseOpenWrtRelease(output)
	device.IsOpenWrt = isOpenWrt
	device.Version = version

	if isOpenWrt {
		// Get more info
		boardOutput, err := sshpkg.ExecuteOnClientPublic(client, "ubus call system board 2>/dev/null || echo '{}'")
		if err == nil {
			info, err := monitoring.ParseBoardInfo(boardOutput)
			if err == nil {
				device.Model = info.Model
				device.Hostname = info.Hostname
			}
		}

		macOutput, err := sshpkg.ExecuteOnClientPublic(client, "cat /sys/class/net/br-lan/address 2>/dev/null || cat /sys/class/net/eth0/address 2>/dev/null || echo ''")
		if err == nil {
			device.MAC = monitoring.ParseMACAddress(macOutput)
		}
	}

	return device
}

// expandCIDR expands a CIDR notation to a list of host IPs (excluding network and broadcast).
func expandCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ips = append(ips, ip.String())
	}

	if len(ips) <= 2 {
		return ips, nil // /31 or /32
	}

	// Remove network and broadcast addresses
	return ips[1 : len(ips)-1], nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

// HealthCheck verifies connectivity to a managed device and returns its status.
func (m *Manager) HealthCheck(deviceID string) (bool, string) {
	if !m.SSH.IsConnected(deviceID) {
		// Try to reconnect
		err := m.SSH.Reconnect(deviceID)
		if err != nil {
			return false, fmt.Sprintf("offline: %v", err)
		}
	}

	err := m.SSH.Ping(deviceID)
	if err != nil {
		return false, fmt.Sprintf("ping failed: %v", err)
	}

	return true, "online"
}

// GetDeviceInfo collects current device information via SSH.
func (m *Manager) GetDeviceInfo(deviceID string) (*AdoptionResult, error) {
	result := &AdoptionResult{DeviceID: deviceID}

	boardOutput, err := m.SSH.Execute(deviceID, "ubus call system board 2>/dev/null || echo '{}'")
	if err != nil {
		return nil, fmt.Errorf("failed to get board info: %w", err)
	}

	info, err := monitoring.ParseBoardInfo(boardOutput)
	if err == nil {
		result.Model = info.Model
		result.Hostname = info.Hostname
		result.Version = info.Release.Version
	}

	macOutput, err := m.SSH.Execute(deviceID, "cat /sys/class/net/br-lan/address 2>/dev/null || echo ''")
	if err == nil {
		result.MAC = monitoring.ParseMACAddress(macOutput)
	}

	releaseOutput, err := m.SSH.Execute(deviceID, "cat /etc/openwrt_release 2>/dev/null || echo ''")
	if err == nil {
		_, result.Version = monitoring.ParseOpenWrtRelease(releaseOutput)
	}

	return result, nil
}

// ListManagedDeviceIDs returns the IDs of all devices currently managed by the SSH manager.
// This is a convenience wrapper that returns what the poller needs.
func (m *Manager) ListManagedDeviceIDs() []string {
	// This will need to be connected to the PocketBase device records in Phase 6.
	// For now, we expose it as an interface the caller can implement.
	return nil
}

// RebootDevice reboots a managed device.
func (m *Manager) RebootDevice(deviceID string) error {
	_, err := m.SSH.Execute(deviceID, "reboot")
	if err != nil && !strings.Contains(err.Error(), "connection reset") {
		return fmt.Errorf("reboot failed: %w", err)
	}
	return nil
}
