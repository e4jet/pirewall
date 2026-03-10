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
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const (
	// DefaultTimeout is the default command timeout in seconds.
	DefaultTimeout = 600
	// FailedToStart indicates the command could not be started.
	FailedToStart = 999
	// FailedWithoutStatus indicates the command failed with no exit status.
	FailedWithoutStatus = 888
)

func execCommandOutput(ctx context.Context, cmd string, args []string, stdinArgs []string, timeout int) (output string, returnCode int, err error) {
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	c := exec.CommandContext(runCtx, cmd, args...)
	slog.InfoContext(ctx, "running", "cmd", c.String())

	var b bytes.Buffer

	c.Stdout = &b

	c.Stderr = &b

	if len(stdinArgs) > 0 {
		c.Stdin = strings.NewReader(strings.Join(stdinArgs, "\n") + "\n")
	}

	if err = c.Start(); err != nil {
		slog.ErrorContext(ctx, "failed to start", "cmd", cmd, "err", err)
		return "", FailedToStart, fmt.Errorf("start %s: %w", cmd, err)
	}

	err = c.Wait()
	out := b.String()

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			rc := exitErr.ExitCode()
			slog.ErrorContext(ctx, "command failed", "cmd", cmd, "rc", rc, "out", out)

			return out, rc, fmt.Errorf("command %s rc=%d: %w", cmd, rc, err)
		}

		slog.ErrorContext(ctx, "command error", "cmd", cmd, "err", err)

		return out, FailedWithoutStatus, fmt.Errorf("command %s: %w", cmd, err)
	}

	slog.InfoContext(ctx, "command succeeded", "cmd", cmd)

	return out, 0, nil
}

// ExecCommandOutputWithTimeout executes cmd with the specified timeout in seconds.
func ExecCommandOutputWithTimeout(ctx context.Context, cmd string, args []string, timeout int) (output string, returnCode int, err error) {
	return execCommandOutput(ctx, cmd, args, []string{}, timeout)
}

// ExecCommandOutput executes cmd with the default timeout.
// Returns stdout+stderr, exit code, and error. A non-zero exit code yields a non-nil error.
// Return code FailedToStart (999) indicates the command could not be started.
func ExecCommandOutput(ctx context.Context, cmd string, args []string) (output string, returnCode int, err error) {
	return ExecCommandOutputWithTimeout(ctx, cmd, args, DefaultTimeout)
}
