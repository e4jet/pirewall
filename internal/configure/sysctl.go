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
	"strings"

	"github.com/e4jet/pirewall/internal/util"
)

const (
	defaultSysctlFile            = "/etc/sysctl.conf"
	defaultSysctlFileBackup      = "/etc/sysctl.conf.original"
	defaultSysctlDFile           = "/usr/lib/sysctl.d/50-default.conf"
	defaultSysctlDOverride       = "/etc/sysctl.d/90-override.conf"
	defaultSysctlDOverrideBackup = "/etc/sysctl.d/90-override.conf.original"
)

// modify sysctl.conf to make us a router.
// file and backup may be set in tests; production code leaves them empty to use
// the default paths.
type configSysCtl struct {
	file   string
	backup string
}

func (c *configSysCtl) Name() string {
	return "configSysCtl"
}

func (c *configSysCtl) Run(ctx context.Context) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := c.backupSysCtl(); err != nil {
		return nil, err
	}

	settings := []string{
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
	}

	sysctlData, err := util.FileGetStrings(c.sysctlFile())
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	err = util.FileWriteStrings(c.sysctlFile(), mergeSysctlSettings(sysctlData, settings))

	return nil, err
}

func (c *configSysCtl) Rollback(_ context.Context) error {
	backupThere, _, err := util.FileExists(c.sysctlFileBackup())
	if err != nil {
		return err
	}

	if backupThere {
		return util.CopyFile(c.sysctlFileBackup(), c.sysctlFile())
	}

	// No backup means the file was newly created; remove it on rollback.
	err = os.Remove(c.sysctlFile())
	if os.IsNotExist(err) {
		return nil
	}

	return err
}

func (c *configSysCtl) backupSysCtl() error {
	srcThere, _, err := util.FileExists(c.sysctlFile())
	if err != nil {
		return err
	}

	if !srcThere {
		return nil // file will be created fresh; nothing to back up
	}

	backupThere, _, err := util.FileExists(c.sysctlFileBackup())
	if err != nil {
		return err
	}

	if !backupThere {
		err = util.CopyFileContents(c.sysctlFile(), c.sysctlFileBackup())
	}

	return err
}

func (c *configSysCtl) sysctlFile() string {
	if c.file != "" {
		return c.file
	}

	file, _ := resolveSysctlPaths()

	return file
}

func (c *configSysCtl) sysctlFileBackup() string {
	if c.backup != "" {
		return c.backup
	}

	_, backup := resolveSysctlPaths()

	return backup
}

// resolveSysctlPaths returns the sysctl file and backup paths to use.
// When /usr/lib/sysctl.d/50-default.conf exists, settings are written to
// /etc/sysctl.d/90-override.conf rather than modifying the default file.
// Otherwise /etc/sysctl.conf is used.
func resolveSysctlPaths() (file, backup string) {
	return resolveSysctlPathsFrom(defaultSysctlDFile, defaultSysctlDOverride, defaultSysctlDOverrideBackup, defaultSysctlFile, defaultSysctlFileBackup)
}

func resolveSysctlPathsFrom(dFile, override, overrideBackup, primary, primaryBackup string) (file, backup string) {
	exists, _, err := util.FileExists(dFile)
	if err == nil && exists {
		return override, overrideBackup
	}

	return primary, primaryBackup
}

// mergeSysctlSettings merges desired settings into the existing file lines.
// For each line in existing, if its key matches a desired setting the line is
// replaced with the desired value (uncommenting it if necessary). Settings
// whose key does not appear in existing are appended at the end.
func mergeSysctlSettings(existing, settings []string) []string {
	type entry struct {
		key     string
		setting string
	}

	ordered := make([]entry, 0, len(settings))
	byKey := make(map[string]string, len(settings))

	for _, s := range settings {
		k := sysctlKey(s)
		byKey[k] = s
		ordered = append(ordered, entry{k, s})
	}

	applied := make(map[string]bool, len(settings))

	var changes []string

	for _, line := range existing {
		k := sysctlKey(line)
		if s, ok := byKey[k]; ok && k != "" {
			// Replace the existing line (or commented-out line) with the desired setting.
			changes = append(changes, s)
			applied[k] = true
		} else {
			changes = append(changes, line)
		}
	}

	// Append any settings whose key was not found anywhere in the file.
	for _, e := range ordered {
		if !applied[e.key] {
			changes = append(changes, e.setting)
		}
	}

	return changes
}

// sysctlKey extracts the parameter name from a sysctl setting or file line.
// It strips leading comment characters and whitespace before splitting on '=',
// so it matches both active lines and commented-out lines with the same key.
// Returns an empty string for lines that contain no '='.
func sysctlKey(s string) string {
	s = strings.TrimLeft(strings.TrimSpace(s), "#")
	s = strings.TrimSpace(s)

	key, _, ok := strings.Cut(s, "=")
	if !ok {
		return ""
	}

	return strings.TrimSpace(key)
}
