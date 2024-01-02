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
	"testing"
)

func TestProcessNmcli(t *testing.T) {
	output := "GENERAL.DEVICE:                         end0\nGENERAL.TYPE:                           ethernet\nGENERAL.HWADDR:                         DC:A6:32:4E:F0:AB\nGENERAL.MTU:                            1500\nGENERAL.STATE:                          100 (connected)\nGENERAL.CONNECTION:                     provider\n"
	info := processNmcli(output)
	result := networkInfo{device: "end0", connection: "provider"}
	if info != result {
		t.FailNow()
	}
}

func TestProcessNmcli2(t *testing.T) {
	output := "connection.id:                          public\nconnection.interface-name:              end0\nipv4.method:                            auto\nipv4.dns:                               192.168.1.1\nipv4.dns-search:                        e4jet.net\nipv4.addresses:                         --\nipv4.gateway:                           --\nipv6.method:                            disabled"
	info := processNmcli(output)
	result := networkInfo{device: "end0", connection: "public", ipv6Method: "disabled", ipv4Method: "auto", ipv4Addresses: "--", ipv4Gateway: "--", ipv4Dns: "192.168.1.1", ipv4DnsSearch: "e4jet.net"}
	if info != result {
		t.FailNow()
	}
}
