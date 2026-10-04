// SPDX-License-Identifier: GPL-2.0-only
// SPDX-FileCopyrightText: 2026 Mono Technologies Inc.

package openwrt

import (
	"slices"
	"sort"
	"strconv"
)

const mainRouteTable uint32 = 254

// InternetAvailable is the pre-session counterpart of WANState.Up: whether
// the kernel has a forwarding default. It exposes no address, route, or config
// and needs no rpcd credential. Like the signed-in overview, it reports an
// available uplink, not an active reachability probe to an external service.
func InternetAvailable() (bool, error) {
	routes, err := kernelDefaultRoutes()
	return len(routes) > 0, err
}

// kernelRoute is a forwarding default route already filtered through the
// kernel's active FIB-rule reachability. One multipath route yields one record
// per live nexthop.
type kernelRoute struct {
	Family int
	Device string
	Table  uint32
	Metric uint32
}

type netifdOwner struct {
	name, device, transport string
	uptime                  int64
	status                  map[string]any
}

// discoverWAN combines stock netifd routing truth with routes installed by
// external managers. netifd supplies logical ownership; a kernel-only device is
// retained without inventing UCI metadata.
func discoverWAN(dump map[string]any, kernel []kernelRoute) WANState {
	entries, _ := dump["interface"].([]any)
	owners := make(map[string][]netifdOwner)
	devices := make(map[string]*WANDevice)

	deviceFor := func(name string) *WANDevice {
		if devices[name] == nil {
			devices[name] = &WANDevice{Device: name}
		}
		return devices[name]
	}
	addOwner := func(device *WANDevice, owner netifdOwner) {
		if owner.name != "" && !slices.Contains(device.Networks, owner.name) {
			device.Networks = append(device.Networks, owner.name)
		}
		if device.Transport == "" {
			device.Transport = owner.transport
		}
		if owner.uptime > device.Uptime {
			device.Uptime = owner.uptime
		}
	}
	addRoute := func(device *WANDevice, route WANRoute) {
		for i := range device.Routes {
			existing := &device.Routes[i]
			if existing.Family == route.Family && existing.Table == route.Table && existing.Metric == route.Metric {
				if existing.Owner == "" {
					existing.Owner = route.Owner
				}
				return
			}
		}
		device.Routes = append(device.Routes, route)
	}

	for _, value := range entries {
		status, _ := value.(map[string]any)
		if !asBool(status["up"]) {
			continue
		}
		l3, _ := status["l3_device"].(string)
		if l3 == "" { // never assign a live role to the configured transport
			continue
		}
		owner := netifdOwner{
			name: stringValue(status["interface"]), device: l3,
			transport: stringValue(status["device"]), uptime: asInt64(status["uptime"]),
			status: status,
		}
		owners[l3] = append(owners[l3], owner)
		for _, route := range netifdDefaultRoutes(status) {
			if route.Table != mainRouteTable {
				continue
			}
			device := deviceFor(l3)
			addOwner(device, owner)
			addRoute(device, WANRoute{
				Family: route.Family, Table: route.Table, Metric: route.Metric,
				Main: true, Owner: owner.name,
			})
		}
	}

	for _, route := range kernel {
		if route.Device == "" {
			continue
		}
		device := deviceFor(route.Device)
		for _, owner := range owners[route.Device] {
			addOwner(device, owner)
		}
		addRoute(device, WANRoute{
			Family: route.Family, Table: route.Table, Metric: route.Metric,
			Main: route.Table == mainRouteTable,
		})
	}

	state := WANState{Devices: make([]WANDevice, 0, len(devices))}
	for _, device := range devices {
		if len(device.Routes) == 0 {
			continue
		}
		sort.Strings(device.Networks)
		sort.Slice(device.Routes, func(i, j int) bool {
			left, right := device.Routes[i], device.Routes[j]
			if routeRank(left) != routeRank(right) {
				return routeRank(left) < routeRank(right)
			}
			if left.Metric != right.Metric {
				return left.Metric < right.Metric
			}
			if left.Table != right.Table {
				return left.Table < right.Table
			}
			return left.Owner < right.Owner
		})
		state.Devices = append(state.Devices, *device)
	}
	sort.Slice(state.Devices, func(i, j int) bool {
		leftRank, leftMetric := deviceRank(state.Devices[i])
		rightRank, rightMetric := deviceRank(state.Devices[j])
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if leftMetric != rightMetric {
			return leftMetric < rightMetric
		}
		return state.Devices[i].Device < state.Devices[j].Device
	})
	return state
}

func routeRank(route WANRoute) int {
	if route.Main && route.Family == 4 {
		return 0
	}
	if route.Main && route.Family == 6 {
		return 1
	}
	return 2
}

func deviceRank(device WANDevice) (int, uint32) {
	rank, metric := 3, ^uint32(0)
	for _, route := range device.Routes {
		if r := routeRank(route); r < rank || (r == rank && route.Metric < metric) {
			rank, metric = r, route.Metric
		}
	}
	return rank, metric
}

type defaultRoute struct {
	Family int
	Table  uint32
	Metric uint32
}

func netifdDefaultRoutes(status map[string]any) []defaultRoute {
	routes, _ := status["route"].([]any)
	interfaceMetric := uintValue(status["metric"])
	out := make([]defaultRoute, 0, len(routes))
	for _, value := range routes {
		route, _ := value.(map[string]any)
		family := defaultRouteFamily(route)
		if family == 0 {
			continue
		}
		table := routeTable(route["table"])
		metric := uintValue(route["metric"])
		if _, present := route["metric"]; !present {
			metric = interfaceMetric
		}
		out = append(out, defaultRoute{Family: family, Table: table, Metric: metric})
	}
	return out
}

func defaultRouteFamily(route map[string]any) int {
	if asInt64(route["mask"]) != 0 {
		return 0
	}
	switch stringValue(route["target"]) {
	case "0.0.0.0":
		return 4
	case "::":
		return 6
	default:
		return 0
	}
}

func routeTable(value any) uint32 {
	if value == nil {
		return mainRouteTable
	}
	if name, ok := value.(string); ok {
		switch name {
		case "", "main":
			return mainRouteTable
		case "default":
			return 253
		case "local":
			return 255
		}
	}
	if table := uintValue(value); table != 0 {
		return table
	}
	return mainRouteTable
}

func preferredStatus(entries []any, family int) map[string]any {
	var best map[string]any
	bestMetric := ^uint32(0)
	bestName := ""
	for _, value := range entries {
		status, _ := value.(map[string]any)
		if !asBool(status["up"]) || stringValue(status["l3_device"]) == "" {
			continue
		}
		metric, found := ^uint32(0), false
		for _, route := range netifdDefaultRoutes(status) {
			if route.Family == family && route.Table == mainRouteTable && (!found || route.Metric < metric) {
				metric, found = route.Metric, true
			}
		}
		name := stringValue(status["interface"])
		if found && (best == nil || metric < bestMetric || (metric == bestMetric && name < bestName)) {
			best, bestMetric, bestName = status, metric, name
		}
	}
	return best
}

func stringValue(value any) string {
	valueString, _ := value.(string)
	return valueString
}

func uintValue(value any) uint32 {
	switch value := value.(type) {
	case uint32:
		return value
	case uint64:
		return uint32(value)
	case int:
		return uint32(value)
	case int32:
		return uint32(value)
	case int64:
		return uint32(value)
	case float64:
		return uint32(value)
	case string:
		parsed, _ := strconv.ParseUint(value, 10, 32)
		return uint32(parsed)
	default:
		return 0
	}
}
