// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/we-are-mono/verso/internal/ubus"
)

// NetworkState is the complete netifd inventory. Keep logical interfaces and
// kernel devices separate: PPPoE and bridges are not their underlying ports.
func (*NativeBackend) NetworkState(_ context.Context, sid string) (json.RawMessage, error) {
	dump, err := fetchNetworkDump("", sid)
	if err != nil {
		return nil, err
	}
	c, err := ubus.Dial("")
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if ok, err := probeAccess(c, sid, "ubus", "network.device", "status"); err != nil || !ok {
		return nil, ErrAccessDenied
	}
	id, err := c.Lookup("network.device")
	if err != nil {
		return nil, err
	}
	devices, err := c.Invoke(id, "status")
	if err != nil {
		return nil, err
	}
	if devices == nil {
		devices = map[string]any{}
	}
	state := networkState(dump, devices)
	kernelNetworkDevices("/sys/class/net", devices)
	kernelNetworkAddresses(devices)
	if ok, err := probeAccess(c, sid, "ubus", "network", "get_proto_handlers"); err == nil && ok {
		if id, err := c.Lookup("network"); err == nil {
			if protocols, err := c.Invoke(id, "get_proto_handlers"); err == nil {
				state["protocols"] = protocols
			}
		}
	}
	return json.Marshal(state)
}

func kernelNetworkAddresses(devices map[string]any) {
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		device, ok := devices[iface.Name].(map[string]any)
		if !ok {
			continue
		}
		addresses, _ := iface.Addrs()
		for _, address := range addresses {
			ip, prefix, err := net.ParseCIDR(address.String())
			if err != nil {
				continue
			}
			key := "ipv6-address"
			if ip.To4() != nil {
				key = "ipv4-address"
			}
			mask, _ := prefix.Mask.Size()
			values, _ := device[key].([]any)
			device[key] = append(values, map[string]any{"address": ip.String(), "mask": mask})
		}
	}
}

// netifd lists the devices it manages. Kernel devices without a UCI owner must
// still report their real link state, address and MTU (including unused ports).
// Existing netifd fields win; failed sysfs reads remain unknown. Only Ethernet
// devices have a MAC address: tunnel address bytes are a different kind of data.
func kernelNetworkDevices(root string, devices map[string]any) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.Name() == "lo" {
			continue
		}
		base := filepath.Join(root, entry.Name())
		read := func(name string) string {
			value, _ := os.ReadFile(filepath.Join(base, name))
			return strings.TrimSpace(string(value))
		}
		d, _ := devices[entry.Name()].(map[string]any)
		if d == nil {
			d = map[string]any{}
		}
		put := func(key string, value any) {
			if _, exists := d[key]; !exists {
				d[key] = value
			}
		}
		put("present", true)
		if flags, err := strconv.ParseUint(read("flags"), 0, 32); err == nil {
			put("up", flags&1 != 0)
		}
		if carrier := read("carrier"); carrier == "0" || carrier == "1" {
			put("carrier", carrier == "1")
		}
		if value := read("operstate"); value != "" {
			put("operstate", value)
		}
		// sysfs "address" follows the device's ARPHRD type. For ip6tnl0 it
		// contains a 16-byte IPv6 tunnel endpoint, not a six-byte Ethernet MAC.
		linkType, typeErr := strconv.ParseUint(read("type"), 10, 16)
		address, _ := d["macaddr"].(string)
		if address == "" {
			address = read("address")
		}
		mac, macErr := net.ParseMAC(address)
		if (typeErr == nil && linkType != 1) || macErr != nil || len(mac) != 6 || mac.String() == "00:00:00:00:00:00" {
			delete(d, "macaddr")
		} else {
			d["macaddr"] = mac.String()
		}
		if mtu, err := strconv.ParseUint(read("mtu"), 10, 32); err == nil {
			put("mtu", mtu)
		}
		if speed, err := strconv.ParseUint(read("speed"), 10, 32); err == nil && speed > 0 {
			duplex := map[string]string{"full": "F", "half": "H"}[read("duplex")]
			put("speed", strconv.FormatUint(speed, 10)+duplex)
		}
		if driver, err := os.Readlink(filepath.Join(base, "device/driver")); err == nil {
			put("driver", filepath.Base(driver))
		}
		stats, _ := d["statistics"].(map[string]any)
		if stats == nil {
			stats = map[string]any{}
		}
		for _, key := range []string{"rx_errors", "tx_errors"} {
			if _, exists := stats[key]; !exists {
				if value, err := strconv.ParseUint(read("statistics/"+key), 10, 64); err == nil {
					stats[key] = value
				}
			}
		}
		d["statistics"] = stats
		devices[entry.Name()] = d
	}
}

// blobmsg BOOL shares its wire type with INT8. The native ubus client keeps
// those values numeric; normalize only the flags netifd defines as booleans at
// this API boundary, leaving counters, MTUs and other numbers untouched.
func networkState(dump, devices map[string]any) map[string]any {
	flags := func(m map[string]any, keys ...string) {
		for _, key := range keys {
			if n, ok := m[key].(int64); ok {
				m[key] = n != 0
			}
		}
	}
	interfaces, _ := dump["interface"].([]any)
	for _, raw := range interfaces {
		iface, _ := raw.(map[string]any)
		flags(iface, "up", "pending", "available", "autostart", "dynamic", "delegation")
	}
	for _, raw := range devices {
		device, _ := raw.(map[string]any)
		flags(device, "present", "up", "carrier", "external", "auth_status", "ipv6")
		bridge, _ := device["bridge-attributes"].(map[string]any)
		flags(bridge, "stp", "stp_kernel", "multicast_snooping", "bridge_empty")
	}
	return map[string]any{"interfaces": interfaces, "devices": devices}
}

// NetworkSetUp changes the running logical interface. Start-on-boot remains a
// separate, staged UCI setting, so a power button reports what actually ran.
func (*NativeBackend) NetworkSetUp(_ context.Context, sid, name string, up bool) error {
	c, err := ubus.Dial("")
	if err != nil {
		return err
	}
	defer c.Close()
	method := "down"
	if up {
		method = "up"
	}
	object := "network.interface." + name
	if ok, err := probeAccess(c, sid, "ubus", object, method); err != nil || !ok {
		return ErrAccessDenied
	}
	id, err := c.Lookup(object)
	if err != nil {
		return fmt.Errorf("interface %s: %w", name, err)
	}
	_, err = c.Invoke(id, method)
	return err
}

// NetworkRestart asks the running netifd object to reconnect. RPC access is
// checked for both operations before either is attempted.
func (*NativeBackend) NetworkRestart(_ context.Context, sid, name string) error {
	c, err := ubus.Dial("")
	if err != nil {
		return err
	}
	defer c.Close()
	object := "network.interface." + name
	for _, method := range []string{"down", "up"} {
		if ok, err := probeAccess(c, sid, "ubus", object, method); err != nil || !ok {
			return ErrAccessDenied
		}
	}
	id, err := c.Lookup(object)
	if err != nil {
		return err
	}
	if _, err = c.Invoke(id, "down"); err != nil {
		return err
	}
	_, err = c.Invoke(id, "up")
	return err
}
