#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   1. 只改下面的 BASE_URL 和 API_KEY
#   2. 直接运行: bash examples/claude-messages.sh

BASE_URL="${NEWAPI_BASE_URL:-http://localhost:3000}"
API_KEY="${NEWAPI_API_KEY:-sk-xxxx}"
MODEL="${NEWAPI_CLAUDE_MODEL:-${NEWAPI_MODEL:-claude-3-5-sonnet-latest}}"
PROMPT="${1:-你好}"
STREAM="${NEWAPI_STREAM:-false}"
ANTHROPIC_VERSION="${NEWAPI_ANTHROPIC_VERSION:-2023-06-01}"
MAX_TOKENS="${NEWAPI_MAX_TOKENS:-512}"

if [[ "${API_KEY}" == "sk-xxxx" ]]; then
  echo "请先把脚本里的 API_KEY 改成你自己的令牌，或通过 NEWAPI_API_KEY 传入。" >&2
  exit 1
fi

json_escape() {
  local value="$1"
  value=${value//\\/\\\\}
  value=${value//\"/\\\"}
  value=${value//$'\n'/\\n}
  value=${value//$'\r'/\\r}
  value=${value//$'\t'/\\t}
  printf '%s' "${value}"
}

PROMPT_JSON="$(json_escape "${PROMPT}")"

curl --silent --show-error "${BASE_URL%/}/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: ${API_KEY}" \
  -H "anthropic-version: ${ANTHROPIC_VERSION}" \
  -d @- <<JSON
{
  "model": "${MODEL}",
  "max_tokens": ${MAX_TOKENS},
  "stream": ${STREAM},
  "messages": [
    {
      "role": "user",
      "content": "${PROMPT_JSON}"
    }
  ]
}
JSON
