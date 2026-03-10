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

	"github.com/e4jet/pirewall/internal/util"
)

const (
	systemctl = "/usr/bin/systemctl"
)

// disableService uses systemctl to disable a service.
type disableService struct {
	service string
}

func (d *disableService) Name() string {
	return fmt.Sprintf("disableService.%s", d.service)
}

func (d *disableService) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, systemctl, []string{"disable", d.service})
	return out, err
}

func (d *disableService) Rollback(_ context.Context) error {
	// no op
	return nil
}

// stopService uses systemctl to stop a service.
type stopService struct {
	service string
}

func (s *stopService) Name() string {
	return fmt.Sprintf("stopService.%s", s.service)
}

func (s *stopService) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, systemctl, []string{"stop", s.service})
	return out, err
}

func (s *stopService) Rollback(_ context.Context) error {
	// no op
	return nil
}

// startService uses systemctl to start a service.
type startService struct {
	service string
}

func (s *startService) Name() string {
	return fmt.Sprintf("startService.%s", s.service)
}

func (s *startService) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, systemctl, []string{"start", s.service})
	return out, err
}

func (s *startService) Rollback(ctx context.Context) error {
	stopper := &stopService{service: s.service}
	_, err := stopper.Run(ctx)

	return err
}

// enableService uses systemctl to enable a service.
type enableService struct {
	service string
}

func (s *enableService) Name() string {
	return fmt.Sprintf("enableService.%s", s.service)
}

func (s *enableService) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, systemctl, []string{"enable", s.service})
	return out, err
}

func (s *enableService) Rollback(ctx context.Context) error {
	disabler := &disableService{service: s.service}
	_, err := disabler.Run(ctx)

	return err
}
