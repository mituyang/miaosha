#!/bin/sh
set -eu

STATE_FILE="${THROTTLED_HEALTHCHECK_STATE_FILE:-/tmp/throttled-healthcheck.state}"
SUCCESS_THRESHOLD="${THROTTLED_HEALTHCHECK_SUCCESS_THRESHOLD:-3}"
STEADY_INTERVAL="${THROTTLED_HEALTHCHECK_STEADY_INTERVAL:-30}"
DOCKER_INTERVAL="${THROTTLED_HEALTHCHECK_DOCKER_INTERVAL:-1}"

if [ "$#" -eq 0 ]; then
    echo "usage: throttled-healthcheck.sh <command> [args...]" >&2
    exit 1
fi

load_state() {
    if [ -f "$STATE_FILE" ]; then
        # shellcheck disable=SC1090
        . "$STATE_FILE"
    fi

    success_count="${success_count:-0}"
    last_probe_ts="${last_probe_ts:-0}"
}

save_state() {
    printf 'success_count=%s\nlast_probe_ts=%s\n' \
        "$success_count" \
        "$last_probe_ts" \
        > "$STATE_FILE"
}

load_state
started_at="$(date +%s)"

if "$@" >/dev/null 2>&1; then
    probed_at="$(date +%s)"

    if [ "$success_count" -lt "$SUCCESS_THRESHOLD" ]; then
        success_count=$((success_count + 1))
    fi

    last_probe_ts="$probed_at"

    save_state

    # 连续成功达到阈值后，主动把本次检查拉长到接近稳态周期，
    # 让 Docker 下一次记录 healthcheck 日志时约为 30 秒后。
    if [ "$success_count" -ge "$SUCCESS_THRESHOLD" ]; then
        elapsed=$((probed_at - started_at))
        remaining=$((STEADY_INTERVAL - DOCKER_INTERVAL - elapsed))
        if [ "$remaining" -gt 0 ]; then
            sleep "$remaining"
        fi
    fi

    exit 0
fi

success_count=0
last_probe_ts=0
save_state
exit 1
