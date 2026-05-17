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

package backup

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/e4jet/pirewall/internal/util"
)

// TrackedPath is a relative path under ~/.pirewall that pirewall mirrors
// between the live filesystem and the backup tree. Hint is the operator
// command to reload the service that consumes Rel after a restore; an empty
// Hint means no reload is required.
type TrackedPath struct {
	Rel  string
	Hint string
}

// trackedPaths is the canonical list of paths pirewall mirrors. Init creates
// a stub for each Rel under ~/.pirewall; restore.hintsFor uses Hint to print
// reload guidance after a restore. Adding a row here is the single change
// needed to track a new file end-to-end.
var trackedPaths = []TrackedPath{
	{Rel: "etc/ddclient.conf", Hint: "systemctl restart ddclient"},
	{Rel: "etc/dnsmasq.d/dhcp.conf", Hint: "systemctl restart dnsmasq"},
	{Rel: "etc/dnsmasq.d/dns.conf", Hint: "systemctl restart dnsmasq"},
	{Rel: "etc/iptables/rules.v4", Hint: "systemctl restart netfilter-persistent"},
	{Rel: "etc/iptables/rules.v6", Hint: "systemctl restart netfilter-persistent"},
	{Rel: "etc/netplan/01-network.yaml", Hint: "netplan apply"},
	{Rel: "etc/ssh/sshd_config", Hint: "systemctl restart ssh"},
	{Rel: "etc/sysctl.conf", Hint: "sysctl --system"},
	{Rel: "etc/sysctl.d/90-override.conf", Hint: "sysctl --system"},
	{Rel: "var/lib/misc/dnsmasq.leases"},
}

// TrackedPaths returns a copy of the canonical tracked-path list.
func TrackedPaths() []TrackedPath {
	out := make([]TrackedPath, len(trackedPaths))
	copy(out, trackedPaths)

	return out
}

// trackedRels returns just the relative paths from trackedPaths, in order.
func trackedRels() []string {
	out := make([]string, len(trackedPaths))
	for i, tp := range trackedPaths {
		out[i] = tp.Rel
	}

	return out
}

// Init creates the stub directory structure under ~/.pirewall from
// trackedPaths, then initialises a git repository there. If git init fails
// due to missing global user configuration, user.email and user.name are set
// automatically using username@localhost.org and username before retrying.
// Existing files are not overwritten. The directory tree is chowned to
// username after creation.
func Init(ctx context.Context, username string) error {
	targetDir, uid, gid, err := UserTargetDir(username)
	if err != nil {
		return err
	}

	if err := initStubs(ctx, targetDir, trackedRels()); err != nil {
		return err
	}

	if err := gitInit(ctx, targetDir, username); err != nil {
		return err
	}

	return chownDir(targetDir, uid, gid)
}

func gitInit(ctx context.Context, dir, username string) error {
	if _, _, err := util.ExecCommandOutput(ctx, "git", []string{"-C", dir, "init"}); err != nil {
		return fmt.Errorf("git init: %w", err)
	}

	if err := gitConfigIfUnset(ctx, dir, "user.email", username+"@localhost.org"); err != nil {
		return err
	}

	if err := gitConfigIfUnset(ctx, dir, "user.name", username); err != nil {
		return err
	}

	return nil
}

func gitConfigIfUnset(ctx context.Context, dir, key, value string) error {
	out, _, err := util.ExecCommandOutput(ctx, "git", []string{"-C", dir, "config", key})
	if err == nil && strings.TrimSpace(out) != "" {
		return nil
	}

	if _, _, err := util.ExecCommandOutput(ctx, "git", []string{"-C", dir, "config", "--local", key, value}); err != nil {
		return fmt.Errorf("git config %s: %w", key, err)
	}

	return nil
}

func initStubs(ctx context.Context, targetDir string, paths []string) error {
	for _, rel := range paths {
		dst := filepath.Join(targetDir, rel)

		if mkErr := os.MkdirAll(filepath.Dir(dst), dirMode); mkErr != nil {
			return fmt.Errorf("create dir for %s: %w", dst, mkErr)
		}

		if _, statErr := os.Stat(dst); statErr == nil {
			slog.DebugContext(ctx, "stub already exists, skipping", "path", dst)
			continue
		}

		slog.InfoContext(ctx, "creating stub", "path", dst)

		f, createErr := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, fileMode)
		if createErr != nil {
			if os.IsExist(createErr) {
				continue
			}

			return fmt.Errorf("create stub %s: %w", dst, createErr)
		}

		if closeErr := f.Close(); closeErr != nil {
			return fmt.Errorf("close stub %s: %w", dst, closeErr)
		}
	}

	return nil
}
