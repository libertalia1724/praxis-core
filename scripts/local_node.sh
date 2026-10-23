#!/usr/bin/env bash

PRAXISD_BIN=${PRAXISD_BIN:=$(which praxisd 2>/dev/null)}

if [ -z "$PRAXISD_BIN" ]; then echo "PRAXISD_BIN is not set. Make sure to run make install before"; exit 1; fi
echo "using $PRAXISD_BIN"
if [ -d "$($PRAXISD_BIN config home)" ]; then rm -rv $($PRAXISD_BIN config home); fi
$PRAXISD_BIN config set client chain-id demo
$PRAXISD_BIN config set client keyring-backend test
$PRAXISD_BIN config set client keyring-default-keyname alice
$PRAXISD_BIN config set app api.enable true
$PRAXISD_BIN keys add alice
$PRAXISD_BIN keys add bob
$PRAXISD_BIN init test --chain-id demo
$PRAXISD_BIN genesis add-genesis-account alice 5000000000upraxis --keyring-backend test
$PRAXISD_BIN genesis add-genesis-account bob 5000000000upraxis --keyring-backend test
$PRAXISD_BIN genesis gentx alice 1000000upraxis --chain-id demo
$PRAXISD_BIN genesis collect-gentxs