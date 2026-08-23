#!/bin/sh
set -e
cd "$(dirname "$0")/../.."
rm -f /tmp/tournamentstudio-e2e.db
export TOURNAMENTSTUDIO_DB=/tmp/tournamentstudio-e2e.db
export TOURNAMENTSTUDIO_ADMIN_USER=organizer1
export TOURNAMENTSTUDIO_ADMIN_PASSWORD=e2e-test-password
go run ./cmd/tournamentstudio
