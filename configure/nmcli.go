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
	"regexp"
	"strings"

	"github.com/e4jet/pirewall/util"
)

const (
	nmcliBin        = "/usr/bin/nmcli"
	device          = "GENERAL.DEVICE:"
	deviceInterface = "connection.interface-name"
	connection      = "GENERAL.CONNECTION:"
	connectionID    = "connection.id"
	ipV6Method      = "ipv6.method"
	ipV4Method      = "ipv4.method"
	ipV4Dns         = "ipv4.dns"
	ipV4DnsSearch   = "ipv4.dns-search"
	ipV4Gateway     = "ipv4.gateway"
	ipV4Addresses   = "ipv4.addresses"
)

type networkInfo struct {
	device        string
	connection    string
	ipv6Method    string
	ipv4Method    string
	ipv4Addresses string
	ipv4Gateway   string
	ipv4Dns       string
	ipv4DnsSearch string
}

type renameConnection struct {
	dev               string
	newConnectionName string
	netInfo           networkInfo
}

func (r *renameConnection) Name() string {
	return fmt.Sprintf("%T - %s.%s", r, r.dev, r.newConnectionName)
}

func (r *renameConnection) Run() (result interface{}, err error) {
	r.netInfo, err = modifyConnection(r.dev, connectionID, r.newConnectionName)
	return r.netInfo, err
}

func (r *renameConnection) Rollback() (err error) {
	_, err = modifyConnection(r.dev, connectionID, r.netInfo.connection)
	return err
}

type disableIPV6 struct {
	dev     string
	netInfo networkInfo
}

func (r *disableIPV6) Name() string {
	return fmt.Sprintf("%T - %s", r, r.dev)
}

func (r *disableIPV6) Run() (result interface{}, err error) {
	r.netInfo, err = modifyConnection(r.dev, ipV6Method, "disabled")
	return r.netInfo, err
}

func (r *disableIPV6) Rollback() (err error) {
	_, err = modifyConnection(r.dev, ipV6Method, r.netInfo.ipv6Method)
	return err
}

type enableIPV4Dhcp struct {
	dev     string
	netInfo networkInfo
}

func (r *enableIPV4Dhcp) Name() string {
	return fmt.Sprintf("%T - %s", r, r.dev)
}

func (r *enableIPV4Dhcp) Run() (result interface{}, err error) {
	r.netInfo, err = modifyConnection(r.dev, ipV4Method, "auto")
	return r.netInfo, err
}

func (r *enableIPV4Dhcp) Rollback() (err error) {
	_, err = modifyConnection(r.dev, ipV4Method, r.netInfo.ipv4Method)
	return err
}

type setIPV4Address struct {
	dev     string
	ip      string
	netInfo networkInfo
}

func (r *setIPV4Address) Name() string {
	return fmt.Sprintf("%T - %s.%s", r, r.dev, r.ip)
}

func (r *setIPV4Address) Run() (result interface{}, err error) {
	r.netInfo, err = modifyConnection(r.dev, ipV4Addresses, r.ip)
	if err != nil {
		return nil, err
	}
	return modifyConnection(r.dev, ipV4Method, "manual")
}

func (r *setIPV4Address) Rollback() (err error) {
	modifyConnection(r.dev, ipV4Addresses, r.netInfo.ipv4Addresses)
	_, err = modifyConnection(r.dev, ipV4Method, r.netInfo.ipv4Method)
	return err
}

type setIPV4Getway struct {
	dev     string
	ip      string
	netInfo networkInfo
}

func (r *setIPV4Getway) Name() string {
	return fmt.Sprintf("%T - %s.%s", r, r.dev, r.ip)
}

func (r *setIPV4Getway) Run() (result interface{}, err error) {
	r.netInfo, err = modifyConnection(r.dev, ipV4Gateway, r.ip)
	return r.netInfo, err
}

func (r *setIPV4Getway) Rollback() (err error) {
	_, err = modifyConnection(r.dev, ipV4Gateway, r.netInfo.ipv4Gateway)
	return err
}

type setIPV4Dns struct {
	dev     string
	ip      string
	netInfo networkInfo
}

func (r *setIPV4Dns) Name() string {
	return fmt.Sprintf("%T - %s.%s", r, r.dev, r.ip)
}

func (r *setIPV4Dns) Run() (result interface{}, err error) {
	r.netInfo, err = modifyConnection(r.dev, ipV4Dns, r.ip)
	return r.netInfo, err
}

func (r *setIPV4Dns) Rollback() (err error) {
	_, err = modifyConnection(r.dev, ipV4Dns, r.netInfo.ipv4Dns)
	return err
}

type setIPV4DnsSearch struct {
	dev     string
	domain  string
	netInfo networkInfo
}

func (r *setIPV4DnsSearch) Name() string {
	return fmt.Sprintf("%T - %s.%s", r, r.dev, r.domain)
}

func (r *setIPV4DnsSearch) Run() (result interface{}, err error) {
	r.netInfo, err = modifyConnection(r.dev, ipV4DnsSearch, r.domain)
	return r.netInfo, err
}

func (r *setIPV4DnsSearch) Rollback() (err error) {
	_, err = modifyConnection(r.dev, ipV4DnsSearch, r.netInfo.ipv4DnsSearch)
	return err
}

// returns the netInfo captured before the change
func modifyConnection(dev string, setting string, value string) (netInfo networkInfo, err error) {
	netInfo, err = getDeviceInfo(dev)
	if err != nil {
		return netInfo, err
	}
	command := []string{"connection", "mod", netInfo.connection, setting, value}
	output, _, err := util.ExecCommandOutput(nmcliBin, command)
	if err != nil {
		fmt.Println(output)
	}
	return netInfo, err
}

func getDeviceInfo(dev string) (netInfo networkInfo, err error) {
	command := []string{"-m", "multiline", "device", "show", dev}
	out, _, err := util.ExecCommandOutput(nmcliBin, command)
	if err != nil {
		return netInfo, err
	}
	netInfo = processNmcli(out)

	return netInfo, err
}

func getConnectionInfo(connection string) (netInfo networkInfo, err error) {
	command := []string{"-m", "multiline", "connection", "show", connection}
	out, _, err := util.ExecCommandOutput(nmcliBin, command)
	if err != nil {
		return netInfo, err
	}
	netInfo = processNmcli(out)
	return netInfo, err
}

func processNmcli(output string) networkInfo {
	regx := regexp.MustCompile(`.*\s+(.*)`)
	info := networkInfo{}
	var regexReturn []string
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, device) || strings.HasPrefix(line, deviceInterface) {
			regexReturn = regx.FindStringSubmatch(line)
			if len(regexReturn) > 1 {
				info.device = regexReturn[1]
			}
			continue
		}
		if strings.HasPrefix(line, connection) || strings.HasPrefix(line, connectionID) {
			regexReturn = regx.FindStringSubmatch(line)
			if len(regexReturn) > 1 {
				info.connection = regexReturn[1]
			}
			continue
		}
		if strings.HasPrefix(line, ipV6Method) {
			regexReturn = regx.FindStringSubmatch(line)
			if len(regexReturn) > 1 {
				info.ipv6Method = regexReturn[1]
			}
			continue
		}
		if strings.HasPrefix(line, ipV4Method) {
			regexReturn = regx.FindStringSubmatch(line)
			if len(regexReturn) > 1 {
				info.ipv4Method = regexReturn[1]
			}
			continue
		}
		if strings.HasPrefix(line, ipV4Addresses) {
			regexReturn = regx.FindStringSubmatch(line)
			if len(regexReturn) > 1 {
				info.ipv4Addresses = regexReturn[1]
			}
			continue
		}
		if strings.HasPrefix(line, ipV4Gateway) {
			regexReturn = regx.FindStringSubmatch(line)
			if len(regexReturn) > 1 {
				info.ipv4Gateway = regexReturn[1]
			}
			continue
		}
		if strings.HasPrefix(line, ipV4Dns+":") {
			regexReturn = regx.FindStringSubmatch(line)
			if len(regexReturn) > 1 {
				info.ipv4Dns = regexReturn[1]
			}
			continue
		}
		if strings.HasPrefix(line, ipV4DnsSearch) {
			regexReturn = regx.FindStringSubmatch(line)
			if len(regexReturn) > 1 {
				info.ipv4DnsSearch = regexReturn[1]
			}
			continue
		}
	}
	return info
}
