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
	raspiconfigBin = "/usr/bin/raspi-config"
)

// 0 = enabled
type screenBlanking struct {
	setting string
}

func (r *screenBlanking) Name() string {
	return fmt.Sprintf("%T - %s", r, r.setting)
}

func (r *screenBlanking) Run() (result interface{}, err error) {
	command := []string{"nonint", "do_blanking", r.setting}
	out, _, err := util.ExecCommandOutput(raspiconfigBin, command)
	return out, err
}

func (r *screenBlanking) Rollback() (err error) {
	return nil
}

// 0 = enabled
type fanControl struct {
	setting string
}

func (r *fanControl) Name() string {
	return fmt.Sprintf("%T - %s", r, r.setting)
}

func (r *fanControl) Run() (result interface{}, err error) {
	command := []string{"nonint", "do_fan", r.setting, "14", "80"}
	out, _, err := util.ExecCommandOutput(raspiconfigBin, command)
	return out, err
}

func (r *fanControl) Rollback() (err error) {
	return nil
}

// 0 = enabled
type predictableNetNames struct {
	setting string
}

func (r *predictableNetNames) Name() string {
	return fmt.Sprintf("%T - %s", r, r.setting)
}

func (r *predictableNetNames) Run() (result interface{}, err error) {
	command := []string{"nonint", "do_net_names", r.setting}
	out, _, err := util.ExecCommandOutput(raspiconfigBin, command)
	return out, err
}

func (r *predictableNetNames) Rollback() (err error) {
	return nil
}

// setting = locale.  Example: en_US.UTF-8
type setLocale struct {
	setting string
}

func (r *setLocale) Name() string {
	return fmt.Sprintf("%T - %s", r, r.setting)
}

func (r *setLocale) Run() (result interface{}, err error) {
	command := []string{"nonint", "do_change_locale", r.setting, "UTF-8"}
	out, _, err := util.ExecCommandOutput(raspiconfigBin, command)
	return out, err
}

func (r *setLocale) Rollback() (err error) {
	return nil
}

// setting = timezone.  Example: America/New_York
type setTimezone struct {
	setting string
}

func (r *setTimezone) Name() string {
	return fmt.Sprintf("%T - %s", r, r.setting)
}

func (r *setTimezone) Run() (result interface{}, err error) {
	command := []string{"nonint", "do_change_timezone", r.setting}
	out, _, err := util.ExecCommandOutput(raspiconfigBin, command)
	return out, err
}

func (r *setTimezone) Rollback() (err error) {
	return nil
}
