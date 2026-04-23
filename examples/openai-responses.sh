#!/usr/bin/env bash
set -euo pipefail

# Usage:
#   1. 只改下面的 BASE_URL 和 API_KEY
#   2. 直接运行: bash examples/openai-responses.sh

BASE_URL="${NEWAPI_BASE_URL:-http://localhost:3000}"
API_KEY="${NEWAPI_API_KEY:-sk-xxxx}"
MODEL="${NEWAPI_OPENAI_RESPONSES_MODEL:-${NEWAPI_MODEL:-gpt-4.1-mini}}"
PROMPT="${1:-你好}"
STREAM="${NEWAPI_STREAM:-false}"

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

curl --silent --show-error "${BASE_URL%/}/v1/responses" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer ${API_KEY}" \
  -d @- <<JSON
{
  "model": "${MODEL}",
  "stream": ${STREAM},
  "input": [
    {
      "role": "user",
      "content": "${PROMPT_JSON}"
    }
  ]
}
JSON
