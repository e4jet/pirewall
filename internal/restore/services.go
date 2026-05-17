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
// /<rel> on the live filesystem, preserving the live target's permission
// bits and ownership.
package restore

import "github.com/e4jet/pirewall/internal/backup"

// hintsFor returns the unique set of reload hints triggered by the given
// relative paths, in the order each hint is first encountered. Paths not in
// backup.TrackedPaths(), or paths whose tracked entry has an empty Hint,
// contribute nothing.
func hintsFor(relPaths []string) []string {
	tracked := backup.TrackedPaths()

	hintByRel := make(map[string]string, len(tracked))
	for _, tp := range tracked {
		if tp.Hint != "" {
			hintByRel[tp.Rel] = tp.Hint
		}
	}

	seen := make(map[string]struct{})

	var out []string

	for _, p := range relPaths {
		hint, ok := hintByRel[p]
		if !ok {
			continue
		}

		if _, dup := seen[hint]; dup {
			continue
		}

		seen[hint] = struct{}{}

		out = append(out, hint)
	}

	return out
}
