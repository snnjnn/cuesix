#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
TESTS_DIR="${ROOT_DIR}/src/buffering/tests"
IMAGE_TAG="${IMAGE_TAG:-apisix-buffering-e2e:local}"
COMPOSE_FILE="${TESTS_DIR}/docker-compose.yml"
PROJECT_NAME="${PROJECT_NAME:-buf-e2e}"
WAIT_TIMEOUT_SECONDS="${WAIT_TIMEOUT_SECONDS:-360}"

cleanup() {
  IMAGE_TAG="${IMAGE_TAG}" \
    docker compose -f "${COMPOSE_FILE}" --project-name "${PROJECT_NAME}" down -v --remove-orphans >/dev/null 2>&1 || true
}
trap cleanup EXIT

# Ensure no stale stack is still running before a new test run.
cleanup

IMAGE_TAG="${IMAGE_TAG}" \
  docker compose -f "${COMPOSE_FILE}" --project-name "${PROJECT_NAME}" up -d --build

tests_cid="$(
  IMAGE_TAG="${IMAGE_TAG}" \
    docker compose -f "${COMPOSE_FILE}" --project-name "${PROJECT_NAME}" ps -q tests
)"
if [[ -z "${tests_cid}" ]]; then
  echo "tests container was not created"
  exit 1
fi

IMAGE_TAG="${IMAGE_TAG}" \
  docker compose -f "${COMPOSE_FILE}" --project-name "${PROJECT_NAME}" logs -f --no-color tests &
logs_pid=$!

if ! timeout --preserve-status "${WAIT_TIMEOUT_SECONDS}" docker wait "${tests_cid}" >/dev/null; then
  rc=$?
  kill "${logs_pid}" >/dev/null 2>&1 || true
  wait "${logs_pid}" >/dev/null 2>&1 || true
  if [[ "${rc}" == "124" ]]; then
    echo "Timed out waiting for tests container after ${WAIT_TIMEOUT_SECONDS}s"
  fi
  IMAGE_TAG="${IMAGE_TAG}" \
    docker compose -f "${COMPOSE_FILE}" --project-name "${PROJECT_NAME}" logs tests apisix upstream || true
  exit "${rc}"
fi

kill "${logs_pid}" >/dev/null 2>&1 || true
wait "${logs_pid}" >/dev/null 2>&1 || true
tests_exit_code="$(docker inspect "${tests_cid}" --format '{{.State.ExitCode}}')"

if [[ "${tests_exit_code}" != "0" ]]; then
  IMAGE_TAG="${IMAGE_TAG}" \
    docker compose -f "${COMPOSE_FILE}" --project-name "${PROJECT_NAME}" logs tests apisix upstream || true
else
  echo "buffering-e2e: PASS"
fi

exit "${tests_exit_code}"
