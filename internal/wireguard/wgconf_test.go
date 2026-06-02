//go:build unit

/*
(c) Copyright 2026 Eric Paul Forgette

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package wireguard

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseServerConf(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want *serverConf
	}{
		{
			name: "server only with endpoint",
			in: `# pirewall WireGuard server
#
# Endpoint = home.example.org:51820

[Interface]
Address = 192.168.192.1/24
ListenPort = 51820
PrivateKey = SERVERPRIVKEY=
`,
			want: &serverConf{
				Endpoint:   "home.example.org:51820",
				Address:    "192.168.192.1/24",
				ListenPort: 51820,
				PrivateKey: "SERVERPRIVKEY=",
			},
		},
		{
			name: "server with one peer",
			in: `# pirewall WireGuard server
# Endpoint = home.example.org:51820

[Interface]
Address = 192.168.192.1/24
ListenPort = 51820
PrivateKey = SERVERPRIVKEY=

# Peer: alice (added 2026-05-31)
[Peer]
PublicKey = ALICEPUBKEY=
AllowedIPs = 192.168.192.2/32
`,
			want: &serverConf{
				Endpoint:   "home.example.org:51820",
				Address:    "192.168.192.1/24",
				ListenPort: 51820,
				PrivateKey: "SERVERPRIVKEY=",
				Peers: []peerEntry{
					{Name: "alice", AddedDate: "2026-05-31", PublicKey: "ALICEPUBKEY=", AllowedIPs: "192.168.192.2/32"},
				},
			},
		},
		{
			name: "server with multiple peers",
			in: `# Endpoint = home.example.org:51820

[Interface]
Address = 192.168.192.1/24
ListenPort = 51820
PrivateKey = SERVERPRIVKEY=

# Peer: alice (added 2026-05-31)
[Peer]
PublicKey = ALICEPUBKEY=
AllowedIPs = 192.168.192.2/32

# Peer: bob (added 2026-06-01)
[Peer]
PublicKey = BOBPUBKEY=
AllowedIPs = 192.168.192.3/32
`,
			want: &serverConf{
				Endpoint:   "home.example.org:51820",
				Address:    "192.168.192.1/24",
				ListenPort: 51820,
				PrivateKey: "SERVERPRIVKEY=",
				Peers: []peerEntry{
					{Name: "alice", AddedDate: "2026-05-31", PublicKey: "ALICEPUBKEY=", AllowedIPs: "192.168.192.2/32"},
					{Name: "bob", AddedDate: "2026-06-01", PublicKey: "BOBPUBKEY=", AllowedIPs: "192.168.192.3/32"},
				},
			},
		},
		{
			name: "no endpoint comment leaves Endpoint empty",
			in: `[Interface]
Address = 192.168.192.1/24
ListenPort = 51820
PrivateKey = SERVERPRIVKEY=
`,
			want: &serverConf{
				Address:    "192.168.192.1/24",
				ListenPort: 51820,
				PrivateKey: "SERVERPRIVKEY=",
			},
		},
		{
			name: "whitespace around equals tolerated",
			in: `# Endpoint=home.example.org:51820

[Interface]
Address   =   192.168.192.1/24
ListenPort=51820
PrivateKey =SERVERPRIVKEY=
`,
			want: &serverConf{
				Endpoint:   "home.example.org:51820",
				Address:    "192.168.192.1/24",
				ListenPort: 51820,
				PrivateKey: "SERVERPRIVKEY=",
			},
		},
		{
			name: "peer block without preceding comment is captured with empty Name",
			in: `# Endpoint = home.example.org:51820

[Interface]
Address = 192.168.192.1/24
ListenPort = 51820
PrivateKey = SERVERPRIVKEY=

[Peer]
PublicKey = ORPHANPUBKEY=
AllowedIPs = 192.168.192.9/32
`,
			want: &serverConf{
				Endpoint:   "home.example.org:51820",
				Address:    "192.168.192.1/24",
				ListenPort: 51820,
				PrivateKey: "SERVERPRIVKEY=",
				Peers: []peerEntry{
					{PublicKey: "ORPHANPUBKEY=", AllowedIPs: "192.168.192.9/32"},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseServerConf([]byte(tc.in))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("parse mismatch\n  got: %+v\n  want: %+v", got, tc.want)
			}
		})
	}
}

func TestParseServerConf_MalformedListenPort(t *testing.T) {
	t.Parallel()

	in := `# Endpoint = home.example.org:51820

[Interface]
Address = 192.168.192.1/24
ListenPort = not-a-number
PrivateKey = SERVERPRIVKEY=
`

	if _, err := parseServerConf([]byte(in)); err == nil {
		t.Fatalf("want error for non-numeric ListenPort, got nil")
	}
}

func TestHasPeerNamed(t *testing.T) {
	t.Parallel()

	s := &serverConf{
		Peers: []peerEntry{
			{Name: "alice"},
			{Name: "bob"},
		},
	}

	cases := []struct {
		name string
		want bool
	}{
		{"alice", true},
		{"bob", true},
		{"carol", false},
		{"Alice", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.hasPeerNamed(tc.name); got != tc.want {
				t.Errorf("hasPeerNamed(%q): got %v want %v", tc.name, got, tc.want)
			}
		})
	}
}

func TestPickFreeIP(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		s    *serverConf
		want string
		err  error
	}{
		{
			name: "no peers picks 2",
			s:    &serverConf{},
			want: "192.168.192.2",
		},
		{
			name: ".2 taken picks 3",
			s: &serverConf{Peers: []peerEntry{
				{AllowedIPs: "192.168.192.2/32"},
			}},
			want: "192.168.192.3",
		},
		{
			name: ".2 and .3 taken picks 4",
			s: &serverConf{Peers: []peerEntry{
				{AllowedIPs: "192.168.192.2/32"},
				{AllowedIPs: "192.168.192.3/32"},
			}},
			want: "192.168.192.4",
		},
		{
			name: "gap at .5 picks 5",
			s: &serverConf{Peers: []peerEntry{
				{AllowedIPs: "192.168.192.2/32"},
				{AllowedIPs: "192.168.192.3/32"},
				{AllowedIPs: "192.168.192.4/32"},
				{AllowedIPs: "192.168.192.6/32"},
			}},
			want: "192.168.192.5",
		},
		{
			name: ".1 never returned (server)",
			s:    &serverConf{},
			want: "192.168.192.2",
		},
		{
			name: "malformed AllowedIPs entries ignored",
			s: &serverConf{Peers: []peerEntry{
				{AllowedIPs: ""},
				{AllowedIPs: "not-an-ip"},
				{AllowedIPs: "10.0.0.1/32"},
				{AllowedIPs: "192.168.192.2/32"},
			}},
			want: "192.168.192.3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.s.pickFreeIP()
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("err: got %v want %v", err, tc.err)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestPickFreeIP_Exhausted(t *testing.T) {
	t.Parallel()

	var peers []peerEntry
	for i := 2; i <= 254; i++ {
		peers = append(peers, peerEntry{AllowedIPs: fmt.Sprintf("192.168.192.%d/32", i)})
	}

	s := &serverConf{Peers: peers}

	_, err := s.pickFreeIP()
	if !errors.Is(err, ErrSubnetExhausted) {
		t.Fatalf("want ErrSubnetExhausted, got %v", err)
	}
}

func TestLoadServerConf_RoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg0.conf")
	pubPath := filepath.Join(dir, "publickey")

	original := &serverConf{
		Endpoint:   "home.example.org:51820",
		Address:    "192.168.192.1/24",
		ListenPort: 51820,
		PrivateKey: "SERVERPRIVKEY=",
		Peers: []peerEntry{
			{Name: "alice", AddedDate: "2026-05-31", PublicKey: "ALICEPUBKEY=", AllowedIPs: "192.168.192.2/32"},
		},
	}

	if err := writeServerConf(confPath, original); err != nil {
		t.Fatalf("writeServerConf: %v", err)
	}

	if err := os.WriteFile(pubPath, []byte("SERVERPUBKEY=\n"), 0o644); err != nil {
		t.Fatalf("seed publickey: %v", err)
	}

	loaded, err := loadServerConf(confPath, pubPath)
	if err != nil {
		t.Fatalf("loadServerConf: %v", err)
	}

	// loadServerConf populates PublicKey from the file.
	want := *original
	want.PublicKey = "SERVERPUBKEY="

	if !reflect.DeepEqual(loaded, &want) {
		t.Errorf("round-trip mismatch\n  got: %+v\n  want: %+v", loaded, &want)
	}

	// File mode of wg0.conf must be 0600.
	info, err := os.Stat(confPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: got %o want 0600", info.Mode().Perm())
	}
}

func TestLoadServerConf_MissingFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg0.conf") // not created
	pubPath := filepath.Join(dir, "publickey")

	if err := os.WriteFile(pubPath, []byte("SERVERPUBKEY=\n"), 0o644); err != nil {
		t.Fatalf("seed publickey: %v", err)
	}

	_, err := loadServerConf(confPath, pubPath)
	if !errors.Is(err, ErrServerNotConfigured) {
		t.Fatalf("want ErrServerNotConfigured, got %v", err)
	}
}

func TestAppendPeer(t *testing.T) {
	t.Parallel()

	s := &serverConf{}
	s.appendPeer(peerEntry{Name: "alice", AllowedIPs: "192.168.192.2/32"})
	s.appendPeer(peerEntry{Name: "bob", AllowedIPs: "192.168.192.3/32"})

	if len(s.Peers) != 2 || s.Peers[0].Name != "alice" || s.Peers[1].Name != "bob" {
		t.Errorf("appendPeer: got %+v", s.Peers)
	}
}
