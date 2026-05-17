/*
(c) Copyright 2017 Hewlett Packard Enterprise Development LP

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

/*
(c) Copyright 2023 Eric Paul Forgette - changes since fork

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.

This work was forked from the following repository
https://github.com/hpe-storage/common-host-libs/
*/

package util

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	defaultFileMode = 0644
)

var (
	// ErrIrregularSourceFile is returned when CopyFile's source is not a regular file.
	ErrIrregularSourceFile = errors.New("CopyFile: non-regular source file")
	// ErrIrregularDestFile is returned when CopyFile's destination is not a regular file.
	ErrIrregularDestFile = errors.New("CopyFile: non-regular destination file")
	// ErrSameFile is returned when CopyFileContents' src and dst are the same file.
	ErrSameFile = errors.New("CopyFileContents: src and dst are the same file")
)

// FileExists reports whether a file or directory exists at path.
// Returns (exists, isDir, err).
func FileExists(path string) (exists bool, dir bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, false, nil
		}

		return false, false, err
	}

	return true, info.IsDir(), nil
}

// FileWriteAtomic writes content to path atomically: it creates a temporary
// file in the same directory, writes and fsyncs the content, applies mode
// (via fchmod on the open fd), optionally chowns the temp file, and renames
// it into place. On any failure after temp creation, the temp file is removed.
//
// chown is the function invoked to apply ownership to the temp file before
// rename; pass nil to skip ownership changes. uid/gid are ignored when chown
// is nil.
func FileWriteAtomic(path string, content []byte, mode os.FileMode,
	chown func(string, int, int) error, uid, gid int) (err error) {
	dir := filepath.Dir(path)

	tmp, err := os.CreateTemp(dir, ".tmp-*")
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

	if err = tmp.Chmod(mode); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("chmod %s: %w", tmpName, err)
	}

	if err = tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}

	if chown != nil {
		if err = chown(tmpName, uid, gid); err != nil {
			return fmt.Errorf("chown %s: %w", tmpName, err)
		}
	}

	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("rename %s -> %s: %w", tmpName, path, err)
	}

	return nil
}

// FileWriteStrings writes lines to path atomically using FileWriteAtomic. Each
// line has any trailing newlines trimmed and is terminated with a single \n.
// The file is written with mode 0644 and no ownership change.
func FileWriteStrings(path string, lines []string) error {
	var buf bytes.Buffer

	for _, line := range lines {
		buf.WriteString(strings.TrimRight(line, "\n"))
		buf.WriteByte('\n')
	}

	return FileWriteAtomic(path, buf.Bytes(), defaultFileMode, nil, 0, 0)
}

// FileGetStringsWithPattern reads a file and returns lines matching pattern.
// If pattern is empty, all lines are returned. When the pattern contains a
// capture group, the first captured group is returned instead of the full line.
func FileGetStringsWithPattern(path string, pattern string) (filelines []string, err error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)

	if pattern == "" {
		var lines []string

		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}

		return lines, scanner.Err()
	}

	r := regexp.MustCompile(pattern)

	var matchingLines []string

	for scanner.Scan() {
		l := scanner.Text()

		m := r.FindStringSubmatch(l)
		if m == nil {
			continue
		}

		if len(m) > 1 {
			matchingLines = append(matchingLines, m[1])
		} else {
			matchingLines = append(matchingLines, l)
		}
	}

	return matchingLines, scanner.Err()
}

// FileGetStrings reads all lines from path.
func FileGetStrings(path string) (line []string, err error) {
	return FileGetStringsWithPattern(path, "")
}

// CopyFileContents copies the contents of src into dst.
// It returns an error if src and dst refer to the same file.
func CopyFileContents(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return err
	}

	dfi, err := os.Stat(dst)
	if err == nil && os.SameFile(sfi, dfi) {
		return fmt.Errorf("%w: %s", ErrSameFile, src)
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}

	defer func() { _ = in.Close() }()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}

	defer func() {
		cerr := out.Close()
		if err == nil {
			err = cerr
		}
	}()

	if _, err = io.Copy(out, in); err != nil {
		return err
	}

	return out.Sync()
}

// CopyFile copies src to dst. It uses a hard link when possible, falling back
// to a full content copy.
func CopyFile(src, dst string) (err error) {
	sfi, err := os.Stat(src)
	if err != nil {
		return err
	}

	if !sfi.Mode().IsRegular() {
		return fmt.Errorf("%w %s (%q)", ErrIrregularSourceFile, sfi.Name(), sfi.Mode().String())
	}

	dfi, err := os.Stat(dst)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		if !dfi.Mode().IsRegular() {
			return fmt.Errorf("%w %s (%q)", ErrIrregularDestFile, dfi.Name(), dfi.Mode().String())
		}

		if os.SameFile(sfi, dfi) {
			return nil
		}
	}

	if err = os.Link(src, dst); err == nil {
		return nil
	}

	return CopyFileContents(src, dst)
}
