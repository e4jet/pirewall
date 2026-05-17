//go:build unix

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
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"

	"github.com/e4jet/pirewall/internal/backup"
	"github.com/e4jet/pirewall/internal/util"
)

var (
	// ErrInvalidPath is returned when a -restore-path value is not a relative
	// path within ~user/.pirewall (e.g., absolute, or contains "..").
	ErrInvalidPath = errors.New("path must be relative and within ~user/.pirewall")

	// ErrSourceMissing is returned when an explicit -restore-path value does
	// not exist in the backup, is a symlink, is not a regular file, or is
	// empty.
	ErrSourceMissing = errors.New("explicit restore path does not exist or is not a non-empty regular file")

	// ErrEmptyUsername is returned when Restore is called with an empty
	// username.
	ErrEmptyUsername = errors.New("username is required")

	// errUnexpectedStatType is returned when a file's underlying stat result
	// is not the expected unix *syscall.Stat_t.
	errUnexpectedStatType = errors.New("unexpected FileInfo.Sys type")
)

// Options configures a Restore call.
type Options struct {
	Username string   // required: backup user, e.g. "pi"
	Paths    []string // optional: when non-empty, restore only these relative paths
	DryRun   bool     // when true, log actions but make no changes
}

// Restore copies regular files from ~Username/.pirewall/<rel> back to /<rel>
// on the live filesystem, preserving each live target's permission bits and
// ownership (defaulting to root:root mode 0644 when the target does not
// exist). Zero-byte sources in the backup are skipped silently.
//
// Symlinks are never followed: a backup entry that is itself a symlink is
// rejected (path-filter mode) or skipped (full-walk mode). Setuid, setgid,
// and sticky bits on the live target are not preserved — only the
// permission bits.
//
// When opts.Paths is non-empty, only those relative paths are restored; each
// must be a relative path under ~Username/.pirewall pointing at a non-empty
// regular file (otherwise ErrInvalidPath or ErrSourceMissing is returned and
// no writes occur).
//
// When opts.DryRun is true, the actions that would be taken are logged but
// no filesystem changes are made.
//
// Missing parent directories of a destination are created with mode 0755
// owned by the process effective uid/gid; this supports the "build a new
// firewall from a previous backup" workflow.
func Restore(ctx context.Context, opts Options) error {
	if opts.Username == "" {
		return ErrEmptyUsername
	}

	targetDir, _, _, err := backup.UserTargetDir(opts.Username)
	if err != nil {
		return fmt.Errorf("resolve target dir: %w", err)
	}

	r := &restorer{
		targetDir: targetDir,
		root:      "/",
		paths:     opts.Paths,
		dryRun:    opts.DryRun,
		chown:     os.Chown,
		logger:    slog.Default(),
	}

	return r.run(ctx)
}

// restorer holds the resolved inputs and dependencies for a single Restore
// call. The chown and logger fields are injectable so tests can substitute
// no-op or capturing implementations without touching package globals.
type restorer struct {
	targetDir string
	root      string
	paths     []string
	dryRun    bool
	chown     func(name string, uid, gid int) error
	logger    *slog.Logger
}

// workItem describes one file to restore.
type workItem struct {
	src string // absolute path under targetDir
	dst string // absolute path under root
	rel string // path relative to targetDir; used for hint matching and logging
}

// buildWorkList expands the restorer's inputs into a concrete list of files
// to restore. When r.paths is empty it walks r.targetDir for every regular
// non-empty file (skipping .git); otherwise it validates and resolves each
// listed path.
func (r *restorer) buildWorkList(ctx context.Context) ([]workItem, error) {
	if len(r.paths) == 0 {
		return r.walkAll(ctx)
	}

	return r.filterPaths()
}

func (r *restorer) walkAll(ctx context.Context) ([]workItem, error) {
	var items []workItem

	walkErr := filepath.WalkDir(r.targetDir, func(path string, d fs.DirEntry, err error) error {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}

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
			r.logger.WarnContext(ctx, "source is empty, skipping", "src", path)

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

func (r *restorer) filterPaths() ([]workItem, error) {
	items := make([]workItem, 0, len(r.paths))

	for _, p := range r.paths {
		if !filepath.IsLocal(p) {
			return nil, fmt.Errorf("%q: %w", p, ErrInvalidPath)
		}

		src := filepath.Join(r.targetDir, p)

		info, err := os.Lstat(src)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%q: %w", p, ErrSourceMissing)
			}

			return nil, fmt.Errorf("lstat %s: %w", src, err)
		}

		if !info.Mode().IsRegular() || info.Size() == 0 {
			return nil, fmt.Errorf("%q: %w", p, ErrSourceMissing)
		}

		items = append(items, workItem{
			src: src,
			dst: filepath.Join(r.root, p),
			rel: filepath.ToSlash(p),
		})
	}

	return items, nil
}

// readSource opens src with O_NOFOLLOW (rejecting last-second symlink
// substitution of the leaf), verifies the open file is a non-empty regular
// file, and reads its content.
func readSource(src string) ([]byte, error) {
	f, err := os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", src, err)
	}

	defer func() { _ = f.Close() }()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("fstat %s: %w", src, err)
	}

	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil, fmt.Errorf("%q: %w", src, ErrSourceMissing)
	}

	content, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", src, err)
	}

	return content, nil
}

// writeOne atomically replaces dst with content, applying mode and (uid, gid)
// to the resulting file. Missing parent directories are created with mode
// 0755. The actual write is delegated to util.FileWriteAtomic; r.chown is
// passed through as the ownership-application step so tests can inject a
// no-op chown.
func (r *restorer) writeOne(ctx context.Context, it workItem, content []byte, mode os.FileMode, uid, gid int) error {
	dir := filepath.Dir(it.dst)

	if _, err := os.Stat(dir); err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat %s: %w", dir, err)
		}

		r.logger.InfoContext(ctx, "creating missing destination directory", "dir", dir)

		if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil { //nolint:mnd
			return fmt.Errorf("mkdir %s: %w", dir, mkErr)
		}
	}

	return util.FileWriteAtomic(it.dst, content, mode, r.chown, uid, gid)
}

// run executes the restore: build the work list, then for each item look at
// the live destination to capture the mode/uid/gid to preserve (defaulting
// to root:root 0644 when missing), and atomically write the source content
// over the destination via writeOne.
func (r *restorer) run(ctx context.Context) error {
	items, err := r.buildWorkList(ctx)
	if err != nil {
		return err
	}

	for _, it := range items {
		if cerr := ctx.Err(); cerr != nil {
			return cerr
		}

		mode, uid, gid, statErr := r.targetAttrs(it.dst)
		if statErr != nil {
			return fmt.Errorf("stat %s: %w", it.dst, statErr)
		}

		if r.dryRun {
			r.logger.InfoContext(ctx, "would restore", "src", it.src, "dst", it.dst,
				"mode", fmt.Sprintf("%#o", mode), "uid", uid, "gid", gid)

			continue
		}

		content, readErr := readSource(it.src)
		if readErr != nil {
			return readErr
		}

		r.logger.InfoContext(ctx, "restoring", "src", it.src, "dst", it.dst,
			"mode", fmt.Sprintf("%#o", mode), "uid", uid, "gid", gid)

		if err = r.writeOne(ctx, it, content, mode, uid, gid); err != nil {
			return err
		}
	}

	rels := make([]string, 0, len(items))
	for _, it := range items {
		rels = append(rels, it.rel)
	}

	if hints := hintsFor(rels); len(hints) > 0 {
		r.logger.InfoContext(ctx, "restore complete; manual reload may be required", "hints", hints)
	}

	return nil
}

// targetAttrs returns the permission bits and ownership of dst if it exists,
// or the defaults (root:root 0644) if it does not. Setuid, setgid, and
// sticky bits on the live target are not preserved.
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
