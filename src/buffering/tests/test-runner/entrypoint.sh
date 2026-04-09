#!/usr/bin/env bash
set -euo pipefail

APISIX_HOST="${APISIX_HOST:-apisix}"
APISIX_PORT="${APISIX_PORT:-9080}"
APISIX_LOG_FILE="${APISIX_LOG_FILE:-/tmp/apisix-logs/error.log}"
TESTS_TIMEOUT_SECONDS="${TESTS_TIMEOUT_SECONDS:-300}"

export TEST_NGINX_BINARY="${TEST_NGINX_BINARY:-/usr/sbin/nginx}"
export TEST_NGINX_SERVROOT="${TEST_NGINX_SERVROOT:-/tmp/test-nginx}"

for _ in $(seq 1 60); do
  if curl -fsS "http://${APISIX_HOST}:${APISIX_PORT}/baseline/ping" >/dev/null 2>&1; then
    break
  fi
  sleep 1
done

if ! curl -fsS "http://${APISIX_HOST}:${APISIX_PORT}/baseline/ping" >/dev/null 2>&1; then
  echo "APISIX did not become ready"
  exit 1
fi

if [[ ! -f "${APISIX_LOG_FILE}" ]]; then
  echo "APISIX log file not found: ${APISIX_LOG_FILE}"
  exit 1
fi

if ! timeout --preserve-status "${TESTS_TIMEOUT_SECONDS}" prove -r t; then
  rc=$?
  if [[ "${rc}" == "124" ]]; then
    echo "Test execution timed out after ${TESTS_TIMEOUT_SECONDS}s"
  fi
  exit "${rc}"
fi
