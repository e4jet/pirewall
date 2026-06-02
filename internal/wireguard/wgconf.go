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

package wireguard

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/e4jet/pirewall/internal/util"
)

// serverConf is the in-memory model of /etc/wireguard/wg0.conf. PublicKey
// is populated by loadServerConf from /etc/wireguard/publickey; it is not
// read from wg0.conf itself.
type serverConf struct {
	Endpoint   string // from "# Endpoint = " comment
	Address    string // [Interface] Address line, verbatim
	ListenPort int
	PrivateKey string // [Interface] PrivateKey line, verbatim
	PublicKey  string
	Peers      []peerEntry
}

// peerEntry is one [Peer] block in wg0.conf along with the "# Peer: name
// (added YYYY-MM-DD)" comment that precedes it.
type peerEntry struct {
	Name       string
	AddedDate  string // YYYY-MM-DD
	PublicKey  string
	AllowedIPs string // typically "192.168.192.N/32"
}

// parseState carries mutable parser state across lines.
type parseState struct {
	inInterface bool
	inPeer      bool
	pending     peerEntry // peer being assembled
	pendingHdr  peerEntry // name/date from the most recent "# Peer:" comment
	havePending bool
}

// flushPeer appends the in-progress peer to s.Peers if one is open.
func (st *parseState) flushPeer(s *serverConf) {
	if st.inPeer {
		s.Peers = append(s.Peers, st.pending)
		st.pending = peerEntry{}
		st.inPeer = false
	}
}

// handleSection processes "[Interface]" and "[Peer]" header lines.
func (st *parseState) handleSection(s *serverConf, line string) bool {
	switch line {
	case "[Interface]":
		st.flushPeer(s)
		st.inInterface = true

		return true
	case "[Peer]":
		st.flushPeer(s)
		st.inPeer = true
		st.inInterface = false

		if st.havePending {
			st.pending.Name = st.pendingHdr.Name
			st.pending.AddedDate = st.pendingHdr.AddedDate
			st.havePending = false
		}

		return true
	}

	return false
}

// parseServerConf parses /etc/wireguard/wg0.conf bytes into a *serverConf.
// The parser is line-oriented and lenient about whitespace around `=`.
// A missing "# Endpoint = " comment is not an error — the returned
// serverConf has Endpoint == "" and the caller decides whether to reject.
// A non-numeric ListenPort is an error.
func parseServerConf(data []byte) (*serverConf, error) {
	s := &serverConf{}
	st := &parseState{}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if err := parseLine(s, st, line); err != nil {
			return nil, err
		}
	}

	st.flushPeer(s)

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan wg0.conf: %w", err)
	}

	return s, nil
}

// parseLine processes a single trimmed line, updating s and st as needed.
func parseLine(s *serverConf, st *parseState, line string) error {
	// Endpoint comment: "# Endpoint = host:port" or "# Endpoint=host:port"
	if rest, ok := stripPrefix(line, "# Endpoint"); ok {
		if v, found := stripEquals(rest); found {
			s.Endpoint = v
		}

		return nil
	}

	// Peer header comment: "# Peer: <name> (added YYYY-MM-DD)"
	if rest, ok := stripPrefix(line, "# Peer:"); ok {
		st.pendingHdr = parsePeerHeader(rest)
		st.havePending = true

		return nil
	}

	// Blank lines and other comments are ignored.
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}

	// Section headers.
	if st.handleSection(s, line) {
		return nil
	}

	// Key = Value lines.
	key, val, ok := splitKV(line)
	if !ok {
		return nil
	}

	return applyKV(s, &st.pending, st.inInterface, st.inPeer, key, val)
}

// applyKV dispatches a parsed key/value pair to the appropriate struct field.
func applyKV(s *serverConf, p *peerEntry, inInterface, inPeer bool, key, val string) error {
	switch {
	case inInterface:
		return applyInterfaceKV(s, key, val)
	case inPeer:
		applyPeerKV(p, key, val)
	}

	return nil
}

// applyInterfaceKV applies a key/value pair to the [Interface] block of s.
func applyInterfaceKV(s *serverConf, key, val string) error {
	switch key {
	case "Address":
		s.Address = val
	case "ListenPort":
		p, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("parse ListenPort %q: %w", val, err)
		}

		s.ListenPort = p
	case "PrivateKey":
		s.PrivateKey = val
	}

	return nil
}

// applyPeerKV applies a key/value pair to the current [Peer] block p.
func applyPeerKV(p *peerEntry, key, val string) {
	switch key {
	case "PublicKey":
		p.PublicKey = val
	case "AllowedIPs":
		p.AllowedIPs = val
	}
}

// stripPrefix checks whether line starts with prefix; if so it returns the
// remainder with surrounding whitespace trimmed and ok=true.
func stripPrefix(line, prefix string) (rest string, ok bool) {
	rest, ok = strings.CutPrefix(line, prefix)
	if !ok {
		return "", false
	}

	return strings.TrimSpace(rest), true
}

// stripEquals strips a leading "=" from rest and returns the trimmed value.
func stripEquals(rest string) (string, bool) {
	v, ok := strings.CutPrefix(rest, "=")
	if !ok {
		return "", false
	}

	return strings.TrimSpace(v), true
}

// splitKV splits a "key = value" line on the first `=`. Returns ok=false if
// there is no `=`.
func splitKV(line string) (key, val string, ok bool) {
	k, v, found := strings.Cut(line, "=")
	if !found {
		return "", "", false
	}

	return strings.TrimSpace(k), strings.TrimSpace(v), true
}

// parsePeerHeader parses the text after "# Peer:" — e.g.
// "alice (added 2026-05-31)" — into Name and AddedDate. Missing pieces are
// left empty.
func parsePeerHeader(s string) peerEntry {
	s = strings.TrimSpace(s)

	name, rest, found := strings.Cut(s, " (")
	if !found {
		return peerEntry{Name: s}
	}

	rest = strings.TrimSuffix(rest, ")")
	date := strings.TrimPrefix(rest, "added ")

	return peerEntry{Name: name, AddedDate: strings.TrimSpace(date)}
}

// hasPeerNamed reports whether s already contains a peer with the given
// case-sensitive name. Empty name is never a match.
func (s *serverConf) hasPeerNamed(name string) bool {
	if name == "" {
		return false
	}

	for _, p := range s.Peers {
		if p.Name == name {
			return true
		}
	}

	return false
}

// pickFreeIP returns the lowest unused host address in the VPN subnet
// (192.168.192.2 through 192.168.192.254). Returns ErrSubnetExhausted when
// every host is taken. Server (.1), network (.0), and broadcast (.255) are
// never returned. Peers with malformed AllowedIPs entries (no `/`, wrong
// subnet, non-numeric octet) are silently ignored when scanning.
func (s *serverConf) pickFreeIP() (string, error) {
	taken := map[int]bool{wgServerHost: true}

	for _, p := range s.Peers {
		if h, ok := hostOctet(p.AllowedIPs); ok {
			taken[h] = true
		}
	}

	for i := wgFirstClient; i <= wgLastClient; i++ {
		if !taken[i] {
			return fmt.Sprintf("%s%d", wgSubnetPrefix, i), nil
		}
	}

	return "", ErrSubnetExhausted
}

// hostOctet extracts the last octet of an AllowedIPs entry in the VPN
// subnet, e.g. "192.168.192.42/32" -> 42. Returns ok=false if the entry
// is not parseable as IPv4-with-prefix in the VPN subnet.
func hostOctet(allowedIPs string) (int, bool) {
	addr, _, found := strings.Cut(allowedIPs, "/")
	if !found {
		return 0, false
	}

	if !strings.HasPrefix(addr, wgSubnetPrefix) {
		return 0, false
	}

	last := strings.TrimPrefix(addr, wgSubnetPrefix)

	h, err := strconv.Atoi(last)
	if err != nil || h < 0 || h > 255 {
		return 0, false
	}

	return h, true
}

// appendPeer adds p to s.Peers.
func (s *serverConf) appendPeer(p peerEntry) {
	s.Peers = append(s.Peers, p)
}

// loadServerConf reads and parses confPath and populates s.PublicKey from
// pubKeyPath. Returns ErrServerNotConfigured if confPath does not exist.
func loadServerConf(confPath, pubKeyPath string) (*serverConf, error) {
	data, err := os.ReadFile(confPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrServerNotConfigured
		}

		return nil, fmt.Errorf("read %s: %w", confPath, err)
	}

	s, err := parseServerConf(data)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", confPath, err)
	}

	pub, err := os.ReadFile(pubKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", pubKeyPath, err)
	}

	s.PublicKey = strings.TrimSpace(string(pub))

	return s, nil
}

// writeServerConf renders s and atomically writes it to path with mode 0600.
func writeServerConf(path string, s *serverConf) error {
	body := renderServerConf(s)

	return util.FileWriteAtomic(path, []byte(body), confMode, nil, 0, 0)
}
