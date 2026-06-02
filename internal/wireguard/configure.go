/*
(c) Copyright 2026 Eric Paul Forgette

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package wireguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/e4jet/pirewall/internal/chain"
	"github.com/e4jet/pirewall/internal/util"
)

// ensureWGDir ensures /etc/wireguard exists with mode 0700. The dirPath
// field overrides the default path for tests. Rollback is a no-op: the
// directory may have pre-existed before this run.
type ensureWGDir struct {
	dirPath string
}

func (r *ensureWGDir) Name() string { return "ensureWGDir" }

func (r *ensureWGDir) Run(_ context.Context) (any, error) {
	path := r.dirPath
	if path == "" {
		path = wgDir
	}

	if err := os.MkdirAll(path, dirMode); err != nil {
		return nil, err
	}

	// Tighten mode if the directory pre-existed with a looser one.
	return nil, os.Chmod(path, dirMode)
}

func (r *ensureWGDir) Rollback(_ context.Context) error {
	// No-op: the directory may have existed before this run; we don't
	// track whether we created it.
	return nil
}

// ensureServerKeys ensures /etc/wireguard/{privatekey,publickey} exist with
// the right permissions. If both are present, modes are tightened and Run
// returns. If neither is present, both are generated via r.keyGen and
// atomic-written. If exactly one is present, ErrServerKeysInconsistent is
// returned (refuse to silently re-derive — could indicate corruption or
// operator intervention).
//
// privPath, pubPath, and keyGen fields override the production defaults
// for tests. Rollback removes only the files this Run created.
type ensureServerKeys struct {
	privPath string
	pubPath  string
	keyGen   func(ctx context.Context) (priv, pub string, err error)

	createdPriv bool
	createdPub  bool
}

func (r *ensureServerKeys) Name() string { return "ensureServerKeys" }

// Run implements chain.Runner. It delegates all logic to run so that the
// (any, error) return type does not trigger the nilnil linter on the happy path.
func (r *ensureServerKeys) Run(ctx context.Context) (any, error) {
	return nil, r.run(ctx)
}

func (r *ensureServerKeys) Rollback(_ context.Context) error {
	priv := r.privPath
	if priv == "" {
		priv = wgPrivKeyPath
	}

	pub := r.pubPath
	if pub == "" {
		pub = wgPubKeyPath
	}

	var first error

	if r.createdPriv {
		if err := removeIfExists(priv); err != nil {
			first = err
		}
	}

	if r.createdPub {
		if err := removeIfExists(pub); err != nil && first == nil {
			first = err
		}
	}

	return first
}

// run performs the key-management work and returns only an error. It is
// factored out of Run to avoid nilnil lint findings on the (any, error)
// return and to keep cyclomatic complexity within limits.
func (r *ensureServerKeys) run(ctx context.Context) error {
	priv := r.privPath
	if priv == "" {
		priv = wgPrivKeyPath
	}

	pub := r.pubPath
	if pub == "" {
		pub = wgPubKeyPath
	}

	keyGen := r.keyGen
	if keyGen == nil {
		keyGen = realGenWGKeypair
	}

	privExists, err := fileExists(priv)
	if err != nil {
		return err
	}

	pubExists, err := fileExists(pub)
	if err != nil {
		return err
	}

	if privExists && pubExists {
		return tightenKeyModes(priv, pub)
	}

	if privExists != pubExists {
		return fmt.Errorf("%w: priv=%v pub=%v", ErrServerKeysInconsistent, privExists, pubExists)
	}

	return r.generateAndWriteKeypair(ctx, priv, pub, keyGen)
}

// generateAndWriteKeypair calls keyGen, then atomic-writes both key files.
// It is factored out of run to keep cyclomatic complexity within limits.
func (r *ensureServerKeys) generateAndWriteKeypair(
	ctx context.Context,
	priv, pub string,
	keyGen func(context.Context) (string, string, error),
) error {
	privKey, pubKey, err := keyGen(ctx)
	if err != nil {
		return fmt.Errorf("generate wireguard keys: %w", err)
	}

	if err := util.FileWriteAtomic(priv, []byte(privKey), privMode, nil, 0, 0); err != nil {
		return fmt.Errorf("write %s: %w", priv, err)
	}

	r.createdPriv = true

	if err := util.FileWriteAtomic(pub, []byte(pubKey), pubMode, nil, 0, 0); err != nil {
		return fmt.Errorf("write %s: %w", pub, err)
	}

	r.createdPub = true

	return nil
}

// tightenKeyModes applies the required permissions to both key files without
// modifying their contents.
func tightenKeyModes(priv, pub string) error {
	if err := os.Chmod(priv, privMode); err != nil {
		return fmt.Errorf("chmod %s: %w", priv, err)
	}

	if err := os.Chmod(pub, pubMode); err != nil {
		return fmt.Errorf("chmod %s: %w", pub, err)
	}

	return nil
}

// fileExists is a small helper that distinguishes "doesn't exist" from
// other stat errors.
func fileExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("stat %s: %w", path, err)
}

// removeIfExists removes path and returns nil if path already doesn't exist.
func removeIfExists(path string) error {
	err := os.Remove(path)
	if err == nil || errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

// realGenWGKeypair shells out to wg(8) to generate a keypair. Provided here
// as a forward declaration so ensureServerKeys can reference it when no
// test override is supplied. The full implementation is wired up in
// client.go via init().
var realGenWGKeypair func(ctx context.Context) (priv, pub string, err error)

// installWGPackage installs the wireguard package via apt-get.
type installWGPackage struct{}

func (r *installWGPackage) Name() string { return "installWGPackage" }

func (r *installWGPackage) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, aptgetBin,
		[]string{"install", "-yqq", "wireguard"})

	return out, err
}

func (r *installWGPackage) Rollback(_ context.Context) error {
	// No-op: consistent with internal/configure's aptInstall. Reinstalling
	// during rollback would be more destructive than the failure itself.
	return nil
}

// enableWGService runs `systemctl enable wg-quick@wg0`. No-op rollback per
// the WireGuard spec.
type enableWGService struct{}

func (r *enableWGService) Name() string { return "enableWGService" }

func (r *enableWGService) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, systemctlBin,
		[]string{"enable", wgQuickUnit})

	return out, err
}

func (r *enableWGService) Rollback(_ context.Context) error {
	// No-op per spec: re-running -wireguard re-enables.
	return nil
}

// startWGService runs `systemctl start wg-quick@wg0`. No-op rollback per
// the WireGuard spec. systemctl start on an already-active unit is a no-op.
type startWGService struct{}

func (r *startWGService) Name() string { return "startWGService" }

func (r *startWGService) Run(ctx context.Context) (any, error) {
	out, _, err := util.ExecCommandOutput(ctx, systemctlBin,
		[]string{"start", wgQuickUnit})

	return out, err
}

func (r *startWGService) Rollback(_ context.Context) error {
	// No-op per spec.
	return nil
}

// ensureServerConf ensures /etc/wireguard/wg0.conf exists with the correct
// content and mode (0600). If it exists, the endpoint comment is checked
// against r.endpoint (when non-empty) and the mode is tightened. If it
// does not exist, r.endpoint is required and the file is rendered from
// the template plus the private key read from r.privPath.
//
// confPath, privPath fields override the production defaults for tests.
// Rollback removes the file only if this Run created it.
type ensureServerConf struct {
	confPath string
	privPath string
	endpoint string

	createdConf bool
}

// Name implements chain.Runner.
func (r *ensureServerConf) Name() string { return "ensureServerConf" }

// Run implements chain.Runner. It delegates to run so that the (any, error)
// return type does not trigger the nilnil linter on the happy path.
func (r *ensureServerConf) Run(_ context.Context) (any, error) {
	return nil, r.run()
}

// Rollback implements chain.Runner. It removes wg0.conf only if this Run
// created it.
func (r *ensureServerConf) Rollback(_ context.Context) error {
	if !r.createdConf {
		return nil
	}

	conf := r.confPath
	if conf == "" {
		conf = wgConfPath
	}

	return removeIfExists(conf)
}

// run performs the conf-management work and returns only an error.
func (r *ensureServerConf) run() error {
	conf := r.confPath
	if conf == "" {
		conf = wgConfPath
	}

	priv := r.privPath
	if priv == "" {
		priv = wgPrivKeyPath
	}

	exists, err := fileExists(conf)
	if err != nil {
		return err
	}

	if exists {
		return r.tightenExisting(conf)
	}

	return r.createConf(conf, priv)
}

// tightenExisting reads and validates the existing conf file, then chmods it.
func (r *ensureServerConf) tightenExisting(conf string) error {
	data, err := os.ReadFile(conf)
	if err != nil {
		return fmt.Errorf("read %s: %w", conf, err)
	}

	parsed, err := parseServerConf(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", conf, err)
	}

	if r.endpoint != "" && parsed.Endpoint != r.endpoint {
		return fmt.Errorf("%w: existing=%q new=%q",
			ErrEndpointMismatch, parsed.Endpoint, r.endpoint)
	}

	if err := os.Chmod(conf, confMode); err != nil {
		return fmt.Errorf("chmod %s: %w", conf, err)
	}

	return nil
}

// createConf renders and atomically writes a new wg0.conf from the private
// key at priv and r.endpoint.
func (r *ensureServerConf) createConf(conf, priv string) error {
	if r.endpoint == "" {
		return ErrEndpointRequired
	}

	privKey, err := os.ReadFile(priv)
	if err != nil {
		return fmt.Errorf("read %s: %w", priv, err)
	}

	body := renderServerConf(&serverConf{
		Endpoint:   r.endpoint,
		Address:    wgServerAddrCIDR,
		ListenPort: wgListenPort,
		PrivateKey: strings.TrimSpace(string(privKey)),
	})

	if err := util.FileWriteAtomic(conf, []byte(body), confMode, nil, 0, 0); err != nil {
		return fmt.Errorf("write %s: %w", conf, err)
	}

	r.createdConf = true

	return nil
}

// Configure installs the wireguard package, writes /etc/wireguard/wg0.conf
// with freshly-generated server keys, and enables + starts wg-quick@wg0.
// Idempotent: a re-run leaves existing keys and config alone, tightens
// permissions, and ensures the service is enabled and running.
//
// opts.Endpoint must be host:port on the first run (when wg0.conf does not
// exist). On subsequent runs it may be omitted; if supplied, it must match
// the existing wg0.conf endpoint comment or ErrEndpointMismatch is
// returned.
func Configure(ctx context.Context, opts ConfigureOptions) error {
	if opts.Endpoint != "" {
		if err := validateEndpoint(opts.Endpoint); err != nil {
			return err
		}
	}

	slog.InfoContext(ctx, "👉 configuring WireGuard server")

	return chain.NewChain(retries, retryDelay,
		&installWGPackage{},
		&ensureWGDir{},
		&ensureServerKeys{},
		&ensureServerConf{endpoint: opts.Endpoint},
		&enableWGService{},
		&startWGService{},
	).Execute(ctx)
}
