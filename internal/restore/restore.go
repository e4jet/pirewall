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
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/e4jet/pirewall/internal/backup"
)

var (
	// ErrInvalidPath is returned when a -restore-path value is not a relative
	// path within ~user/.pirewall (e.g., absolute, or contains "..").
	ErrInvalidPath = errors.New("path must be relative and within ~user/.pirewall")

	// ErrSourceMissing is returned when an explicit -restore-path value does
	// not exist in the backup, is not a regular file, or is empty.
	ErrSourceMissing = errors.New("explicit restore path does not exist or is not a non-empty regular file")
)

// Options configures a Restore call.
type Options struct {
	Username string   // required: backup user, e.g. "pi"
	Paths    []string // optional: when non-empty, restore only these relative paths
	DryRun   bool     // when true, log actions but make no changes
}

// Restore copies regular files from ~Username/.pirewall/<rel> back to /<rel>
// on the live filesystem, preserving each live target's mode and ownership
// (defaulting to root:root mode 0644 when the target does not exist).
// Zero-byte sources in the backup are skipped silently.
//
// When opts.Paths is non-empty, only those relative paths are restored; each
// must be a relative path under ~Username/.pirewall pointing at a non-empty
// regular file (otherwise ErrInvalidPath or ErrSourceMissing is returned and
// no writes occur).
//
// When opts.DryRun is true, the actions that would be taken are logged but
// no filesystem changes are made.
func Restore(ctx context.Context, opts Options) error {
	targetDir, _, _, err := backup.UserTargetDir(opts.Username)
	if err != nil {
		return err
	}

	r := &restorer{
		targetDir: targetDir,
		root:      "/",
		paths:     opts.Paths,
		dryRun:    opts.DryRun,
		chown:     os.Chown,
	}

	return r.run(ctx)
}

// restorer holds the resolved inputs and dependencies for a single Restore
// call. The chown field is a function so tests can inject a no-op or capturing
// implementation without touching package globals.
type restorer struct {
	targetDir string                                // source: ~user/.pirewall
	root      string                                // dest root: "/" in production, t.TempDir() in tests
	paths     []string                              // optional restrict-to set; nil/empty = walk all
	dryRun    bool                                  // when true, log actions but make no changes
	chown     func(name string, uid, gid int) error // injection seam for tests; production = os.Chown
}

// workItem describes one file to restore.
type workItem struct {
	src string // absolute path under targetDir
	dst string // absolute path under root
	rel string // path relative to targetDir; used for hint matching and logging
}

// buildWorkList expands the restorer's inputs into a concrete list of files
// to restore. When r.paths is empty, it walks r.targetDir and returns every
// regular non-empty file (skipping .git). When r.paths is non-empty, see the
// path-filter overload added in a later task.
func (r *restorer) buildWorkList(ctx context.Context) ([]workItem, error) {
	if len(r.paths) == 0 {
		return r.buildWorkListFromWalk(ctx)
	}

	return r.buildWorkListFromPaths()
}

func (r *restorer) buildWorkListFromWalk(ctx context.Context) ([]workItem, error) {
	var items []workItem

	walkErr := filepath.WalkDir(r.targetDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}

			return nil
		}

		if !d.Type().IsRegular() {
			return nil
		}

		info, statErr := d.Info()
		if statErr != nil {
			return fmt.Errorf("stat %s: %w", path, statErr)
		}

		if info.Size() == 0 {
			slog.WarnContext(ctx, "source is empty, skipping", "src", path)

			return nil
		}

		rel, relErr := filepath.Rel(r.targetDir, path)
		if relErr != nil {
			return fmt.Errorf("rel %s: %w", path, relErr)
		}

		items = append(items, workItem{
			src: path,
			dst: filepath.Join(r.root, rel),
			rel: filepath.ToSlash(rel),
		})

		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walk %s: %w", r.targetDir, walkErr)
	}

	return items, nil
}

// writeOne atomically replaces dst with content, applying mode and (uid, gid)
// to the resulting file. The temp file is created in dst's directory so the
// rename is a single-filesystem operation. On any failure after the temp file
// is created, the temp file is removed.
func (r *restorer) writeOne(it workItem, content []byte, mode os.FileMode, uid, gid int) (err error) {
	dir := filepath.Dir(it.dst)

	if err = os.MkdirAll(dir, 0o755); err != nil { //nolint:mnd
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, ".pirewall-restore-*")
	if err != nil {
		return fmt.Errorf("create temp in %s: %w", dir, err)
	}

	tmpName := tmp.Name()

	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(content); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("write %s: %w", tmpName, err)
	}

	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("sync %s: %w", tmpName, err)
	}

	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}

	if err = os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}

	if err = r.chown(tmpName, uid, gid); err != nil {
		return fmt.Errorf("chown %s: %w", tmpName, err)
	}

	if err = os.Rename(tmpName, it.dst); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpName, it.dst, err)
	}

	return nil
}

// errUnexpectedStatType is returned by targetAttrs when info.Sys() is not a
// *syscall.Stat_t (i.e., running on a non-Linux host in tests).
var errUnexpectedStatType = errors.New("unexpected FileInfo.Sys type")

func (r *restorer) buildWorkListFromPaths() ([]workItem, error) {
	items := make([]workItem, 0, len(r.paths))

	for _, p := range r.paths {
		if !filepath.IsLocal(p) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidPath, p)
		}

		src := filepath.Join(r.targetDir, p)

		info, err := os.Stat(src)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: %q", ErrSourceMissing, p)
			}

			return nil, fmt.Errorf("stat %s: %w", src, err)
		}

		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, fmt.Errorf("%w: %q", ErrSourceMissing, p)
		}

		items = append(items, workItem{
			src: src,
			dst: filepath.Join(r.root, p),
			rel: filepath.ToSlash(p),
		})
	}

	return items, nil
}

// run executes the restore: build the work list, then for each item, look at
// the live destination to capture the mode/uid/gid to preserve (defaulting to
// root:root 0644 when missing), and atomically write the source content over
// the destination via writeOne.
func (r *restorer) run(ctx context.Context) error {
	items, err := r.buildWorkList(ctx)
	if err != nil {
		return err
	}

	for _, it := range items {
		mode, uid, gid, statErr := r.targetAttrs(it.dst)
		if statErr != nil {
			return fmt.Errorf("stat %s: %w", it.dst, statErr)
		}

		if r.dryRun {
			slog.InfoContext(ctx, "would restore", "src", it.src, "dst", it.dst,
				"mode", fmt.Sprintf("%#o", mode), "uid", uid, "gid", gid)

			continue
		}

		content, readErr := os.ReadFile(it.src)
		if readErr != nil {
			return fmt.Errorf("read %s: %w", it.src, readErr)
		}

		slog.InfoContext(ctx, "restoring", "src", it.src, "dst", it.dst,
			"mode", fmt.Sprintf("%#o", mode), "uid", uid, "gid", gid)

		if err = r.writeOne(it, content, mode, uid, gid); err != nil {
			return err
		}
	}

	if hints := hintsFor(itemRels(items)); len(hints) > 0 {
		slog.InfoContext(ctx, "restore complete; manual reload may be required", "hints", hints)
	}

	return nil
}

// itemRels extracts the rel field from each work item, preserving order.
func itemRels(items []workItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.rel)
	}

	return out
}

// targetAttrs returns the mode and ownership of dst if it exists, or the
// defaults (root:root 0644) if it does not. Any other stat error is returned.
func (r *restorer) targetAttrs(dst string) (mode os.FileMode, uid, gid int, err error) {
	info, statErr := os.Stat(dst)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return 0o644, 0, 0, nil //nolint:mnd
		}

		return 0, 0, 0, statErr
	}

	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0, fmt.Errorf("%w: %T for %s", errUnexpectedStatType, info.Sys(), dst)
	}

	return info.Mode().Perm(), int(st.Uid), int(st.Gid), nil
}
