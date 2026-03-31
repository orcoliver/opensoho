package uci

import (
	"fmt"
	"sort"
	"strings"
)

// ServiceDisableMap maps service names to the commands needed to disable and stop them.
// Used for dumb AP profiles that need to neutralize firewall/dnsmasq/odhcpd.
var ServiceDisableMap = map[string]string{
	"firewall": "/etc/init.d/firewall disable 2>/dev/null; /etc/init.d/firewall stop 2>/dev/null",
	"dnsmasq":  "/etc/init.d/dnsmasq disable 2>/dev/null; /etc/init.d/dnsmasq stop 2>/dev/null",
	"odhcpd":   "/etc/init.d/odhcpd disable 2>/dev/null; /etc/init.d/odhcpd stop 2>/dev/null",
}

// ServiceReloadMap maps UCI package names to the commands needed to reload them.
var ServiceReloadMap = map[string]string{
	"wireless": "wifi reload",
	"network":  "/etc/init.d/network reload",
	"system":   "/etc/init.d/system reload",
	"dhcp":     "/etc/init.d/dnsmasq reload",
	"firewall": "/etc/init.d/firewall reload",
}

// Diff generates the sequence of UCI commands needed to transform `current` into `desired`.
// Returns uci set/delete/add_list commands (without commit — caller handles that).
func Diff(current, desired *Config) []string {
	commands := []string{}
	pkg := desired.Package
	if pkg == "" && current != nil {
		pkg = current.Package
	}

	currentSections := make(map[string]*Section)
	if current != nil {
		for i := range current.Sections {
			s := &current.Sections[i]
			key := sectionKey(s)
			currentSections[key] = s
		}
	}

	desiredSections := make(map[string]*Section)
	for i := range desired.Sections {
		s := &desired.Sections[i]
		key := sectionKey(s)
		desiredSections[key] = s
	}

	// Delete sections that exist in current but not in desired
	for key, s := range currentSections {
		if _, exists := desiredSections[key]; !exists {
			name := sectionUCIName(s)
			commands = append(commands, fmt.Sprintf("uci delete %s.%s", pkg, name))
		}
	}

	// Add or update sections in desired
	for _, s := range desired.Sections {
		key := sectionKey(&s)
		name := sectionUCIName(&s)

		if existing, ok := currentSections[key]; ok {
			// Section exists — diff options
			optCmds := diffOptions(pkg, name, existing, &s)
			commands = append(commands, optCmds...)
		} else {
			// New section — create it and set all options
			cmds := SectionToCommands(pkg, s)
			commands = append(commands, cmds...)
		}
	}

	return commands
}

// SectionToCommands generates UCI commands to create a section with all its options.
// Does not include uci commit.
func SectionToCommands(pkg string, s Section) []string {
	commands := []string{}
	name := sectionUCIName(&s)

	if s.Name != "" {
		// Named section: uci set pkg.name=type
		commands = append(commands, fmt.Sprintf("uci set %s.%s=%s", pkg, name, s.Type))
	} else {
		// Anonymous section: uci add pkg type
		commands = append(commands, fmt.Sprintf("uci add %s %s", pkg, s.Type))
		// For anonymous sections, subsequent commands use @type[-1]
		name = fmt.Sprintf("@%s[-1]", s.Type)
	}

	// Set options in sorted order for determinism
	keys := sortedKeys(s.Options)
	for _, key := range keys {
		value := s.Options[key]
		switch v := value.(type) {
		case string:
			commands = append(commands, fmt.Sprintf("uci set %s.%s.%s='%s'", pkg, name, key, v))
		case []string:
			for _, item := range v {
				commands = append(commands, fmt.Sprintf("uci add_list %s.%s.%s='%s'", pkg, name, key, item))
			}
		}
	}

	return commands
}

// ReloadCommand returns the shell command to reload the service for a given UCI package.
func ReloadCommand(pkg string) string {
	// Strip path prefix (e.g., "etc/config/wireless" -> "wireless")
	pkg = strings.TrimPrefix(pkg, "etc/config/")

	if cmd, ok := ServiceReloadMap[pkg]; ok {
		return cmd
	}
	// Fallback: try to restart the service by package name
	return fmt.Sprintf("/etc/init.d/%s reload 2>/dev/null || true", pkg)
}

// PackageFromPath extracts the UCI package name from a config file path.
// e.g., "etc/config/wireless" -> "wireless"
func PackageFromPath(path string) string {
	return strings.TrimPrefix(path, "etc/config/")
}

// --- diff helpers ---

func diffOptions(pkg, sectionName string, current, desired *Section) []string {
	commands := []string{}

	currentOpts := current.Options
	desiredOpts := desired.Options

	// Delete options in current that are not in desired
	for key := range currentOpts {
		if _, exists := desiredOpts[key]; !exists {
			commands = append(commands, fmt.Sprintf("uci delete %s.%s.%s", pkg, sectionName, key))
		}
	}

	// Add or update options
	keys := sortedKeys(desiredOpts)
	for _, key := range keys {
		desiredVal := desiredOpts[key]
		currentVal, exists := currentOpts[key]

		if exists && optionEqual(currentVal, desiredVal) {
			continue // No change
		}

		switch v := desiredVal.(type) {
		case string:
			commands = append(commands, fmt.Sprintf("uci set %s.%s.%s='%s'", pkg, sectionName, key, v))
		case []string:
			// For lists, delete and re-add
			if exists {
				commands = append(commands, fmt.Sprintf("uci delete %s.%s.%s", pkg, sectionName, key))
			}
			for _, item := range v {
				commands = append(commands, fmt.Sprintf("uci add_list %s.%s.%s='%s'", pkg, sectionName, key, item))
			}
		}
	}

	return commands
}

func optionEqual(a, b interface{}) bool {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case []string:
		bv, ok := b.([]string)
		if !ok || len(av) != len(bv) {
			return false
		}
		for i := range av {
			if av[i] != bv[i] {
				return false
			}
		}
		return true
	}
	return false
}

func sectionKey(s *Section) string {
	if s.Name != "" {
		return s.Type + ":" + s.Name
	}
	return fmt.Sprintf("%s:@%d", s.Type, s.Index)
}

func sectionUCIName(s *Section) string {
	if s.Name != "" {
		return s.Name
	}
	return fmt.Sprintf("@%s[%d]", s.Type, s.Index)
}

func sortedKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
