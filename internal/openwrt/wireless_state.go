// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"context"
	"encoding/json"

	"github.com/we-are-mono/verso/internal/ubus"
)

// WirelessState is what the radios are doing now, keyed by the config sections
// a Wi-Fi page already holds: whether each radio and each network is up, the
// channel and transmit power a radio is really using, how busy its channel is,
// and how many clients each network carries. netifd says which kernel
// interface a section became; iwinfo reads that interface.
func (*NativeBackend) WirelessState(_ context.Context, sid string) (json.RawMessage, error) {
	c, err := ubus.Dial("")
	if err != nil {
		return nil, err
	}
	defer c.Close()
	if ok, err := probeAccess(c, sid, "ubus", "network.wireless", "status"); err != nil || !ok {
		return nil, ErrAccessDenied
	}
	id, err := c.Lookup("network.wireless")
	if err != nil {
		return nil, err
	}
	status, err := c.Invoke(id, "status")
	if err != nil {
		return nil, err
	}
	// iwinfo is a read the operator's grant has to cover too; without it the
	// radios still say whether they are up, and every number stays unknown.
	iwinfo := func(string, string) (map[string]any, error) { return nil, ErrAccessDenied }
	if ok, err := probeAccess(c, sid, "ubus", "iwinfo", "info"); err == nil && ok {
		if iw, err := c.Lookup("iwinfo"); err == nil {
			iwinfo = func(method, device string) (map[string]any, error) {
				return c.InvokeTable(iw, method, map[string]any{"device": device})
			}
		}
	}
	return json.Marshal(wirelessState(status, iwinfo))
}

// wirelessState joins netifd's wireless status to iwinfo's readings. A number
// iwinfo cannot give is left out rather than written as zero: no clients is a
// fact, could-not-ask is not. A radio's channel, power and busy share are read
// from its first interface that answers, since every interface on a radio
// shares the one channel.
func wirelessState(status map[string]any, iwinfo func(method, device string) (map[string]any, error)) map[string]any {
	radios := map[string]any{}
	networks := map[string]any{}
	for name, raw := range status {
		radio, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		up := asBool(radio["up"])
		reading := map[string]any{"up": up}
		interfaces, _ := radio["interfaces"].([]any)
		for _, rawIface := range interfaces {
			iface, ok := rawIface.(map[string]any)
			if !ok {
				continue
			}
			section, _ := iface["section"].(string)
			if section == "" {
				continue
			}
			ifname, _ := iface["ifname"].(string)
			network := map[string]any{"up": up && ifname != ""}
			networks[section] = network
			if !up || ifname == "" {
				continue
			}
			if assoc, err := iwinfo("assoclist", ifname); err == nil {
				stations, _ := assoc["results"].([]any)
				network["clients"] = len(stations)
			}
			if _, read := reading["channel"]; read {
				continue
			}
			info, err := iwinfo("info", ifname)
			if err != nil {
				continue
			}
			reading["channel"] = asInt64(info["channel"])
			reading["txpower"] = asInt64(info["txpower"])
			if hardware, ok := info["hardware"].(map[string]any); ok {
				if name, _ := hardware["name"].(string); name != "" {
					reading["hardware"] = name
				}
			}
			if busy, ok := channelBusy(iwinfo, ifname, asInt64(info["frequency"])); ok {
				reading["busy"] = busy
			}
		}
		radios[name] = reading
	}
	return map[string]any{"radios": radios, "networks": networks}
}

// channelBusy is the share of the in-use channel's airtime spent busy, as a
// whole percent: the survey entry for the frequency the radio is on, busy time
// over active time. Other surveyed frequencies are the scan, not the channel.
func channelBusy(iwinfo func(method, device string) (map[string]any, error), ifname string, frequency int64) (int64, bool) {
	survey, err := iwinfo("survey", ifname)
	if err != nil || frequency == 0 {
		return 0, false
	}
	entries, _ := survey["results"].([]any)
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if !ok || asInt64(entry["mhz"]) != frequency {
			continue
		}
		active := asInt64(entry["active_time"])
		if active <= 0 {
			return 0, false
		}
		return asInt64(entry["busy_time"]) * 100 / active, true
	}
	return 0, false
}
