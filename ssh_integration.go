package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
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
			Host         string `json:"host"`
			Port         int    `json:"port"`
			Password     string `json:"password"`
			ImportConfig bool   `json:"import_config"`
		}
		if err := json.NewDecoder(e.Request.Body).Decode(&req); err != nil {
			return e.BadRequestError("Invalid request body", err)
		}
		if req.Host == "" {
			return e.BadRequestError("host is required", nil)
		}

		// If import_config, read the device config before adoption changes anything
		var importedCfg *devicepkg.ImportedConfig
		if req.ImportConfig {
			var err error
			importedCfg, err = services.DeviceManager.ImportDeviceConfig(req.Host, req.Port, req.Password)
			if err != nil {
				log.Printf("[ssh] warning: config import failed for %s: %v", req.Host, err)
				// Non-fatal: continue with adoption, just won't import
				importedCfg = nil
			}
		}

		result, err := services.DeviceManager.Adopt(req.Host, req.Port, req.Password)
		if err != nil {
			return e.JSON(http.StatusUnprocessableEntity, map[string]interface{}{
				"error":  err.Error(),
				"result": result,
			})
		}

		// Register the adopted device in PocketBase and push initial config
		if result.Adopted {
			if err := registerAdoptedDevice(app, result); err != nil {
				log.Printf("[ssh] warning: device adopted but DB registration failed: %v", err)
			} else if importedCfg != nil {
				// Import config: write to DB, skip push (device already has this config)
				record, err := app.FindFirstRecordByData("devices", "mac_address", result.DeviceID)
				if err == nil {
					if err := applyImportedConfig(app, record, importedCfg); err != nil {
						log.Printf("[ssh] warning: config import to DB failed: %v", err)
					} else {
						log.Printf("[ssh] imported config from %s into DB", req.Host)
					}
				}
			} else {
				go pushConfigAfterAdoption(app, services, result.DeviceID)
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

// applyImportedConfig writes an ImportedConfig into the PocketBase DB,
// linked to the given device record.
func applyImportedConfig(app core.App, record *core.Record, cfg *devicepkg.ImportedConfig) error {
	// --- hostname & numradios ---
	if cfg.Hostname != "" {
		record.Set("name", cfg.Hostname)
	}
	if len(cfg.Radios) > 0 {
		record.Set("numradios", len(cfg.Radios))
	}
	if err := app.Save(record); err != nil {
		return fmt.Errorf("applyImportedConfig: save device: %w", err)
	}

	// --- radios ---
	radioColl, err := app.FindCollectionByNameOrId("radios")
	if err != nil {
		return fmt.Errorf("applyImportedConfig: radios collection: %w", err)
	}
	for _, r := range cfg.Radios {
		existing, err := app.FindFirstRecordByFilter(
			radioColl,
			"device = {:device} && radio = {:radio}",
			map[string]any{"device": record.Id, "radio": r.Number},
		)
		if err != nil {
			existing = core.NewRecord(radioColl)
			existing.Set("device", record.Id)
			existing.Set("radio", r.Number)
		}
		existing.Set("band", r.Band)
		existing.Set("channel", r.Channel)
		existing.Set("htmode", r.Htmode)
		existing.Set("auto_frequency", r.AutoFrequency)
		existing.Set("enabled", true)
		if err := app.Save(existing); err != nil {
			log.Printf("[import] failed to save radio %d: %v", r.Number, err)
		}
	}

	// --- wifis ---
	wifiColl, err := app.FindCollectionByNameOrId("wifi")
	if err != nil {
		return fmt.Errorf("applyImportedConfig: wifi collection: %w", err)
	}
	wifiIDs := []string{}
	for _, w := range cfg.Wifis {
		// Deduplicate by SSID (unique index)
		wifiRec, err := app.FindFirstRecordByData(wifiColl, "ssid", w.SSID)
		if err != nil {
			wifiRec = core.NewRecord(wifiColl)
		}
		wifiRec.Set("ssid", w.SSID)
		wifiRec.Set("key", w.Key)
		wifiRec.Set("encryption", w.Encryption)
		wifiRec.Set("hidden", w.Hidden)
		wifiRec.Set("isolate_clients", w.Isolate)
		wifiRec.Set("enabled", w.Enabled)
		wifiRec.Set("ieee80211r", w.Ieee80211r)
		wifiRec.Set("ieee80211k", w.Ieee80211k)
		wifiRec.Set("ieee80211v_bss_transition", w.BssTransition)
		wifiRec.Set("ieee80211v_wnm_sleep_mode", w.WnmSleepMode)
		wifiRec.Set("ieee80211v_proxy_arp", w.ProxyArp)
		wifiRec.Set("ft_over_ds", w.FtOverDs)
		wifiRec.Set("ft_psk_generate_local", w.FtPskGenerateLocal)
		wifiRec.Set("mobility_domain", w.MobilityDomain)
		wifiRec.Set("ieee80211r_reassoc_deadline", w.ReassocDeadline)
		wifiRec.Set("nasid", w.Nasid)
		wifiRec.Set("rrm_neighbor_report", w.RrmNeighborReport)
		wifiRec.Set("rrm_beacon_report", w.RrmBeaconReport)
		if err := app.Save(wifiRec); err != nil {
			log.Printf("[import] failed to save wifi %s: %v", w.SSID, err)
			continue
		}
		wifiIDs = append(wifiIDs, wifiRec.Id)
	}
	if len(wifiIDs) > 0 {
		record.Set("wifis", wifiIDs)
		if err := app.Save(record); err != nil {
			log.Printf("[import] failed to link wifis to device: %v", err)
		}
	}

	// --- leds ---
	ledColl, err := app.FindCollectionByNameOrId("leds")
	if err == nil {
		ledIDs := record.GetStringSlice("leds")
		for _, l := range cfg.Leds {
			// Deduplicate by sysfs path
			ledRec, err := app.FindFirstRecordByData(ledColl, "led_name", l.Sysfs)
			if err != nil {
				ledRec = core.NewRecord(ledColl)
			}
			ledRec.Set("name", l.Name)
			ledRec.Set("led_name", l.Sysfs)
			ledRec.Set("trigger", l.Trigger)
			ledRec.Set("dev", l.Dev)
			ledRec.Set("mode", l.Mode)
			if l.DelayOn > 0 {
				ledRec.Set("delayon", l.DelayOn)
			}
			if l.DelayOff > 0 {
				ledRec.Set("delayoff", l.DelayOff)
			}
			if err := app.Save(ledRec); err != nil {
				log.Printf("[import] failed to save led %s: %v", l.Sysfs, err)
				continue
			}
			ledIDs = append(ledIDs, ledRec.Id)
		}
		if len(cfg.Leds) > 0 {
			record.Set("leds", ledIDs)
			app.Save(record)
		}
	}

	// --- dawn (singleton) ---
	if cfg.Dawn != nil {
		dawnColl, err := app.FindCollectionByNameOrId("dawn")
		if err == nil {
			dawnRec, err := app.FindFirstRecordByFilter(dawnColl, "id != ''", nil)
			if err != nil {
				dawnRec = core.NewRecord(dawnColl)
			}
			d := cfg.Dawn
			dawnRec.Set("kicking", d.Kicking)
			dawnRec.Set("set_hostapd_nr", d.SetHostapdNr)
			dawnRec.Set("rrm_mode", d.RrmMode)
			dawnRec.Set("initial_score", d.InitialScore)
			dawnRec.Set("ht_support", d.HtSupport)
			dawnRec.Set("vht_support", d.VhtSupport)
			dawnRec.Set("he_support", d.HeSupport)
			dawnRec.Set("rssi", d.Rssi)
			dawnRec.Set("rssi_val", d.RssiVal)
			dawnRec.Set("low_rssi", d.LowRssi)
			dawnRec.Set("low_rssi_val", d.LowRssiVal)
			dawnRec.Set("freq_5", d.Freq5)
			dawnRec.Set("chan_util", d.ChanUtil)
			dawnRec.Set("rssi_weight", d.RssiWeight)
			dawnRec.Set("rssi_center", d.RssiCenter)
			dawnRec.Set("update_client", d.UpdateClient)
			dawnRec.Set("remove_client", d.RemoveClient)
			dawnRec.Set("remove_probe", d.RemoveProbe)
			dawnRec.Set("update_hostapd", d.UpdateHostapd)
			dawnRec.Set("update_tcp_con", d.UpdateTcpCon)
			dawnRec.Set("update_chan_util", d.UpdateChanUtil)
			dawnRec.Set("update_beacon_reports", d.UpdateBeaconReports)
			dawnRec.Set("kicking_threshold", d.KickingThreshold)
			dawnRec.Set("min_probe_count", d.MinProbeCount)
			dawnRec.Set("bandwidth_threshold", d.BandwidthThreshold)
			dawnRec.Set("use_station_count", d.UseStationCount)
			dawnRec.Set("max_station_diff", d.MaxStationDiff)
			dawnRec.Set("min_number_to_kick", d.MinNumberToKick)
			dawnRec.Set("chan_util_avg_period", d.ChanUtilAvgPeriod)
			dawnRec.Set("min_kick_count", d.MinKickCount)
			if err := app.Save(dawnRec); err != nil {
				log.Printf("[import] failed to save dawn config: %v", err)
			}
		}
	}

	// --- profile assignment from detected role ---
	if cfg.DetectedRole != "" {
		profile, err := app.FindFirstRecordByData("device_profile", "name", cfg.DetectedRole)
		if err == nil {
			record.Set("profile", profile.Id)
			if err := app.Save(record); err != nil {
				log.Printf("[import] failed to assign profile %s to device: %v", cfg.DetectedRole, err)
			} else {
				log.Printf("[import] assigned profile %s to device %s", cfg.DetectedRole, record.Id)
			}
		} else {
			log.Printf("[import] profile %q not found, skipping assignment: %v", cfg.DetectedRole, err)
		}
	}

	return nil
}

func pushConfigAfterAdoption(app core.App, services *SSHServices, deviceID string) {
	record, err := app.FindFirstRecordByData("devices", "mac_address", deviceID)
	if err != nil {
		log.Printf("[ssh-adopt] device %s not found in DB for config push: %v", deviceID, err)
		return
	}

	configFiles, err := GenerateDeviceConfigFiles(app, record)
	if err != nil {
		log.Printf("[ssh-adopt] config generation failed for %s: %v", deviceID, err)
		return
	}

	// SSH-managed devices don't use the openwisp-config agent for config delivery
	delete(configFiles, "etc/config/openwisp")
	delete(configFiles, "etc/config/openwisp-monitoring")

	// Disable services required by the device profile before pushing config
	if profileID := record.GetString("profile"); profileID != "" {
		if profile, err := app.FindRecordById("device_profile", profileID); err == nil {
			var toDisable []string
			if profile.GetBool("disable_firewall") {
				toDisable = append(toDisable, "firewall")
			}
			if profile.GetBool("disable_dnsmasq") {
				toDisable = append(toDisable, "dnsmasq")
			}
			if profile.GetBool("disable_odhcpd") {
				toDisable = append(toDisable, "odhcpd")
			}
			if len(toDisable) > 0 {
				if err := services.ConfigApplier.DisableServices(deviceID, toDisable); err != nil {
					log.Printf("[ssh-adopt] DisableServices failed for %s: %v", deviceID, err)
				}
			}
		}
	}

	if err := services.ConfigApplier.FullApply(deviceID, configFiles); err != nil {
		log.Printf("[ssh-adopt] config push failed for %s: %v", deviceID, err)
		record.Set("config_status", "error")
		app.Save(record)
		return
	}

	record.Set("config_status", "applied")
	if err := app.Save(record); err != nil {
		log.Printf("[ssh-adopt] failed to update config_status for %s: %v", deviceID, err)
	}
	log.Printf("[ssh-adopt] config pushed successfully to %s", deviceID)
}

func registerAdoptedDevice(app core.App, result *devicepkg.AdoptionResult) error {
	collection, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		return fmt.Errorf("devices collection not found: %w", err)
	}

	// Check if device already exists by MAC
	existing, err := app.FindFirstRecordByData("devices", "mac_address", result.MAC)
	if err == nil {
		existing.Set("ip_address", result.Host)
		existing.Set("health_status", "healthy")
		existing.Set("model", result.Model)
		return app.Save(existing)
	}

	// Generate key (32 hex chars) and derive PocketBase ID from it
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		return fmt.Errorf("failed to generate key: %w", err)
	}
	key := hex.EncodeToString(keyBytes)
	pbID, err := hexToPocketBaseID(key)
	if err != nil {
		return fmt.Errorf("failed to derive pb id: %w", err)
	}

	record := core.NewRecord(collection)
	record.Set("id", pbID)
	record.Set("key", key)
	record.Set("uuid", uuid.New().String())
	record.Set("name", result.Hostname)
	record.Set("mac_address", result.MAC)
	record.Set("ip_address", result.Host)
	record.Set("model", result.Model)
	record.Set("health_status", "healthy")
	record.Set("config_status", "applied")
	record.Set("enabled", false)
	record.Set("numradios", 0)
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
