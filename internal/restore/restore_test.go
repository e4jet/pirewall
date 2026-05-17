//go:build unit && unix

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
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
)

// discardLogger returns a slog.Logger that writes to io.Discard so tests don't
// pollute stdout/stderr.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// newTestRestorer builds a restorer wired to the given source/dest temp dirs
// and a no-op chown so tests pass when not running as root.
func newTestRestorer(t *testing.T, targetDir, root string, paths []string, dryRun bool) *restorer {
	t.Helper()

	return &restorer{
		targetDir: targetDir,
		root:      root,
		paths:     paths,
		dryRun:    dryRun,
		chown:     func(string, int, int) error { return nil },
		logger:    discardLogger(),
	}
}

// writeFile is a small helper for building test fixtures.
func writeFile(t *testing.T, path string, content []byte, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, content, mode); err != nil {
		t.Fatal(err)
	}
}

// rels extracts the rel field from a slice of workItems for stable comparison.
func rels(items []workItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.rel)
	}

	sort.Strings(out)

	return out
}

func TestBuildWorkList_FullWalk(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("includes regular non-empty files", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/foo.conf"), []byte("a"), 0o600)
		writeFile(t, filepath.Join(target, "etc/bar.conf"), []byte("b"), 0o600)

		r := newTestRestorer(t, target, t.TempDir(), nil, false)

		items, err := r.buildWorkList(ctx)
		if err != nil {
			t.Fatalf("buildWorkList: %v", err)
		}

		got := rels(items)
		want := []string{"etc/bar.conf", "etc/foo.conf"}

		if !equalStringSlices(got, want) {
			t.Fatalf("rels: got %v want %v", got, want)
		}
	})

	t.Run("skips zero-byte sources", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/empty.conf"), nil, 0o600)
		writeFile(t, filepath.Join(target, "etc/full.conf"), []byte("x"), 0o600)

		r := newTestRestorer(t, target, t.TempDir(), nil, false)

		items, err := r.buildWorkList(ctx)
		if err != nil {
			t.Fatalf("buildWorkList: %v", err)
		}

		got := rels(items)
		want := []string{"etc/full.conf"}

		if !equalStringSlices(got, want) {
			t.Fatalf("rels: got %v want %v", got, want)
		}
	})

	t.Run("skips .git directory", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, ".git/config"), []byte("git"), 0o600)
		writeFile(t, filepath.Join(target, "etc/keep.conf"), []byte("keep"), 0o600)

		r := newTestRestorer(t, target, t.TempDir(), nil, false)

		items, err := r.buildWorkList(ctx)
		if err != nil {
			t.Fatalf("buildWorkList: %v", err)
		}

		got := rels(items)
		want := []string{"etc/keep.conf"}

		if !equalStringSlices(got, want) {
			t.Fatalf("rels: got %v want %v", got, want)
		}
	})

	t.Run("skips non-regular entries", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/keep.conf"), []byte("keep"), 0o600)

		// Add a symlink to the regular file.
		if err := os.Symlink(filepath.Join(target, "etc/keep.conf"),
			filepath.Join(target, "etc/link.conf")); err != nil {
			t.Fatal(err)
		}

		r := newTestRestorer(t, target, t.TempDir(), nil, false)

		items, err := r.buildWorkList(ctx)
		if err != nil {
			t.Fatalf("buildWorkList: %v", err)
		}

		got := rels(items)
		want := []string{"etc/keep.conf"}

		if !equalStringSlices(got, want) {
			t.Fatalf("rels: got %v want %v", got, want)
		}
	})

	t.Run("aborts on cancelled context", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/foo.conf"), []byte("x"), 0o600)

		r := newTestRestorer(t, target, t.TempDir(), nil, false)

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := r.buildWorkList(cancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled; got %v", err)
		}
	})
}

func TestBuildWorkList_PathFilter(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("returns only listed paths", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/a.conf"), []byte("a"), 0o600)
		writeFile(t, filepath.Join(target, "etc/b.conf"), []byte("b"), 0o600)
		writeFile(t, filepath.Join(target, "etc/c.conf"), []byte("c"), 0o600)

		r := newTestRestorer(t, target, t.TempDir(),
			[]string{"etc/a.conf", "etc/c.conf"}, false)

		items, err := r.buildWorkList(ctx)
		if err != nil {
			t.Fatalf("buildWorkList: %v", err)
		}

		got := rels(items)
		want := []string{"etc/a.conf", "etc/c.conf"}

		if !equalStringSlices(got, want) {
			t.Fatalf("rels: got %v want %v", got, want)
		}
	})

	t.Run("rejects absolute path", func(t *testing.T) {
		t.Parallel()

		r := newTestRestorer(t, t.TempDir(), t.TempDir(),
			[]string{"/etc/foo.conf"}, false)

		_, err := r.buildWorkList(ctx)
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath; got %v", err)
		}
	})

	t.Run("rejects parent traversal", func(t *testing.T) {
		t.Parallel()

		r := newTestRestorer(t, t.TempDir(), t.TempDir(),
			[]string{"../etc/foo.conf"}, false)

		_, err := r.buildWorkList(ctx)
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath; got %v", err)
		}
	})

	t.Run("rejects missing source file", func(t *testing.T) {
		t.Parallel()

		r := newTestRestorer(t, t.TempDir(), t.TempDir(),
			[]string{"etc/missing.conf"}, false)

		_, err := r.buildWorkList(ctx)
		if !errors.Is(err, ErrSourceMissing) {
			t.Fatalf("expected ErrSourceMissing; got %v", err)
		}
	})

	t.Run("rejects zero-byte source", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/empty.conf"), nil, 0o600)

		r := newTestRestorer(t, target, t.TempDir(),
			[]string{"etc/empty.conf"}, false)

		_, err := r.buildWorkList(ctx)
		if !errors.Is(err, ErrSourceMissing) {
			t.Fatalf("expected ErrSourceMissing; got %v", err)
		}
	})

	t.Run("rejects directory source", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		if err := os.MkdirAll(filepath.Join(target, "etc/sub"), 0o755); err != nil {
			t.Fatal(err)
		}

		r := newTestRestorer(t, target, t.TempDir(),
			[]string{"etc/sub"}, false)

		_, err := r.buildWorkList(ctx)
		if !errors.Is(err, ErrSourceMissing) {
			t.Fatalf("expected ErrSourceMissing; got %v", err)
		}
	})

	t.Run("rejects symlink source", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/real.conf"), []byte("real"), 0o600)

		if err := os.Symlink(filepath.Join(target, "etc/real.conf"),
			filepath.Join(target, "etc/link.conf")); err != nil {
			t.Fatal(err)
		}

		r := newTestRestorer(t, target, t.TempDir(),
			[]string{"etc/link.conf"}, false)

		_, err := r.buildWorkList(ctx)
		if !errors.Is(err, ErrSourceMissing) {
			t.Fatalf("expected ErrSourceMissing for symlink; got %v", err)
		}
	})
}

func TestReadSource(t *testing.T) {
	t.Parallel()

	t.Run("reads regular file content", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "foo.conf")
		writeFile(t, src, []byte("hello"), 0o600)

		got, err := readSource(src)
		if err != nil {
			t.Fatalf("readSource: %v", err)
		}

		if string(got) != "hello" {
			t.Errorf("content: got %q; want %q", got, "hello")
		}
	})

	t.Run("rejects symlink via O_NOFOLLOW", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		real := filepath.Join(dir, "real.conf")
		writeFile(t, real, []byte("real"), 0o600)

		link := filepath.Join(dir, "link.conf")
		if err := os.Symlink(real, link); err != nil {
			t.Fatal(err)
		}

		_, err := readSource(link)
		if err == nil {
			t.Fatal("expected error opening symlink with O_NOFOLLOW; got nil")
		}
	})

	t.Run("rejects empty regular file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "empty.conf")
		writeFile(t, src, nil, 0o600)

		_, err := readSource(src)
		if !errors.Is(err, ErrSourceMissing) {
			t.Fatalf("expected ErrSourceMissing; got %v", err)
		}
	})
}

// equalStringSlices is a tiny helper used across tests in this file.
func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

func TestWriteOne(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("writes content with mode and chowns", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		dst := filepath.Join(root, "etc", "foo.conf")

		var (
			capturedPath   string
			capturedUID    int
			capturedGID    int
			capturedCalled bool
		)

		r := &restorer{
			root: root,
			chown: func(name string, uid, gid int) error {
				capturedPath = name
				capturedUID = uid
				capturedGID = gid
				capturedCalled = true

				return nil
			},
			logger: discardLogger(),
		}

		if err := r.writeOne(ctx, workItem{dst: dst}, []byte("hello"), 0o640, 1234, 5678); err != nil {
			t.Fatalf("writeOne: %v", err)
		}

		if !capturedCalled {
			t.Fatal("chown was not called")
		}

		if capturedUID != 1234 || capturedGID != 5678 {
			t.Errorf("chown args: uid=%d gid=%d; want 1234/5678", capturedUID, capturedGID)
		}

		if filepath.Dir(capturedPath) != filepath.Dir(dst) {
			t.Errorf("chown path %q is not in dst dir %q", capturedPath, filepath.Dir(dst))
		}

		got, err := os.ReadFile(dst)
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "hello" {
			t.Errorf("content: got %q; want %q", got, "hello")
		}

		info, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != 0o640 {
			t.Errorf("mode: got %v; want 0640", info.Mode().Perm())
		}
	})

	t.Run("removes temp file on chown failure", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		dst := filepath.Join(root, "etc", "foo.conf")

		r := &restorer{
			root: root,
			chown: func(string, int, int) error {
				return errors.New("simulated chown failure")
			},
			logger: discardLogger(),
		}

		err := r.writeOne(ctx, workItem{dst: dst}, []byte("hello"), 0o644, 0, 0)
		if err == nil {
			t.Fatal("expected error; got nil")
		}

		// dst must not exist (rename never happened).
		if _, statErr := os.Stat(dst); !os.IsNotExist(statErr) {
			t.Errorf("dst should not exist; stat err: %v", statErr)
		}

		// No leftover temp file in the dst dir.
		entries, err := os.ReadDir(filepath.Dir(dst))
		if err != nil {
			t.Fatal(err)
		}

		for _, e := range entries {
			t.Errorf("leftover file in dst dir: %s", e.Name())
		}
	})

	t.Run("creates parent directory if missing", func(t *testing.T) {
		t.Parallel()

		root := t.TempDir()
		dst := filepath.Join(root, "deep", "nested", "etc", "foo.conf")

		r := &restorer{
			root:   root,
			chown:  func(string, int, int) error { return nil },
			logger: discardLogger(),
		}

		if err := r.writeOne(ctx, workItem{dst: dst}, []byte("x"), 0o644, 0, 0); err != nil {
			t.Fatalf("writeOne: %v", err)
		}

		if _, err := os.Stat(dst); err != nil {
			t.Fatalf("dst not created: %v", err)
		}
	})

	t.Run("fails when parent dir cannot be created", func(t *testing.T) {
		t.Parallel()

		// Make a read-only directory and try to create a subdir under it.
		root := t.TempDir()
		readonly := filepath.Join(root, "ro")

		if err := os.Mkdir(readonly, 0o555); err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() {
			_ = os.Chmod(readonly, 0o755)
		})

		dst := filepath.Join(readonly, "sub", "foo.conf")

		r := &restorer{
			root:   root,
			chown:  func(string, int, int) error { return nil },
			logger: discardLogger(),
		}

		err := r.writeOne(ctx, workItem{dst: dst}, []byte("x"), 0o644, 0, 0)
		if err == nil {
			t.Fatal("expected mkdir error; got nil")
		}
	})
}

// statOwner returns the uid/gid of an existing file (Linux). Used to assert
// that ownership was preserved after restore.
func statOwner(t *testing.T, path string) (uid, gid int) {
	t.Helper()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}

	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Fatalf("not a syscall.Stat_t: %T", info.Sys())
	}

	return int(st.Uid), int(st.Gid)
}

func TestRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("copies all matching files into root", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/a.conf"), []byte("aaa"), 0o600)
		writeFile(t, filepath.Join(target, "etc/b.conf"), []byte("bbb"), 0o600)

		root := t.TempDir()
		// Pre-create the dst files so chown(uid==testUid) is a no-op even as a
		// non-root test runner.
		writeFile(t, filepath.Join(root, "etc/a.conf"), []byte("old"), 0o644)
		writeFile(t, filepath.Join(root, "etc/b.conf"), []byte("old"), 0o644)

		r := newTestRestorer(t, target, root, nil, false)

		if err := r.run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}

		gotA, _ := os.ReadFile(filepath.Join(root, "etc/a.conf"))
		if string(gotA) != "aaa" {
			t.Errorf("a.conf: got %q; want %q", gotA, "aaa")
		}

		gotB, _ := os.ReadFile(filepath.Join(root, "etc/b.conf"))
		if string(gotB) != "bbb" {
			t.Errorf("b.conf: got %q; want %q", gotB, "bbb")
		}
	})

	t.Run("preserves existing target mode", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/foo.conf"), []byte("new"), 0o600)

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "etc/foo.conf"), []byte("old"), 0o640)

		r := newTestRestorer(t, target, root, nil, false)

		if err := r.run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}

		info, err := os.Stat(filepath.Join(root, "etc/foo.conf"))
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != 0o640 {
			t.Errorf("mode: got %v; want 0640", info.Mode().Perm())
		}
	})

	t.Run("preserves existing target ownership", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/foo.conf"), []byte("new"), 0o600)

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "etc/foo.conf"), []byte("old"), 0o600)

		// Capture what uid/gid would be applied by chown to verify the live
		// target's ownership was read and propagated.
		liveUID, liveGID := statOwner(t, filepath.Join(root, "etc/foo.conf"))

		var (
			capturedUID = -1
			capturedGID = -1
		)

		r := &restorer{
			targetDir: target,
			root:      root,
			chown: func(_ string, uid, gid int) error {
				capturedUID = uid
				capturedGID = gid

				return nil
			},
			logger: discardLogger(),
		}

		if err := r.run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}

		if capturedUID != liveUID || capturedGID != liveGID {
			t.Errorf("chown args: uid=%d gid=%d; want %d/%d (existing target)",
				capturedUID, capturedGID, liveUID, liveGID)
		}
	})

	t.Run("uses defaults when target file is missing", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/foo.conf"), []byte("new"), 0o600)

		root := t.TempDir()

		var (
			capturedUID = -1
			capturedGID = -1
		)

		r := &restorer{
			targetDir: target,
			root:      root,
			chown: func(_ string, uid, gid int) error {
				capturedUID = uid
				capturedGID = gid

				return nil
			},
			logger: discardLogger(),
		}

		if err := r.run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}

		if capturedUID != 0 || capturedGID != 0 {
			t.Errorf("chown args: uid=%d gid=%d; want 0/0", capturedUID, capturedGID)
		}

		info, err := os.Stat(filepath.Join(root, "etc/foo.conf"))
		if err != nil {
			t.Fatal(err)
		}

		if info.Mode().Perm() != 0o644 {
			t.Errorf("mode: got %v; want 0644", info.Mode().Perm())
		}
	})

	t.Run("validation failure makes no writes", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/good.conf"), []byte("good"), 0o600)

		root := t.TempDir()

		// Mix one valid path with one invalid (absolute) path. Validation must
		// fail before any work item is written.
		r := newTestRestorer(t, target, root,
			[]string{"etc/good.conf", "/etc/bad.conf"}, false)

		err := r.run(ctx)
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath; got %v", err)
		}

		if _, statErr := os.Stat(filepath.Join(root, "etc/good.conf")); !os.IsNotExist(statErr) {
			t.Errorf("good.conf should not exist; stat err: %v", statErr)
		}
	})

	t.Run("aborts on cancelled context before writing", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/foo.conf"), []byte("new"), 0o600)

		root := t.TempDir()

		r := newTestRestorer(t, target, root, nil, false)

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()

		err := r.run(cancelled)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled; got %v", err)
		}

		if _, statErr := os.Stat(filepath.Join(root, "etc/foo.conf")); !os.IsNotExist(statErr) {
			t.Errorf("foo.conf should not exist after cancellation; stat err: %v", statErr)
		}
	})
}

func TestRun_DryRun(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("makes no changes to existing target", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/foo.conf"), []byte("new"), 0o600)

		root := t.TempDir()
		writeFile(t, filepath.Join(root, "etc/foo.conf"), []byte("old"), 0o644)

		r := newTestRestorer(t, target, root, nil, true)

		if err := r.run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}

		got, err := os.ReadFile(filepath.Join(root, "etc/foo.conf"))
		if err != nil {
			t.Fatal(err)
		}

		if string(got) != "old" {
			t.Errorf("content: got %q; want %q (dry-run must not overwrite)", got, "old")
		}
	})

	t.Run("does not create missing target", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/foo.conf"), []byte("new"), 0o600)

		root := t.TempDir()

		r := newTestRestorer(t, target, root, nil, true)

		if err := r.run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}

		if _, statErr := os.Stat(filepath.Join(root, "etc/foo.conf")); !os.IsNotExist(statErr) {
			t.Errorf("dst should not exist after dry-run; stat err: %v", statErr)
		}
	})

	t.Run("validation still fires", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()

		r := newTestRestorer(t, target, t.TempDir(),
			[]string{"/etc/bad.conf"}, true)

		err := r.run(ctx)
		if !errors.Is(err, ErrInvalidPath) {
			t.Fatalf("expected ErrInvalidPath; got %v", err)
		}
	})
}

func TestRun_PrintsHints(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("emits deduplicated reload hints", func(t *testing.T) {
		t.Parallel()

		target := t.TempDir()
		writeFile(t, filepath.Join(target, "etc/sysctl.conf"), []byte("a"), 0o600)
		writeFile(t, filepath.Join(target, "etc/sysctl.d/90-override.conf"), []byte("b"), 0o600)
		writeFile(t, filepath.Join(target, "etc/dnsmasq.d/dns.conf"), []byte("c"), 0o600)

		root := t.TempDir()

		var buf bytes.Buffer

		r := &restorer{
			targetDir: target,
			root:      root,
			chown:     func(string, int, int) error { return nil },
			logger:    slog.New(slog.NewTextHandler(&buf, nil)),
		}

		if err := r.run(ctx); err != nil {
			t.Fatalf("run: %v", err)
		}

		out := buf.String()

		if !strings.Contains(out, "sysctl --system") {
			t.Errorf("output missing 'sysctl --system' hint:\n%s", out)
		}

		if !strings.Contains(out, "systemctl restart dnsmasq") {
			t.Errorf("output missing 'systemctl restart dnsmasq' hint:\n%s", out)
		}

		if got := strings.Count(out, "sysctl --system"); got != 1 {
			t.Errorf("expected 'sysctl --system' to appear exactly once; got %d:\n%s", got, out)
		}
	})
}

func TestRestore_EmptyUsername(t *testing.T) {
	t.Parallel()

	err := Restore(context.Background(), Options{Username: ""})
	if !errors.Is(err, ErrEmptyUsername) {
		t.Fatalf("expected ErrEmptyUsername; got %v", err)
	}
}
