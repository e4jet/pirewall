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
	"os"

	"github.com/e4jet/pirewall/configure"
)

const (
	// my name
	me   = "pirewall"
	fail = 3
	pass = 0
)

func main() {
	fmt.Printf("%s\n", me)
	err := configure.RemoveUnwantedPackages()
	if err != nil {
		fmt.Println(err)
		os.Exit(fail)
	}

	err = configure.AddPackages()
	if err != nil {
		fmt.Println(err)
		os.Exit(fail)
	}

	err = configure.EnableNewServices()
	if err != nil {
		fmt.Println(err)
		os.Exit(fail)
	}

	err = configure.DisableUnwantedServices()
	if err != nil {
		fmt.Println(err)
		os.Exit(fail)
	}

	err = configure.ConfigSysCtl()
	if err != nil {
		fmt.Println(err)
		os.Exit(fail)
	}

	fmt.Println("Done!")
	os.Exit(0)
}
