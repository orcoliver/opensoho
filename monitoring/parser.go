package monitoring

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Client represents a WiFi client connected to an AP.
type Client struct {
	MAC       string `json:"mac"`
	Auth      bool   `json:"auth"`
	Assoc     bool   `json:"assoc"`
	Signal    int    `json:"signal"`
	RxRate    int    `json:"rx_rate"`
	TxRate    int    `json:"tx_rate"`
	RxBytes   int64  `json:"rx_bytes"`
	TxBytes   int64  `json:"tx_bytes"`
	Connected int    `json:"connected_time"` // seconds
	Interface string `json:"interface"`      // populated by poller
}

// DHCPLease represents a DHCP lease entry from /tmp/dhcp.leases.
type DHCPLease struct {
	Expiry   int64  `json:"expiry"`
	MAC      string `json:"mac"`
	IP       string `json:"ip"`
	Hostname string `json:"hostname"`
	ClientID string `json:"client_id"`
}

// BoardInfo contains system board information from ubus.
type BoardInfo struct {
	Model    string `json:"model"`
	Hostname string `json:"hostname"`
	System   string `json:"system"`
	Release  struct {
		Distribution string `json:"distribution"`
		Version      string `json:"version"`
		Revision     string `json:"revision"`
		Target       string `json:"target"`
		Description  string `json:"description"`
	} `json:"release"`
}

// InterfaceStats represents network interface statistics.
type InterfaceStats struct {
	Name      string `json:"name"`
	State     string `json:"state"`       // "UP", "DOWN"
	Speed     string `json:"speed"`       // e.g., "1000F"
	TxBytes   int64  `json:"tx_bytes"`
	RxBytes   int64  `json:"rx_bytes"`
	TxPackets int64  `json:"tx_packets"`
	RxPackets int64  `json:"rx_packets"`
	MTU       int    `json:"mtu"`
}

// LoadAverage represents system load average.
type LoadAverage struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

// ParseHostapdClients parses the JSON output of `ubus call hostapd.wlanX get_clients`.
//
// Expected format:
//
//	{
//	  "freq": 5180,
//	  "clients": {
//	    "AA:BB:CC:DD:EE:FF": {
//	      "auth": true,
//	      "assoc": true,
//	      "signal": -54,
//	      "rx": {"rate": 866700},
//	      "tx": {"rate": 433300},
//	      "bytes": {"rx": 123456, "tx": 789012},
//	      "connected_time": 3600
//	    }
//	  }
//	}
func ParseHostapdClients(jsonStr string, ifaceName string) ([]Client, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return nil, nil
	}

	var result struct {
		Freq    int `json:"freq"`
		Clients map[string]struct {
			Auth      bool `json:"auth"`
			Assoc     bool `json:"assoc"`
			Signal    int  `json:"signal"`
			Rx        struct {
				Rate int `json:"rate"`
			} `json:"rx"`
			Tx struct {
				Rate int `json:"rate"`
			} `json:"tx"`
			Bytes struct {
				Rx int64 `json:"rx"`
				Tx int64 `json:"tx"`
			} `json:"bytes"`
			ConnectedTime int `json:"connected_time"`
		} `json:"clients"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("failed to parse hostapd clients: %w", err)
	}

	clients := make([]Client, 0, len(result.Clients))
	for mac, c := range result.Clients {
		clients = append(clients, Client{
			MAC:       strings.ToUpper(mac),
			Auth:      c.Auth,
			Assoc:     c.Assoc,
			Signal:    c.Signal,
			RxRate:    c.Rx.Rate,
			TxRate:    c.Tx.Rate,
			RxBytes:   c.Bytes.Rx,
			TxBytes:   c.Bytes.Tx,
			Connected: c.ConnectedTime,
			Interface: ifaceName,
		})
	}

	return clients, nil
}

// ParseDHCPLeases parses the content of /tmp/dhcp.leases.
// Format: <expiry_timestamp> <mac> <ip> <hostname> <client_id>
// Example: 1711209600 aa:bb:cc:dd:ee:ff 192.168.1.100 my-laptop *
func ParseDHCPLeases(content string) ([]DHCPLease, error) {
	leases := []DHCPLease{}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 4 {
			continue // Skip malformed lines
		}

		expiry, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			continue // Skip lines with invalid timestamp
		}

		lease := DHCPLease{
			Expiry:   expiry,
			MAC:      strings.ToUpper(parts[1]),
			IP:       parts[2],
			Hostname: parts[3],
		}
		if len(parts) >= 5 {
			lease.ClientID = parts[4]
		}

		leases = append(leases, lease)
	}

	return leases, nil
}

// ParseBoardInfo parses the JSON output of `ubus call system board`.
//
// Expected format:
//
//	{
//	  "kernel": "5.15.134",
//	  "hostname": "OpenWrt",
//	  "system": "MediaTek MT7981B ver:1 eco:2",
//	  "model": "TP-Link Deco X50-PoE v2",
//	  "board_name": "tplink,deco-x50-poe-v2",
//	  "release": {
//	    "distribution": "OpenWrt",
//	    "version": "23.05.3",
//	    "revision": "r24076-4b684d9",
//	    "target": "mediatek/filogic",
//	    "description": "OpenWrt 23.05.3 r24076-4b684d9"
//	  }
//	}
func ParseBoardInfo(jsonStr string) (*BoardInfo, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return nil, fmt.Errorf("empty board info")
	}

	var info BoardInfo
	if err := json.Unmarshal([]byte(jsonStr), &info); err != nil {
		return nil, fmt.Errorf("failed to parse board info: %w", err)
	}

	return &info, nil
}

// ParseLinkShow parses the JSON output of `ip -j link show`.
// Returns stats for physical interfaces (excludes lo and wireless).
func ParseLinkShow(jsonStr string) ([]InterfaceStats, error) {
	if strings.TrimSpace(jsonStr) == "" {
		return nil, nil
	}

	var rawLinks []struct {
		IfName   string `json:"ifname"`
		OperState string `json:"operstate"`
		MTU      int    `json:"mtu"`
		Stats64  struct {
			Rx struct {
				Bytes   int64 `json:"bytes"`
				Packets int64 `json:"packets"`
			} `json:"rx"`
			Tx struct {
				Bytes   int64 `json:"bytes"`
				Packets int64 `json:"packets"`
			} `json:"tx"`
		} `json:"stats64"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &rawLinks); err != nil {
		return nil, fmt.Errorf("failed to parse ip link show: %w", err)
	}

	interfaces := []InterfaceStats{}
	for _, link := range rawLinks {
		// Skip loopback and wireless interfaces
		if link.IfName == "lo" || strings.HasPrefix(link.IfName, "wlan") ||
			strings.HasPrefix(link.IfName, "phy") {
			continue
		}

		state := "DOWN"
		if strings.ToUpper(link.OperState) == "UP" {
			state = "UP"
		}

		interfaces = append(interfaces, InterfaceStats{
			Name:      link.IfName,
			State:     state,
			TxBytes:   link.Stats64.Tx.Bytes,
			RxBytes:   link.Stats64.Rx.Bytes,
			TxPackets: link.Stats64.Tx.Packets,
			RxPackets: link.Stats64.Rx.Packets,
			MTU:       link.MTU,
		})
	}

	return interfaces, nil
}

// ParseLoadAverage parses the content of /proc/loadavg.
// Format: 0.12 0.05 0.01 1/89 12345
func ParseLoadAverage(content string) (*LoadAverage, error) {
	parts := strings.Fields(strings.TrimSpace(content))
	if len(parts) < 3 {
		return nil, fmt.Errorf("invalid loadavg format: %s", content)
	}

	load1, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse load1: %w", err)
	}
	load5, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse load5: %w", err)
	}
	load15, err := strconv.ParseFloat(parts[2], 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse load15: %w", err)
	}

	return &LoadAverage{
		Load1:  load1,
		Load5:  load5,
		Load15: load15,
	}, nil
}

// ParseMACAddress parses a MAC address from cat /sys/class/net/br-lan/address.
func ParseMACAddress(content string) string {
	return strings.ToUpper(strings.TrimSpace(content))
}

// ParseOpenWrtRelease checks if the output of "cat /etc/openwrt_release"
// confirms this is an OpenWrt system.
func ParseOpenWrtRelease(content string) (isOpenWrt bool, version string) {
	if !strings.Contains(content, "DISTRIB_ID") {
		return false, ""
	}

	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "DISTRIB_RELEASE=") {
			version = strings.Trim(strings.TrimPrefix(line, "DISTRIB_RELEASE="), "'\"")
		}
	}

	return true, version
}
