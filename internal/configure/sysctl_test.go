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

package configure

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/e4jet/pirewall/internal/util"
)

// withTempSysctl creates temporary sysctl files and returns a configSysCtl
// instance configured to use them. Each call is independent, making it safe
// for parallel tests.
func withTempSysctl(t *testing.T, content string) (path, backup string, runner *configSysCtl) {
	t.Helper()

	dir := t.TempDir()
	path = filepath.Join(dir, "sysctl.conf")
	backup = filepath.Join(dir, "sysctl.conf.original")

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp sysctl.conf: %v", err)
	}

	runner = &configSysCtl{file: path, backup: backup}

	return path, backup, runner
}

// readLines reads a file and returns its lines.
func readLines(t *testing.T, path string) []string {
	t.Helper()

	lines, err := util.FileGetStrings(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return lines
}

// containsSetting reports whether lines contains a line equal to setting.
func containsSetting(lines []string, setting string) bool {
	for _, l := range lines {
		if l == setting {
			return true
		}
	}

	return false
}

func TestConfigSysCtlAllPresent(t *testing.T) {
	t.Parallel()

	content := strings.Join([]string{
		"net.ipv4.conf.default.rp_filter=1",
		"net.ipv4.conf.all.rp_filter=1",
		"net.ipv4.tcp_syncookies=1",
		"net.ipv4.ip_forward=1",
		"net.ipv4.conf.all.accept_redirects=0",
		"net.ipv6.conf.all.accept_redirects=0",
		"net.ipv4.conf.all.send_redirects=0",
		"net.ipv4.conf.all.accept_source_route=0",
		"net.ipv6.conf.all.accept_source_route=0",
		"net.ipv4.conf.all.log_martians=1",
	}, "\n") + "\n"

	path, _, runner := withTempSysctl(t, content)

	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := readLines(t, path)

	if !containsSetting(lines, "net.ipv4.ip_forward=1") {
		t.Error("expected net.ipv4.ip_forward=1 in output")
	}
}

func TestConfigSysCtlWrongValues(t *testing.T) {
	t.Parallel()

	// Settings are present but with wrong values.
	content := strings.Join([]string{
		"net.ipv4.conf.default.rp_filter=0",
		"net.ipv4.conf.all.rp_filter=0",
		"net.ipv4.tcp_syncookies=0",
		"net.ipv4.ip_forward=0",
		"net.ipv4.conf.all.accept_redirects=1",
		"net.ipv6.conf.all.accept_redirects=1",
		"net.ipv4.conf.all.send_redirects=1",
		"net.ipv4.conf.all.accept_source_route=1",
		"net.ipv6.conf.all.accept_source_route=1",
		"net.ipv4.conf.all.log_martians=0",
	}, "\n") + "\n"

	path, _, runner := withTempSysctl(t, content)

	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := readLines(t, path)

	for _, want := range []string{
		"net.ipv4.ip_forward=1",
		"net.ipv4.conf.all.log_martians=1",
		"net.ipv4.conf.all.accept_redirects=0",
	} {
		if !containsSetting(lines, want) {
			t.Errorf("expected %q in output", want)
		}
	}
}

func TestConfigSysCtlCommentedOut(t *testing.T) {
	t.Parallel()

	// Settings are commented out; Run should uncomment and set the correct value.
	content := strings.Join([]string{
		"#net.ipv4.conf.default.rp_filter=1",
		"#net.ipv4.conf.all.rp_filter=1",
		"#net.ipv4.tcp_syncookies=1",
		"#net.ipv4.ip_forward=0",
		"#net.ipv4.conf.all.accept_redirects=0",
		"#net.ipv6.conf.all.accept_redirects=0",
		"#net.ipv4.conf.all.send_redirects=0",
		"#net.ipv4.conf.all.accept_source_route=0",
		"#net.ipv6.conf.all.accept_source_route=0",
		"#net.ipv4.conf.all.log_martians=1",
	}, "\n") + "\n"

	path, _, runner := withTempSysctl(t, content)

	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := readLines(t, path)

	// The commented line for ip_forward had value 0; should now be 1.
	if !containsSetting(lines, "net.ipv4.ip_forward=1") {
		t.Error("expected net.ipv4.ip_forward=1 after uncommenting")
	}

	// No commented-out versions of the keys should remain.
	for _, l := range lines {
		if strings.HasPrefix(l, "#net.ipv4.ip_forward") {
			t.Errorf("unexpected commented line in output: %q", l)
		}
	}
}

func TestConfigSysCtlMissingSettings(t *testing.T) {
	t.Parallel()

	// File contains only unrelated content; all settings must be appended.
	content := "# sysctl.conf\nkernel.sysrq=0\n"

	path, _, runner := withTempSysctl(t, content)

	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := readLines(t, path)

	for _, want := range []string{
		"net.ipv4.ip_forward=1",
		"net.ipv4.conf.all.log_martians=1",
		"net.ipv6.conf.all.accept_redirects=0",
	} {
		if !containsSetting(lines, want) {
			t.Errorf("expected appended setting %q in output", want)
		}
	}

	// Unrelated content must be preserved.
	if !containsSetting(lines, "kernel.sysrq=0") {
		t.Error("expected unrelated setting kernel.sysrq=0 to be preserved")
	}
}

func TestConfigSysCtlMixed(t *testing.T) {
	t.Parallel()

	// Some settings present with correct values, some with wrong values,
	// some commented out, and some missing entirely.
	content := strings.Join([]string{
		"net.ipv4.conf.default.rp_filter=1", // correct
		"net.ipv4.conf.all.rp_filter=0",     // wrong value
		"#net.ipv4.tcp_syncookies=1",        // commented
		"net.ipv4.ip_forward=0",             // wrong value
		"# unrelated comment",
		"kernel.sysrq=0", // unrelated, must be preserved
		// accept_redirects through log_martians are absent
	}, "\n") + "\n"

	path, _, runner := withTempSysctl(t, content)

	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := readLines(t, path)

	for _, want := range []string{
		"net.ipv4.conf.default.rp_filter=1",
		"net.ipv4.conf.all.rp_filter=1",
		"net.ipv4.tcp_syncookies=1",
		"net.ipv4.ip_forward=1",
		"net.ipv4.conf.all.accept_redirects=0",
		"net.ipv6.conf.all.accept_redirects=0",
		"net.ipv4.conf.all.send_redirects=0",
		"net.ipv4.conf.all.accept_source_route=0",
		"net.ipv6.conf.all.accept_source_route=0",
		"net.ipv4.conf.all.log_martians=1",
		"kernel.sysrq=0",
	} {
		if !containsSetting(lines, want) {
			t.Errorf("expected %q in output", want)
		}
	}
}

func TestConfigSysCtlBackupCreated(t *testing.T) {
	t.Parallel()

	content := "net.ipv4.ip_forward=0\n"
	_, backup, runner := withTempSysctl(t, content)

	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(backup); err != nil {
		t.Errorf("expected backup file to exist at %s: %v", backup, err)
	}
}

func TestConfigSysCtlRollback(t *testing.T) {
	t.Parallel()

	original := "net.ipv4.ip_forward=0\n"
	path, _, runner := withTempSysctl(t, original)

	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Verify the file was modified.
	lines := readLines(t, path)
	if containsSetting(lines, "net.ipv4.ip_forward=0") && !containsSetting(lines, "net.ipv4.ip_forward=1") {
		t.Fatal("Run did not update ip_forward")
	}

	if err := runner.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// After rollback the file should match the original content.
	restored, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after rollback: %v", err)
	}

	if string(restored) != original {
		t.Errorf("after rollback: got %q, want %q", string(restored), original)
	}
}

func TestConfigSysCtlRollbackNewFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "90-override.conf") // does not exist yet
	backup := filepath.Join(dir, "90-override.conf.original")
	runner := &configSysCtl{file: path, backup: backup}

	if _, err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected override file to exist after Run: %v", err)
	}

	if err := runner.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected override file to be removed after rollback, got: %v", err)
	}
}

func TestConfigSysCtlContextCancelled(t *testing.T) {
	t.Parallel()

	content := "net.ipv4.ip_forward=0\n"
	_, _, runner := withTempSysctl(t, content)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := runner.Run(ctx); err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestResolveSysctlPathsUsesOverrideWhenDFileExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dFile := filepath.Join(dir, "50-default.conf")
	override := filepath.Join(dir, "90-override.conf")
	overrideBackup := filepath.Join(dir, "90-override.conf.original")
	primary := filepath.Join(dir, "sysctl.conf")
	primaryBackup := filepath.Join(dir, "sysctl.conf.original")

	if err := os.WriteFile(dFile, []byte(""), 0644); err != nil {
		t.Fatalf("write dFile: %v", err)
	}

	file, backup := resolveSysctlPathsFrom(dFile, override, overrideBackup, primary, primaryBackup)

	if file != override {
		t.Errorf("file = %q, want %q", file, override)
	}

	if backup != overrideBackup {
		t.Errorf("backup = %q, want %q", backup, overrideBackup)
	}
}

func TestResolveSysctlPathsFallsBackToPrimary(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	dFile := filepath.Join(dir, "50-default.conf") // does not exist
	override := filepath.Join(dir, "90-override.conf")
	overrideBackup := filepath.Join(dir, "90-override.conf.original")
	primary := filepath.Join(dir, "sysctl.conf")
	primaryBackup := filepath.Join(dir, "sysctl.conf.original")

	file, backup := resolveSysctlPathsFrom(dFile, override, overrideBackup, primary, primaryBackup)

	if file != primary {
		t.Errorf("file = %q, want %q", file, primary)
	}

	if backup != primaryBackup {
		t.Errorf("backup = %q, want %q", backup, primaryBackup)
	}
}

func TestSysctlKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  string
	}{
		{"net.ipv4.ip_forward=1", "net.ipv4.ip_forward"},
		{"net.ipv4.ip_forward=0", "net.ipv4.ip_forward"},
		{"#net.ipv4.ip_forward=1", "net.ipv4.ip_forward"},
		{"# net.ipv4.ip_forward = 1", "net.ipv4.ip_forward"},
		{"net.ipv4.conf.all.accept_redirects = 0", "net.ipv4.conf.all.accept_redirects"},
		{"# just a comment", ""},
		{"", ""},
		{"kernel.sysrq=0", "kernel.sysrq"},
	}

	for _, tc := range cases {
		got := sysctlKey(tc.input)
		if got != tc.want {
			t.Errorf("sysctlKey(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}
