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

// Package restore copies regular files from ~user/.pirewall/<rel> back to
// /<rel> on the live filesystem, preserving the live target's mode and
// ownership.
package restore

import "strings"

// pathHint maps a restored file's relative-path prefix to a short reload
// hint to be shown to the operator after restore. The first matching prefix
// wins; entries are ordered with the more-specific prefix first when the
// space could overlap.
var pathHint = []struct {
	prefix string
	hint   string
}{
	{"etc/dnsmasq.d/", "systemctl restart dnsmasq"},
	{"etc/iptables/", "systemctl restart netfilter-persistent"},
	{"etc/netplan/", "netplan apply"},
	{"etc/ssh/sshd_config", "systemctl restart ssh"},
	{"etc/sysctl.conf", "sysctl --system"},
	{"etc/sysctl.d/", "sysctl --system"},
	{"etc/ddclient.conf", "systemctl restart ddclient"},
}

// hintsFor returns the unique set of reload hints triggered by the given
// relative paths, in the order each hint is first encountered. Paths matching
// no prefix contribute nothing.
func hintsFor(relPaths []string) []string {
	seen := make(map[string]struct{})

	var out []string

	for _, p := range relPaths {
		for _, ph := range pathHint {
			if strings.HasPrefix(p, ph.prefix) {
				if _, ok := seen[ph.hint]; !ok {
					seen[ph.hint] = struct{}{}
					out = append(out, ph.hint)
				}

				break
			}
		}
	}

	return out
}
