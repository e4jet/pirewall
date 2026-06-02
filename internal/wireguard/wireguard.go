/*
(c) Copyright 2026 Eric Paul Forgette

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

// Package wireguard configures a WireGuard server (wg0) on a pirewall host
// and manages its client peers. The package is invoked via the -wireguard
// and -wg-add-client flags of the pirewall binary; runtime traffic is
// handled by the in-kernel WireGuard module driven by wg-quick@wg0.
package wireguard

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	wgDir         = "/etc/wireguard"
	wgConfPath    = "/etc/wireguard/wg0.conf"
	wgPrivKeyPath = "/etc/wireguard/privatekey"
	wgPubKeyPath  = "/etc/wireguard/publickey"
	wgClientsDir  = "/etc/wireguard/clients"

	wgBin      = "/usr/bin/wg"
	wgQuickBin = "/usr/bin/wg-quick"
	// aptgetBin and systemctlBin are duplicated from internal/configure
	// rather than imported, because internal/configure's runners are
	// package-private and this package deliberately has no dependency on
	// it (see spec architecture section).
	aptgetBin    = "/usr/bin/apt-get"
	systemctlBin = "/usr/bin/systemctl"
	wgQuickUnit  = "wg-quick@wg0"

	// VPN subnet 192.168.192.0/24 — chosen during brainstorming to avoid
	// the common consumer-router defaults (192.168.0.x, 192.168.1.x).
	// wgSubnetPrefix and wgClientDNS share the address space of
	// wgServerAddrCIDR; if the subnet ever changes, update all three.
	wgServerAddrCIDR = "192.168.192.1/24"
	wgSubnetPrefix   = "192.168.192." // keep in sync with wgServerAddrCIDR
	wgServerHost     = 1
	wgFirstClient    = 2
	wgLastClient     = 254
	wgListenPort     = 51820
	wgClientDNS      = "192.168.192.1" // server address; keep in sync with wgServerAddrCIDR
	wgKeepalive      = 25

	dirMode    = 0o700
	privMode   = 0o600
	confMode   = 0o600
	pubMode    = 0o644
	clientMode = 0o600

	retries    = 2
	retryDelay = 5 * time.Second

	clientNameMaxLen = 32
)

// Sentinel errors. Per project guidelines, control flow uses errors.Is /
// errors.As against these rather than string matching.
var (
	ErrEndpointRequired       = errors.New("wireguard: -wg-endpoint=host:port required on first run")
	ErrEndpointMismatch       = errors.New("wireguard: -wg-endpoint does not match existing wg0.conf endpoint")
	ErrServerNotConfigured    = errors.New("wireguard: wg0.conf not found; run -wireguard first")
	ErrServerKeysInconsistent = errors.New("wireguard: only one of privatekey/publickey present")
	ErrClientExists           = errors.New("wireguard: client with that name already exists")
	ErrSubnetExhausted        = errors.New("wireguard: no free IPs in VPN subnet")
	ErrInvalidClientName      = errors.New("wireguard: client name must match [A-Za-z0-9_-]+, 1-32 chars")
	ErrInvalidEndpoint        = errors.New("wireguard: endpoint must be host:port with port 1-65535")
)

// ConfigureOptions is the input to Configure. Endpoint is required on the
// first run (when wg0.conf does not exist) and optional on subsequent
// idempotent runs.
type ConfigureOptions struct {
	Endpoint string // host:port
}

// AddClientOptions is the input to AddClient. Name must match
// [A-Za-z0-9_-]{1,32} (see ErrInvalidClientName); validation happens
// inside AddClient.
type AddClientOptions struct {
	Name string
}

// validateEndpoint reports whether s is a non-empty host:port with a port
// in 1..65535. Returns ErrInvalidEndpoint with a descriptive wrapping
// otherwise.
func validateEndpoint(s string) error {
	host, port, found := strings.Cut(s, ":")
	if !found || host == "" || port == "" {
		return fmt.Errorf("%w: %q", ErrInvalidEndpoint, s)
	}

	p, err := strconv.Atoi(port)
	if err != nil || p < 1 || p > 65535 {
		return fmt.Errorf("%w: %q", ErrInvalidEndpoint, s)
	}

	return nil
}

// isClientNameRune reports whether r is allowed in a client name
// ([A-Za-z0-9_-]).
func isClientNameRune(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') || r == '_' || r == '-'
}

// validateClientName reports whether s matches ^[A-Za-z0-9_-]{1,32}$.
// Returns ErrInvalidClientName otherwise.
func validateClientName(s string) error {
	if s == "" || len(s) > clientNameMaxLen {
		return fmt.Errorf("%w: %q", ErrInvalidClientName, s)
	}

	for _, r := range s {
		if !isClientNameRune(r) {
			return fmt.Errorf("%w: %q", ErrInvalidClientName, s)
		}
	}

	return nil
}
