//go:build unit

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
	"os"
	"path/filepath"
	"testing"

	"github.com/e4jet/pirewall/internal/util"
)

func TestGitInit(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("initialises git repo in target directory", func(t *testing.T) {
		t.Parallel()

		if _, _, err := util.ExecCommandOutput(ctx, "git", []string{"--version"}); err != nil {
			t.Skip("git not available")
		}

		target := t.TempDir()

		if err := gitInit(ctx, target, "testuser"); err != nil {
			t.Fatalf("gitInit: %v", err)
		}

		if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
			t.Errorf("expected .git directory to exist: %v", err)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		t.Parallel()

		if _, _, err := util.ExecCommandOutput(ctx, "git", []string{"--version"}); err != nil {
			t.Skip("git not available")
		}

		target := t.TempDir()

		if err := gitInit(ctx, target, "testuser"); err != nil {
			t.Fatalf("first gitInit: %v", err)
		}

		if err := gitInit(ctx, target, "testuser"); err != nil {
			t.Fatalf("second gitInit: %v", err)
		}
	})
}

func TestInitStubs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("creates directories and empty stub files", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()

		paths := []string{
			"etc/ssh/sshd_config",
			"etc/dnsmasq.d/dns.conf",
		}

		if err := initStubs(ctx, target, paths); err != nil {
			t.Fatalf("initStubs: %v", err)
		}

		for _, rel := range paths {
			if _, err := os.Stat(filepath.Join(target, rel)); err != nil {
				t.Errorf("expected stub %s to exist: %v", rel, err)
			}
		}
	})

	t.Run("does not overwrite existing files", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()

		existing := filepath.Join(target, "etc", "sshd_config")
		if err := os.MkdirAll(filepath.Dir(existing), 0755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(existing, []byte("existing content"), 0644); err != nil {
			t.Fatal(err)
		}

		if err := initStubs(ctx, target, []string{"etc/sshd_config"}); err != nil {
			t.Fatalf("initStubs: %v", err)
		}

		got, err := os.ReadFile(existing)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "existing content" {
			t.Fatalf("expected existing content to be preserved; got %q", string(got))
		}
	})

	t.Run("creates intermediate directories with mode 0700", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()

		if err := initStubs(ctx, target, []string{"var/lib/misc/dnsmasq.leases"}); err != nil {
			t.Fatalf("initStubs: %v", err)
		}

		info, err := os.Stat(filepath.Join(target, "var", "lib", "misc"))
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != dirMode {
			t.Fatalf("expected dir mode %v; got %v", dirMode, info.Mode().Perm())
		}
	})
}
