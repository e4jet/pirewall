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
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/e4jet/pirewall/internal/backup"
	"github.com/e4jet/pirewall/internal/configure"
	"github.com/e4jet/pirewall/internal/doctor"
	"github.com/e4jet/pirewall/internal/restore"
)

const (
	me      = "pirewall"
	version = "v1.0.2"
	fail    = 3
)

// stringSlice is a flag.Value that accepts a repeated flag (each occurrence
// appends to the slice). Used for -restore-path.
type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)

	return nil
}

// flags holds the parsed command-line flags passed to run.
type flags struct {
	config       bool
	backup       string
	restore      string
	dryRun       bool
	doctor       bool
	restorePaths []string
}

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	configFlag := flag.Bool("config", false, "run installation and configuration")
	backupFlag := flag.String("backup", "", "mirror live config files into ~`user`/.pirewall and commit to git")
	restoreFlag := flag.String("restore", "", "restore live config files from ~`user`/.pirewall (preserves existing target mode and ownership)")
	dryRunFlag := flag.Bool("dry-run", false, "only valid with -restore: log restore actions but make no filesystem changes; ignored otherwise")
	doctorFlag := flag.Bool("doctor", false, "print a diagnostic report of firewall services, iptables, and network interfaces")

	var restorePaths stringSlice

	flag.Var(&restorePaths, "restore-path", "only valid with -restore: restrict restore to this relative path (repeatable); ignored otherwise")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s %s\n", me, version)
		return
	}

	slog.Info("starting", "name", me, "version", version)

	f := flags{
		config:       *configFlag,
		backup:       *backupFlag,
		restore:      *restoreFlag,
		dryRun:       *dryRunFlag,
		doctor:       *doctorFlag,
		restorePaths: []string(restorePaths),
	}

	if err := run(context.Background(), f); err != nil {
		slog.Error("run failed", "err", err)
		os.Exit(fail)
	}

	slog.Info("done", "name", me)
}

func run(ctx context.Context, f flags) error {
	if f.doctor {
		return doctor.Run(ctx, os.Stdout)
	}

	if f.backup != "" {
		if err := backup.Init(ctx, f.backup); err != nil {
			return fmt.Errorf("init: %w", err)
		}

		return backup.Backup(ctx, f.backup)
	}

	if f.restore != "" {
		opts := restore.Options{
			Username: f.restore,
			Paths:    f.restorePaths,
			DryRun:   f.dryRun,
		}

		return restore.Restore(ctx, opts)
	}

	if !f.config {
		flag.Usage()

		return nil
	}

	return install(ctx)
}

func install(ctx context.Context) error {
	if err := configure.ConfigRaspi(ctx); err != nil {
		return fmt.Errorf("ConfigRaspi: %w", err)
	}

	if err := configure.RemoveUnwantedPackages(ctx); err != nil {
		return fmt.Errorf("RemoveUnwantedPackages: %w", err)
	}

	if err := configure.AddPackages(ctx); err != nil {
		return fmt.Errorf("AddPackages: %w", err)
	}

	if err := configure.EnableNewServices(ctx); err != nil {
		return fmt.Errorf("EnableNewServices: %w", err)
	}

	if err := configure.DisableUnwantedServices(ctx); err != nil {
		return fmt.Errorf("DisableUnwantedServices: %w", err)
	}

	if err := configure.ConfigSysCtl(ctx); err != nil {
		return fmt.Errorf("ConfigSysCtl: %w", err)
	}

	return nil
}
