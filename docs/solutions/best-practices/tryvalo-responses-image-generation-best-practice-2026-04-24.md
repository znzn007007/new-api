---
title: Use /v1/responses image generation correctly on new.tryvalo.com
date: 2026-04-24
category: best-practices
module: openai-responses image-generation integration (tryvalo gateway)
problem_type: best_practice
component: tooling
severity: medium
applies_when:
  - You need to run image generation through https://new.tryvalo.com using OpenAI Responses API compatibility
  - You are sending model gpt-5.4 with tools.type=image_generation
  - You need to parse streaming events until response.output_item.done returns image_generation_call.result
  - You must keep the upstream API key on the backend rather than shipping it in frontend code
tags: [new-api, tryvalo, responses-api, image-generation, streaming, backend-security]
---

# Use /v1/responses image generation correctly on new.tryvalo.com

## Context
A user asked whether the sample integration in `生图功能.txt` could generate images through `https://new.tryvalo.com/` using the OpenAI Responses API shape.

Local repo inspection and live verification on **2026-04-24** confirmed the path is valid:
- `docs/openapi/relay.json` documents `POST /v1/responses`
- `dto/openai_response.go` defines `image_generation_call` as a supported Responses output type
- `relay/channel/openai/relay_responses.go` handles image-generation response metadata
- `GET https://new.tryvalo.com/v1/models` returned `gpt-5.4` with `supported_endpoint_types` including `openai-response`
- `POST https://new.tryvalo.com/v1/responses` succeeded in both non-stream and stream modes
- stream mode emitted `response.output_item.done`, with `item.type = image_generation_call` and base64 image data in `item.result`

The original sample was already close; the main integration questions were the correct base URL, the exact endpoint shape, and whether the gateway actually accepted the request end to end.

## Guidance
Use the gateway as a standard Responses API base:

- `BASE_URL = https://new.tryvalo.com/v1`
- request path: `POST /responses`
- model: `gpt-5.4`
- built-in tool: `{"type": "image_generation"}`

The verified request shape is:

```json
{
  "model": "gpt-5.4",
  "input": [{"role": "user", "content": "Generate a simple icon"}],
  "tools": [{"type": "image_generation"}],
  "stream": true
}
```

Recommended operational refinements:
1. Add optional `quality` and `size` fields on the image-generation tool when you want predictable cost/output tradeoffs.
2. For stream mode, keep reading SSE events until `response.output_item.done` contains an `image_generation_call` item.
3. Decode `item.result` from base64 and write the bytes to a file if you want a local PNG.
4. Keep the real API key on the backend only. Frontend code should call your own server, not the upstream gateway directly.

A concise Python pattern:

```python
import httpx
import json
import base64

API_KEY = "<backend secret>"
BASE_URL = "https://new.tryvalo.com/v1"

body = {
    "model": "gpt-5.4",
    "input": [{"role": "user", "content": "Generate a simple icon"}],
    "tools": [{
        "type": "image_generation",
        "quality": "low",
        "size": "1024x1024"
    }],
    "stream": True,
}

image_b64 = None
with httpx.stream("POST", f"{BASE_URL}/responses", headers={
    "Authorization": f"Bearer {API_KEY}",
    "Content-Type": "application/json",
}, json=body, timeout=180) as r:
    for line in r.iter_lines():
        line = line.strip()
        if not line.startswith("data:"):
            continue
        data = json.loads(line[len("data:"):].strip())
        if data.get("type") == "response.output_item.done":
            item = data.get("item", {})
            if item.get("type") == "image_generation_call":
                image_b64 = item.get("result")
                break

if image_b64:
    with open("output.png", "wb") as f:
        f.write(base64.b64decode(image_b64))
```

## Why This Matters
This verification removes two common sources of wasted time:

- guessing whether the gateway needs a custom image endpoint instead of standard Responses API
- debugging client code when the real issue is an incorrect base URL or response parsing strategy

It also establishes a safe deployment pattern. Even when the gateway is OpenAI-compatible, the secret key still belongs on the backend. Treating the browser as a direct caller creates an avoidable credential leak.

## When to Apply
- You are validating whether a new-api deployment or proxy domain supports OpenAI Responses image generation
- You have a sample script similar to `生图功能.txt` and need to point it at a real gateway domain
- You want streaming image-generation support rather than polling or a custom image endpoint
- You are wiring image generation into your own app and need to decide whether frontend code can call the gateway directly

## Examples
### Correct base URL and endpoint
```text
BASE_URL = https://new.tryvalo.com/v1
POST https://new.tryvalo.com/v1/responses
```

### Minimal non-stream request
```json
{
  "model": "gpt-5.4",
  "input": "Generate a blue square icon.",
  "tools": [{"type": "image_generation"}],
  "stream": false
}
```

### Stream event to wait for
```json
{
  "type": "response.output_item.done",
  "item": {
    "type": "image_generation_call",
    "result": "<base64-png>"
  }
}
```

### Cost-control variant
```json
{
  "tools": [{
    "type": "image_generation",
    "quality": "low",
    "size": "1024x1024"
  }]
}
```

### Deployment rule
- Correct: browser -> your backend -> `https://new.tryvalo.com/v1/responses`
- Avoid: browser -> `https://new.tryvalo.com/v1/responses` with the real upstream key embedded in JS

## Related
- `生图功能.txt`
- `examples/openai-responses.sh`
- `docs/openapi/relay.json`
- `dto/openai_response.go`
- `relay/channel/openai/relay_responses.go`
- `docs/solutions/integration-issues/channel-test-503-no-available-accounts-2026-04-20.md` — useful when reproducing an upstream request exactly to separate gateway problems from local integration mistakes
- `docs/solutions/best-practices/header-based-cc-routing-with-single-key-2026-04-22.md` — another gateway-facing integration note for this repo
