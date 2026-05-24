//go:build unit

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

package restore

import (
	"reflect"
	"testing"
)

func TestHintsFor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{name: "empty input", in: nil, want: nil},
		{name: "no matching prefix", in: []string{"var/lib/misc/dnsmasq.leases"}, want: nil},
		{name: "single match", in: []string{"etc/sysctl.conf"}, want: []string{"sysctl --system"}},
		{
			name: "deduplicated identical hints",
			in:   []string{"etc/sysctl.conf", "etc/sysctl.d/90-override.conf"},
			want: []string{"sysctl --system"},
		},
		{
			name: "multiple distinct hints in input order",
			in: []string{
				"etc/dnsmasq.d/dns.conf",
				"etc/network/interfaces",
				"etc/sysctl.conf",
			},
			want: []string{
				"systemctl restart dnsmasq",
				"systemctl restart networking",
				"sysctl --system",
			},
		},
		{
			name: "match interleaved with non-match",
			in: []string{
				"var/lib/misc/dnsmasq.leases",
				"etc/iptables/rules.v4",
			},
			want: []string{"systemctl restart netfilter-persistent"},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := hintsFor(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("hintsFor(%v) = %v; want %v", tc.in, got, tc.want)
			}
		})
	}
}
