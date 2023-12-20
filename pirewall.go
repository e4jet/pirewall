/*
(c) Copyright Eric Paul Forgette

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
package main

import (
	"fmt"

	"github.com/e4jet/pirewall/chain"
	"github.com/e4jet/pirewall/configure"
)

const (
	// my name
	me = "pirewall"
)

func main() {
	fmt.Printf("%s\n", me)
	firsttry := chain.NewChain(1, 1)
	firsttry.AppendRunner(&configure.QuickTest{})
	err := firsttry.Execute()
	if err != nil {
		fmt.Println("Woohoo")
	}
}
