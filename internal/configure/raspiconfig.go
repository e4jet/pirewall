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
	"fmt"
	"strings"

	"github.com/e4jet/pirewall/internal/util"
)

const (
	raspiconfigBin = "/usr/bin/raspi-config"
)

// screenBlanking controls whether the screen blanks after inactivity.
// setting: "0" = blanking disabled (screen stays on), "1" = blanking enabled.
type screenBlanking struct {
	setting string
}

func (r *screenBlanking) Name() string {
	return fmt.Sprintf("screenBlanking-%s", r.setting)
}

func (r *screenBlanking) Run(ctx context.Context) (any, error) {
	command := []string{"nonint", "do_blanking", r.setting}
	out, _, err := util.ExecCommandOutput(ctx, raspiconfigBin, command)

	return out, err
}

func (r *screenBlanking) Rollback(_ context.Context) error {
	return nil
}

// fanControl controls whether GPIO fan control is active.
// setting: "0" = fan control disabled, "1" = fan control enabled.
// pin is the GPIO pin number; temp is the temperature threshold in Celsius.
type fanControl struct {
	setting string
	pin     string
	temp    string
}

func (r *fanControl) Name() string {
	return fmt.Sprintf("fanControl-%s", r.setting)
}

func (r *fanControl) Run(ctx context.Context) (any, error) {
	command := []string{"nonint", "do_fan", r.setting, r.pin, r.temp}
	out, _, err := util.ExecCommandOutput(ctx, raspiconfigBin, command)

	return out, err
}

func (r *fanControl) Rollback(_ context.Context) error {
	return nil
}

// predictableNetNames controls whether predictable network interface names
// (e.g. enp3s0) are used.
// setting: "0" = predictable names disabled (uses eth0/wlan0 style), "1" = enabled.
type predictableNetNames struct {
	setting string
}

func (r *predictableNetNames) Name() string {
	return fmt.Sprintf("predictableNetNames-%s", r.setting)
}

func (r *predictableNetNames) Run(ctx context.Context) (any, error) {
	command := []string{"nonint", "do_net_names", r.setting}
	out, _, err := util.ExecCommandOutput(ctx, raspiconfigBin, command)

	return out, err
}

func (r *predictableNetNames) Rollback(_ context.Context) error {
	return nil
}

// setLocale sets the system locale. setting should be a full locale string
// such as "en_US.UTF-8". The charset is derived from the locale string
// (the part after '.') and passed as a second argument to raspi-config.
type setLocale struct {
	setting string
}

func (r *setLocale) Name() string {
	return fmt.Sprintf("setLocale-%s", r.setting)
}

func (r *setLocale) Run(ctx context.Context) (any, error) {
	args := []string{"nonint", "do_change_locale", r.setting}

	if idx := strings.IndexByte(r.setting, '.'); idx >= 0 {
		args = append(args, r.setting[idx+1:])
	}

	out, _, err := util.ExecCommandOutput(ctx, raspiconfigBin, args)

	return out, err
}

func (r *setLocale) Rollback(_ context.Context) error {
	return nil
}

// setTimezone sets the system timezone. setting should be a tz database name
// such as "America/New_York".
type setTimezone struct {
	setting string
}

func (r *setTimezone) Name() string {
	return fmt.Sprintf("setTimezone-%s", r.setting)
}

func (r *setTimezone) Run(ctx context.Context) (any, error) {
	command := []string{"nonint", "do_change_timezone", r.setting}
	out, _, err := util.ExecCommandOutput(ctx, raspiconfigBin, command)

	return out, err
}

func (r *setTimezone) Rollback(_ context.Context) error {
	return nil
}
