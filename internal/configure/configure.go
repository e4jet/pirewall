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
	"log/slog"
	"time"

	"github.com/e4jet/pirewall/internal/chain"
)

const (
	retries    = 2
	retryDelay = 5 * time.Second

	defaultLocale   = "en_US.UTF-8"
	defaultTimezone = "America/New_York"

	// fanGPIOPin is the GPIO pin used for the fan control relay.
	fanGPIOPin = "14"
	// fanTempThreshold is the temperature in Celsius at which the fan turns on.
	fanTempThreshold = "80"
)

// RemoveUnwantedPackages purges OS packages that are not needed on a firewall.
func RemoveUnwantedPackages(ctx context.Context) error {
	slog.InfoContext(ctx, "👉 removing unwanted packages")

	return chain.NewChain(retries, retryDelay,
		&trimPackages{},
		&aptUpdate{},
		&aptUpgrade{},
		&aptPurge{},
		&aptClean{},
	).Execute(ctx)
}

// AddPackages installs packages that are useful for a firewall node.
func AddPackages(ctx context.Context) error {
	slog.InfoContext(ctx, "👉 adding useful packages")

	return chain.NewChain(retries, retryDelay,
		&aptUpdate{},
		&aptUpgrade{},
		&aptInstall{[]string{"bmon", "dnsmasq", "dnsutils", "iptables-persistent"}},
		&aptInstall{[]string{"git", "unattended-upgrades", "apt-listchanges", "vlan"}},
		&createDdclientConf{},
		&aptInstall{[]string{"netplan.io", "ddclient", "nload", "iftop"}},
	).Execute(ctx)
}

// EnableNewServices starts and enables services required by the firewall.
func EnableNewServices(ctx context.Context) error {
	slog.InfoContext(ctx, "👉 enabling new services")

	return chain.NewChain(retries, retryDelay,
		&startService{"unattended-upgrades"},
		&enableService{"unattended-upgrades"},
	).Execute(ctx)
}

// DisableUnwantedServices stops and disables services not needed on a firewall.
func DisableUnwantedServices(ctx context.Context) error {
	slog.InfoContext(ctx, "👉 disabling unneeded services")

	return chain.NewChain(retries, retryDelay,
		&stopService{service: "bluetooth"},
		&disableService{service: "bluetooth"},
		&stopService{service: "sound.target"},
		&disableService{service: "sound.target"},
	).Execute(ctx)
}

// ConfigSysCtl updates /etc/sysctl.conf with firewall-appropriate kernel settings.
func ConfigSysCtl(ctx context.Context) error {
	slog.InfoContext(ctx, "👉 adjusting /etc/sysctl.conf")

	runner := &configSysCtl{}
	if _, err := runner.Run(ctx); err != nil {
		_ = runner.Rollback(ctx)
		return err
	}

	return nil
}

// ConfigRaspi applies Raspberry Pi-specific configuration via raspi-config.
func ConfigRaspi(ctx context.Context) error {
	slog.InfoContext(ctx, "👉 adjusting settings using raspi-config")

	return chain.NewChain(retries, retryDelay,
		&screenBlanking{setting: "0"},
		&fanControl{setting: "0", pin: fanGPIOPin, temp: fanTempThreshold},
		&predictableNetNames{setting: "0"},
		&setLocale{setting: defaultLocale},
		&setTimezone{setting: defaultTimezone},
	).Execute(ctx)
}
