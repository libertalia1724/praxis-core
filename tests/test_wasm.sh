#!/usr/bin/env bash
# wasm smoke test for a running local praxisd node
#
#   terminal 1:  make start
#   terminal 2:  bash scripts/test_wasm.sh
#
# flow: params -> store code -> instantiate -> smart query -> execute -> balance check
set -euo pipefail

BIN=${PRAXISD_BIN:-praxisd}
CHAIN_ID=${CHAIN_ID:-demo}
FROM=${FROM:-alice}
TO=${TO:-bob}
DENOM=${DENOM:-upraxis}
AMOUNT=${AMOUNT:-1000}
WASM_URL=${WASM_URL:-https://raw.githubusercontent.com/libertalia1724/praxis-wasmd/main/x/wasm/keeper/testdata/hackatom.wasm}
WASM_FILE=${WASM_FILE:-/tmp/hackatom.wasm}

command -v jq >/dev/null || { echo "jq is required (sudo apt install jq)"; exit 1; }
command -v "$BIN" >/dev/null || { echo "$BIN not found, run 'make install' first"; exit 1; }

TX_FLAGS=(--from "$FROM" --chain-id "$CHAIN_ID" --keyring-backend test
          --gas auto --gas-adjustment 1.5 -y -o json)

step() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }
ok()   { printf '    \033[32mOK\033[0m %s\n' "$*"; }
die()  { printf '    \033[31mFAIL\033[0m %s\n' "$*" >&2; exit 1; }

# send_tx <tx subcommand args...>: broadcast, wait for inclusion, fail on non-zero code.
send_tx() {
  local out hash res code
  out=$("$BIN" tx "$@" "${TX_FLAGS[@]}" | grep '^{' | tail -n1)
  [ -n "$out" ] || die "no JSON output from: praxisd tx $*"
  code=$(jq -r '.code // 0' <<<"$out")
  [ "$code" = "0" ] || die "CheckTx rejected (code $code): $(jq -r '.raw_log' <<<"$out")"
  hash=$(jq -r '.txhash' <<<"$out")
  res=$("$BIN" query wait-tx "$hash" -o json | grep '^{' | tail -n1)
  code=$(jq -r '.code // 0' <<<"$res")
  [ "$code" = "0" ] || die "DeliverTx failed (code $code): $(jq -r '.raw_log' <<<"$res")"
  echo "$hash"
}

step "node is reachable and producing blocks"
H1=$("$BIN" status | jq -r '.sync_info.latest_block_height // .SyncInfo.latest_block_height')
sleep 6
H2=$("$BIN" status | jq -r '.sync_info.latest_block_height // .SyncInfo.latest_block_height')
[ "$H2" -gt "$H1" ] || die "block height is not increasing ($H1 -> $H2)"
ok "height $H1 -> $H2"

step "wasm module is registered and libwasmvm is linked"
"$BIN" query wasm libwasmvm-version | head -n1
"$BIN" query wasm params -o json | jq -c '.params // .' || die "wasm params query failed"
ok "wasm queries work"

step "addresses"
FROM_ADDR=$("$BIN" keys show "$FROM" -a --keyring-backend test)
TO_ADDR=$("$BIN" keys show "$TO" -a --keyring-backend test)
echo "    $FROM -> $FROM_ADDR"
echo "    $TO   -> $TO_ADDR"

step "download test contract (hackatom)"
[ -s "$WASM_FILE" ] || curl -fsSL -o "$WASM_FILE" "$WASM_URL"
ls -l "$WASM_FILE" | awk '{print "    " $5 " bytes"}'

step "store code"
send_tx wasm store "$WASM_FILE" >/dev/null
CODE_ID=$("$BIN" query wasm list-code -o json | jq -r '.code_infos[-1].code_id')
[ -n "$CODE_ID" ] && [ "$CODE_ID" != "null" ] || die "could not read code_id"
ok "code_id = $CODE_ID"

step "instantiate (contract funded with ${AMOUNT}${DENOM})"
send_tx wasm instantiate "$CODE_ID" \
  "{\"verifier\":\"$FROM_ADDR\",\"beneficiary\":\"$TO_ADDR\"}" \
  --label hackatom-test --no-admin --amount "${AMOUNT}${DENOM}" >/dev/null
CONTRACT=$("$BIN" query wasm list-contract-by-code "$CODE_ID" -o json | jq -r '.contracts[-1]')
[ -n "$CONTRACT" ] && [ "$CONTRACT" != "null" ] || die "could not read contract address"
ok "contract = $CONTRACT (${#CONTRACT} chars; account addresses are ${#FROM_ADDR}, 32-byte contract addresses are longer)"

step "smart query"
VERIFIER=$("$BIN" query wasm contract-state smart "$CONTRACT" '{"verifier":{}}' -o json | jq -r '.data.verifier')
[ "$VERIFIER" = "$FROM_ADDR" ] || die "unexpected verifier: $VERIFIER"
ok "verifier = $VERIFIER"

step "execute release -> funds go to $TO"
bal() { "$BIN" query bank balance "$1" "$DENOM" -o json | jq -r '.balance.amount'; }
BEFORE=$(bal "$TO_ADDR")
send_tx wasm execute "$CONTRACT" '{"release":{}}' >/dev/null
AFTER=$(bal "$TO_ADDR")
[ "$((AFTER - BEFORE))" -eq "$AMOUNT" ] || die "expected +$AMOUNT$DENOM, got $((AFTER - BEFORE))"
ok "$TO balance $BEFORE -> $AFTER (+$AMOUNT$DENOM)"

printf '\n\033[32mAll wasm checks passed.\033[0m\n'