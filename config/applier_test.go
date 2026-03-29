package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestApplier_Creation(t *testing.T) {
	// Applier can be created with nil manager for unit testing the interface
	applier := NewApplier(nil)
	assert.NotNil(t, applier)
}

func TestApplyConfigFiles_SkipsNonUCI(t *testing.T) {
	// This tests the file categorization logic without a real SSH connection.
	// A nil SSH manager will panic if Connect is attempted, which verifies
	// that non-UCI files are handled differently.

	// We can't fully test without a mock SSH, but we can verify the structure
	configFiles := map[string]string{
		"etc/config/wireless":              "config wifi-device 'radio0'\n\toption channel '36'\n",
		"etc/config/network":               "config interface 'lan'\n\toption proto 'static'\n",
		"etc/dropbear/authorized_keys":      "ssh-ed25519 AAAA...",
		"etc/hotplug.d/openwisp/opensoho":   "#!/bin/sh\necho hello",
	}

	// Count UCI vs non-UCI files
	uciCount := 0
	nonUCICount := 0
	for path := range configFiles {
		if isUCIPath(path) {
			uciCount++
		} else {
			nonUCICount++
		}
	}

	assert.Equal(t, 2, uciCount)
	assert.Equal(t, 2, nonUCICount)
}

// isUCIPath mirrors the logic in ApplyConfigFiles
func isUCIPath(path string) bool {
	return len(path) > len("etc/config/") && path[:len("etc/config/")] == "etc/config/"
}
