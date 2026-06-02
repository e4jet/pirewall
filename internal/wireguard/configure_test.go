//go:build unit

/*
(c) Copyright 2026 Eric Paul Forgette

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package wireguard

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWGDir_CreatesMissingDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "wireguard")

	r := &ensureWGDir{dirPath: dir}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if !info.IsDir() {
		t.Errorf("not a dir")
	}

	if info.Mode().Perm() != 0o700 {
		t.Errorf("mode: got %o want 0700", info.Mode().Perm())
	}
}

func TestEnsureWGDir_TightensExistingDir(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "wireguard")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("seed: %v", err)
	}

	r := &ensureWGDir{dirPath: dir}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if info.Mode().Perm() != 0o700 {
		t.Errorf("mode: got %o want 0700", info.Mode().Perm())
	}
}

func TestEnsureWGDir_Rollback(t *testing.T) {
	t.Parallel()

	r := &ensureWGDir{dirPath: t.TempDir()}

	if err := r.Rollback(context.Background()); err != nil {
		t.Errorf("Rollback should be no-op, got %v", err)
	}
}

// stubKeyGen returns fixed strings, useful for deterministic tests.
func stubKeyGen(priv, pub string) func(context.Context) (string, string, error) {
	return func(_ context.Context) (string, string, error) {
		return priv, pub, nil
	}
}

func TestEnsureServerKeys_GeneratesWhenMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	r := &ensureServerKeys{
		privPath: filepath.Join(dir, "privatekey"),
		pubPath:  filepath.Join(dir, "publickey"),
		keyGen:   stubKeyGen("PRIVKEY=", "PUBKEY="),
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	privInfo, err := os.Stat(r.privPath)
	if err != nil {
		t.Fatalf("stat priv: %v", err)
	}

	if privInfo.Mode().Perm() != 0o600 {
		t.Errorf("priv mode: got %o want 0600", privInfo.Mode().Perm())
	}

	pubInfo, err := os.Stat(r.pubPath)
	if err != nil {
		t.Fatalf("stat pub: %v", err)
	}

	if pubInfo.Mode().Perm() != 0o644 {
		t.Errorf("pub mode: got %o want 0644", pubInfo.Mode().Perm())
	}

	priv, _ := os.ReadFile(r.privPath)
	if string(priv) != "PRIVKEY=" {
		t.Errorf("priv content: got %q", priv)
	}

	pub, _ := os.ReadFile(r.pubPath)
	if string(pub) != "PUBKEY=" {
		t.Errorf("pub content: got %q", pub)
	}

	if !r.createdPriv || !r.createdPub {
		t.Errorf("createdPriv=%v createdPub=%v; want both true", r.createdPriv, r.createdPub)
	}
}

func TestEnsureServerKeys_BothPresentTightensModes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	privPath := filepath.Join(dir, "privatekey")
	pubPath := filepath.Join(dir, "publickey")

	if err := os.WriteFile(privPath, []byte("EXISTING-PRIV"), 0o644); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	if err := os.WriteFile(pubPath, []byte("EXISTING-PUB"), 0o600); err != nil {
		t.Fatalf("seed pub: %v", err)
	}

	r := &ensureServerKeys{
		privPath: privPath,
		pubPath:  pubPath,
		keyGen:   stubKeyGen("SHOULD-NOT-BE-USED", "SHOULD-NOT-BE-USED"),
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	privInfo, _ := os.Stat(privPath)
	if privInfo.Mode().Perm() != 0o600 {
		t.Errorf("priv mode: got %o want 0600", privInfo.Mode().Perm())
	}

	pubInfo, _ := os.Stat(pubPath)
	if pubInfo.Mode().Perm() != 0o644 {
		t.Errorf("pub mode: got %o want 0644", pubInfo.Mode().Perm())
	}

	if r.createdPriv || r.createdPub {
		t.Errorf("createdPriv=%v createdPub=%v; want both false", r.createdPriv, r.createdPub)
	}

	priv, _ := os.ReadFile(privPath)
	if string(priv) != "EXISTING-PRIV" {
		t.Errorf("priv was overwritten: got %q", priv)
	}
}

func TestEnsureServerKeys_OneMissingIsInconsistent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	privPath := filepath.Join(dir, "privatekey")
	pubPath := filepath.Join(dir, "publickey")

	if err := os.WriteFile(privPath, []byte("EXISTING-PRIV"), 0o600); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	r := &ensureServerKeys{
		privPath: privPath,
		pubPath:  pubPath,
		keyGen:   stubKeyGen("PRIVKEY=", "PUBKEY="),
	}

	_, err := r.Run(context.Background())
	if !errors.Is(err, ErrServerKeysInconsistent) {
		t.Fatalf("want ErrServerKeysInconsistent, got %v", err)
	}
}

func TestEnsureServerKeys_RollbackRemovesOnlyCreated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	privPath := filepath.Join(dir, "privatekey")
	pubPath := filepath.Join(dir, "publickey")

	r := &ensureServerKeys{
		privPath: privPath,
		pubPath:  pubPath,
		keyGen:   stubKeyGen("PRIVKEY=", "PUBKEY="),
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := r.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := os.Stat(privPath); !os.IsNotExist(err) {
		t.Errorf("priv still present after rollback: err=%v", err)
	}

	if _, err := os.Stat(pubPath); !os.IsNotExist(err) {
		t.Errorf("pub still present after rollback: err=%v", err)
	}
}

func TestEnsureServerKeys_RollbackNoopWhenNotCreated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	privPath := filepath.Join(dir, "privatekey")
	pubPath := filepath.Join(dir, "publickey")

	if err := os.WriteFile(privPath, []byte("EXISTING-PRIV"), 0o600); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	if err := os.WriteFile(pubPath, []byte("EXISTING-PUB"), 0o644); err != nil {
		t.Fatalf("seed pub: %v", err)
	}

	r := &ensureServerKeys{
		privPath: privPath,
		pubPath:  pubPath,
		keyGen:   stubKeyGen("UNUSED", "UNUSED"),
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := r.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	// Both files still present.
	if _, err := os.Stat(privPath); err != nil {
		t.Errorf("priv removed by rollback: %v", err)
	}

	if _, err := os.Stat(pubPath); err != nil {
		t.Errorf("pub removed by rollback: %v", err)
	}
}

func TestEnsureServerConf_CreatesWhenMissing(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg0.conf")
	privPath := filepath.Join(dir, "privatekey")

	if err := os.WriteFile(privPath, []byte("SERVERPRIVKEY="), 0o600); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	r := &ensureServerConf{
		confPath: confPath,
		privPath: privPath,
		endpoint: "home.example.org:51820",
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	info, err := os.Stat(confPath)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}

	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: got %o want 0600", info.Mode().Perm())
	}

	body, _ := os.ReadFile(confPath)
	if !strings.Contains(string(body), "# Endpoint = home.example.org:51820") {
		t.Errorf("endpoint comment missing:\n%s", body)
	}

	if !strings.Contains(string(body), "PrivateKey = SERVERPRIVKEY=") {
		t.Errorf("private key missing:\n%s", body)
	}

	if !r.createdConf {
		t.Errorf("createdConf=false, want true")
	}
}

func TestEnsureServerConf_MissingEndpointWhenCreating(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg0.conf")
	privPath := filepath.Join(dir, "privatekey")

	if err := os.WriteFile(privPath, []byte("SERVERPRIVKEY="), 0o600); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	r := &ensureServerConf{
		confPath: confPath,
		privPath: privPath,
		endpoint: "",
	}

	_, err := r.Run(context.Background())
	if !errors.Is(err, ErrEndpointRequired) {
		t.Fatalf("want ErrEndpointRequired, got %v", err)
	}
}

func TestEnsureServerConf_ExistingMatchingEndpointTightensMode(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg0.conf")
	privPath := filepath.Join(dir, "privatekey")

	body := renderServerConf(&serverConf{
		Endpoint:   "home.example.org:51820",
		Address:    wgServerAddrCIDR,
		ListenPort: wgListenPort,
		PrivateKey: "SERVERPRIVKEY=",
	})

	if err := os.WriteFile(confPath, []byte(body), 0o644); err != nil {
		t.Fatalf("seed conf: %v", err)
	}

	if err := os.WriteFile(privPath, []byte("SERVERPRIVKEY="), 0o600); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	r := &ensureServerConf{
		confPath: confPath,
		privPath: privPath,
		endpoint: "home.example.org:51820",
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	info, _ := os.Stat(confPath)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: got %o want 0600", info.Mode().Perm())
	}

	if r.createdConf {
		t.Errorf("createdConf=true, want false (existing file)")
	}
}

func TestEnsureServerConf_ExistingMismatchedEndpointFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg0.conf")
	privPath := filepath.Join(dir, "privatekey")

	body := renderServerConf(&serverConf{
		Endpoint:   "home.example.org:51820",
		Address:    wgServerAddrCIDR,
		ListenPort: wgListenPort,
		PrivateKey: "SERVERPRIVKEY=",
	})

	if err := os.WriteFile(confPath, []byte(body), 0o600); err != nil {
		t.Fatalf("seed conf: %v", err)
	}

	if err := os.WriteFile(privPath, []byte("SERVERPRIVKEY="), 0o600); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	r := &ensureServerConf{
		confPath: confPath,
		privPath: privPath,
		endpoint: "other.example.org:51820",
	}

	_, err := r.Run(context.Background())
	if !errors.Is(err, ErrEndpointMismatch) {
		t.Fatalf("want ErrEndpointMismatch, got %v", err)
	}
}

func TestEnsureServerConf_ExistingEmptyEndpointArgPasses(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg0.conf")
	privPath := filepath.Join(dir, "privatekey")

	body := renderServerConf(&serverConf{
		Endpoint:   "home.example.org:51820",
		Address:    wgServerAddrCIDR,
		ListenPort: wgListenPort,
		PrivateKey: "SERVERPRIVKEY=",
	})

	if err := os.WriteFile(confPath, []byte(body), 0o600); err != nil {
		t.Fatalf("seed conf: %v", err)
	}

	if err := os.WriteFile(privPath, []byte("SERVERPRIVKEY="), 0o600); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	r := &ensureServerConf{
		confPath: confPath,
		privPath: privPath,
		endpoint: "",
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestEnsureServerConf_RollbackRemovesOnlyCreated(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	confPath := filepath.Join(dir, "wg0.conf")
	privPath := filepath.Join(dir, "privatekey")

	if err := os.WriteFile(privPath, []byte("SERVERPRIVKEY="), 0o600); err != nil {
		t.Fatalf("seed priv: %v", err)
	}

	r := &ensureServerConf{
		confPath: confPath,
		privPath: privPath,
		endpoint: "home.example.org:51820",
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if err := r.Rollback(context.Background()); err != nil {
		t.Fatalf("Rollback: %v", err)
	}

	if _, err := os.Stat(confPath); !os.IsNotExist(err) {
		t.Errorf("conf still present after rollback: err=%v", err)
	}
}
