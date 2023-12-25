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
	"fmt"
	"strings"

	"github.com/e4jet/pirewall/util"
)

const (
	sysctlFile       = "/etc/sysctl.conf"
	sysctlFileBackup = "/etc/sysctl.conf.orignial"
)

// modify sysctl.conf to make us a router
type configSysCtl struct {
}

func (c *configSysCtl) Name() string {
	return fmt.Sprintf("%T", c)
}

func (c *configSysCtl) Run() (result interface{}, err error) {
	err = backupSysCtl()
	if err != nil {
		return nil, err
	}
	settings := []string{"net.ipv4.conf.default.rp_filter=1", "net.ipv4.conf.all.rp_filter=1", "net.ipv4.tcp_syncookies=1",
		"net.ipv4.ip_forward=1", "net.ipv4.conf.all.accept_redirects = 0", "net.ipv6.conf.all.accept_redirects = 0",
		"net.ipv4.conf.all.send_redirects = 0", "net.ipv4.conf.all.accept_source_route = 0", "net.ipv6.conf.all.accept_source_route = 0",
		"net.ipv4.conf.all.log_martians = 1"}
	sysctlData, err := util.FileGetStrings(sysctlFile)
	if err != nil {
		return nil, err
	}
	var changes []string
	changeCount := 0
	for _, line := range sysctlData {
		written := false
		for _, setting := range settings {
			if strings.Contains(line, setting) {
				changes = append(changes, setting)
				changeCount++
				written = true
				break
			}
		}
		if !written {
			changes = append(changes, line)
		}
	}
	if len(settings) != changeCount {
		return nil, fmt.Errorf("not all changes expected were made")
	}
	err = util.FileWriteStrings(sysctlFile, changes)
	return nil, err
}

func (c *configSysCtl) Rollback() (err error) {
	there, _, err := util.FileExists(sysctlFileBackup)
	if err != nil {
		return err
	}
	if there {
		err = util.CopyFile(sysctlFileBackup, sysctlFile)
	}
	return err
}

func backupSysCtl() error {
	there, _, err := util.FileExists(sysctlFileBackup)
	if err != nil {
		return err
	}
	if !there {
		err = util.CopyFileContents(sysctlFile, sysctlFileBackup)
	}
	return err
}
