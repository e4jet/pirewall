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

// Package backup mirrors live filesystem configuration files into
// ~/.pirewall for a given system user and optionally commits any changes
// to a git repository there.
//
// The intended workflow is:
//  1. Populate ~/.pirewall with empty stub files at the paths you want to
//     track (e.g. ~/.pirewall/etc/sysctl.conf).
//  2. Run Mirror (or Backup) periodically — Mirror copies the live content
//     from the root filesystem over each stub, then chowns the directory to
//     the user; Backup additionally commits any changes to git.
package backup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/e4jet/pirewall/internal/util"
)

const (
	dirMode     = os.FileMode(0700)
	fileMode    = os.FileMode(0600)
	pirewallDir = ".pirewall"
)

var (
	// ErrNoHomeDir is returned when the specified user has no home directory.
	ErrNoHomeDir = errors.New("user has no home directory")
)

// Mirror looks up username, creates ~/.pirewall with mode 0700, copies each
// stub file's live content from the root filesystem into it, and chowns the
// entire directory tree to username.
//
// Source files that do not exist or are not regular files are silently skipped.
func Mirror(ctx context.Context, username string) error {
	targetDir, uid, gid, err := userTargetDir(username)
	if err != nil {
		return err
	}

	if err := mirror(ctx, targetDir, "/"); err != nil {
		return err
	}

	return chownDir(targetDir, uid, gid)
}

// Backup runs Mirror and then commits any changes to the git repository
// rooted at ~/.pirewall using "auto commit" as the commit message.
// If there is nothing to commit, no git operations are performed.
func Backup(ctx context.Context, username string) error {
	targetDir, uid, gid, err := userTargetDir(username)
	if err != nil {
		return err
	}

	if err := mirror(ctx, targetDir, "/"); err != nil {
		return err
	}

	if err := chownDir(targetDir, uid, gid); err != nil {
		return err
	}

	return gitCommit(ctx, targetDir)
}

func mirror(ctx context.Context, targetDir, root string) error {
	if err := os.MkdirAll(targetDir, dirMode); err != nil {
		return fmt.Errorf("create target dir %s: %w", targetDir, err)
	}

	if err := os.Chmod(targetDir, dirMode); err != nil {
		return fmt.Errorf("chmod target dir %s: %w", targetDir, err)
	}

	return filepath.WalkDir(targetDir, func(path string, d fs.DirEntry, err error) error {
		return mirrorEntry(ctx, targetDir, root, path, d, err)
	})
}

func mirrorEntry(ctx context.Context, targetDir, root, path string, d fs.DirEntry, err error) error {
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

	rel, err := filepath.Rel(targetDir, path)
	if err != nil {
		return err
	}

	src := filepath.Join(root, rel)

	info, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			slog.WarnContext(ctx, "source file not found, skipping", "src", src)

			return nil
		}

		return fmt.Errorf("stat %s: %w", src, err)
	}

	if !info.Mode().IsRegular() {
		slog.InfoContext(ctx, "source is not a regular file, skipping", "src", src)

		return nil
	}

	slog.InfoContext(ctx, "mirroring", "src", src, "dst", path)

	return util.CopyFileContents(src, path)
}

func chownDir(targetDir string, uid, gid int) error {
	return filepath.WalkDir(targetDir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		return os.Lchown(path, uid, gid)
	})
}

func gitCommit(ctx context.Context, dir string) error {
	out, _, err := util.ExecCommandOutput(ctx, "git", []string{"-C", dir, "status"})
	if err != nil {
		return fmt.Errorf("git status: %w", err)
	}

	if strings.Contains(out, "nothing to commit") {
		slog.InfoContext(ctx, "nothing to commit", "dir", dir)
		return nil
	}

	if _, _, err = util.ExecCommandOutput(ctx, "git", []string{"-C", dir, "add", "."}); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	if _, _, err = util.ExecCommandOutput(ctx, "git", []string{"-C", dir, "commit", "-m", "auto commit"}); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	return nil
}

func userTargetDir(username string) (targetDir string, uid, gid int, err error) {
	u, err := user.Lookup(username)
	if err != nil {
		return "", 0, 0, fmt.Errorf("lookup user %q: %w", username, err)
	}

	uid, err = strconv.Atoi(u.Uid)
	if err != nil {
		return "", 0, 0, fmt.Errorf("parse uid for %q: %w", username, err)
	}

	gid, err = strconv.Atoi(u.Gid)
	if err != nil {
		return "", 0, 0, fmt.Errorf("parse gid for %q: %w", username, err)
	}

	if u.HomeDir == "" {
		return "", 0, 0, fmt.Errorf("user %q: %w", username, ErrNoHomeDir)
	}

	return filepath.Join(u.HomeDir, pirewallDir), uid, gid, nil
}
