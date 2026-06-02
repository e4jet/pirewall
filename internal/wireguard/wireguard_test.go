//go:build unit

/*
(c) Copyright 2026 Eric Paul Forgette

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
*/

package wireguard

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateEndpoint(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"hostname:port", "home.example.org:51820", true},
		{"ip:port", "203.0.113.10:51820", true},
		{"port 1", "host:1", true},
		{"port 65535", "host:65535", true},
		{"empty", "", false},
		{"no colon", "home.example.org", false},
		{"missing port", "home.example.org:", false},
		{"missing host", ":51820", false},
		{"non-numeric port", "host:abc", false},
		{"port 0", "host:0", false},
		{"port 65536", "host:65536", false},
		{"negative port", "host:-1", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateEndpoint(tc.in)

			if tc.ok && err != nil {
				t.Fatalf("validateEndpoint(%q): unexpected error: %v", tc.in, err)
			}

			if !tc.ok && err == nil {
				t.Fatalf("validateEndpoint(%q): want error, got nil", tc.in)
			}

			if !tc.ok && !errors.Is(err, ErrInvalidEndpoint) {
				t.Fatalf("validateEndpoint(%q): want ErrInvalidEndpoint, got %v", tc.in, err)
			}
		})
	}
}

func TestValidateClientName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"simple", "alice", true},
		{"with-dash", "alice-laptop", true},
		{"with_underscore", "alice_laptop", true},
		{"digits", "client42", true},
		{"max length 32", strings.Repeat("a", 32), true},
		{"empty", "", false},
		{"too long 33", strings.Repeat("a", 33), false},
		{"space", "alice phone", false},
		{"slash", "alice/phone", false},
		{"dot", "alice.phone", false},
		{"unicode", "alicé", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateClientName(tc.in)

			if tc.ok && err != nil {
				t.Fatalf("validateClientName(%q): unexpected error: %v", tc.in, err)
			}

			if !tc.ok && err == nil {
				t.Fatalf("validateClientName(%q): want error, got nil", tc.in)
			}

			if !tc.ok && !errors.Is(err, ErrInvalidClientName) {
				t.Fatalf("validateClientName(%q): want ErrInvalidClientName, got %v", tc.in, err)
			}
		})
	}
}
