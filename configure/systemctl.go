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
	systemctl = "/usr/bin/systemctl"
)

// systemctl is used to disable a service
type disableService struct {
	service string
}

func (d *disableService) Name() string {
	return fmt.Sprintf("%T.%s", d, d.service)
}

func (d *disableService) Run() (result interface{}, err error) {
	command := []string{"disable", d.service}
	out, _, err := util.ExecCommandOutput(systemctl, command)
	return out, err
}

func (d *disableService) Rollback() (err error) {
	// no op
	return nil
}

// systemctl is used to stop a service
type stopService struct {
	service string
}

func (s *stopService) Name() string {
	return fmt.Sprintf("%T.%s", s, s.service)
}

func (s *stopService) Run() (result interface{}, err error) {
	command := []string{"stop", s.service}
	out, _, err := util.ExecCommandOutput(systemctl, command)
	return out, err
}

func (s *stopService) Rollback() (err error) {
	// no op
	return nil
}

// systemctl is used to start a service
type startService struct {
	service string
}

func (s *startService) Name() string {
	return fmt.Sprintf("%T.%s", s, s.service)
}

func (s *startService) Run() (result interface{}, err error) {
	command := []string{"start", s.service}
	out, _, err := util.ExecCommandOutput(systemctl, command)
	return out, err
}

func (s *startService) Rollback() (err error) {
	stopper := &stopService{service: s.service}
	_, err = stopper.Run()
	return err
}

// systemctl is used to enable a service
type enableService struct {
	service string
}

func (s *enableService) Name() string {
	return fmt.Sprintf("%T.%s", s, s.service)
}

func (s *enableService) Run() (result interface{}, err error) {
	command := []string{"enable", s.service}
	out, _, err := util.ExecCommandOutput(systemctl, command)
	return out, err
}

func (s *enableService) Rollback() (err error) {
	disabler := &disableService{service: s.service}
	_, err = disabler.Run()
	return err
}
