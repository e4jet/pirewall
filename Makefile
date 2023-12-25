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

GO_VERSION = 1.20
PACKAGE_ROOT = github.com/e4jet/pirewall
##PLATFORM = linux/arm/v6
TAG = 1.0.0

# prefixes to make things pretty
A1 = $(shell printf "»")
A2 = $(shell printf "»»")
A3 = $(shell printf "»»»")
S0 = 😁

.PHONY: help
help:
	@echo "Main:"
	@echo "    all                                 - clean, debug, lint, test, and build"
	@echo "    lint                                - Run a linter on the source code"
	@echo "    test                                - Execute all tests"
	@echo "    clean                               - Remove existing binaries and empty database"
	@echo " "
	@echo "$(S0)"

.PHONY: debug
debug:
	@echo "Debug:"
	@echo "  Go:           `go version`"
	@echo "  GOPATH:       $(GOPATH)"
##	@echo "  PLATFORM:     $(PLATFORM)"
	@echo "  PACKAGE_ROOT: $(PACKAGE_ROOT)"
	@echo "$(S0)"

clean: ; $(info $(A1) clean)
	rm -f pirewall
	@echo "🧹"

.PHONY: lint
lint: test ; $(info $(A1) lint)
	@echo "$(A2) Lint go source code"
	golangci-lint run ./...
	@echo "$(A2) $(S0)"

.PHONY: test
test: ; $(info $(A1) test)
	@echo "$(A2) test pirewall"
	go test -coverprofile=pirewall.coverprofile ./...
	@echo "$(A2) $(S0)"

pirewall: debug lint test ; $(info $(A1) pirewall)
	@echo "$(A2) build pirewall"
	go build pirewall.go
	@echo "$(A2) $(S0)"

.PHONY: all
all: clean pirewall; $(info $(A1) all)
	@echo "$(S0)"

.PHONY: rsync
rsync: ; $(info $(A1) $@)
	rsync -e ssh -urlt ~/go/src/github.com/e4jet/pirewall/ tom:go/src/github.com/e4jet/pirewall/
	@echo "$(S0)"

.PHONY: tom
tom: clean; $(info $(A1) tom)
	go build pirewall.go;sudo ./pirewall
	@echo "$(S0)"
