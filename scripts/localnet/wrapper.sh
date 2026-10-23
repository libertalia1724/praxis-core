#!/usr/bin/env sh
set -eu

ID=${ID:-0}
LOG=${LOG:-praxisd.log}

export PRAXISD_HOME="/data/node${ID}"

if [ ! -d "$PRAXISD_HOME" ]; then
    echo "Home directory $PRAXISD_HOME does not exist"
    exit 1
fi

exec praxisd --home "$PRAXISD_HOME" "$@" 2>&1 | tee "$PRAXISD_HOME/$LOG"
