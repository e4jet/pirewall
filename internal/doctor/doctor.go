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

package doctor

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	colorReset  = "\033[0m"
	colorGreen  = "\033[32m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"

	execTimeout = 30 * time.Second

	dnsResolver = "1.1.1.1:53"
	dnsTarget   = "example.com"
	dnsTimeout  = 5 * time.Second
)

var services = []string{
	"dnsmasq",
	"netfilter-persistent",
	"unattended-upgrades",
	"ddclient",
	"sshd",
	"systemd-networkd.service",
	"systemd-timesyncd.service",
}

type runner struct {
	services []string
	execFn   func(ctx context.Context, name string, args ...string) ([]byte, error)
	lookupFn func(ctx context.Context, host string) ([]string, error)
}

func colorize(color, text string, useColor bool) string {
	if !useColor {
		return text
	}

	return color + text + colorReset
}

func isColorTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}

	info, err := f.Stat()
	if err != nil {
		return false
	}

	return info.Mode()&os.ModeCharDevice != 0 && os.Getenv("TERM") != "dumb"
}

// Run prints a diagnostic report of services, iptables, and network interfaces to w.
func Run(ctx context.Context, w io.Writer) error {
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer

			return d.DialContext(ctx, "udp", dnsResolver)
		},
	}

	r := &runner{
		services: services,
		execFn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, execTimeout)
			defer cancel()

			return exec.CommandContext(ctx, name, args...).CombinedOutput()
		},
		lookupFn: func(ctx context.Context, host string) ([]string, error) {
			ctx, cancel := context.WithTimeout(ctx, dnsTimeout)
			defer cancel()

			return resolver.LookupHost(ctx, host)
		},
	}

	return r.run(ctx, w, isColorTerminal(w), os.Getuid() == 0)
}

func (r *runner) run(ctx context.Context, w io.Writer, useColor bool, isRoot bool) error {
	if err := r.checkNetwork(ctx, w); err != nil {
		return err
	}

	if err := r.checkIPTables(ctx, w, useColor, isRoot); err != nil {
		return err
	}

	if err := r.checkServices(ctx, w, useColor); err != nil {
		return err
	}

	return r.checkPublicNetwork(ctx, w, useColor, isRoot)
}

func (r *runner) checkServices(ctx context.Context, w io.Writer, useColor bool) error {
	if _, err := fmt.Fprintln(w, "\n=== 👉 Services 👈 ==="); err != nil {
		return err
	}

	for _, svc := range r.services {
		// systemctl is-active exits 0 only when the service is active.
		_, err := r.execFn(ctx, "/usr/bin/systemctl", "is-active", svc)
		active := err == nil

		var status string
		if active {
			status = colorize(colorGreen, "✅ [OK]  ", useColor) + svc
		} else {
			status = colorize(colorRed, "❌ [FAIL]", useColor) + " " + svc
		}

		if _, err := fmt.Fprintln(w, status); err != nil {
			return err
		}

		if !active {
			if _, err := fmt.Fprintf(w, "  → journalctl -u %s -n 50 --no-pager\n", svc); err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *runner) checkIPTables(ctx context.Context, w io.Writer, useColor bool, isRoot bool) error {
	if _, err := fmt.Fprintln(w, "\n=== 👉 iptables 👈 ==="); err != nil {
		return err
	}

	if !isRoot {
		err := printSkip(w, useColor, "[iptables]", "skipped — re-run as root to see firewall rules")
		return err
	}

	for _, bin := range []string{"/usr/sbin/iptables", "/usr/sbin/ip6tables"} {
		out, err := r.execFn(ctx, bin, "-L", "-n", "-v")
		if err != nil {
			if _, werr := fmt.Fprintf(w, "%s error: %v\n", bin, err); werr != nil {
				return werr
			}

			continue
		}

		if _, err := fmt.Fprintf(w, "--- %s ---\n%s\n", bin, out); err != nil {
			return err
		}
	}

	return nil
}

func (r *runner) checkNetwork(ctx context.Context, w io.Writer) error {
	if _, err := fmt.Fprintln(w, "\n=== 👉 Network Interfaces 👈 ==="); err != nil {
		return err
	}

	out, err := r.execFn(ctx, "/usr/sbin/ip", "addr", "show")
	if err != nil {
		if _, werr := fmt.Fprintf(w, "ip addr show error: %v\n", err); werr != nil {
			return werr
		}

		return nil
	}

	_, err = fmt.Fprintf(w, "%s\n", out)

	return err
}

// parseDefaultRoute extracts the interface and gateway from `ip -4 route show default`
// output. Returns ok=false if the first non-empty line is missing either "via <gw>"
// or "dev <iface>". When multiple default routes exist, the first line wins —
// `ip route show default` orders by metric (lowest first).
func parseDefaultRoute(out []byte) (iface, gateway string, ok bool) {
	for raw := range strings.SplitSeq(string(out), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		for i, f := range fields {
			switch f {
			case "via":
				if i+1 < len(fields) {
					gateway = fields[i+1]
				}
			case "dev":
				if i+1 < len(fields) {
					iface = fields[i+1]
				}
			}
		}

		// First non-empty line wins — see function doc.
		return iface, gateway, iface != "" && gateway != ""
	}

	return "", "", false
}

// parseInetAddr extracts the first IPv4 address from `ip -4 -o addr show dev <iface>`
// output. The address has its /NN prefix stripped. Returns "" if no inet field found.
func parseInetAddr(out []byte) string {
	fields := strings.Fields(string(out))
	for i, f := range fields {
		if f == "inet" && i+1 < len(fields) {
			addr := fields[i+1]
			if idx := strings.IndexByte(addr, '/'); idx > 0 {
				addr = addr[:idx]
			}

			return addr
		}
	}

	return ""
}

func printStatus(w io.Writer, useColor, ok bool, label, msg string) error {
	var prefix string
	if ok {
		prefix = colorize(colorGreen, "✅ [OK]  ", useColor)
	} else {
		prefix = colorize(colorRed, "❌ [FAIL]", useColor) + " "
	}

	_, err := fmt.Fprintf(w, "%s%s: %s\n", prefix, label, msg)

	return err
}

func printSkip(w io.Writer, useColor bool, label, msg string) error {
	prefix := colorize(colorYellow, "⏭️ [SKIP]", useColor) + " "
	_, err := fmt.Fprintf(w, "%s%s: %s\n", prefix, label, msg)

	return err
}

func (r *runner) checkPublicNetwork(ctx context.Context, w io.Writer, useColor, isRoot bool) error {
	if _, err := fmt.Fprintln(w, "\n=== 👉 Public Network 👈 ==="); err != nil {
		return err
	}

	routeOut, err := r.execFn(ctx, "/usr/sbin/ip", "-4", "route", "show", "default")
	if err != nil {
		return printStatus(w, useColor, false, "WAN interface", err.Error())
	}

	iface, gateway, ok := parseDefaultRoute(routeOut)
	if !ok {
		return printStatus(w, useColor, false, "WAN interface", "no default route")
	}

	if err := r.reportWANInterface(ctx, w, useColor, iface); err != nil {
		return err
	}

	if err := r.reportGateway(w, useColor, isRoot, gateway); err != nil {
		return err
	}

	return r.reportDNS(ctx, w, useColor)
}

func (r *runner) reportWANInterface(ctx context.Context, w io.Writer, useColor bool, iface string) error {
	out, err := r.execFn(ctx, "/usr/sbin/ip", "-4", "-o", "addr", "show", "dev", iface)
	if err != nil {
		return printStatus(w, useColor, false, "WAN interface", iface+" ("+err.Error()+")")
	}

	ip := parseInetAddr(out)
	if ip == "" {
		return printStatus(w, useColor, false, "WAN interface", iface+" (no IPv4)")
	}

	return printStatus(w, useColor, true, "WAN interface", iface+" ("+ip+")")
}

func (r *runner) reportGateway(w io.Writer, useColor, isRoot bool, gateway string) error {
	//	if !isRoot {
	//		return printSkip(w, useColor, "Gateway reachable", "skipped — requires root for ICMP")
	//	}

	//	if _, err := r.execFn(ctx, "/usr/bin/ping", "-c", "1", "-W", "2", "-n", gateway); err != nil {
	//		return printStatus(w, useColor, false, "Gateway reachable", gateway+" ("+err.Error()+")")
	//	}
	return printStatus(w, useColor, true, "Gateway found", gateway)
}

func (r *runner) reportDNS(ctx context.Context, w io.Writer, useColor bool) error {
	addrs, err := r.lookupFn(ctx, dnsTarget)
	if err != nil {
		return printStatus(w, useColor, false, "External DNS lookup", err.Error())
	}

	if len(addrs) == 0 {
		return printStatus(w, useColor, false, "External DNS lookup", "no addresses returned")
	}

	msg := dnsTarget + " → " + addrs[0] + " (via " + dnsResolver + ")"

	return printStatus(w, useColor, true, "External DNS lookup", msg)
}
