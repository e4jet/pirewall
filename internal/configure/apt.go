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
	aptgetBin = "/usr/bin/apt-get"
)

// trimPackages uses apt-get purge to remove unnecessary OS packages.
// --ignore-missing is included so that glob patterns (e.g. libx11.*) that
// match no installed packages do not cause a hard failure.
type trimPackages struct{}

func (t *trimPackages) Name() string {
	return "trimPackages"
}

func (t *trimPackages) Run(ctx context.Context) (any, error) {
	command := []string{"purge", "-y", "--ignore-missing"}
	packages := []string{"libx11.*", "libqt.*", "aardvark-dns", "wireless-*", "triggerhappy", "avahi-daemon"}
	out, _, err := util.ExecCommandOutput(ctx, aptgetBin, append(command, packages...))

	return out, err
}

func (t *trimPackages) Rollback(_ context.Context) error {
	// no op, we don't reinstall packages...
	return nil
}

// createDdclientConf creates an empty /etc/ddclient.conf so that the
// subsequent ddclient install does not trigger an interactive debconf prompt.
type createDdclientConf struct{}

func (c *createDdclientConf) Name() string {
	return "createDdclientConf"
}

func (c *createDdclientConf) Run(_ context.Context) (any, error) {
	f, err := os.Create("/etc/ddclient.conf")
	if err != nil {
		return nil, err
	}

	return nil, f.Close()
}

func (c *createDdclientConf) Rollback(_ context.Context) error {
	return os.Remove("/etc/ddclient.conf")
}

// aptInstall uses apt-get install to add packages.
type aptInstall struct {
	packages []string
}

func (a *aptInstall) Name() string {
	return "aptInstall(" + strings.Join(a.packages, ",") + ")"
}

func (a *aptInstall) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, aptgetBin, append([]string{"install", "-yqq"}, a.packages...))

	return out, err
}

func (a *aptInstall) Rollback(_ context.Context) error {
	// no op, not an issue if these are already there...
	return nil
}

// aptPurge uses apt-get autopurge to remove packages that were automatically
// installed to satisfy dependencies and are now no longer needed. Purge removes
// config files as well.
type aptPurge struct{}

func (a *aptPurge) Name() string {
	return "aptPurge"
}

func (a *aptPurge) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, aptgetBin, []string{"autopurge", "-y"})
	return out, err
}

func (a *aptPurge) Rollback(_ context.Context) error {
	// no op
	return nil
}

// aptUpdate uses apt-get update to download package information from all
// configured sources.
type aptUpdate struct{}

func (a *aptUpdate) Name() string {
	return "aptUpdate"
}

func (a *aptUpdate) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, aptgetBin, []string{"update"})
	return out, err
}

func (a *aptUpdate) Rollback(_ context.Context) error {
	// no op
	return nil
}

// aptUpgrade uses apt-get upgrade to install the newest versions of all
// currently installed packages.
type aptUpgrade struct{}

func (a *aptUpgrade) Name() string {
	return "aptUpgrade"
}

func (a *aptUpgrade) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, aptgetBin, []string{"upgrade", "-y"})
	return out, err
}

func (a *aptUpgrade) Rollback(_ context.Context) error {
	// no op, we don't reinstall packages...
	return nil
}

// aptClean uses apt-get clean to clear the local repository of retrieved
// package files. It removes everything but the lock file from
// /var/cache/apt/archives/ and /var/cache/apt/archives/partial/.
type aptClean struct{}

func (a *aptClean) Name() string {
	return "aptClean"
}

func (a *aptClean) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, aptgetBin, []string{"clean"})
	return out, err
}

func (a *aptClean) Rollback(_ context.Context) error {
	// no op, we don't reinstall packages...
	return nil
}
