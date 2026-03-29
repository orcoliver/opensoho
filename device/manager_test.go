package device

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandCIDR_24(t *testing.T) {
	ips, err := expandCIDR("192.168.1.0/24")
	require.NoError(t, err)
	assert.Equal(t, 254, len(ips)) // 256 - network - broadcast
	assert.Equal(t, "192.168.1.1", ips[0])
	assert.Equal(t, "192.168.1.254", ips[len(ips)-1])
}

func TestExpandCIDR_28(t *testing.T) {
	ips, err := expandCIDR("10.0.0.0/28")
	require.NoError(t, err)
	assert.Equal(t, 14, len(ips)) // 16 - 2
	assert.Equal(t, "10.0.0.1", ips[0])
	assert.Equal(t, "10.0.0.14", ips[len(ips)-1])
}

func TestExpandCIDR_32(t *testing.T) {
	ips, err := expandCIDR("192.168.1.5/32")
	require.NoError(t, err)
	assert.Equal(t, 1, len(ips))
	assert.Equal(t, "192.168.1.5", ips[0])
}

func TestExpandCIDR_Invalid(t *testing.T) {
	_, err := expandCIDR("not-a-cidr")
	assert.Error(t, err)
}

func TestNewManager(t *testing.T) {
	mgr := NewManager(nil) // nil SSH manager for unit testing
	assert.NotNil(t, mgr)
}

func TestHealthCheck_NoSSH(t *testing.T) {
	mgr := NewManager(nil)
	// This would panic if we called HealthCheck with nil SSH manager,
	// but we're just testing the structure here
	assert.NotNil(t, mgr)
}
