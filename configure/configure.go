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

	"github.com/e4jet/pirewall/chain"
	"github.com/e4jet/pirewall/util"
)

const (
	retries = 2
)

func RemoveUnwantedPackages() error {
	fmt.Println("Removing packages that aren't needed.")
	cmdChain := chain.NewChain(retries, util.DefaultTimeout)
	cmdChain.AppendRunner(&trimPackages{})
	cmdChain.AppendRunner(&aptUpdate{})
	cmdChain.AppendRunner(&aptUpgrade{})
	cmdChain.AppendRunner(&aptPurge{})
	cmdChain.AppendRunner(&aptClean{})
	return cmdChain.Execute()
}

func AddPackages() error {
	fmt.Println("Adding useful packages.")
	cmdChain := chain.NewChain(retries, util.DefaultTimeout)
	cmdChain.AppendRunner(&aptUpdate{})
	cmdChain.AppendRunner(&aptUpgrade{})
	cmdChain.AppendRunner(&aptInstall{})
	return cmdChain.Execute()
}

func EnableNewServices() error {
	fmt.Println("Enabling new services.")
	cmdChain := chain.NewChain(retries, util.DefaultTimeout)
	cmdChain.AppendRunner(&startService{"unattended-upgrades"})
	cmdChain.AppendRunner(&enableService{"unattended-upgrades"})
	return cmdChain.Execute()
}

func DisableUnwantedServices() error {
	fmt.Println("Disabling unneeded services.")
	cmdChain := chain.NewChain(retries, util.DefaultTimeout)
	cmdChain.AppendRunner(&stopService{service: "bluetooth"})
	cmdChain.AppendRunner(&disableService{service: "bluetooth"})
	cmdChain.AppendRunner(&stopService{service: "sound.target"})
	cmdChain.AppendRunner(&disableService{service: "sound.target"})
	return cmdChain.Execute()
}

func ConfigSysCtl() error {
	fmt.Println("Adjusting /etc/sysctl.conf.")
	cmdChain := chain.NewChain(retries, util.DefaultTimeout)
	cmdChain.AppendRunner(&configSysCtl{})
	return cmdChain.Execute()
}

func ConfigRaspi() error {
	fmt.Println("Adjusting settings using raspi-conf.")
	cmdChain := chain.NewChain(retries, util.DefaultTimeout)
	cmdChain.AppendRunner(&screenBlanking{setting: "0"})
	cmdChain.AppendRunner(&fanControl{setting: "0"})
	cmdChain.AppendRunner(&predictableNetNames{setting: "0"})
	cmdChain.AppendRunner(&setLocale{setting: "en_US.UTF-8"})
	cmdChain.AppendRunner(&setTimezone{setting: "America/New_York"})
	return cmdChain.Execute()
}

func ConfigNetworkPublic() error {
	fmt.Println("Configuraing public facing network connection.")
	cmdChain := chain.NewChain(retries, util.DefaultTimeout)
	cmdChain.AppendRunner(&renameConnection{dev: "eth0", newConnectionName: "public"})
	cmdChain.AppendRunner(&disableIPV6{dev: "eth0"})
	return cmdChain.Execute()
}
