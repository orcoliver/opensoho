package monitoring

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	sshpkg "github.com/rubenbe/opensoho/ssh"
)

// MonitoringData holds all collected monitoring data for a single device.
type MonitoringData struct {
	Clients    []Client         `json:"clients"`
	Leases     []DHCPLease      `json:"leases"`
	Board      *BoardInfo       `json:"board"`
	Interfaces []InterfaceStats `json:"interfaces"`
	Load       *LoadAverage     `json:"load"`
	Timestamp  time.Time        `json:"timestamp"`
}

// Poller periodically collects monitoring data from all connected devices via SSH.
type Poller struct {
	sshManager *sshpkg.Manager
	interval   time.Duration
	stop       chan struct{}
	wg         sync.WaitGroup
	onData     func(deviceID string, data *MonitoringData) // callback for new data
}

// NewPoller creates a new monitoring poller.
func NewPoller(sshManager *sshpkg.Manager, interval time.Duration, onData func(string, *MonitoringData)) *Poller {
	if interval == 0 {
		interval = 15 * time.Second
	}
	return &Poller{
		sshManager: sshManager,
		interval:   interval,
		stop:       make(chan struct{}),
		onData:     onData,
	}
}

// Start begins the polling loop in a background goroutine.
func (p *Poller) Start(deviceIDs func() []string) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(p.interval)
		defer ticker.Stop()

		for {
			select {
			case <-p.stop:
				return
			case <-ticker.C:
				ids := deviceIDs()
				for _, id := range ids {
					if !p.sshManager.IsConnected(id) {
						continue
					}
					data, err := p.PollDevice(id)
					if err != nil {
						log.Printf("[monitoring] poll %s failed: %v", id, err)
						continue
					}
					if p.onData != nil {
						p.onData(id, data)
					}
				}
			}
		}
	}()
}

// Stop gracefully stops the polling loop.
func (p *Poller) Stop() {
	close(p.stop)
	p.wg.Wait()
}

// monitoringScript is the batch of commands executed in a single SSH session.
// Each command's output is separated by a unique delimiter for parsing.
const delimiter = "---OPENSOHO_DELIM---"

func monitoringScript(interfaces []string) string {
	cmds := []string{}

	// Hostapd clients for each wireless interface
	for _, iface := range interfaces {
		cmds = append(cmds, fmt.Sprintf("ubus call hostapd.%s get_clients 2>/dev/null || echo '{}'", iface))
		cmds = append(cmds, fmt.Sprintf("echo '%s'", delimiter))
	}

	// DHCP leases
	cmds = append(cmds, "cat /tmp/dhcp.leases 2>/dev/null || echo ''")
	cmds = append(cmds, fmt.Sprintf("echo '%s'", delimiter))

	// Board info
	cmds = append(cmds, "ubus call system board 2>/dev/null || echo '{}'")
	cmds = append(cmds, fmt.Sprintf("echo '%s'", delimiter))

	// Load average
	cmds = append(cmds, "cat /proc/loadavg 2>/dev/null || echo '0 0 0'")
	cmds = append(cmds, fmt.Sprintf("echo '%s'", delimiter))

	// Interface stats
	cmds = append(cmds, "ip -j link show 2>/dev/null || echo '[]'")

	return strings.Join(cmds, "\n")
}

// PollDevice collects all monitoring data from a single device in one SSH session.
func (p *Poller) PollDevice(deviceID string) (*MonitoringData, error) {
	data := &MonitoringData{
		Timestamp: time.Now(),
	}

	// First, discover wireless interfaces
	ifaceOutput, err := p.sshManager.Execute(deviceID,
		"ls /var/run/hostapd/ 2>/dev/null | sed 's/\\..*//g' | sort -u || echo ''")
	if err != nil {
		return nil, fmt.Errorf("failed to discover wireless interfaces: %w", err)
	}

	wifiInterfaces := []string{}
	for _, line := range strings.Split(strings.TrimSpace(ifaceOutput), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			wifiInterfaces = append(wifiInterfaces, line)
		}
	}

	// Execute the monitoring script
	script := monitoringScript(wifiInterfaces)
	output, err := p.sshManager.Execute(deviceID, script)
	if err != nil {
		return nil, fmt.Errorf("monitoring script failed: %w", err)
	}

	// Split output by delimiter
	parts := strings.Split(output, delimiter)

	idx := 0

	// Parse hostapd clients for each interface
	for _, iface := range wifiInterfaces {
		if idx >= len(parts) {
			break
		}
		clients, err := ParseHostapdClients(strings.TrimSpace(parts[idx]), iface)
		if err != nil {
			log.Printf("[monitoring] parse clients for %s failed: %v", iface, err)
		} else {
			data.Clients = append(data.Clients, clients...)
		}
		idx++
	}

	// Parse DHCP leases
	if idx < len(parts) {
		leases, err := ParseDHCPLeases(strings.TrimSpace(parts[idx]))
		if err != nil {
			log.Printf("[monitoring] parse DHCP leases failed: %v", err)
		} else {
			data.Leases = leases
		}
		idx++
	}

	// Parse board info
	if idx < len(parts) {
		board, err := ParseBoardInfo(strings.TrimSpace(parts[idx]))
		if err != nil {
			log.Printf("[monitoring] parse board info failed: %v", err)
		} else {
			data.Board = board
		}
		idx++
	}

	// Parse load average
	if idx < len(parts) {
		load, err := ParseLoadAverage(strings.TrimSpace(parts[idx]))
		if err != nil {
			log.Printf("[monitoring] parse load avg failed: %v", err)
		} else {
			data.Load = load
		}
		idx++
	}

	// Parse interface stats
	if idx < len(parts) {
		interfaces, err := ParseLinkShow(strings.TrimSpace(parts[idx]))
		if err != nil {
			log.Printf("[monitoring] parse link show failed: %v", err)
		} else {
			data.Interfaces = interfaces
		}
	}

	return data, nil
}
