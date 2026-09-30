#!/usr/bin/env bash
set -euo pipefail

SLOSERVE_PORT="${SLOSERVE_REFERENCE_PORT:-18089}"
SLOSERVE_ENDPOINT="http://127.0.0.1:${SLOSERVE_PORT}"
mkdir -p work bench/results

cleanup() {
  if [[ -n "${SLOSERVE_PID:-}" ]]; then
    kill "${SLOSERVE_PID}" 2>/dev/null || true
    wait "${SLOSERVE_PID}" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

env \
  PORT="${SLOSERVE_PORT}" \
  ENGINE_MODE=fake \
  DEFAULT_MODEL=fake-engine/v1 \
  MAX_RUNNING=4 \
  INTERACTIVE_RESERVE=1 \
  MAX_QUEUED_TOKENS=4000 \
  MAX_QUEUE_WAIT=350ms \
  MAX_PRE_TTFT_RETRIES=1 \
  TTFT_TARGET=300ms \
  FAKE_TTFT=40ms \
  FAKE_TPOT=5ms \
  go -C gateway run ./cmd/gateway >work/reference-gateway.log 2>&1 &
SLOSERVE_PID=$!

for _ in {1..100}; do
  if curl --silent --fail "${SLOSERVE_ENDPOINT}/readyz" >/dev/null; then
    break
  fi
  sleep 0.1
done
curl --silent --fail "${SLOSERVE_ENDPOINT}/readyz" >/dev/null

python3 bench/run.py \
  --endpoint "${SLOSERVE_ENDPOINT}" \
  --requests 48 \
  --concurrency 12 \
  --max-tokens 10 \
  --batch-ratio 0.35 \
  --ttft-slo-ms 300 \
  --output bench/results/reference-fake-overload.json

python3 bench/run.py \
  --endpoint "${SLOSERVE_ENDPOINT}" \
  --requests 32 \
  --concurrency 4 \
  --max-tokens 10 \
  --batch-ratio 0 \
  --ttft-slo-ms 300 \
  --output bench/results/reference-fake-steady.json

python3 bench/run.py \
  --endpoint "${SLOSERVE_ENDPOINT}" \
  --requests 1 \
  --concurrency 1 \
  --max-tokens 5 \
  --batch-ratio 0 \
  --prompt "[fail-after]" \
  --output work/reference-failure-smoke.json >/dev/null

python3 -c 'import json; report=json.load(open("work/reference-failure-smoke.json")); assert report["summary"]["errors"] == 1 and report["summary"]["completed"] == 0'
