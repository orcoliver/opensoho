// Package config bridges OpenSoHo's existing config generators with SSH-based
// config push. It takes the UCI file strings produced by generateDeviceConfig()
// and applies them to remote devices via the SSH manager.
package config

import (
	"fmt"
	"log"
	"strings"

	sshpkg "github.com/rubenbe/opensoho/ssh"
	ucipkg "github.com/rubenbe/opensoho/uci"
)

// Applier handles applying UCI configuration changes to remote devices via SSH.
type Applier struct {
	SSH *sshpkg.Manager
}

// NewApplier creates a new config Applier.
func NewApplier(sshManager *sshpkg.Manager) *Applier {
	return &Applier{SSH: sshManager}
}

// ApplyConfigFiles takes a map of config files (path → content), as produced by
// generateDeviceConfig(), and pushes the changes to the device via SSH.
//
// For each config file:
//  1. Reads the current config from the device via `uci show <package>`
//  2. Parses the desired config from the generated UCI file content
//  3. Computes a diff (only changed options/sections)
//  4. Applies the diff via `uci set/delete/add_list` commands
//  5. Commits with `uci commit <package>`
//  6. Reloads the affected service
//
// Returns the list of packages that were changed.
func (a *Applier) ApplyConfigFiles(deviceID string, configFiles map[string]string) ([]string, error) {
	changedPackages := []string{}

	for path, content := range configFiles {
		// Skip non-UCI files (scripts, key files, etc.)
		if !strings.HasPrefix(path, "etc/config/") {
			continue
		}

		pkg := ucipkg.PackageFromPath(path)
		changed, err := a.applyPackage(deviceID, pkg, content)
		if err != nil {
			return changedPackages, fmt.Errorf("failed to apply %s: %w", pkg, err)
		}
		if changed {
			changedPackages = append(changedPackages, pkg)
		}
	}

	// Handle non-UCI files separately
	for path, content := range configFiles {
		if strings.HasPrefix(path, "etc/config/") {
			continue
		}
		if err := a.applyRawFile(deviceID, "/"+path, content); err != nil {
			log.Printf("[config] failed to write %s: %v", path, err)
		}
	}

	return changedPackages, nil
}

// applyPackage applies a single UCI package diff to the device.
// Returns true if any changes were made.
func (a *Applier) applyPackage(deviceID, pkg, desiredContent string) (bool, error) {
	// 1. Get current config from device
	currentOutput, err := a.SSH.Execute(deviceID, fmt.Sprintf("uci show %s 2>/dev/null || echo ''", pkg))
	if err != nil {
		return false, fmt.Errorf("failed to read current %s config: %w", pkg, err)
	}

	// 2. Parse both configs
	var currentCfg *ucipkg.Config
	if strings.TrimSpace(currentOutput) != "" {
		currentCfg, err = ucipkg.ParseShow(currentOutput)
		if err != nil {
			return false, fmt.Errorf("failed to parse current %s config: %w", pkg, err)
		}
	}

	desiredCfg, err := ucipkg.ParseUCIFile(desiredContent)
	if err != nil {
		return false, fmt.Errorf("failed to parse desired %s config: %w", pkg, err)
	}
	desiredCfg.Package = pkg

	// 3. Compute diff
	commands := ucipkg.Diff(currentCfg, desiredCfg)

	if len(commands) == 0 {
		log.Printf("[config] %s: no changes for device %s", pkg, deviceID)
		return false, nil
	}

	log.Printf("[config] %s: %d changes for device %s", pkg, len(commands), deviceID)

	// 4. Apply changes + commit + reload
	allCommands := make([]string, 0, len(commands)+2)
	allCommands = append(allCommands, commands...)
	allCommands = append(allCommands, fmt.Sprintf("uci commit %s", pkg))
	allCommands = append(allCommands, ucipkg.ReloadCommand(pkg))

	script := strings.Join(allCommands, " && ")
	output, err := a.SSH.Execute(deviceID, script)
	if err != nil {
		return false, fmt.Errorf("failed to apply %s config: %w (output: %s)", pkg, err, output)
	}

	log.Printf("[config] %s: applied successfully to device %s", pkg, deviceID)
	return true, nil
}

// applyRawFile writes a non-UCI file directly to the device via SSH.
// Used for scripts, authorized_keys, hostapd PSK files, etc.
func (a *Applier) applyRawFile(deviceID, remotePath, content string) error {
	// Ensure parent directory exists
	dir := remotePath[:strings.LastIndex(remotePath, "/")]
	mkdirCmd := fmt.Sprintf("mkdir -p '%s'", dir)

	// Write content via a heredoc to handle special characters
	// Using base64 to safely transport content with any characters
	writeCmd := fmt.Sprintf("cat > '%s' << 'OPENSOHO_EOF'\n%s\nOPENSOHO_EOF", remotePath, content)

	script := mkdirCmd + " && " + writeCmd
	_, err := a.SSH.Execute(deviceID, script)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", remotePath, err)
	}

	// Make executable if it's a hotplug/init script
	if strings.Contains(remotePath, "hotplug") || strings.Contains(remotePath, "init.d") {
		a.SSH.Execute(deviceID, fmt.Sprintf("chmod +x '%s'", remotePath))
	}

	return nil
}

// FullApply generates and applies the complete configuration for a device.
// This is the SSH equivalent of the old generateDeviceConfig() → download tar.gz flow.
// It takes the configFiles map directly from generateDeviceConfig() internals.
func (a *Applier) FullApply(deviceID string, configFiles map[string]string) error {
	changedPkgs, err := a.ApplyConfigFiles(deviceID, configFiles)
	if err != nil {
		return fmt.Errorf("config apply failed for device %s: %w", deviceID, err)
	}

	if len(changedPkgs) > 0 {
		log.Printf("[config] device %s: applied changes to %v", deviceID, changedPkgs)
	} else {
		log.Printf("[config] device %s: no changes needed", deviceID)
	}

	return nil
}

// RevertPackage reverts a UCI package to its saved state on the device.
func (a *Applier) RevertPackage(deviceID, pkg string) error {
	_, err := a.SSH.Execute(deviceID, fmt.Sprintf("uci revert %s", pkg))
	return err
}
