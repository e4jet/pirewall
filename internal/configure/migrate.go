/*
(c) Copyright 2023 Eric Paul Forgette

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package configure

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/e4jet/pirewall/internal/chain"
	"github.com/e4jet/pirewall/internal/util"
)

const (
	methodDHCP   = "dhcp"
	methodStatic = "static"

	dpkgQueryBin = "/usr/bin/dpkg-query"
	ipBin        = "/usr/sbin/ip"
	udevadmBin   = "/usr/bin/udevadm"
	nmcliBin     = "/usr/bin/nmcli"

	interfacesPath = "/etc/network/interfaces"

	// skipped is the runner output value when a runner short-circuits
	// because NetworkManager is absent. Returning a non-nil value with
	// a nil error keeps the nilnil linter happy and is informative if
	// surfaced via Chain.GetRunnerOutput.
	skipped = "skipped (NetworkManager absent)"
)

// ifaceState is the captured per-interface state used to render the
// safety-net /etc/network/interfaces file.
type ifaceState struct {
	CurrentName     string
	PredictableName string
	MAC             string
	IPv4            string
	Prefix          int
	Gateway         string
	Method          string // "dhcp" or "static"
}

// virtualPrefixes are interface-name prefixes we treat as virtual and exclude
// from discovery. lo is excluded by name match in parseIPAddrJSON.
var virtualPrefixes = []string{"vlan", "br", "tun", "docker", "veth", "wg"}

func isVirtualName(name string) bool {
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}

	return false
}

// parseIPAddrJSON parses `ip -j addr show` JSON output. It returns one
// ifaceState per non-lo, non-virtual interface, populated with CurrentName,
// MAC, IPv4, and Prefix when an inet address is present. Interfaces without
// an inet address are still returned with IPv4="" and Prefix=0.
func parseIPAddrJSON(b []byte) ([]ifaceState, error) {
	var raw []struct {
		IfName   string `json:"ifname"`
		Address  string `json:"address"`
		AddrInfo []struct {
			Family    string `json:"family"`
			Local     string `json:"local"`
			Prefixlen int    `json:"prefixlen"`
		} `json:"addr_info"`
	}

	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("parse ip addr json: %w", err)
	}

	var out []ifaceState

	for _, r := range raw {
		if r.IfName == "lo" || isVirtualName(r.IfName) {
			continue
		}

		s := ifaceState{
			CurrentName: r.IfName,
			MAC:         r.Address,
		}

		for _, ai := range r.AddrInfo {
			if ai.Family != "inet" {
				continue
			}

			s.IPv4 = ai.Local
			s.Prefix = ai.Prefixlen

			break
		}

		out = append(out, s)
	}

	return out, nil
}

// parseDefaultRouteJSON parses `ip -j route show default` JSON output and
// returns the gateway and device of the IPv4 default route with the lowest
// metric. ok is false when no IPv4 default route is present or the input
// can't be parsed.
func parseDefaultRouteJSON(b []byte) (gateway, dev string, ok bool) {
	var raw []struct {
		Dst     string `json:"dst"`
		Gateway string `json:"gateway"`
		Dev     string `json:"dev"`
		Metric  int    `json:"metric"`
		Family  string `json:"family"`
	}

	if err := json.Unmarshal(b, &raw); err != nil {
		return "", "", false
	}

	bestMetric := -1

	for _, r := range raw {
		if r.Dst != "default" {
			continue
		}

		// Skip IPv6 default routes. ip -j sometimes omits family for IPv4;
		// the gateway format is the most reliable discriminator.
		if r.Family == "inet6" || strings.Contains(r.Gateway, ":") {
			continue
		}

		if bestMetric == -1 || r.Metric < bestMetric {
			bestMetric = r.Metric
			gateway = r.Gateway
			dev = r.Dev
			ok = true
		}
	}

	return gateway, dev, ok
}

// parseUdevadmInfo parses `udevadm info -q property` output and returns the
// value of ID_NET_NAME_MAC if present. The "E: " prefix udevadm uses in some
// output modes is tolerated. ok is false when the key is absent.
func parseUdevadmInfo(b []byte) (name string, ok bool) {
	const key = "ID_NET_NAME_MAC="

	for line := range strings.SplitSeq(string(b), "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "E: ")

		if v, found := strings.CutPrefix(line, key); found {
			return strings.TrimSpace(v), true
		}
	}

	return "", false
}

// parseNmcliMethod converts `nmcli -t -g ipv4.method connection show` output
// to "dhcp" when nmcli reports "auto", or "static" for any other value
// (including empty, "manual", and "disabled" — all of which we render as a
// static stanza so the operator can fill in addresses if needed).
func parseNmcliMethod(b []byte) string {
	if strings.TrimSpace(string(b)) == "auto" {
		return methodDHCP
	}

	return methodStatic
}

// isLoOnly reports whether /etc/network/interfaces is missing, empty, or
// declares no `iface` stanza other than `lo`. Used as the safety-net gate.
func isLoOnly(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}

		return false, fmt.Errorf("open %s: %w", path, err)
	}

	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 { //nolint:mnd // "iface NAME ..." needs ≥ 2 fields
			continue
		}

		if fields[0] == "iface" && fields[1] != "lo" {
			return false, nil
		}
	}

	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("scan %s: %w", path, err)
	}

	return true, nil
}

// renderInterfacesFile produces the exact body of /etc/network/interfaces
// for the safety-net write. generatedAt is rendered in the header so the
// output is deterministic given a fixed input.
func renderInterfacesFile(ifaces []ifaceState, generatedAt time.Time) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Auto-generated by pirewall -config on %s.\n", generatedAt.UTC().Format(time.RFC3339))
	b.WriteString("# Safety-net file mirroring NetworkManager-driven state at install time.\n")
	b.WriteString("# Edit freely; pirewall will not overwrite this file on subsequent runs.\n")
	b.WriteString("\n")
	b.WriteString("auto lo\n")
	b.WriteString("iface lo inet loopback\n")

	for _, iface := range ifaces {
		name := iface.PredictableName
		if name == "" {
			name = iface.CurrentName
		}

		b.WriteString("\n")
		fmt.Fprintf(&b, "auto %s\n", name)

		if iface.Method == methodDHCP {
			fmt.Fprintf(&b, "iface %s inet dhcp\n", name)

			continue
		}

		fmt.Fprintf(&b, "iface %s inet static\n", name)

		if iface.IPv4 != "" {
			fmt.Fprintf(&b, "    address %s/%d\n", iface.IPv4, iface.Prefix)
		}

		if iface.Gateway != "" {
			fmt.Fprintf(&b, "    gateway %s\n", iface.Gateway)
		}
	}

	return b.String()
}

// migrateState is the per-chain shared state passed by pointer into each
// runner. Runners after detectNetworkManager check nmPresent and return
// early when it is false.
type migrateState struct {
	nmPresent      bool
	interfaces     []ifaceState
	loOnly         bool
	loOnlyComputed bool
	backedUp       bool
	backupPath     string
	wroteFile      bool
}

// checkLoOnly returns whether /etc/network/interfaces is missing or lo-only,
// computing the value the first time it is called and caching the result
// for subsequent calls within the same chain run.
func (s *migrateState) checkLoOnly() (bool, error) {
	if s.loOnlyComputed {
		return s.loOnly, nil
	}

	v, err := isLoOnly(interfacesPath)
	if err != nil {
		return false, err
	}

	s.loOnly = v
	s.loOnlyComputed = true

	return v, nil
}

// detectNetworkManager checks whether the network-manager package is
// installed via dpkg-query. Sets state.nmPresent. dpkg-query exit code 1
// (package unknown) is the expected absent signal; other failures are
// logged as warnings but still treated as absent so the chain can proceed
// on a box with no NM at all.
type detectNetworkManager struct {
	state *migrateState
}

func (r *detectNetworkManager) Name() string { return "detectNetworkManager" }

func (r *detectNetworkManager) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, dpkgQueryBin,
		[]string{"-W", "-f=${Status}", "network-manager"})
	if err != nil {
		var cerr *util.CommandError
		if !errors.As(err, &cerr) || cerr.ExitCode != 1 {
			slog.WarnContext(ctx, "dpkg-query failed unexpectedly; treating NM as absent", "err", err)
		}
	}

	r.state.nmPresent = strings.Contains(out, "install ok installed")

	slog.InfoContext(ctx, "NetworkManager presence", "present", r.state.nmPresent)

	return r.state.nmPresent, nil
}

func (r *detectNetworkManager) Rollback(_ context.Context) error { return nil }

// discoverInterfaces inspects the live IPv4 state and populates
// state.interfaces. Reads `ip -j addr show`, `ip -j route show default`,
// `udevadm info -q property` per interface, and `nmcli` per interface.
// Skips work and returns nil when state.nmPresent is false.
type discoverInterfaces struct {
	state *migrateState
}

func (r *discoverInterfaces) Name() string { return "discoverInterfaces" }

func (r *discoverInterfaces) Run(ctx context.Context) (any, error) {
	if !r.state.nmPresent {
		return skipped, nil
	}

	addrOut, _, err := util.ExecCommandOutput(ctx, ipBin, []string{"-j", "addr", "show"}) //nolint:goconst
	if err != nil {
		return nil, fmt.Errorf("ip addr show: %w", err)
	}

	ifaces, err := parseIPAddrJSON([]byte(addrOut))
	if err != nil {
		return nil, fmt.Errorf("parse ip addr: %w", err)
	}

	routeOut, _, err := util.ExecCommandOutput(ctx, ipBin, []string{"-j", "route", "show", "default"})
	if err != nil {
		// No default route is not fatal — interfaces still get written
		// without a gateway directive.
		slog.WarnContext(ctx, "no default route", "err", err)
	} else if gw, dev, ok := parseDefaultRouteJSON([]byte(routeOut)); ok {
		for i := range ifaces {
			if ifaces[i].CurrentName == dev {
				ifaces[i].Gateway = gw
			}
		}
	}

	for i := range ifaces {
		ifaces[i].PredictableName = predictableNameFor(ctx, ifaces[i].CurrentName)
		ifaces[i].Method = nmcliMethodFor(ctx, ifaces[i].CurrentName)
	}

	r.state.interfaces = ifaces

	slog.InfoContext(ctx, "discovered interfaces", "count", len(ifaces))

	return ifaces, nil
}

func (r *discoverInterfaces) Rollback(_ context.Context) error { return nil }

// predictableNameFor returns the post-reboot predictable name for iface.
// Falls back to the current name on any failure (with a WARN log).
func predictableNameFor(ctx context.Context, iface string) string {
	out, _, err := util.ExecCommandOutput(ctx, udevadmBin,
		[]string{"info", "-q", "property", "/sys/class/net/" + iface})
	if err != nil {
		slog.WarnContext(ctx, "udevadm info failed", "iface", iface, "err", err)

		return iface
	}

	if name, ok := parseUdevadmInfo([]byte(out)); ok {
		return name
	}

	slog.WarnContext(ctx, "no ID_NET_NAME_MAC; keeping current name", "iface", iface)

	return iface
}

// nmcliMethodFor returns "dhcp" or "static" for iface, based on the active
// NM connection's ipv4.method. Defaults to "static" on any failure.
func nmcliMethodFor(ctx context.Context, iface string) string {
	connOut, _, err := util.ExecCommandOutput(ctx, nmcliBin,
		[]string{"-t", "-g", "GENERAL.CONNECTION", "device", "show", iface})
	if err != nil {
		slog.WarnContext(ctx, "nmcli device show failed; defaulting to static", "iface", iface, "err", err)

		return methodStatic
	}

	conn := strings.TrimSpace(connOut)
	if conn == "" || conn == "--" {
		return methodStatic
	}

	methodOut, _, err := util.ExecCommandOutput(ctx, nmcliBin,
		[]string{"-t", "-g", "ipv4.method", "connection", "show", conn})
	if err != nil {
		slog.WarnContext(ctx, "nmcli connection show failed; defaulting to static", "iface", iface, "err", err)

		return methodStatic
	}

	return parseNmcliMethod([]byte(methodOut))
}

// backupInterfaces copies /etc/network/interfaces to a timestamped sibling
// when the file exists with content beyond lo. Rollback is a no-op: the
// backup is kept as a recovery point even if a later runner fails.
type backupInterfaces struct {
	state *migrateState
}

func (r *backupInterfaces) Name() string { return "backupInterfaces" }

func (r *backupInterfaces) Run(ctx context.Context) (any, error) {
	if !r.state.nmPresent {
		return skipped, nil
	}

	if r.state.backedUp {
		return r.state.backupPath, nil
	}

	loOnly, err := r.state.checkLoOnly()
	if err != nil {
		return nil, fmt.Errorf("check %s: %w", interfacesPath, err)
	}

	if loOnly {
		// Either the file is missing or it only declares lo; nothing
		// worth backing up.
		return "no backup needed", nil
	}

	dst := fmt.Sprintf("%s.bak-%d", interfacesPath, time.Now().Unix())
	if err := util.CopyFile(interfacesPath, dst); err != nil {
		return nil, fmt.Errorf("backup %s -> %s: %w", interfacesPath, dst, err)
	}

	r.state.backedUp = true
	r.state.backupPath = dst

	slog.InfoContext(ctx, "backed up existing interfaces file", "path", dst)

	return dst, nil
}

func (r *backupInterfaces) Rollback(_ context.Context) error {
	// no op: keep the backup as a recovery point
	return nil
}

// writeInterfacesSafetyNet writes /etc/network/interfaces when the file is
// missing or lo-only. Operator-edited files (anything with a non-lo iface
// stanza) are left untouched. Rollback is a no-op: leaving the safety-net
// in place is safer than deleting it after a downstream failure, since by
// that point NM may already be stopped/disabled.
type writeInterfacesSafetyNet struct {
	state *migrateState
}

func (r *writeInterfacesSafetyNet) Name() string { return "writeInterfacesSafetyNet" }

func (r *writeInterfacesSafetyNet) Run(ctx context.Context) (any, error) {
	if !r.state.nmPresent {
		return skipped, nil
	}

	if r.state.wroteFile {
		return interfacesPath, nil
	}

	loOnly, err := r.state.checkLoOnly()
	if err != nil {
		return nil, fmt.Errorf("check %s: %w", interfacesPath, err)
	}

	if !loOnly {
		slog.InfoContext(ctx, "operator interfaces file present; not overwriting", "path", interfacesPath)

		return "operator file present", nil
	}

	body := renderInterfacesFile(r.state.interfaces, time.Now().UTC())

	if err := util.FileWriteStrings(interfacesPath, []string{body}); err != nil {
		return nil, fmt.Errorf("write %s: %w", interfacesPath, err)
	}

	r.state.wroteFile = true

	slog.InfoContext(ctx, "wrote safety-net interfaces file", "path", interfacesPath, "bytes", len(body))

	return interfacesPath, nil
}

func (r *writeInterfacesSafetyNet) Rollback(_ context.Context) error {
	// no op: keep the safety-net in place
	return nil
}

// nmServiceName is the systemd unit name for NetworkManager.
const nmServiceName = "NetworkManager"

// systemctlExitNoSuchFile and systemctlExitUnitNotLoaded are the systemctl
// exit codes that mean "the unit is already gone" — which is the success
// state for the teardown work in this file.
const (
	systemctlExitNoSuchFile    = 4
	systemctlExitUnitNotLoaded = 5
)

// isUnitGoneRC reports whether err is a CommandError carrying an exit code
// that means the systemd unit is already gone (rc=4 "no such file or
// directory" or rc=5 "unit not loaded"). Both are treated as success on
// re-runs after the unit has been purged.
func isUnitGoneRC(err error) bool {
	var cerr *util.CommandError
	if !errors.As(err, &cerr) {
		return false
	}

	return cerr.ExitCode == systemctlExitNoSuchFile || cerr.ExitCode == systemctlExitUnitNotLoaded
}

// tolerantRunner wraps any chain.Runner so that systemctl exit codes 4 ("no
// such file or directory") and 5 ("unit not loaded") are treated as
// success, and Run is skipped when state.nmPresent is false. This keeps
// the chain idempotent across re-invocations after NM has already been
// stopped/disabled/purged.
type tolerantRunner struct {
	inner chain.Runner
	state *migrateState
}

func (r *tolerantRunner) Name() string { return "tolerant." + r.inner.Name() }

func (r *tolerantRunner) Run(ctx context.Context) (any, error) {
	if !r.state.nmPresent {
		return skipped, nil
	}

	out, err := r.inner.Run(ctx)
	if err == nil {
		return out, nil
	}

	if isUnitGoneRC(err) {
		slog.InfoContext(ctx, "unit already absent; treating as success", "runner", r.inner.Name())

		return out, nil
	}

	return out, err
}

func (r *tolerantRunner) Rollback(ctx context.Context) error {
	if !r.state.nmPresent {
		return nil
	}

	return r.inner.Rollback(ctx)
}

// purgeNetworkManager runs apt-get purge -y --autoremove network-manager.
// Idempotent: purging an already-purged package is a no-op with -y.
// Rollback is a no-op (matches the project convention for apt installs).
type purgeNetworkManager struct {
	state *migrateState
}

func (r *purgeNetworkManager) Name() string { return "purgeNetworkManager" }

func (r *purgeNetworkManager) Run(ctx context.Context) (any, error) {
	if !r.state.nmPresent {
		return skipped, nil
	}

	out, _, err := util.ExecCommandOutput(ctx, aptgetBin,
		[]string{"purge", "-y", "--autoremove", "network-manager"})

	return out, err
}

func (r *purgeNetworkManager) Rollback(_ context.Context) error {
	// No-op: reinstalling NM mid-rollback is more destructive than the
	// failure itself. Same pattern as aptInstall.Rollback.
	return nil
}

// MigrateToIfupdown stops, disables, and purges NetworkManager, and writes
// a safety-net /etc/network/interfaces when the file is missing or lo-only.
// The first runner (detectNetworkManager) is the gate: if NM is absent,
// every other runner returns immediately and rebootRequired is false.
//
// rebootRequired is true when NM was present at the start of this run —
// the caller should emit a reboot banner because both the NM purge and the
// pending predictable-naming change require a reboot to fully take effect.
func MigrateToIfupdown(ctx context.Context) (rebootRequired bool, err error) {
	slog.InfoContext(ctx, "👉 migrating from NetworkManager to ifupdown")

	state := &migrateState{}

	err = chain.NewChain(retries, retryDelay,
		&detectNetworkManager{state: state},
		&discoverInterfaces{state: state},
		&backupInterfaces{state: state},
		&writeInterfacesSafetyNet{state: state},
		&tolerantRunner{inner: &stopService{service: nmServiceName}, state: state},
		&tolerantRunner{inner: &disableService{service: nmServiceName}, state: state},
		&purgeNetworkManager{state: state},
	).Execute(ctx)

	return state.nmPresent, err
}
