package uci

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSectionToCommands_NamedSection(t *testing.T) {
	s := Section{
		Type: "wifi-device",
		Name: "radio0",
		Options: map[string]interface{}{
			"channel": "36",
			"htmode":  "VHT80",
			"type":    "mac80211",
		},
	}

	cmds := SectionToCommands("wireless", s)
	require.NotEmpty(t, cmds)

	// First command creates the section
	assert.Equal(t, "uci set wireless.radio0=wifi-device", cmds[0])

	// Remaining commands set options (sorted by key)
	assert.Contains(t, cmds, "uci set wireless.radio0.channel='36'")
	assert.Contains(t, cmds, "uci set wireless.radio0.htmode='VHT80'")
	assert.Contains(t, cmds, "uci set wireless.radio0.type='mac80211'")
}

func TestSectionToCommands_AnonymousSection(t *testing.T) {
	s := Section{
		Type: "wifi-iface",
		Name: "", // anonymous
		Options: map[string]interface{}{
			"ssid": "TestNet",
		},
	}

	cmds := SectionToCommands("wireless", s)
	require.NotEmpty(t, cmds)

	// First command should use "uci add"
	assert.Equal(t, "uci add wireless wifi-iface", cmds[0])

	// Subsequent commands should reference @type[-1]
	assert.Equal(t, "uci set wireless.@wifi-iface[-1].ssid='TestNet'", cmds[1])
}

func TestSectionToCommands_WithList(t *testing.T) {
	s := Section{
		Type: "device",
		Name: "br_lan",
		Options: map[string]interface{}{
			"name":  "br-lan",
			"ports": []string{"lan1", "lan2", "lan3"},
			"type":  "bridge",
		},
	}

	cmds := SectionToCommands("network", s)

	// Should contain add_list for each port
	assert.Contains(t, cmds, "uci add_list network.br_lan.ports='lan1'")
	assert.Contains(t, cmds, "uci add_list network.br_lan.ports='lan2'")
	assert.Contains(t, cmds, "uci add_list network.br_lan.ports='lan3'")
}

func TestDiff_NoChanges(t *testing.T) {
	cfg := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{"channel": "36"}},
		},
	}

	cmds := Diff(cfg, cfg)
	assert.Empty(t, cmds, "no diff should produce no commands")
}

func TestDiff_AddOption(t *testing.T) {
	current := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{
				"channel": "36",
			}},
		},
	}

	desired := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{
				"channel": "36",
				"htmode":  "VHT80",
			}},
		},
	}

	cmds := Diff(current, desired)
	assert.Equal(t, 1, len(cmds))
	assert.Equal(t, "uci set wireless.radio0.htmode='VHT80'", cmds[0])
}

func TestDiff_ChangeOption(t *testing.T) {
	current := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{
				"channel": "36",
			}},
		},
	}

	desired := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{
				"channel": "44",
			}},
		},
	}

	cmds := Diff(current, desired)
	assert.Equal(t, 1, len(cmds))
	assert.Equal(t, "uci set wireless.radio0.channel='44'", cmds[0])
}

func TestDiff_RemoveOption(t *testing.T) {
	current := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{
				"channel": "36",
				"htmode":  "VHT80",
			}},
		},
	}

	desired := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{
				"channel": "36",
			}},
		},
	}

	cmds := Diff(current, desired)
	assert.Equal(t, 1, len(cmds))
	assert.Equal(t, "uci delete wireless.radio0.htmode", cmds[0])
}

func TestDiff_AddSection(t *testing.T) {
	current := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{"channel": "36"}},
		},
	}

	desired := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{"channel": "36"}},
			{Type: "wifi-iface", Name: "wifinet0", Options: map[string]interface{}{"ssid": "TestNet"}},
		},
	}

	cmds := Diff(current, desired)
	assert.Equal(t, 2, len(cmds))
	assert.Equal(t, "uci set wireless.wifinet0=wifi-iface", cmds[0])
	assert.Equal(t, "uci set wireless.wifinet0.ssid='TestNet'", cmds[1])
}

func TestDiff_RemoveSection(t *testing.T) {
	current := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{"channel": "36"}},
			{Type: "wifi-iface", Name: "wifinet0", Options: map[string]interface{}{"ssid": "OldNet"}},
		},
	}

	desired := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{"channel": "36"}},
		},
	}

	cmds := Diff(current, desired)
	assert.Equal(t, 1, len(cmds))
	assert.Equal(t, "uci delete wireless.wifinet0", cmds[0])
}

func TestDiff_FromNilCurrent(t *testing.T) {
	desired := &Config{
		Package: "wireless",
		Sections: []Section{
			{Type: "wifi-device", Name: "radio0", Options: map[string]interface{}{"channel": "36"}},
		},
	}

	cmds := Diff(nil, desired)
	// Should create the section from scratch
	assert.Contains(t, cmds, "uci set wireless.radio0=wifi-device")
	assert.Contains(t, cmds, "uci set wireless.radio0.channel='36'")
}

func TestDiff_ListChanged(t *testing.T) {
	current := &Config{
		Package: "network",
		Sections: []Section{
			{Type: "device", Name: "br_lan", Options: map[string]interface{}{
				"ports": []string{"lan1", "lan2"},
			}},
		},
	}

	desired := &Config{
		Package: "network",
		Sections: []Section{
			{Type: "device", Name: "br_lan", Options: map[string]interface{}{
				"ports": []string{"lan1", "lan2", "lan3"},
			}},
		},
	}

	cmds := Diff(current, desired)
	// Should delete old list and re-add
	assert.True(t, len(cmds) >= 4) // delete + 3 add_list
	hasDelete := false
	addCount := 0
	for _, cmd := range cmds {
		if strings.Contains(cmd, "uci delete") {
			hasDelete = true
		}
		if strings.Contains(cmd, "uci add_list") {
			addCount++
		}
	}
	assert.True(t, hasDelete)
	assert.Equal(t, 3, addCount)
}

func TestReloadCommand(t *testing.T) {
	assert.Equal(t, "wifi reload", ReloadCommand("wireless"))
	assert.Equal(t, "wifi reload", ReloadCommand("etc/config/wireless"))
	assert.Equal(t, "/etc/init.d/network reload", ReloadCommand("network"))
	assert.Equal(t, "/etc/init.d/dnsmasq reload", ReloadCommand("dhcp"))
	assert.Equal(t, "/etc/init.d/firewall reload", ReloadCommand("firewall"))
	assert.Contains(t, ReloadCommand("unknown_pkg"), "unknown_pkg")
}

func TestPackageFromPath(t *testing.T) {
	assert.Equal(t, "wireless", PackageFromPath("etc/config/wireless"))
	assert.Equal(t, "network", PackageFromPath("etc/config/network"))
	assert.Equal(t, "system", PackageFromPath("etc/config/system"))
	assert.Equal(t, "raw_pkg", PackageFromPath("raw_pkg"))
}
