package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	configpkg "github.com/rubenbe/opensoho/config"
	devicepkg "github.com/rubenbe/opensoho/device"
	monitoringpkg "github.com/rubenbe/opensoho/monitoring"
	sshpkg "github.com/rubenbe/opensoho/ssh"
	"github.com/rubenbe/pocketbase/apis"
	"github.com/rubenbe/pocketbase/core"
)

// SSHServices holds all SSH-based service instances.
type SSHServices struct {
	SSHManager    *sshpkg.Manager
	ConfigApplier *configpkg.Applier
	DeviceManager *devicepkg.Manager
	Poller        *monitoringpkg.Poller
}

// InitSSHServices initializes all SSH-based services.
// Called once at application startup.
func InitSSHServices(app core.App) (*SSHServices, error) {
	// Determine SSH key path
	dataDir := app.DataDir()
	sshKeyPath := filepath.Join(dataDir, "ssh_server_key")

	sshMgr, err := sshpkg.NewManager(sshKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to init SSH manager: %w", err)
	}

	log.Printf("[ssh] initialized — public key: %s", truncateKey(sshMgr.PublicKey()))

	applier := configpkg.NewApplier(sshMgr)
	devMgr := devicepkg.NewManager(sshMgr)

	services := &SSHServices{
		SSHManager:    sshMgr,
		ConfigApplier: applier,
		DeviceManager: devMgr,
	}

	return services, nil
}

// BindSSHRoutes registers all SSH-related API endpoints.
func BindSSHRoutes(se *core.ServeEvent, app core.App, services *SSHServices) {
	// --- Device Adoption ---
	se.Router.POST("/api/ssh/adopt", func(e *core.RequestEvent) error {
		var req struct {
			Host     string `json:"host"`
			Port     int    `json:"port"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
			return e.BadRequestError("Invalid request body", err)
		}
		if req.Host == "" {
			return e.BadRequestError("host is required", nil)
		}

		result, err := services.DeviceManager.Adopt(req.Host, req.Port, req.Password)
		if err != nil {
			return e.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
				"error":  err.Error(),
				"result": result,
			})
		}

		// Register the adopted device in PocketBase
		if result.Adopted {
			if err := registerAdoptedDevice(app, result); err != nil {
				log.Printf("[ssh] warning: device adopted but DB registration failed: %v", err)
			}
		}

		return e.JSON(http.StatusOK, result)
	}).Bind(apis.RequireAuth())

	// --- Network Discovery ---
	se.Router.GET("/api/ssh/discover", func(e *core.RequestEvent) error {
		cidr := e.Request.URL.Query().Get("cidr")
		if cidr == "" {
			// Auto-detect: scan common SOHO subnets
			cidr = "192.168.1.0/24"
		}

		timeoutStr := e.Request.URL.Query().Get("timeout")
		timeout := 2 * time.Second
		if timeoutStr != "" {
			if d, err := time.ParseDuration(timeoutStr); err == nil {
				timeout = d
			}
		}

		devices, err := services.DeviceManager.Discover(cidr, timeout)
		if err != nil {
			return e.InternalServerError("Discovery failed", err)
		}

		return e.JSON(http.StatusOK, map[string]interface{}{
			"cidr":    cidr,
			"count":   len(devices),
			"devices": devices,
		})
	}).Bind(apis.RequireAuth())

	// --- Push Config to Device ---
	se.Router.POST("/api/ssh/push-config/{deviceId}", func(e *core.RequestEvent) error {
		deviceId := e.Request.PathValue("deviceId")
		if deviceId == "" {
			return e.BadRequestError("deviceId is required", nil)
		}

		// Find the device record
		record, err := app.FindRecordById("devices", deviceId)
		if err != nil {
			return e.NotFoundError("Device not found", err)
		}

		// Generate config files
		configFiles, err := GenerateDeviceConfigFiles(app, record)
		if err != nil {
			return e.InternalServerError("Config generation failed", err)
		}

		// Determine SSH device ID (MAC address)
		sshDeviceID := record.GetString("mac_address")
		if sshDeviceID == "" {
			return e.BadRequestError("Device has no MAC address", nil)
		}

		// Check connection
		if !services.SSHManager.IsConnected(sshDeviceID) {
			return e.JSON(http.StatusServiceUnavailable, map[string]string{
				"error": "Device is not connected via SSH. Adopt it first.",
			})
		}

		// Apply config
		err = services.ConfigApplier.FullApply(sshDeviceID, configFiles)
		if err != nil {
			record.Set("config_status", "error")
			app.Save(record)
			return e.InternalServerError("Config push failed", err)
		}

		// Update status
		record.Set("config_status", "applied")
		record.Set("health_status", "healthy")
		if err := app.Save(record); err != nil {
			log.Printf("[ssh] warning: config applied but status update failed: %v", err)
		}

		return e.JSON(http.StatusOK, map[string]string{
			"status":  "applied",
			"device":  record.GetString("name"),
			"message": "Configuration pushed successfully via SSH",
		})
	}).Bind(apis.RequireAuth())

	// --- Device Status ---
	se.Router.GET("/api/ssh/device/{deviceId}/status", func(e *core.RequestEvent) error {
		deviceId := e.Request.PathValue("deviceId")

		record, err := app.FindRecordById("devices", deviceId)
		if err != nil {
			return e.NotFoundError("Device not found", err)
		}

		sshDeviceID := record.GetString("mac_address")
		ok, status := services.DeviceManager.HealthCheck(sshDeviceID)

		return e.JSON(http.StatusOK, map[string]interface{}{
			"device":    record.GetString("name"),
			"connected": ok,
			"status":    status,
		})
	}).Bind(apis.RequireAuth())

	// --- Reboot Device ---
	se.Router.POST("/api/ssh/device/{deviceId}/reboot", func(e *core.RequestEvent) error {
		deviceId := e.Request.PathValue("deviceId")

		record, err := app.FindRecordById("devices", deviceId)
		if err != nil {
			return e.NotFoundError("Device not found", err)
		}

		sshDeviceID := record.GetString("mac_address")
		err = services.DeviceManager.RebootDevice(sshDeviceID)
		if err != nil {
			return e.InternalServerError("Reboot failed", err)
		}

		return e.JSON(http.StatusOK, map[string]string{
			"status":  "rebooting",
			"device":  record.GetString("name"),
		})
	}).Bind(apis.RequireAuth())

	// --- SSH Public Key (for manual setup) ---
	se.Router.GET("/api/ssh/public-key", func(e *core.RequestEvent) error {
		return e.JSON(http.StatusOK, map[string]string{
			"public_key": strings.TrimSpace(services.SSHManager.PublicKey()),
		})
	}).Bind(apis.RequireAuth())
}

// StartSSHCronJobs sets up periodic tasks for SSH-based device management.
func StartSSHCronJobs(app core.App, services *SSHServices) {
	// Monitoring poller — collect data from all connected devices every 15s
	services.Poller = monitoringpkg.NewPoller(services.SSHManager, 15*time.Second, func(deviceID string, data *monitoringpkg.MonitoringData) {
		handleSSHMonitoringData(app, deviceID, data)
	})

	services.Poller.Start(func() []string {
		return getConnectedDeviceIDs(app, services.SSHManager)
	})
	log.Println("[ssh] monitoring poller started (15s interval)")

	// Health check cron — every minute, check connected devices
	app.Cron().MustAdd("sshHealthCheck", "* * * * *", func() {
		runSSHHealthChecks(app, services)
	})

	// Auto-reconnect cron — every 5 minutes, try to reconnect dropped devices
	app.Cron().MustAdd("sshReconnect", "*/5 * * * *", func() {
		runSSHReconnect(app, services)
	})
}

// StopSSHServices gracefully shuts down SSH services.
func StopSSHServices(services *SSHServices) {
	if services == nil {
		return
	}
	if services.Poller != nil {
		services.Poller.Stop()
	}
	services.SSHManager.DisconnectAll()
	log.Println("[ssh] all services stopped")
}

// --- internal helpers ---

func handleSSHMonitoringData(app core.App, deviceID string, data *monitoringpkg.MonitoringData) {
	// Find device record by MAC address
	record, err := app.FindFirstRecordByData("devices", "mac_address", deviceID)
	if err != nil {
		log.Printf("[ssh-monitor] device %s not found in DB: %v", deviceID, err)
		return
	}

	// Update health status
	record.Set("health_status", "healthy")

	// Update client counts
	if len(data.Clients) > 0 {
		record.Set("num_clients", len(data.Clients))
	}

	// Update load average
	if data.Load != nil {
		record.Set("load_avg", fmt.Sprintf("%.2f", data.Load.Load1))
	}

	if err := app.Save(record); err != nil {
		log.Printf("[ssh-monitor] failed to save device %s: %v", deviceID, err)
	}

	// Update per-client records
	if len(data.Clients) > 0 {
		updateSSHClientRecords(app, record, data.Clients)
	}
}

func updateSSHClientRecords(app core.App, device *core.Record, clients []monitoringpkg.Client) {
	collection, err := app.FindCollectionByNameOrId("clients")
	if err != nil {
		return
	}

	for _, client := range clients {
		record, err := app.FindFirstRecordByFilter(collection,
			"device = {:device} && mac_address = {:mac}",
			map[string]interface{}{"device": device.Id, "mac": client.MAC},
		)
		if err != nil {
			record = core.NewRecord(collection)
			record.Set("device", device.Id)
			record.Set("mac_address", client.MAC)
		}

		record.Set("signal", client.Signal)
		record.Set("tx_rate", client.TxRate)
		record.Set("rx_rate", client.RxRate)
		record.Set("tx_bytes", client.TxBytes)
		record.Set("rx_bytes", client.RxBytes)
		record.Set("interface", client.Interface)

		if err := app.Save(record); err != nil {
			log.Printf("[ssh-monitor] failed to save client %s: %v", client.MAC, err)
		}
	}
}

func getConnectedDeviceIDs(app core.App, sshMgr *sshpkg.Manager) []string {
	// Get all device MAC addresses from DB
	records, err := app.FindAllRecords("devices")
	if err != nil {
		return nil
	}

	ids := make([]string, 0)
	for _, r := range records {
		mac := r.GetString("mac_address")
		if mac != "" && sshMgr.IsConnected(mac) {
			ids = append(ids, mac)
		}
	}
	return ids
}

func runSSHHealthChecks(app core.App, services *SSHServices) {
	records, err := app.FindAllRecords("devices")
	if err != nil {
		return
	}

	for _, record := range records {
		mac := record.GetString("mac_address")
		if mac == "" {
			continue
		}

		if services.SSHManager.IsConnected(mac) {
			ok, _ := services.DeviceManager.HealthCheck(mac)
			if ok {
				if record.GetString("health_status") != "healthy" {
					record.Set("health_status", "healthy")
					app.Save(record)
				}
			} else {
				record.Set("health_status", "unhealthy")
				app.Save(record)
			}
		}
	}
}

func runSSHReconnect(app core.App, services *SSHServices) {
	records, err := app.FindAllRecords("devices")
	if err != nil {
		return
	}

	for _, record := range records {
		mac := record.GetString("mac_address")
		if mac == "" || services.SSHManager.IsConnected(mac) {
			continue
		}

		// Try to reconnect adopted devices
		host := record.GetString("ip_address")
		if host == "" {
			continue
		}

		log.Printf("[ssh-reconnect] attempting reconnect to %s (%s)", record.GetString("name"), host)
		err := services.SSHManager.Connect(sshpkg.DeviceConn{
			ID:   mac,
			Host: host,
			Port: 22,
			User: "root",
		})
		if err != nil {
			log.Printf("[ssh-reconnect] failed: %v", err)
		} else {
			log.Printf("[ssh-reconnect] reconnected to %s", record.GetString("name"))
			record.Set("health_status", "healthy")
			app.Save(record)
		}
	}
}

func registerAdoptedDevice(app core.App, result *devicepkg.AdoptionResult) error {
	collection, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		return fmt.Errorf("devices collection not found: %w", err)
	}

	// Check if device already exists by MAC
	existing, err := app.FindFirstRecordByData("devices", "mac_address", result.MAC)
	if err == nil {
		// Update existing record
		existing.Set("ip_address", result.Host)
		existing.Set("health_status", "healthy")
		existing.Set("model", result.Model)
		return app.Save(existing)
	}

	// Create new device record
	record := core.NewRecord(collection)
	record.Set("name", result.Hostname)
	record.Set("mac_address", result.MAC)
	record.Set("ip_address", result.Host)
	record.Set("model", result.Model)
	record.Set("health_status", "healthy")
	record.Set("enabled", true)
	record.Set("numradios", 0) // Will be updated after first monitoring poll
	record.Set("leds", []string{})
	record.Set("wifis", []string{})

	return app.Save(record)
}

// truncateKey shows the first 20 chars of a public key for logging.
func truncateKey(key string) string {
	key = strings.TrimSpace(key)
	if len(key) > 40 {
		return key[:40] + "..."
	}
	return key
}

// isSSHEnabled checks if SSH mode is enabled via environment variable.
// Falls back to true if not set (SSH mode is the default for the fork).
func isSSHEnabled() bool {
	v := os.Getenv("OPENSOHO_SSH_ENABLED")
	if v == "" || v == "1" || strings.ToLower(v) == "true" {
		return true
	}
	return false
}
