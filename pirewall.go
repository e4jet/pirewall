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

	"github.com/e4jet/pirewall/internal/backup"
	"github.com/e4jet/pirewall/internal/configure"
)

const (
	me      = "pirewall"
	version = "1.0.0"
	fail    = 3
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	configFlag := flag.Bool("config", false, "run installation and configuration")
	backupFlag := flag.String("backup", "", "mirror live config files into ~`user`/.pirewall and commit to git")

	flag.Parse()

	if *versionFlag {
		fmt.Printf("%s %s\n", me, version)
		return
	}

	if *backupFlag != "" {
		if err := backup.Init(context.Background(), *backupFlag); err != nil {
			slog.Error("Init failed", "err", err)
			os.Exit(fail)
		}

		if err := backup.Backup(context.Background(), *backupFlag); err != nil {
			slog.Error("Backup failed", "err", err)
			os.Exit(fail)
		}

		return
	}

	if !*configFlag {
		flag.Usage()
		return
	}

	slog.Info("starting", "name", me, "version", version)

	if err := install(context.Background()); err != nil {
		slog.Error("install failed", "err", err)
		os.Exit(fail)
	}

	slog.Info("done", "name", me)
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
