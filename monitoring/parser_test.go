package monitoring

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHostapdClients(t *testing.T) {
	jsonStr := `{
		"freq": 5180,
		"clients": {
			"AA:BB:CC:DD:EE:01": {
				"auth": true,
				"assoc": true,
				"signal": -54,
				"rx": {"rate": 866700},
				"tx": {"rate": 433300},
				"bytes": {"rx": 123456, "tx": 789012},
				"connected_time": 3600
			},
			"AA:BB:CC:DD:EE:02": {
				"auth": true,
				"assoc": true,
				"signal": -72,
				"rx": {"rate": 144400},
				"tx": {"rate": 72200},
				"bytes": {"rx": 5000, "tx": 10000},
				"connected_time": 120
			}
		}
	}`

	clients, err := ParseHostapdClients(jsonStr, "wlan0")
	require.NoError(t, err)
	assert.Equal(t, 2, len(clients))

	// Find client by MAC (map order is not guaranteed)
	var client1 *Client
	for i := range clients {
		if clients[i].MAC == "AA:BB:CC:DD:EE:01" {
			client1 = &clients[i]
		}
	}
	require.NotNil(t, client1)
	assert.True(t, client1.Auth)
	assert.True(t, client1.Assoc)
	assert.Equal(t, -54, client1.Signal)
	assert.Equal(t, 866700, client1.RxRate)
	assert.Equal(t, int64(123456), client1.RxBytes)
	assert.Equal(t, 3600, client1.Connected)
	assert.Equal(t, "wlan0", client1.Interface)
}

func TestParseHostapdClients_Empty(t *testing.T) {
	clients, err := ParseHostapdClients("", "wlan0")
	assert.NoError(t, err)
	assert.Nil(t, clients)
}

func TestParseHostapdClients_NoClients(t *testing.T) {
	jsonStr := `{"freq": 2412, "clients": {}}`
	clients, err := ParseHostapdClients(jsonStr, "wlan0")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(clients))
}

func TestParseDHCPLeases(t *testing.T) {
	content := `1711209600 aa:bb:cc:dd:ee:01 192.168.1.100 my-laptop 01:aa:bb:cc:dd:ee:01
1711209700 11:22:33:44:55:66 192.168.1.101 iphone *
1711209800 ff:ee:dd:cc:bb:aa 192.168.1.102 * *`

	leases, err := ParseDHCPLeases(content)
	require.NoError(t, err)
	assert.Equal(t, 3, len(leases))

	assert.Equal(t, int64(1711209600), leases[0].Expiry)
	assert.Equal(t, "AA:BB:CC:DD:EE:01", leases[0].MAC)
	assert.Equal(t, "192.168.1.100", leases[0].IP)
	assert.Equal(t, "my-laptop", leases[0].Hostname)

	assert.Equal(t, "11:22:33:44:55:66", leases[1].MAC)
	assert.Equal(t, "*", leases[1].ClientID)
}

func TestParseDHCPLeases_Empty(t *testing.T) {
	leases, err := ParseDHCPLeases("")
	assert.NoError(t, err)
	assert.Equal(t, 0, len(leases))
}

func TestParseBoardInfo(t *testing.T) {
	jsonStr := `{
		"kernel": "5.15.134",
		"hostname": "OpenWrt",
		"system": "MediaTek MT7981B ver:1 eco:2",
		"model": "TP-Link Deco X50-PoE v2",
		"board_name": "tplink,deco-x50-poe-v2",
		"release": {
			"distribution": "OpenWrt",
			"version": "23.05.3",
			"revision": "r24076-4b684d9",
			"target": "mediatek/filogic",
			"description": "OpenWrt 23.05.3 r24076-4b684d9"
		}
	}`

	info, err := ParseBoardInfo(jsonStr)
	require.NoError(t, err)
	assert.Equal(t, "TP-Link Deco X50-PoE v2", info.Model)
	assert.Equal(t, "OpenWrt", info.Hostname)
	assert.Equal(t, "MediaTek MT7981B ver:1 eco:2", info.System)
	assert.Equal(t, "23.05.3", info.Release.Version)
	assert.Equal(t, "mediatek/filogic", info.Release.Target)
}

func TestParseBoardInfo_Empty(t *testing.T) {
	_, err := ParseBoardInfo("")
	assert.Error(t, err)
}

func TestParseLinkShow(t *testing.T) {
	jsonStr := `[
		{"ifname":"lo","operstate":"UNKNOWN","mtu":65536,"stats64":{"rx":{"bytes":0,"packets":0},"tx":{"bytes":0,"packets":0}}},
		{"ifname":"eth0","operstate":"UP","mtu":1500,"stats64":{"rx":{"bytes":1000000,"packets":5000},"tx":{"bytes":2000000,"packets":3000}}},
		{"ifname":"br-lan","operstate":"UP","mtu":1500,"stats64":{"rx":{"bytes":500000,"packets":2500},"tx":{"bytes":1000000,"packets":1500}}},
		{"ifname":"wlan0","operstate":"UP","mtu":1500,"stats64":{"rx":{"bytes":300000,"packets":1000},"tx":{"bytes":400000,"packets":800}}}
	]`

	interfaces, err := ParseLinkShow(jsonStr)
	require.NoError(t, err)

	// Should exclude lo and wlan0
	assert.Equal(t, 2, len(interfaces))

	// eth0
	assert.Equal(t, "eth0", interfaces[0].Name)
	assert.Equal(t, "UP", interfaces[0].State)
	assert.Equal(t, int64(1000000), interfaces[0].RxBytes)
	assert.Equal(t, int64(2000000), interfaces[0].TxBytes)

	// br-lan
	assert.Equal(t, "br-lan", interfaces[1].Name)
}

func TestParseLinkShow_Empty(t *testing.T) {
	interfaces, err := ParseLinkShow("")
	assert.NoError(t, err)
	assert.Nil(t, interfaces)
}

func TestParseLoadAverage(t *testing.T) {
	content := "0.12 0.05 0.01 1/89 12345"

	load, err := ParseLoadAverage(content)
	require.NoError(t, err)
	assert.InDelta(t, 0.12, load.Load1, 0.001)
	assert.InDelta(t, 0.05, load.Load5, 0.001)
	assert.InDelta(t, 0.01, load.Load15, 0.001)
}

func TestParseLoadAverage_Invalid(t *testing.T) {
	_, err := ParseLoadAverage("invalid")
	assert.Error(t, err)
}

func TestParseMACAddress(t *testing.T) {
	assert.Equal(t, "AA:BB:CC:DD:EE:FF", ParseMACAddress("aa:bb:cc:dd:ee:ff\n"))
	assert.Equal(t, "11:22:33:44:55:66", ParseMACAddress("  11:22:33:44:55:66  "))
}

func TestParseOpenWrtRelease(t *testing.T) {
	content := `DISTRIB_ID='OpenWrt'
DISTRIB_RELEASE='23.05.3'
DISTRIB_REVISION='r24076-4b684d9'
DISTRIB_TARGET='mediatek/filogic'
DISTRIB_ARCH='aarch64_cortex-a53'
DISTRIB_DESCRIPTION='OpenWrt 23.05.3 r24076-4b684d9'
DISTRIB_TAINTS=''`

	isOpenwrt, version := ParseOpenWrtRelease(content)
	assert.True(t, isOpenwrt)
	assert.Equal(t, "23.05.3", version)
}

func TestParseOpenWrtRelease_NotOpenWrt(t *testing.T) {
	isOpenwrt, _ := ParseOpenWrtRelease("something else entirely")
	assert.False(t, isOpenwrt)
}
