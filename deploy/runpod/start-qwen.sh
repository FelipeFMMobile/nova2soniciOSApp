#!/usr/bin/env bash
set -euo pipefail

: "${VLLM_API_KEY:?VLLM_API_KEY must be set}"

QWEN_MODEL="${QWEN_MODEL:-Qwen/Qwen3-Omni-30B-A3B-Instruct}"
QWEN_MODEL_REVISION="${QWEN_MODEL_REVISION:-26291f7}"
MODEL_CACHE="${MODEL_CACHE:-/workspace/models}"
MAX_MODEL_LEN="${MAX_MODEL_LEN:-32768}"

mkdir -p "${MODEL_CACHE}"

exec vllm serve "${QWEN_MODEL}" \
  --revision "${QWEN_MODEL_REVISION}" \
  --omni \
  --host 0.0.0.0 \
  --port 8091 \
  --api-key "${VLLM_API_KEY}" \
  --download-dir "${MODEL_CACHE}" \
  --max-model-len "${MAX_MODEL_LEN}" \
  --trust-remote-code

