#!/bin/bash
#
# (c) Copyright 2023 Eric Paul Forgette
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# pirewall self-extracting installer
# Must be run as root on the target Raspberry Pi 4.
# Usage: sudo bash pirewall-<version>-linux-arm64.run
#

set -euo pipefail

INSTALL_BIN=/usr/local/bin
INSTALL_SHARE=/usr/local/share/pirewall

cat <<'BANNER'
___________________________________________________________________
|_|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|
|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|_|
|_|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|
|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|_|
|_|___|___|__|  ____  _                        _ _  |_|___|___|___|
|___|___|____| |  _ \(_)_ __ _____      ____ _| | | |___|___|___|_|
|_|___|___|__| | |_) | | '__/ _ \ \ /\ / / _` | | | |_|___|___|___|
|___|___|____| |  __/| | | |  __/\ V  V / (_| | | | |___|___|___|_|
|_|___|___|__| |_|   |_|_|  \___| \_/\_/ \__,_|_|_| |_|___|___|___|
|___|___|____|______________________________________|___|___|___|_|
|_|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|
|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|_|
|_|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|
|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|___|_|
BANNER

echo "pirewall installer"
echo ""

if [[ $EUID -ne 0 ]]; then
    echo "Error: this installer must be run as root." >&2
    exit 1
fi

# Locate the payload: the line after the #__PAYLOAD__ marker.
PAYLOAD_LINE=$(awk '/^#__PAYLOAD__$/{print NR+1; exit}' "$0")
if [[ -z "${PAYLOAD_LINE}" ]]; then
    echo "Error: payload marker not found. Archive may be corrupt." >&2
    exit 1
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "${TMPDIR}"' EXIT

echo "Extracting payload..."
tail -n +"${PAYLOAD_LINE}" "$0" | tar xz -C "${TMPDIR}"

# Install pirewall binary.
echo "Installing pirewall -> ${INSTALL_BIN}/pirewall"
install -m 755 "${TMPDIR}/pirewall" "${INSTALL_BIN}/pirewall"

# Install helper scripts.
for script in rebootOnWatchdog; do
    echo "Installing ${script} -> ${INSTALL_BIN}/${script}"
    install -m 755 "${TMPDIR}/bin/${script}" "${INSTALL_BIN}/${script}"
done

# Install example configs.
echo "Installing examples -> ${INSTALL_SHARE}/examples"
install -d -m 755 "${INSTALL_SHARE}/examples"
cp -r "${TMPDIR}/examples/." "${INSTALL_SHARE}/examples/"
find "${INSTALL_SHARE}/examples" -type f -exec chmod 644 {} +

echo ""
echo "Installation complete."
echo "Example configuration files are in ${INSTALL_SHARE}/examples"

exit 0
#__PAYLOAD__
