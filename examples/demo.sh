#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

PORT=18080
URL="http://127.0.0.1:$PORT"
STORE=$(mktemp)
BIN=/tmp/schema-registry-server
rm -f "$STORE"

(cd go && go build -o "$BIN" ./cmd/server)

"$BIN" -addr "127.0.0.1:$PORT" -store "$STORE" >/tmp/schema-registry.log 2>&1 &
SERVER_PID=$!
cleanup() {
  kill "$SERVER_PID" 2>/dev/null || true
  wait "$SERVER_PID" 2>/dev/null || true
  rm -f "$STORE"
}
trap cleanup EXIT

curl -s --retry 30 --retry-connrefused --retry-delay 1 "$URL/healthz" >/dev/null

export PYTHONPATH="$ROOT/python"

echo "1. первая версия:"
python3 -m schemaregistry --url "$URL" push user.created 1.0.0 examples/schemas/user-v1.json

echo "2. совместимое изменение:"
python3 -m schemaregistry --url "$URL" push user.created 1.1.0 examples/schemas/user-v2.json

echo "3. breaking change (ожидается отказ):"
if python3 -m schemaregistry --url "$URL" push user.created 2.0.0 examples/schemas/user-v3.json; then
  echo "ошибка: breaking change прошёл" >&2
  exit 1
fi

echo "4. принудительная регистрация:"
python3 -m schemaregistry --url "$URL" push user.created 2.0.0 examples/schemas/user-v3.json --force >/dev/null

echo "5. проверка против старой версии:"
python3 -m schemaregistry --url "$URL" server-check user.created 1.0.0 examples/schemas/user-v3.json || true

echo "6. список:"
python3 -m schemaregistry --url "$URL" list
