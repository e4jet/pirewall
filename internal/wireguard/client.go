/*
(c) Copyright 2026 Eric Paul Forgette

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package wireguard

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/e4jet/pirewall/internal/util"
)

// errEmptyWGOutput is returned when `wg genkey` produces no output.
var errEmptyWGOutput = fmt.Errorf("wg genkey: empty output")

// init wires the real wg(8) keypair generator into the forward-declared
// var in configure.go. Tests that need a deterministic substitute set the
// keyGen field on individual runners instead.
//
//nolint:gochecknoinits // intentional: wires realGenWGKeypair before any call reaches ensureServerKeys
func init() {
	realGenWGKeypair = realGenWGKeypairImpl
}

// realGenWGKeypairImpl shells out to `wg genkey` and pipes its output into
// `wg pubkey` to produce a fresh keypair. Returned strings are trimmed of
// trailing whitespace.
//
// `wg pubkey` reads the private key from stdin, so it can't go through
// util.ExecCommandOutput (which doesn't expose a stdin argument). os/exec
// is used directly for that one call.
func realGenWGKeypairImpl(ctx context.Context) (priv, pub string, err error) {
	privOut, _, err := util.ExecCommandOutput(ctx, wgBin, []string{"genkey"})
	if err != nil {
		return "", "", fmt.Errorf("wg genkey: %w", err)
	}

	priv = strings.TrimSpace(privOut)
	if priv == "" {
		return "", "", fmt.Errorf("%w", errEmptyWGOutput)
	}

	cmd := exec.CommandContext(ctx, wgBin, "pubkey")
	cmd.Stdin = strings.NewReader(priv + "\n")

	pubBytes, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("wg pubkey: %w", err)
	}

	pub = strings.TrimSpace(string(pubBytes))

	return priv, pub, nil
}

// syncRunningConf live-reloads wg0 to pick up changes to wg0.conf without
// dropping existing peer sessions. The process is:
//
//	wg-quick strip wg0  > <tempfile in same dir as wg0.conf>
//	wg syncconf wg0 <tempfile>
//
// Failures are returned to the caller, which logs and continues — the
// on-disk wg0.conf is the source of truth, and a service restart will pick
// up the new state at next boot.
func syncRunningConf(ctx context.Context) error {
	strip, _, err := util.ExecCommandOutput(ctx, wgQuickBin, []string{"strip", "wg0"})
	if err != nil {
		return fmt.Errorf("wg-quick strip wg0: %w", err)
	}

	tmp, err := os.CreateTemp(wgDir, ".wg-sync-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}

	tmpName := tmp.Name()

	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.WriteString(strip); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write %s: %w", tmpName, err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", tmpName, err)
	}

	if _, _, err := util.ExecCommandOutput(ctx, wgBin, []string{"syncconf", "wg0", tmpName}); err != nil {
		return fmt.Errorf("wg syncconf wg0: %w", err)
	}

	return nil
}

// addClientPreflight performs all read-only validation and queries before
// any disk mutation: name shape, server conf load, endpoint comment
// presence, duplicate-name check, and free-IP selection. Factored out of
// AddClient to keep its cyclomatic complexity within limits.
func addClientPreflight(name string) (*serverConf, string, error) {
	if err := validateClientName(name); err != nil {
		return nil, "", err
	}

	server, err := loadServerConf(wgConfPath, wgPubKeyPath)
	if err != nil {
		return nil, "", err
	}

	if server.Endpoint == "" {
		return nil, "", ErrEndpointRequired
	}

	if server.hasPeerNamed(name) {
		return nil, "", fmt.Errorf("%w: %q", ErrClientExists, name)
	}

	ip, err := server.pickFreeIP()
	if err != nil {
		return nil, "", err
	}

	return server, ip, nil
}

// AddClient generates a client keypair, appends a peer block to wg0.conf,
// writes /etc/wireguard/clients/<name>.conf, live-reloads the running
// tunnel, and prints the client config to out.
//
// All pre-flight validation happens before any disk mutation. The first
// mutation is the wg0.conf rewrite; failures after that point return the
// error to the caller without rolling back the on-disk file (wg0.conf is
// the source of truth and a service restart picks up the on-disk state).
//
// Live-reload failures (wg-quick strip / wg syncconf) are logged at WARN
// and otherwise swallowed — same rationale.
func AddClient(ctx context.Context, opts AddClientOptions, out io.Writer) error {
	server, ip, err := addClientPreflight(opts.Name)
	if err != nil {
		return err
	}

	priv, pub, err := realGenWGKeypair(ctx)
	if err != nil {
		return fmt.Errorf("generate client keys: %w", err)
	}

	today := time.Now().UTC().Format("2006-01-02")

	server.appendPeer(peerEntry{
		Name:       opts.Name,
		AddedDate:  today,
		PublicKey:  pub,
		AllowedIPs: ip + "/32",
	})

	if err := writeServerConf(wgConfPath, server); err != nil {
		return fmt.Errorf("write server conf: %w", err)
	}

	if err := os.MkdirAll(wgClientsDir, dirMode); err != nil {
		return fmt.Errorf("mkdir %s: %w", wgClientsDir, err)
	}

	clientPath := filepath.Join(wgClientsDir, opts.Name+".conf")

	clientBody := renderClientConf(clientConfData{
		Name:             opts.Name,
		GeneratedDate:    today,
		ClientPrivateKey: priv,
		ClientAddress:    ip + "/24",
		DNS:              wgClientDNS,
		ServerPublicKey:  server.PublicKey,
		Endpoint:         server.Endpoint,
		AllowedIPs:       "0.0.0.0/0, ::/0",
		Keepalive:        wgKeepalive,
	})

	if err := util.FileWriteAtomic(clientPath, []byte(clientBody), clientMode, nil, 0, 0); err != nil {
		return fmt.Errorf("write %s: %w", clientPath, err)
	}

	if err := syncRunningConf(ctx); err != nil {
		slog.WarnContext(ctx, "wg syncconf failed; disk state is source of truth",
			"err", err, "remediation", "systemctl restart "+wgQuickUnit)
	}

	if _, err := fmt.Fprint(out, clientBody); err != nil {
		return fmt.Errorf("print client conf: %w", err)
	}

	slog.InfoContext(ctx, "added wireguard peer",
		"name", opts.Name, "address", ip, "conf", clientPath)

	return nil
}
