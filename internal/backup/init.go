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

// stubPaths lists the relative paths that Init creates under ~/.pirewall.
var stubPaths = []string{
	"etc/ddclient.conf",
	"etc/dnsmasq.d/dhcp.conf",
	"etc/dnsmasq.d/dns.conf",
	"etc/iptables/rules.v4",
	"etc/iptables/rules.v6",
	"etc/netplan/01-network.yaml",
	"etc/ssh/sshd_config",
	"etc/sysctl.conf",
	"etc/sysctl.d/90-override.conf",
	"var/lib/misc/dnsmasq.leases",
}

// Init creates the stub directory structure under ~/.pirewall from stubPaths,
// then initialises a git repository there. If git init fails due to missing
// global user configuration, user.email and user.name are set automatically
// using username@localhost.org and username before retrying.
// Existing files are not overwritten. The directory tree is chowned to
// username after creation.
func Init(ctx context.Context, username string) error {
	targetDir, uid, gid, err := userTargetDir(username)
	if err != nil {
		return err
	}

	if err := initStubs(ctx, targetDir, stubPaths); err != nil {
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
