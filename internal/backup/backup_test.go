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
)

func TestMirror(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("copies matching source files over stubs", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := t.TempDir()

		// Populate root with a file to mirror.
		if err := os.MkdirAll(filepath.Join(root, "etc"), 0755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(root, "etc", "foo.conf"), []byte("live content"), 0644); err != nil {
			t.Fatal(err)
		}

		// Create the corresponding stub in target.
		if err := os.MkdirAll(filepath.Join(target, "etc"), 0755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(target, "etc", "foo.conf"), []byte(""), 0644); err != nil {
			t.Fatal(err)
		}

		if err := mirror(ctx, target, root); err != nil {
			t.Fatalf("mirror: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(target, "etc", "foo.conf"))
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "live content" {
			t.Fatalf("expected %q; got %q", "live content", string(got))
		}
	})

	t.Run("skips stubs with no corresponding source file", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := t.TempDir()

		// Stub exists but there is no matching file under root.
		if err := os.WriteFile(filepath.Join(target, "ghost.conf"), []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}

		if err := mirror(ctx, target, root); err != nil {
			t.Fatalf("mirror: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(target, "ghost.conf"))
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "original" {
			t.Fatalf("stub should be untouched; got %q", string(got))
		}
	})

	t.Run("skips stubs when source path is a directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := t.TempDir()

		// Source path is a directory, not a regular file.
		if err := os.MkdirAll(filepath.Join(root, "isdir"), 0755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(target, "isdir"), []byte("stub"), 0644); err != nil {
			t.Fatal(err)
		}

		if err := mirror(ctx, target, root); err != nil {
			t.Fatalf("mirror: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(target, "isdir"))
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "stub" {
			t.Fatalf("stub should be untouched; got %q", string(got))
		}
	})

	t.Run("skips .git directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := t.TempDir()

		// Create a .git directory with a file in target.
		gitDir := filepath.Join(target, ".git")
		if err := os.MkdirAll(gitDir, 0755); err != nil {
			t.Fatal(err)
		}

		gitFile := filepath.Join(gitDir, "config")
		if err := os.WriteFile(gitFile, []byte("git config"), 0644); err != nil {
			t.Fatal(err)
		}

		// Create a matching file under root to ensure it would be mirrored if not skipped.
		if err := os.MkdirAll(filepath.Join(root, ".git"), 0755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("overwritten"), 0644); err != nil {
			t.Fatal(err)
		}

		if err := mirror(ctx, target, root); err != nil {
			t.Fatalf("mirror: %v", err)
		}

		got, err := os.ReadFile(gitFile)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "git config" {
			t.Fatalf(".git/config should be untouched; got %q", string(got))
		}
	})

	t.Run("creates target directory with mode 0700", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := filepath.Join(t.TempDir(), "newdir")

		if err := mirror(ctx, target, root); err != nil {
			t.Fatalf("mirror: %v", err)
		}

		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != 0700 {
			t.Fatalf("expected dir mode 0700; got %v", info.Mode().Perm())
		}
	})

	t.Run("enforces mode 0700 on existing target directory", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		target := t.TempDir()

		if err := os.Chmod(target, 0755); err != nil {
			t.Fatal(err)
		}

		if err := mirror(ctx, target, root); err != nil {
			t.Fatalf("mirror: %v", err)
		}

		info, err := os.Stat(target)
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != 0700 {
			t.Fatalf("expected dir mode 0700; got %v", info.Mode().Perm())
		}
	})
}
