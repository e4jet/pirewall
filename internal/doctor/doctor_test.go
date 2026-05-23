//go:build unit

package doctor

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestColorize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		color    string
		text     string
		useColor bool
		want     string
	}{
		{"with green", colorGreen, "hello", true, colorGreen + "hello" + colorReset},
		{"no color passthrough", colorGreen, "hello", false, "hello"},
		{"with red", colorRed, "FAIL", true, colorRed + "FAIL" + colorReset},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := colorize(tc.color, tc.text, tc.useColor)
			if got != tc.want {
				t.Errorf("colorize(%q, %q, %v) = %q, want %q", tc.color, tc.text, tc.useColor, got, tc.want)
			}
		})
	}
}

func TestCheckServices(t *testing.T) {
	t.Parallel()
	// systemctl is-active exits 0 (nil error) only for active services.
	// Non-zero exit codes (inactive=3, unknown=4) surface as non-nil errors.
	tests := []struct {
		name     string
		execErr  error
		wantOK   bool
		wantHint bool
	}{
		{
			name:     "active shows OK no hint",
			execErr:  nil,
			wantOK:   true,
			wantHint: false,
		},
		{
			name:     "inactive shows FAIL with hint",
			execErr:  errors.New("exit status 3"),
			wantOK:   false,
			wantHint: true,
		},
		{
			name:     "unknown shows FAIL with hint",
			execErr:  errors.New("exit status 4"),
			wantOK:   false,
			wantHint: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const svc = "dnsmasq"
			r := &runner{
				services: []string{svc},
				execFn: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
					return nil, tc.execErr
				},
			}
			var buf bytes.Buffer
			if err := r.checkServices(context.Background(), &buf, false); err != nil {
				t.Fatalf("checkServices() unexpected error: %v", err)
			}
			out := buf.String()
			if tc.wantOK && !strings.Contains(out, "[OK]") {
				t.Errorf("want [OK] in output; got:\n%s", out)
			}
			if !tc.wantOK && !strings.Contains(out, "[FAIL]") {
				t.Errorf("want [FAIL] in output; got:\n%s", out)
			}
			hint := "journalctl -u " + svc
			if tc.wantHint && !strings.Contains(out, hint) {
				t.Errorf("want journalctl hint in output; got:\n%s", out)
			}
			if !tc.wantHint && strings.Contains(out, hint) {
				t.Errorf("unexpected journalctl hint in output; got:\n%s", out)
			}
		})
	}
}

func TestCheckIPTablesRoot(t *testing.T) {
	t.Parallel()
	calls := 0
	r := &runner{
		execFn: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			return []byte("Chain INPUT (policy ACCEPT)\n"), nil
		},
	}
	var buf bytes.Buffer
	if err := r.checkIPTables(context.Background(), &buf, false, true); err != nil {
		t.Fatalf("checkIPTables() unexpected error: %v", err)
	}
	if calls != 2 {
		t.Errorf("want 2 exec calls (iptables + ip6tables), got %d", calls)
	}
	out := buf.String()
	if !strings.Contains(out, "iptables") {
		t.Errorf("want iptables header in output; got:\n%s", out)
	}
}

func TestCheckIPTablesRootExecError(t *testing.T) {
	t.Parallel()
	calls := 0
	r := &runner{
		execFn: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			calls++
			return nil, errors.New("exec failed")
		},
	}
	var buf bytes.Buffer
	if err := r.checkIPTables(context.Background(), &buf, false, true); err != nil {
		t.Fatalf("checkIPTables() must not return error for exec failures; got: %v", err)
	}
	if calls != 2 {
		t.Errorf("want 2 exec calls (both binaries attempted), got %d", calls)
	}
	if !strings.Contains(buf.String(), "error") {
		t.Errorf("want exec error printed inline; got:\n%s", buf.String())
	}
}

func TestCheckNetworkSuccess(t *testing.T) {
	t.Parallel()
	r := &runner{
		execFn: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return []byte("1: lo: <LOOPBACK,UP>\n"), nil
		},
	}
	var buf bytes.Buffer
	if err := r.checkNetwork(context.Background(), &buf); err != nil {
		t.Fatalf("checkNetwork() unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "lo") {
		t.Errorf("want interface output in buf; got:\n%s", buf.String())
	}
}

func TestCheckNetworkExecError(t *testing.T) {
	t.Parallel()
	r := &runner{
		execFn: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, errors.New("ip not found")
		},
	}
	var buf bytes.Buffer
	if err := r.checkNetwork(context.Background(), &buf); err != nil {
		t.Fatalf("checkNetwork() must return nil for exec errors; got: %v", err)
	}
	if !strings.Contains(buf.String(), "error") {
		t.Errorf("want exec error printed inline; got:\n%s", buf.String())
	}
}

func TestCheckIPTablesNonRoot(t *testing.T) {
	t.Parallel()
	r := &runner{
		execFn: func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			t.Fatal("exec must not be called for non-root iptables check")
			return nil, nil
		},
	}
	var buf bytes.Buffer
	if err := r.checkIPTables(context.Background(), &buf, false, false); err != nil {
		t.Fatalf("checkIPTables() unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "skipped") {
		t.Errorf("want 'skipped' in non-root output; got:\n%s", buf.String())
	}
}

func TestParseDefaultRoute(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		in          string
		wantIface   string
		wantGateway string
		wantOK      bool
	}{
		{"empty input", "", "", "", false},
		{"typical line", "default via 192.168.1.1 dev eth0\n", "eth0", "192.168.1.1", true},
		{
			"line with extras",
			"default via 192.168.1.1 dev eth0 proto dhcp src 192.168.1.5 metric 100\n",
			"eth0", "192.168.1.1", true,
		},
		{"missing dev", "default via 192.168.1.1\n", "", "192.168.1.1", false},
		{"missing via", "default dev eth0\n", "eth0", "", false},
		{"leading blank lines", "\n\ndefault via 192.168.1.1 dev eth0\n", "eth0", "192.168.1.1", true},
		{
			"multiple default routes — first wins",
			"default via 192.168.1.1 dev eth0 metric 100\ndefault via 10.0.0.1 dev eth1 metric 200\n",
			"eth0", "192.168.1.1", true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			iface, gw, ok := parseDefaultRoute([]byte(tc.in))
			if ok != tc.wantOK {
				t.Errorf("ok = %v, want %v", ok, tc.wantOK)
			}
			if ok {
				if iface != tc.wantIface {
					t.Errorf("iface = %q, want %q", iface, tc.wantIface)
				}
				if gw != tc.wantGateway {
					t.Errorf("gateway = %q, want %q", gw, tc.wantGateway)
				}
			}
		})
	}
}

func TestParseInetAddr(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"no inet field", "2: eth0 link/ether aa:bb:cc:dd:ee:ff brd ff:ff:ff:ff:ff:ff\n", ""},
		{
			"typical -o output",
			"2: eth0    inet 192.168.1.5/24 brd 192.168.1.255 scope global eth0\n",
			"192.168.1.5",
		},
		{
			"multi-line output (non -o)",
			"2: eth0:\n    link/ether aa:bb:cc:dd:ee:ff\n    inet 10.0.0.1/8 scope global eth0\n",
			"10.0.0.1",
		},
		{
			"first inet wins when multiple",
			"2: eth0    inet 1.2.3.4/24 ... inet 5.6.7.8/24 ...\n",
			"1.2.3.4",
		},
		{
			"address without /NN prefix",
			"2: eth0    inet 192.168.1.5 brd 192.168.1.255\n",
			"192.168.1.5",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := parseInetAddr([]byte(tc.in))
			if got != tc.want {
				t.Errorf("parseInetAddr(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestCheckPublicNetwork_HappyPath(t *testing.T) {
	t.Parallel()
	r := &runner{
		execFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			cmd := name + " " + strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "route show default"):
				return []byte("default via 192.168.1.1 dev eth0 proto dhcp src 192.168.1.5 metric 100\n"), nil
			case strings.Contains(cmd, "addr show dev"):
				return []byte("2: eth0    inet 192.168.1.5/24 brd 192.168.1.255 scope global eth0\n"), nil
			case strings.Contains(cmd, "ping"):
				return []byte("PING ok\n"), nil
			}
			t.Fatalf("unexpected exec: %s", cmd)
			return nil, nil
		},
		lookupFn: func(_ context.Context, _ string) ([]string, error) {
			return []string{"93.184.216.34"}, nil
		},
	}
	var buf bytes.Buffer
	if err := r.checkPublicNetwork(context.Background(), &buf, false, true); err != nil {
		t.Fatalf("checkPublicNetwork() unexpected error: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"[OK]", "eth0", "192.168.1.5", "192.168.1.1", "example.com", "93.184.216.34", "1.1.1.1:53"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %q in output; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[FAIL]") {
		t.Errorf("unexpected [FAIL] in happy path; got:\n%s", out)
	}
}

func TestCheckPublicNetwork_NoDefaultRoute(t *testing.T) {
	t.Parallel()
	calls := 0
	r := &runner{
		execFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			calls++
			cmd := name + " " + strings.Join(args, " ")
			if strings.Contains(cmd, "route show default") {
				return []byte(""), nil
			}
			t.Fatalf("unexpected exec after WAN detection failed: %s", cmd)
			return nil, nil
		},
		lookupFn: func(_ context.Context, _ string) ([]string, error) {
			t.Fatal("lookupFn must not be called when WAN detection fails")
			return nil, nil
		},
	}
	var buf bytes.Buffer
	if err := r.checkPublicNetwork(context.Background(), &buf, false, true); err != nil {
		t.Fatalf("checkPublicNetwork() unexpected error: %v", err)
	}
	if calls != 1 {
		t.Errorf("want exactly 1 exec call (ip route only), got %d", calls)
	}
	out := buf.String()
	if !strings.Contains(out, "[FAIL]") || !strings.Contains(out, "no default route") {
		t.Errorf("want [FAIL] with 'no default route'; got:\n%s", out)
	}
}

func TestCheckPublicNetwork_NonRoot_PingSkipped(t *testing.T) {
	t.Parallel()
	pingCalls := 0
	lookupCalls := 0
	r := &runner{
		execFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			cmd := name + " " + strings.Join(args, " ")
			switch {
			case strings.Contains(cmd, "route show default"):
				return []byte("default via 192.168.1.1 dev eth0\n"), nil
			case strings.Contains(cmd, "addr show dev"):
				return []byte("2: eth0    inet 192.168.1.5/24\n"), nil
			case strings.Contains(cmd, "ping"):
				pingCalls++
				return nil, nil
			}
			t.Fatalf("unexpected exec: %s", cmd)
			return nil, nil
		},
		lookupFn: func(_ context.Context, _ string) ([]string, error) {
			lookupCalls++
			return []string{"93.184.216.34"}, nil
		},
	}
	var buf bytes.Buffer
	if err := r.checkPublicNetwork(context.Background(), &buf, false, false); err != nil {
		t.Fatalf("checkPublicNetwork() unexpected error: %v", err)
	}
	if pingCalls != 0 {
		t.Errorf("want 0 ping calls as non-root, got %d", pingCalls)
	}
	if lookupCalls != 1 {
		t.Errorf("want 1 DNS lookup, got %d", lookupCalls)
	}
	if !strings.Contains(buf.String(), "found") {
		t.Errorf("want found in output; got:\n%s", buf.String())
	}
}
