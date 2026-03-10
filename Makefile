# (c) Copyright 2023 Eric Paul Forgette

# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at

# http://www.apache.org/licenses/LICENSE-2.0

# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# P r o j e c t
#       _                        _ _
# _ __ (_)_ __ _____      ____ _| | |
#| '_ \| | '__/ _ \ \ /\ / / _` | | |
#| |_) | | | |  __/\ V  V / (_| | | |
#| .__/|_|_|  \___| \_/\_/ \__,_|_|_|
#|_|
#
# Raspi based firewall

GO_VERSION = 1.26
PACKAGE_ROOT = github.com/e4jet/pirewall
TAG = 1.0.0
GOOS=linux
GOARCH=arm64

# prefixes to make things pretty
A1 = $(shell printf "»")
A2 = $(shell printf "»»")
A3 = $(shell printf "»»»")
S0 = 😁

.PHONY: help
help:
	@echo "Main:"
	@echo "    all                                 - clean, debug, lint, test, and build"
	@echo "    check                               - Run a linters, test, fmt, etc. on the source code"
	@echo "    test                                - Execute all tests"
	@echo "    sfx                                 - Build self-extracting installer for Raspberry Pi 4"
	@echo "    clean                               - Remove existing binaries and empty database"
	@echo " "
	@echo "$(S0)"

.PHONY: debug
debug:
	@echo "Debug:"
	@echo "  Go:           `go version`"
	@echo "  GOPATH:       $(GOPATH)"
	@echo "  GOOS:         $(GOOS)"
	@echo "  GOARCH:       $(GOARCH)"
	@echo "  PACKAGE_ROOT: $(PACKAGE_ROOT)"
	@echo "$(S0)"

.PHONY: all
all: clean debug check pirewall; $(info $(A1) $@)
	@echo "$(S0)"

.PHONY: check
check: fmt vet lint ; $(info $(A1) $@)
	@echo "$(S0)"

clean: ; $(info $(A1) $@)
	rm -f pirewall pirewall-*.install
	@echo "🧹"

.PHONY: fmt
fmt: ; $(info $(A1) $@)
	@echo "$(A2) format go source code"
	go fmt ./...
	@echo "$(A2) $(S0)"

.PHONY: vet
vet: ; $(info $(A1) $@)
	@echo "$(A2) vet go source code"
	go vet ./...
	@echo "$(A2) $(S0)"

.PHONY: lint
lint: ; $(info $(A1) $@)
	@echo "$(A2) Lint go source code"
	golangci-lint run ./...
	@echo "$(A2) $(S0)"

.PHONY: test
test: ; $(info $(A1) $@)
	@echo "$(A2) test pirewall"
	go test -race -tags unit -coverprofile=pirewall.coverprofile $$(go list -f '{{if .TestGoFiles}}{{.ImportPath}}{{end}}' -tags unit ./...)
	@echo "$(A2) $(S0)"

pirewall: debug test ; $(info $(A1) $@)
	@echo "$(A2) build pirewall"
	env GOOS=$(GOOS) GOARCH=$(GOARCH) go build pirewall.go
	@echo "$(A2) $(S0)"

.PHONY: installer
installer: ; $(info $(A1) $@)
	tar -cvzf install.tgz install

.PHONY: sfx
sfx: pirewall ; $(info $(A1) $@)
	@echo "$(A2) build self-extracting archive"
	@mkdir -p _sfx/bin
	@cp pirewall _sfx/
	@cp install/bin/rebootOnWatchdog _sfx/bin/
	@cp -r install/examples _sfx/
	@tar czf _payload.tgz -C _sfx .
	@cat install.sh _payload.tgz > pirewall-$(TAG)-linux-arm64.install
	@chmod +x pirewall-$(TAG)-linux-arm64.install
	@rm -rf _sfx _payload.tgz
	@sha512sum pirewall-1.0.0-linux-arm64.install
	@echo "$(A2) $(S0)"