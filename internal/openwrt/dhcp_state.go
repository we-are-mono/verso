// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.
package openwrt

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/we-are-mono/verso/internal/ubus"
)

// DHCPState describes the DHCPv4 servers actually loaded by dnsmasq. The
// operator's staged UCI snapshot is deliberately not a source of live state.
func (*NativeBackend) DHCPState(ctx context.Context, sid string) (json.RawMessage, error) {
	c, err := ubus.Dial("")
	if err != nil {
		return nil, err
	}
	defer c.Close()
	for _, access := range [][3]string{{"uci", "dhcp", "read"}, {"ubus", "service", "list"}, {"ubus", "network.interface", "dump"}} {
		if ok, err := probeAccess(c, sid, access[0], access[1], access[2]); err != nil || !ok {
			return nil, ErrAccessDenied
		}
	}
	// No session means rpcd's committed view, after explicitly checking the
	// operator's read grant above. Including sid here would read pending edits.
	config, err := invoke(c, "uci", "get", map[string]any{"config": "dhcp"})
	if err != nil {
		return nil, err
	}
	services, err := invoke(c, "service", "list", map[string]any{"name": "dnsmasq"})
	if err != nil {
		return nil, err
	}
	dump, err := invoke(c, "network.interface", "dump", nil)
	if err != nil {
		return nil, err
	}
	values, _ := config["values"].(map[string]any)
	var protected struct {
		Files       map[string]string   `json:"files"`
		Directories map[string][]string `json:"directories"`
	}
	// Stock OpenWrt keeps its custom and built-in includes root-readable only.
	// The helper authorizes this read independently and returns only DHCP scope
	// directives from fixed roots, never staged edits or unrelated file contents.
	_ = callHelperWithin(ctx, "", "dhcpState", sid, nil, &protected, 5*time.Second)
	reader := func(path string) ([]byte, error) {
		if body, ok := protected.Files[path]; ok {
			return []byte(body), nil
		}
		return readDHCPConfigFile(path)
	}
	listing := func(path string) ([]string, error) {
		if names, ok := protected.Directories[path]; ok {
			return names, nil
		}
		return listDHCPFiles(path)
	}
	return json.Marshal(map[string]any{"networks": dhcpStates(values, services, dump, reader, readDHCPFile, listing, time.Now().Unix())})
}

type dhcpNetworkState struct {
	Enabled   bool   `json:"enabled"`
	State     string `json:"state"`
	Reason    string `json:"reason,omitempty"`
	Pool      string `json:"pool,omitempty"`
	LeaseTime string `json:"lease_time,omitempty"`
	Leases    *int   `json:"leases,omitempty"`
}
type dhcpRange struct{ tag, first, last, mask, duration string }
type dhcpInstance struct {
	id                        string
	running, failed, readable bool
	ranges                    []dhcpRange
	excluded                  []string
	interfaces                []string
	leaseFile                 string
}

func dhcpString(v any) string { s, _ := v.(string); return s }

// dhcpScopeKeys are the directives that decide what a dnsmasq instance serves
// and where else its configuration lives; nothing else in a config file is
// read.
var dhcpScopeKeys = map[string]bool{
	"dhcp-range": true, "dhcp-leasefile": true, "interface": true, "no-dhcp-interface": true,
	"except-interface": true, "conf-file": true, "conf-dir": true, "conf-script": true,
}

// dhcpConfigScanLimit bounds how much of one file is scanned for those lines:
// far past any blocklist a package drops beside the configuration (adblock's
// runs to tens of megabytes), short of reading without end.
const dhcpConfigScanLimit = 256 << 20

type dhcpScanned struct {
	size  int64
	mtime time.Time
	body  []byte
}

// dhcpScans keeps each config file's scope lines until the file changes, so a
// blocklist many megabytes long is scanned once rather than at every look.
var dhcpScans = struct {
	sync.Mutex
	files map[string]dhcpScanned
}{files: map[string]dhcpScanned{}}

// readDHCPConfigFile is a config file's DHCP scope lines, scanned line by line
// so a file of any size up to the scan limit costs no more memory than its
// longest line.
func readDHCPConfigFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > dhcpConfigScanLimit {
		return nil, fmt.Errorf("DHCP file is not a bounded regular file")
	}
	dhcpScans.Lock()
	kept, ok := dhcpScans.files[path]
	dhcpScans.Unlock()
	if ok && kept.size == info.Size() && kept.mtime.Equal(info.ModTime()) {
		return kept.body, nil
	}
	var body []byte
	scanner := bufio.NewScanner(io.LimitReader(f, dhcpConfigScanLimit))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		line := scanner.Bytes()
		key, _, found := strings.Cut(strings.TrimSpace(string(line)), "=")
		if found && dhcpScopeKeys[key] {
			body = append(body, line...)
			body = append(body, '\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	dhcpScans.Lock()
	dhcpScans.files[path] = dhcpScanned{size: info.Size(), mtime: info.ModTime(), body: body}
	dhcpScans.Unlock()
	return body, nil
}

func readDHCPFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return nil, fmt.Errorf("DHCP file is not a bounded regular file")
	}
	return io.ReadAll(io.LimitReader(f, 1<<20))
}
func parseDHCPConfig(body []byte, instance *dhcpInstance) {
	for _, line := range strings.Split(string(body), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch key {
		case "dhcp-leasefile":
			instance.leaseFile = value
		case "interface":
			instance.interfaces = append(instance.interfaces, value)
		case "no-dhcp-interface", "except-interface":
			instance.excluded = append(instance.excluded, value)
		case "dhcp-range":
			fields := strings.Split(value, ",")
			r := dhcpRange{}
			for len(fields) > 0 && (strings.HasPrefix(fields[0], "set:") || strings.HasPrefix(fields[0], "tag:")) {
				if strings.HasPrefix(fields[0], "set:") {
					r.tag = strings.TrimPrefix(fields[0], "set:")
				}
				fields = fields[1:]
			}
			if len(fields) < 2 || net.ParseIP(fields[0]).To4() == nil {
				continue
			}
			r.first, r.last = fields[0], fields[1]
			if r.last != "static" && net.ParseIP(r.last).To4() == nil {
				continue
			}
			for _, field := range fields[2:] {
				if ip := net.ParseIP(field).To4(); ip != nil && r.mask == "" {
					r.mask = field
				} else if ip == nil && strings.TrimSpace(field) != "" {
					r.duration = strings.Fields(field)[0]
				}
			}
			instance.ranges = append(instance.ranges, r)
		}
	}
}
func readDHCPInstances(services map[string]any, read func(string) ([]byte, error), list func(string) ([]string, error)) []dhcpInstance {
	service, _ := services["dnsmasq"].(map[string]any)
	instances, _ := service["instances"].(map[string]any)
	var out []dhcpInstance
	for id, raw := range instances {
		v, _ := raw.(map[string]any)
		running := asBool(v["running"])
		instance := dhcpInstance{id: id, running: running, failed: !running && (asBool(v["respawn_failed"]) || asInt64(v["exit_code"]) != 0), leaseFile: "/tmp/dhcp.leases"}
		args, _ := v["command"].([]any)
		path := ""
		for i, rawArg := range args {
			arg := dhcpString(rawArg)
			if arg == "-C" && i+1 < len(args) {
				path = dhcpString(args[i+1])
			}
			if strings.HasPrefix(arg, "--conf-file=") {
				path = strings.TrimPrefix(arg, "--conf-file=")
			}
		}
		// procd's known generated file, never an arbitrary command argument path.
		if filepath.Dir(path) == "/var/etc" && strings.HasPrefix(filepath.Base(path), "dnsmasq.conf.") {
			body, err := expandDHCPConfig(path, read, list)
			instance.readable = err == nil
			parseDHCPConfig(body, &instance)
		}
		out = append(out, instance)
	}
	return out
}

// dhcpStates reads dnsmasq's configuration through readConfig (its DHCP scope
// lines) and the lease files through readLeases (whole).
func dhcpStates(config, services, dump map[string]any, readConfig, readLeases func(string) ([]byte, error), list func(string) ([]string, error), now int64) map[string]dhcpNetworkState {
	instances := readDHCPInstances(services, readConfig, list)
	networks := map[string]map[string]any{}
	entries, _ := dump["interface"].([]any)
	for _, raw := range entries {
		v, _ := raw.(map[string]any)
		networks[dhcpString(v["interface"])] = v
	}
	out := map[string]dhcpNetworkState{}
	for _, raw := range config {
		section, _ := raw.(map[string]any)
		if section[".type"] != "dhcp" {
			continue
		}
		name := dhcpString(section["interface"])
		if name == "" {
			continue
		}
		live := networks[name]
		tag := dhcpString(section["networkid"])
		if tag == "" {
			tag = name
		}
		enabled := section["ignore"] != "1" && section["dhcpv4"] != "disabled"
		selected := dhcpString(section["instance"])
		device := dhcpString(live["l3_device"])
		if device == "" {
			device = dhcpString(live["device"])
		}
		var matched []dhcpRange
		leaseFiles := map[string]bool{}
		running, stopped, failed, unknown, blocked := false, false, false, false, false
		for _, instance := range instances {
			if selected != "" && instance.id != selected {
				continue
			}
			var ranges []dhcpRange
			for _, r := range instance.ranges {
				if r.tag == tag {
					ranges = append(ranges, r)
				}
			}
			// An explicit instance binding must not borrow another daemon's health.
			// Without a binding, a range's network tag identifies the serving instance.
			if len(ranges) == 0 && selected == "" {
				continue
			}
			running = running || instance.running
			stopped = stopped || !instance.running
			failed = failed || instance.failed
			unknown = unknown || !instance.readable
			if len(instance.interfaces) > 0 {
				allowed := false
				for _, pattern := range instance.interfaces {
					allowed = allowed || dhcpDeviceMatches(pattern, device)
				}
				blocked = blocked || !allowed
			}
			for _, excluded := range instance.excluded {
				blocked = blocked || dhcpDeviceMatches(excluded, device)
			}
			matched = append(matched, ranges...)
			leaseFiles[instance.leaseFile] = true
		}
		addressed := len(matched) > 0
		for _, r := range matched {
			addressed = addressed && dhcpAddressOnNetwork(r.first, live)
		}
		state := dhcpNetworkState{Enabled: enabled, State: "disabled"}
		switch {
		case !enabled && len(matched) == 0:
		case !running && (failed || stopped):
			state.State = "stopped"
			if failed {
				state.Reason = "failed"
			}
		case !running && len(matched) == 0:
			if enabled {
				anyRunning := false
				anyUnknown := false
				for _, instance := range instances {
					if selected == "" || instance.id == selected {
						anyRunning = anyRunning || instance.running
						anyUnknown = anyUnknown || !instance.readable
					}
				}
				if !anyRunning {
					state.State = "stopped"
				} else {
					state.State = "warning"
					state.Reason = "not-serving"
					if anyUnknown {
						state.Reason = "unknown"
					}
				}
			}
		case live == nil:
			state.State = "warning"
			state.Reason = "interface-unavailable"
		case asBool(live["pending"]) || !asBool(live["up"]):
			state.State = "warning"
			state.Reason = "interface-down"
		case unknown:
			state.State = "warning"
			state.Reason = "unknown"
		case blocked || len(matched) == 0 || !addressed:
			state.State = "warning"
			state.Reason = "not-serving"
		case stopped || failed:
			state.State = "warning"
			state.Reason = "partial"
		default:
			state.State = "running"
		}
		if len(matched) > 0 {
			pools := []string{}
			leased := map[string]bool{}
			leaseOK := true
			for path := range leaseFiles {
				data, err := readLeases(path)
				if err != nil {
					leaseOK = false
					continue
				}
				for _, line := range strings.Split(string(data), "\n") {
					fields := strings.Fields(line)
					if len(fields) < 3 {
						continue
					}
					expiry, err := strconv.ParseInt(fields[0], 10, 64)
					if err != nil || expiry != 0 && expiry <= now {
						continue
					}
					if dhcpAddressOnNetwork(fields[2], live) {
						leased[fields[2]] = true
					}
				}
			}
			var capacity, used uint64
			for _, r := range matched {
				if r.last == "static" {
					pools = append(pools, "reservations")
				} else {
					pools = append(pools, r.first+"–"+r.last)
					first, last := dhcpIPv4(r.first), dhcpIPv4(r.last)
					if last >= first {
						capacity += uint64(last) - uint64(first) + 1
						for ip := range leased {
							n := dhcpIPv4(ip)
							if n >= first && n <= last {
								used++
							}
						}
					}
				}
				if state.LeaseTime == "" {
					state.LeaseTime = r.duration
				}
			}
			state.Pool = strings.Join(pools, ", ")
			if leaseOK && live != nil {
				count := len(leased)
				state.Leases = &count
			}
			if state.State == "running" && !leaseOK {
				state.State = "warning"
				state.Reason = "leases-unavailable"
			}
			if state.State == "running" && capacity > 0 && used >= capacity {
				state.State = "warning"
				state.Reason = "pool-full"
			}
		}
		out[name] = state
	}
	return out
}
func dhcpIPv4(address string) uint32 {
	ip := net.ParseIP(address).To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}
func dhcpAddressOnNetwork(address string, live map[string]any) bool {
	ip := net.ParseIP(address).To4()
	if ip == nil {
		return false
	}
	entries, _ := live["ipv4-address"].([]any)
	for _, raw := range entries {
		v, _ := raw.(map[string]any)
		mask := int(asInt64(v["mask"]))
		router := net.ParseIP(dhcpString(v["address"])).To4()
		if router != nil && mask > 0 && mask <= 32 && (&net.IPNet{IP: router, Mask: net.CIDRMask(mask, 32)}).Contains(ip) {
			return true
		}
	}
	return false
}

func dhcpDeviceMatches(pattern, device string) bool {
	return device != "" && (pattern == device || strings.HasSuffix(pattern, "*") && strings.HasPrefix(device, strings.TrimSuffix(pattern, "*")))
}

// Follow the daemon's includes too: a custom no-dhcp-interface must not leave
// a green chip merely because the generated main file contains a range.
// This is read-only and bounded; executable config sources remain unverifiable.
func expandDHCPConfig(path string, read func(string) ([]byte, error), list func(string) ([]string, error)) ([]byte, error) {
	seen := map[string]bool{}
	var output []byte
	var problem error
	var visit func(string)
	visit = func(path string) {
		path = filepath.Clean(path)
		if seen[path] {
			return
		}
		if len(seen) >= 64 || len(output) >= 1<<20 {
			problem = fmt.Errorf("DHCP includes exceed read limit")
			return
		}
		seen[path] = true
		body, err := read(path)
		if err != nil {
			problem = err
			return
		}
		if len(output)+len(body) > 1<<20 {
			problem = fmt.Errorf("DHCP includes exceed read limit")
			return
		}
		output = append(output, body...)
		output = append(output, '\n')
		for _, line := range strings.Split(string(body), "\n") {
			key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			switch key {
			case "conf-file":
				if !filepath.IsAbs(value) {
					problem = fmt.Errorf("unverifiable DHCP include")
					continue
				}
				visit(value)
			case "conf-script":
				problem = fmt.Errorf("DHCP uses an executable configuration source")
			case "conf-dir":
				parts := strings.Split(value, ",")
				if !filepath.IsAbs(parts[0]) {
					problem = fmt.Errorf("unverifiable DHCP directory")
					continue
				}
				entries, err := list(parts[0])
				if err != nil {
					problem = err
					continue
				}
				for _, name := range entries {
					if strings.HasPrefix(name, ".") || strings.HasSuffix(name, "~") || strings.HasPrefix(name, "#") && strings.HasSuffix(name, "#") {
						continue
					}
					excluded, restricted, included := false, false, false
					for _, ext := range parts[1:] {
						if strings.HasPrefix(ext, "*") {
							restricted = true
							included = included || strings.HasSuffix(name, strings.TrimPrefix(ext, "*"))
						} else {
							excluded = excluded || strings.HasSuffix(name, ext)
						}
					}
					if !excluded && (!restricted || included) {
						visit(filepath.Join(parts[0], name))
					}
				}
			}
		}
	}
	visit(path)
	return output, problem
}

func listDHCPFiles(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names, nil
}
