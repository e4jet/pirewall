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

	"github.com/e4jet/pirewall/chain"
	"github.com/e4jet/pirewall/util"
)

func TestCommand(t *testing.T) {
	cmdChain := chain.NewChain(retries, util.DefaultTimeout)
	cmdChain.AppendRunner(&trimPackages{})
	cmdChain.AppendRunner(&aptUpdate{})
	cmdChain.AppendRunner(&aptUpgrade{})
	cmdChain.AppendRunner(&aptPurge{})
	cmdChain.AppendRunner(&aptClean{})
	cmdChain.Execute()
}
