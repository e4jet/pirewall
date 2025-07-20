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

	"github.com/e4jet/pirewall/util"
)

const (
	aptgetBin = "/usr/bin/apt-get"
)

// apt-get purge is leveraged to remove unnecessary OS packages
type trimPackages struct {
}

func (t *trimPackages) Name() string {
	return fmt.Sprintf("%T", t)
}

func (t *trimPackages) Run() (result interface{}, err error) {
	command := []string{"purge", "-y"}
	packages := []string{"libx11.*", "libqt.*", "aardvark-dns", "wireless-*", "triggerhappy", "avahi-daemon"}
	out, _, err := util.ExecCommandOutput(aptgetBin, append(command, packages...))
	return out, err
}

func (t *trimPackages) Rollback() (err error) {
	// no op, we don't reinstall packages...
	return nil
}

// apt-get install is used to add packages that are useful (apt-get install -y bmon)
type aptInstall struct {
}

func (a *aptInstall) Name() string {
	return fmt.Sprintf("%T", a)
}

func (a *aptInstall) Run() (result interface{}, err error) {
	command := []string{"--yes", "--force-yes", "install", "-yqq"}
	// add ddclient eventually
	packages := []string{"bmon", "dnsmasq", "dnsutils", "iptables-persistent", "git", "unattended-upgrades", "apt-listchanges"}
	out, _, err := util.ExecCommandOutput(aptgetBin, append(command, packages...))
	return out, err
}

func (a *aptInstall) Rollback() (err error) {
	// no op, not an issue if these are already there...
	return nil
}

// apt-get autopurge used to remove packages that were automatically installed to satisfy dependencies for other packages
// and are now no longer needed.  Purge removes config files as well.
type aptPurge struct {
}

func (a *aptPurge) Name() string {
	return fmt.Sprintf("%T", a)
}

func (a *aptPurge) Run() (result interface{}, err error) {
	out, _, err := util.ExecCommandOutput(aptgetBin, []string{"autopurge", "-y"})
	return out, err
}

func (a *aptPurge) Rollback() (err error) {
	// no op
	return nil
}

// apt-get update is used to download package information from all configured sources
type aptUpdate struct {
}

func (a *aptUpdate) Name() string {
	return fmt.Sprintf("%T", a)
}

func (a *aptUpdate) Run() (result interface{}, err error) {
	out, _, err := util.ExecCommandOutput(aptgetBin, []string{"update"})
	return out, err
}

func (a *aptUpdate) Rollback() (err error) {
	// no op
	return nil
}

// apt-get upgrade is used to install the newest versions of all packages currently installed on the system
type aptUpgrade struct {
}

func (a *aptUpgrade) Name() string {
	return fmt.Sprintf("%T", a)
}

func (a *aptUpgrade) Run() (result interface{}, err error) {
	out, _, err := util.ExecCommandOutput(aptgetBin, []string{"upgrade", "-y"})
	return out, err
}

func (a *aptUpgrade) Rollback() (err error) {
	// no op, we don't reinstall packages...
	return nil
}

// apt-get clean clears out the local repository of retrieved package files. It removes
// everything but the lock file from /var/cache/apt/archives/ and /var/cache/apt/archives/partial/.
type aptClean struct {
}

func (a *aptClean) Name() string {
	return fmt.Sprintf("%T", a)
}

func (a *aptClean) Run() (result interface{}, err error) {
	out, _, err := util.ExecCommandOutput(aptgetBin, []string{"upgrade", "-y"})
	return out, err
}

func (a *aptClean) Rollback() (err error) {
	// no op, we don't reinstall packages...
	return nil
}
