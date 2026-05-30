//go:build unit

/*
(c) Copyright 2023 Eric Paul Forgette

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package configure

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/e4jet/pirewall/internal/util"
)

func TestParseIPAddrJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		want    []ifaceState
		wantErr bool
	}{
		{
			name: "empty array",
			in:   `[]`,
			want: nil,
		},
		{
			name: "lo excluded",
			in: `[
				{"ifname":"lo","address":"00:00:00:00:00:00","addr_info":[
					{"family":"inet","local":"127.0.0.1","prefixlen":8}
				]}
			]`,
			want: nil,
		},
		{
			name: "single interface with ipv4",
			in: `[
				{"ifname":"eth0","address":"6c:6e:07:17:9f:aa","addr_info":[
					{"family":"inet","local":"192.168.1.10","prefixlen":24}
				]}
			]`,
			want: []ifaceState{
				{CurrentName: "eth0", MAC: "6c:6e:07:17:9f:aa", IPv4: "192.168.1.10", Prefix: 24},
			},
		},
		{
			name: "interface with no ipv4",
			in: `[
				{"ifname":"eth1","address":"aa:bb:cc:dd:ee:ff","addr_info":[]}
			]`,
			want: []ifaceState{
				{CurrentName: "eth1", MAC: "aa:bb:cc:dd:ee:ff"},
			},
		},
		{
			name: "vlan excluded",
			in: `[
				{"ifname":"vlan2","address":"6c:6e:07:17:9f:aa","addr_info":[
					{"family":"inet","local":"10.0.2.1","prefixlen":24}
				]}
			]`,
			want: nil,
		},
		{
			name: "lo + real interface yields only real",
			in: `[
				{"ifname":"lo","address":"00:00:00:00:00:00","addr_info":[
					{"family":"inet","local":"127.0.0.1","prefixlen":8}
				]},
				{"ifname":"eth0","address":"6c:6e:07:17:9f:aa","addr_info":[
					{"family":"inet","local":"192.168.1.10","prefixlen":24}
				]}
			]`,
			want: []ifaceState{
				{CurrentName: "eth0", MAC: "6c:6e:07:17:9f:aa", IPv4: "192.168.1.10", Prefix: 24},
			},
		},
		{
			name:    "malformed json",
			in:      `not json`,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseIPAddrJSON([]byte(tc.in))

			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(got) != len(tc.want) {
				t.Fatalf("len mismatch: got %d, want %d (got=%+v want=%+v)", len(got), len(tc.want), got, tc.want)
			}

			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("idx %d: got %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestParseDefaultRouteJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      string
		wantGW  string
		wantDev string
		wantOK  bool
	}{
		{
			name:   "no default route",
			in:     `[]`,
			wantOK: false,
		},
		{
			name:    "single default route",
			in:      `[{"dst":"default","gateway":"192.168.1.1","dev":"eth0","protocol":"dhcp"}]`,
			wantGW:  "192.168.1.1",
			wantDev: "eth0",
			wantOK:  true,
		},
		{
			name: "multiple routes, lowest metric wins",
			in: `[
				{"dst":"default","gateway":"10.0.0.1","dev":"eth1","metric":200},
				{"dst":"default","gateway":"192.168.1.1","dev":"eth0","metric":100}
			]`,
			wantGW:  "192.168.1.1",
			wantDev: "eth0",
			wantOK:  true,
		},
		{
			name:   "ipv6 only default route ignored",
			in:     `[{"dst":"default","gateway":"fe80::1","dev":"eth0","family":"inet6"}]`,
			wantOK: false,
		},
		{
			name:   "malformed json",
			in:     `nope`,
			wantOK: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gw, dev, ok := parseDefaultRouteJSON([]byte(tc.in))

			if ok != tc.wantOK {
				t.Fatalf("ok: got %v want %v", ok, tc.wantOK)
			}

			if !ok {
				return
			}

			if gw != tc.wantGW {
				t.Errorf("gw: got %q want %q", gw, tc.wantGW)
			}

			if dev != tc.wantDev {
				t.Errorf("dev: got %q want %q", dev, tc.wantDev)
			}
		})
	}
}

func TestParseUdevadmInfo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		wantName string
		wantOK   bool
	}{
		{
			name: "happy path",
			in: `E: DEVPATH=/devices/platform/...
E: ID_NET_NAME_MAC=enx6c6e07179faa
E: ID_NET_NAME_PATH=enp0s3
E: ID_BUS=usb
`,
			wantName: "enx6c6e07179faa",
			wantOK:   true,
		},
		{
			name: "no ID_NET_NAME_MAC",
			in: `E: DEVPATH=/devices/platform/...
E: ID_BUS=pci
`,
			wantOK: false,
		},
		{
			name:   "empty input",
			in:     ``,
			wantOK: false,
		},
		{
			name: "key without E: prefix is accepted too",
			in: `ID_NET_NAME_MAC=enxabcdef012345
`,
			wantName: "enxabcdef012345",
			wantOK:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name, ok := parseUdevadmInfo([]byte(tc.in))

			if ok != tc.wantOK {
				t.Fatalf("ok: got %v want %v", ok, tc.wantOK)
			}

			if !ok {
				return
			}

			if name != tc.wantName {
				t.Errorf("name: got %q want %q", name, tc.wantName)
			}
		})
	}
}

func TestParseNmcliMethod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "auto means dhcp", in: "auto\n", want: methodDHCP},
		{name: "manual means static", in: "manual\n", want: methodStatic},
		{name: "disabled means static", in: "disabled\n", want: methodStatic},
		{name: "leading/trailing whitespace tolerated", in: "  auto  \n", want: methodDHCP},
		{name: "empty output defaults to static", in: "", want: methodStatic},
		{name: "whitespace-only output defaults to static", in: "   \n", want: methodStatic},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseNmcliMethod([]byte(tc.in)); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestIsLoOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content *string // nil means file should not be created
		want    bool
	}{
		{name: "missing file is lo-only", content: nil, want: true},
		{name: "empty file is lo-only", content: ptr(""), want: true},
		{
			name: "only lo stanza is lo-only",
			content: ptr(`auto lo
iface lo inet loopback
`),
			want: true,
		},
		{
			name: "comments and whitespace ignored",
			content: ptr(`# a comment

auto lo
iface lo inet loopback

# another comment
`),
			want: true,
		},
		{
			name: "real iface stanza is not lo-only",
			content: ptr(`auto lo
iface lo inet loopback

auto eth0
iface eth0 inet dhcp
`),
			want: false,
		},
		{
			name:    "real iface only is not lo-only",
			content: ptr(`iface end0 inet dhcp` + "\n"),
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := dir + "/interfaces"

			if tc.content != nil {
				if err := os.WriteFile(path, []byte(*tc.content), 0o644); err != nil {
					t.Fatalf("seed file: %v", err)
				}
			}

			got, err := isLoOnly(path)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}

			if got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestRenderInterfacesFile(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 5, 25, 14, 32, 1, 0, time.UTC)

	ifaces := []ifaceState{
		{
			CurrentName:     "eth0",
			PredictableName: "end0",
			IPv4:            "10.10.10.1",
			Prefix:          24,
			Method:          methodStatic,
		},
		{
			CurrentName:     "eth1",
			PredictableName: "enx6c6e07179faa",
			IPv4:            "192.168.1.42",
			Prefix:          24,
			Gateway:         "192.168.1.1",
			Method:          methodDHCP,
		},
	}

	want := `# Auto-generated by pirewall -config on 2026-05-25T14:32:01Z.
# Safety-net file mirroring NetworkManager-driven state at install time.
# Edit freely; pirewall will not overwrite this file on subsequent runs.

auto lo
iface lo inet loopback

auto end0
iface end0 inet static
    address 10.10.10.1/24

auto enx6c6e07179faa
iface enx6c6e07179faa inet dhcp
`

	got := renderInterfacesFile(ifaces, ts)
	if got != want {
		t.Errorf("render mismatch:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestIsUnitGoneRC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil err is not gone", err: nil, want: false},
		{name: "plain error is not gone", err: errors.New("boom"), want: false},
		{name: "CommandError rc=4 is gone", err: &util.CommandError{Cmd: "systemctl", ExitCode: 4, Err: errors.New("x")}, want: true},
		{name: "CommandError rc=5 is gone", err: &util.CommandError{Cmd: "systemctl", ExitCode: 5, Err: errors.New("x")}, want: true},
		{name: "CommandError rc=1 is not gone", err: &util.CommandError{Cmd: "systemctl", ExitCode: 1, Err: errors.New("x")}, want: false},
		{
			name: "wrapped CommandError rc=4 is gone",
			err:  fmt.Errorf("outer: %w", &util.CommandError{Cmd: "systemctl", ExitCode: 4, Err: errors.New("x")}),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isUnitGoneRC(tc.err); got != tc.want {
				t.Errorf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestRenderInterfacesFile_StaticWithGateway(t *testing.T) {
	t.Parallel()

	ts := time.Date(2026, 5, 25, 14, 32, 1, 0, time.UTC)

	ifaces := []ifaceState{
		{
			CurrentName:     "eth0",
			PredictableName: "enx112233445566",
			IPv4:            "203.0.113.10",
			Prefix:          24,
			Gateway:         "203.0.113.1",
			Method:          methodStatic,
		},
	}

	want := `# Auto-generated by pirewall -config on 2026-05-25T14:32:01Z.
# Safety-net file mirroring NetworkManager-driven state at install time.
# Edit freely; pirewall will not overwrite this file on subsequent runs.

auto lo
iface lo inet loopback

auto enx112233445566
iface enx112233445566 inet static
    address 203.0.113.10/24
    gateway 203.0.113.1
`

	got := renderInterfacesFile(ifaces, ts)
	if got != want {
		t.Errorf("render mismatch:\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}
