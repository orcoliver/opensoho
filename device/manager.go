// Package device handles device lifecycle: adoption, discovery, and health checks.
package device

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rubenbe/opensoho/monitoring"
	sshpkg "github.com/rubenbe/opensoho/ssh"
	ucipkg "github.com/rubenbe/opensoho/uci"
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

// ImportedConfig holds the configuration read from a live OpenWrt device.
type ImportedConfig struct {
	Hostname      string
	Radios        []ImportedRadio
	Wifis         []ImportedWifi
	Leds          []ImportedLed
	Dawn          *ImportedDawn
	DetectedRole  string           // "dumb_ap", "router", or ""
	EthernetPorts []string         // e.g. ["eth0", "eth1"] from network.br_lan_dev.ports
	ExtraNetworks []ImportedNetwork // additional networks detected in the device
}

// ImportedNetwork represents an extra network interface detected during import.
type ImportedNetwork struct {
	Name        string
	Device      string
	Proto       string
	BridgePorts []string
}

// ImportedRadio represents a wifi-device section from uci show wireless.
type ImportedRadio struct {
	Number        int
	Band          string
	Frequency     int
	Channel       int
	Htmode        string
	AutoFrequency bool
}

// ImportedWifi represents a wifi-iface section from uci show wireless.
type ImportedWifi struct {
	SSID               string
	Key                string
	Encryption         string
	Hidden             bool
	Isolate            bool
	Enabled            bool
	Ieee80211r         bool
	Ieee80211k         bool
	BssTransition      bool
	WnmSleepMode       bool
	ProxyArp           bool
	FtOverDs           bool
	FtPskGenerateLocal bool
	MobilityDomain     string
	ReassocDeadline    int
	Nasid              string
	RrmNeighborReport  bool
	RrmBeaconReport    bool
	Network            string // VLAN name
}

// ImportedLed represents a led section from uci show system.
type ImportedLed struct {
	Name     string // human name
	Sysfs    string // sysfs path (led_name in DB)
	Trigger  string
	Dev      string   // netdev trigger
	Mode     []string // netdev modes
	DelayOn  int      // timer trigger
	DelayOff int      // timer trigger
}

// ImportedDawn holds the dawn config read from uci show dawn.
type ImportedDawn struct {
	// global
	Kicking      bool
	SetHostapdNr bool
	RrmMode      string
	// metric
	InitialScore int
	HtSupport    int
	VhtSupport   int
	HeSupport    int
	Rssi         int
	RssiVal      int
	LowRssi      int
	LowRssiVal   int
	Freq5        int
	ChanUtil     int
	RssiWeight   int
	RssiCenter   int
	// times
	UpdateClient        int
	RemoveClient        int
	RemoveProbe         int
	UpdateHostapd       int
	UpdateTcpCon        int
	UpdateChanUtil      int
	UpdateBeaconReports int
	// behaviour
	KickingThreshold  int
	MinProbeCount     int
	BandwidthThreshold int
	UseStationCount   bool
	MaxStationDiff    int
	MinNumberToKick   int
	ChanUtilAvgPeriod int
	MinKickCount      int
}

// ImportDeviceConfig reads the current UCI configuration from a live device via SSH
// and returns a structured ImportedConfig ready to be stored in the DB.
func (m *Manager) ImportDeviceConfig(host string, port int, password string) (*ImportedConfig, error) {
	var client interface{ Close() error }
	var execFunc func(cmd string) (string, error)

	if password == "" {
		c, err := m.SSH.ConnectNoAuth(host, port, "root")
		if err != nil {
			return nil, fmt.Errorf("import: connect to %s failed: %w", host, err)
		}
		client = c
		execFunc = func(cmd string) (string, error) { return sshpkg.ExecuteOnClientPublic(c, cmd) }
	} else {
		c, err := m.SSH.ConnectWithPassword(host, port, "root", password)
		if err != nil {
			return nil, fmt.Errorf("import: connect to %s failed: %w", host, err)
		}
		client = c
		execFunc = func(cmd string) (string, error) { return sshpkg.ExecuteOnClientPublic(c, cmd) }
	}
	defer client.Close()

	cfg := &ImportedConfig{}

	// --- hostname ---
	systemOut, err := execFunc("uci show system 2>/dev/null || echo ''")
	if err == nil {
		sysCfg, err := ucipkg.ParseShow(systemOut)
		if err == nil {
			cfg.Hostname, cfg.Leds = parseSystemConfig(sysCfg)
		}
	}

	// --- wireless ---
	wirelessOut, err := execFunc("uci show wireless 2>/dev/null || echo ''")
	if err == nil {
		wCfg, err := ucipkg.ParseShow(wirelessOut)
		if err == nil {
			cfg.Radios, cfg.Wifis = parseWirelessConfig(wCfg)
		}
	}

	// --- dawn (optional) ---
	dawnOut, err := execFunc("uci show dawn 2>/dev/null || echo ''")
	if err == nil && strings.TrimSpace(dawnOut) != "" {
		dCfg, err := ucipkg.ParseShow(dawnOut)
		if err == nil {
			cfg.Dawn = parseDawnConfig(dCfg)
		}
	}

	// --- network: ethernet ports and extra networks ---
	networkOut, err := execFunc("uci show network 2>/dev/null || echo ''")
	if err == nil && strings.TrimSpace(networkOut) != "" {
		nCfg, err := ucipkg.ParseShow(networkOut)
		if err == nil {
			cfg.EthernetPorts, cfg.ExtraNetworks = parseNetworkConfig(nCfg)
		}
	}

	// --- role detection via firewall + network ---
	firewallOut, err := execFunc("uci show firewall 2>/dev/null || echo ''")
	firewallStr := ""
	if err == nil {
		firewallStr = firewallOut
	}
	cfg.DetectedRole = detectDeviceRole(networkOut, firewallStr)

	return cfg, nil
}

// detectDeviceRole inspects network and firewall UCI output to determine the device role.
// Returns "dumb_ap", "router", or "" if undetermined.
func detectDeviceRole(networkOut, firewallOut string) string {
	// Dumb AP heuristic: firewall accepts all and LAN is DHCP client
	isDumbAp := false
	if strings.Contains(firewallOut, "input='ACCEPT'") || strings.Contains(firewallOut, "input=ACCEPT") {
		isDumbAp = true
	}
	if strings.Contains(networkOut, "network.lan.proto='dhcp'") || strings.Contains(networkOut, "network.lan.proto=dhcp") {
		isDumbAp = true
	}

	// Router heuristic: has a WAN interface with a real proto
	if strings.Contains(networkOut, "network.wan.proto='pppoe'") ||
		strings.Contains(networkOut, "network.wan.proto='dhcp'") ||
		strings.Contains(networkOut, "network.wan.proto=pppoe") ||
		strings.Contains(networkOut, "network.wan.proto=dhcp") {
		return "router"
	}

	if isDumbAp {
		return "dumb_ap"
	}
	return "dumb_ap" // sensible default for SOHO
}

// parseNetworkConfig extracts ethernet ports from br-lan device and detects extra networks.
func parseNetworkConfig(cfg *ucipkg.Config) (ethernetPorts []string, extras []ImportedNetwork) {
	// Track which interface names are known (loopback, lan) to identify extras
	knownInterfaces := map[string]bool{"loopback": true, "lan": true, "wan": true, "wan6": true}

	// First pass: find br_lan device ports
	for _, s := range cfg.Sections {
		if s.Type == "device" {
			nameVal := s.GetOption("name")
			if nameVal == "br-lan" {
				if ports, ok := s.Options["ports"]; ok {
					switch v := ports.(type) {
					case string:
						ethernetPorts = append(ethernetPorts, v)
					case []string:
						ethernetPorts = append(ethernetPorts, v...)
					}
				}
			}
		}
	}

	// Second pass: find extra interface sections (not loopback/lan/wan)
	for _, s := range cfg.Sections {
		if s.Type != "interface" {
			continue
		}
		if knownInterfaces[s.Name] {
			continue
		}
		en := ImportedNetwork{
			Name:   s.Name,
			Device: s.GetOption("device"),
			Proto:  s.GetOption("proto"),
		}
		extras = append(extras, en)
	}

	return ethernetPorts, extras
}

// --- import parsers ---

func parseSystemConfig(cfg *ucipkg.Config) (hostname string, leds []ImportedLed) {
	for _, s := range cfg.Sections {
		switch s.Type {
		case "system":
			hostname = s.GetOption("hostname")
		case "led":
			led := ImportedLed{
				Name:    s.GetOption("name"),
				Sysfs:   s.GetOption("sysfs"),
				Trigger: s.GetOption("trigger"),
				Dev:     s.GetOption("dev"),
			}
			if modeStr := s.GetOption("mode"); modeStr != "" {
				led.Mode = strings.Fields(modeStr)
			}
			led.DelayOn, _ = strconv.Atoi(s.GetOption("delayon"))
			led.DelayOff, _ = strconv.Atoi(s.GetOption("delayoff"))
			if led.Sysfs != "" {
				leds = append(leds, led)
			}
		}
	}
	return
}

func parseWirelessConfig(cfg *ucipkg.Config) (radios []ImportedRadio, wifis []ImportedWifi) {
	for _, s := range cfg.Sections {
		switch s.Type {
		case "wifi-device":
			radio := ImportedRadio{}
			// Extract radio number from section name (radio0, radio1, ...)
			if n := strings.TrimPrefix(s.Name, "radio"); n != s.Name {
				radio.Number, _ = strconv.Atoi(n)
			}
			chanStr := s.GetOption("channel")
			if chanStr == "auto" || chanStr == "" {
				radio.AutoFrequency = true
			} else {
				radio.Channel, _ = strconv.Atoi(chanStr)
			}
			radio.Htmode = s.GetOption("htmode")
			radio.Band = normalizeBand(s.GetOption("band"))
			radios = append(radios, radio)

		case "wifi-iface":
			enc := s.GetOption("encryption")
			// Skip open/WEP — not supported by design
			if enc == "" || enc == "none" || strings.HasPrefix(enc, "wep") {
				log.Printf("[import] skipping wifi-iface %s: unsupported encryption %q", s.Name, enc)
				continue
			}
			wifi := ImportedWifi{
				SSID:               s.GetOption("ssid"),
				Key:                s.GetOption("key"),
				Encryption:         enc,
				Hidden:             s.GetOption("hidden") == "1",
				Isolate:            s.GetOption("isolate") == "1",
				Enabled:            s.GetOption("disabled") != "1",
				Ieee80211r:         s.GetOption("ieee80211r") == "1",
				Ieee80211k:         s.GetOption("ieee80211k") == "1",
				BssTransition:      s.GetOption("bss_transition") == "1",
				WnmSleepMode:       s.GetOption("wnm_sleep_mode") == "1",
				ProxyArp:           s.GetOption("proxy_arp") == "1",
				FtOverDs:           s.GetOption("ft_over_ds") == "1",
				FtPskGenerateLocal: s.GetOption("ft_psk_generate_local") != "0",
				MobilityDomain:     s.GetOption("mobility_domain"),
				Nasid:              s.GetOption("nasid"),
				RrmNeighborReport:  s.GetOption("rrm_neighbor_report") == "1",
				RrmBeaconReport:    s.GetOption("rrm_beacon_report") == "1",
				Network:            s.GetOption("network"),
			}
			wifi.ReassocDeadline, _ = strconv.Atoi(s.GetOption("reassociation_deadline"))
			if wifi.ReassocDeadline < 1000 {
				wifi.ReassocDeadline = 1000
			}
			if wifi.SSID != "" {
				wifis = append(wifis, wifi)
			}
		}
	}
	return
}

func parseDawnConfig(cfg *ucipkg.Config) *ImportedDawn {
	d := &ImportedDawn{}
	for _, s := range cfg.Sections {
		switch s.Type {
		case "global":
			d.Kicking = s.GetOption("kicking") == "1"
			d.SetHostapdNr = s.GetOption("set_hostapd_nr") == "1"
			d.RrmMode = s.GetOption("rrm_mode")
		case "metric":
			d.InitialScore, _ = strconv.Atoi(s.GetOption("initial_score"))
			d.HtSupport, _ = strconv.Atoi(s.GetOption("ht_support"))
			d.VhtSupport, _ = strconv.Atoi(s.GetOption("vht_support"))
			d.HeSupport, _ = strconv.Atoi(s.GetOption("he_support"))
			d.Rssi, _ = strconv.Atoi(s.GetOption("rssi"))
			d.RssiVal, _ = strconv.Atoi(s.GetOption("rssi_val"))
			d.LowRssi, _ = strconv.Atoi(s.GetOption("low_rssi"))
			d.LowRssiVal, _ = strconv.Atoi(s.GetOption("low_rssi_val"))
			d.Freq5, _ = strconv.Atoi(s.GetOption("freq_5"))
			d.ChanUtil, _ = strconv.Atoi(s.GetOption("chan_util"))
			d.RssiWeight, _ = strconv.Atoi(s.GetOption("rssi_weight"))
			d.RssiCenter, _ = strconv.Atoi(s.GetOption("rssi_center"))
		case "times":
			d.UpdateClient, _ = strconv.Atoi(s.GetOption("update_client"))
			d.RemoveClient, _ = strconv.Atoi(s.GetOption("remove_client"))
			d.RemoveProbe, _ = strconv.Atoi(s.GetOption("remove_probe"))
			d.UpdateHostapd, _ = strconv.Atoi(s.GetOption("update_hostapd"))
			d.UpdateTcpCon, _ = strconv.Atoi(s.GetOption("update_tcp_con"))
			d.UpdateChanUtil, _ = strconv.Atoi(s.GetOption("update_chan_util"))
			d.UpdateBeaconReports, _ = strconv.Atoi(s.GetOption("update_beacon_reports"))
		case "behaviour":
			d.KickingThreshold, _ = strconv.Atoi(s.GetOption("kicking_threshold"))
			d.MinProbeCount, _ = strconv.Atoi(s.GetOption("min_probe_count"))
			d.BandwidthThreshold, _ = strconv.Atoi(s.GetOption("bandwidth_threshold"))
			d.UseStationCount = s.GetOption("use_station_count") == "1"
			d.MaxStationDiff, _ = strconv.Atoi(s.GetOption("max_station_diff"))
			d.MinNumberToKick, _ = strconv.Atoi(s.GetOption("min_number_to_kick"))
			d.ChanUtilAvgPeriod, _ = strconv.Atoi(s.GetOption("chan_util_avg_period"))
			d.MinKickCount, _ = strconv.Atoi(s.GetOption("min_kick_count"))
		}
	}
	return d
}

// normalizeBand converts OpenWRT band values (2g, 5g, 6g) to OpenSOHO format (2.4, 5, 6).
func normalizeBand(band string) string {
	switch strings.ToLower(band) {
	case "2g":
		return "2.4"
	case "5g":
		return "5"
	case "6g":
		return "6"
	}
	return band
}
