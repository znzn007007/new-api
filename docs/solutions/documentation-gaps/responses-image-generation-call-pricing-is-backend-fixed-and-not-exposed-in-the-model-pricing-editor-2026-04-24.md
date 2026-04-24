---
title: Responses image generation call pricing is backend-fixed and not exposed in the model pricing editor
date: 2026-04-24
category: documentation-gaps
module: responses image-generation billing and admin pricing ui
problem_type: documentation_gap
component: documentation
severity: low
applies_when:
  - You are debugging why logs show Image Generation Call charges for /v1/responses requests
  - You are checking the admin model pricing page and cannot find where image generation per-call pricing is configured
  - You need to explain the difference between 图片输入价格 and 图片生成调用 to operators or users
tags: [new-api, responses-api, image-generation, billing, pricing-ui, gpt-image-1]
symptoms:
  - The log details show Image Generation Call 花费 with an additional quota amount such as 21000
  - The admin pricing editor only shows 输入价格, 补全价格, 缓存价格, 图片输入价格, and音频价格 fields
  - Operators assume the image generation per-call rule should be editable from the same pricing page
root_cause: inadequate_documentation
resolution_type: documentation_update
---

# Responses image generation call pricing is backend-fixed and not exposed in the model pricing editor

## Context
A user saw `Image Generation Call 花费 21000` in the request log for a `/v1/responses` request and then checked the admin model pricing page for `gpt-image-1`. The UI showed token-oriented fields such as `输入价格`, `补全价格`, and `图片输入价格`, but it did not show any rule for image generation per-call billing.

Code inspection confirmed that the confusion comes from two different billing paths being present at the same time:

1. **token/ratio pricing configured in the admin pricing editor**
2. **backend-fixed image generation per-call pricing for Responses image-generation calls**

## Guidance
Treat these as two separate concepts.

### 1. 图片输入价格 is not 图片生成调用
The admin pricing editor exposes `图片输入价格`, which maps to `ImageRatio` and affects **image input token billing**.

Relevant frontend/editor paths:
- `web/src/pages/Setting/Ratio/components/ModelPricingEditor.jsx`
- `web/src/pages/Setting/Ratio/hooks/useModelPricingEditorState.js`

The preview rows on that page only derive values such as:
- `ModelRatio`
- `CompletionRatio`
- `CacheRatio`
- `CreateCacheRatio`
- `ImageRatio`
- `AudioRatio`
- `AudioCompletionRatio`

So that page explains how token-based pricing is persisted, not how a Responses image generation tool call is billed.

### 2. Image Generation Call is a backend-fixed per-call surcharge
For `/v1/responses`, the backend checks whether the OpenAI-compatible response output contains:

```go
const ResponsesOutputTypeImageGenerationCall = "image_generation_call"
```

Relevant code:
- `dto/openai_response.go`
- `relay/channel/openai/relay_responses.go`

When the response contains `image_generation_call`, the relay stores these context fields:
- `image_generation_call = true`
- `image_generation_call_quality`
- `image_generation_call_size`

Then settlement uses a dedicated fixed-price lookup:

```go
summary.ImageGenerationCallPrice = operation_setting.GetGPTImage1PriceOnceCall(
    ctx.GetString("image_generation_call_quality"),
    ctx.GetString("image_generation_call_size"),
)
```

Relevant billing path:
- `service/text_quota.go`
- `service/tool_billing.go`
- `setting/operation_setting/tools.go`

### 3. The backend-fixed matrix currently has 9 price combinations
The current fixed table in `setting/operation_setting/tools.go` is:

| quality | size | price |
|---|---|---:|
| low | 1024x1024 | 0.011 |
| low | 1024x1536 | 0.016 |
| low | 1536x1024 | 0.016 |
| medium | 1024x1024 | 0.042 |
| medium | 1024x1536 | 0.063 |
| medium | 1536x1024 | 0.063 |
| high | 1024x1024 | 0.167 |
| high | 1024x1536 | 0.25 |
| high | 1536x1024 | 0.25 |

If the pair does not match, the helper falls back to `high + 1024x1024`.

### 4. Why the log showed 21000
The surcharge shown in logs is quota, not dollars.

The calculation path is:

```text
image generation surcharge quota = price_per_call × group_ratio × QuotaPerUnit
```

`common/constants.go` defines:

```go
var QuotaPerUnit = 500 * 1000.0
```

So when the call is priced at `$0.042` and the group ratio is `1x`:

```text
0.042 × 1 × 500000 = 21000
```

That is why the log can show:

```text
Image Generation Call 花费 21000
```

while the detailed pricing article also shows:

```text
图片生成调用：$0.042000 / 1次
```

## Why This Matters
If operators treat `图片输入价格` as the configuration source for `Image Generation Call`, they will misread both the UI and the logs.

That causes three common mistakes:
- trying to explain a per-call surcharge using token fields
- assuming the backend is using the admin pricing page for all image-related charges
- believing the admin UI is missing a field due to rendering or deployment drift, when the real reason is that the per-call matrix is not exposed there at all

The correct mental model is:

> **图片输入价格 covers image-as-input token billing; Image Generation Call is a separate per-call surcharge driven by response metadata and a backend-fixed price table.**

## When to Apply
- A `/v1/responses` request used the built-in image generation tool and settlement includes an extra image generation charge
- Someone opens the pricing UI for `gpt-image-1` and asks where the per-call rule is configured
- You need to reconcile request-log pricing details with the admin pricing editor
- You are considering whether the admin UI needs a new configuration surface for image generation call pricing

## Examples
### Example: response metadata that triggers the surcharge
`relay/channel/openai/relay_responses.go` marks the request when the response output contains `image_generation_call` and extracts `quality` and `size`.

### Example: why the UI does not show it
`ModelPricingEditor.jsx` renders fields for input/output/cache/image-input/audio pricing, but no field for a dedicated image-generation-per-call matrix.

### Example: the specific user-visible mismatch
- UI summary: `输入 $10, 额外价格项 3`
- Log detail: `Image Generation Call 花费 21000`
- Explanation: the UI summary is about token-priced fields saved as ratios, while the log surcharge comes from the backend-fixed `GetGPTImage1PriceOnceCall()` table.

## Related
- `docs/solutions/best-practices/tryvalo-responses-image-generation-best-practice-2026-04-24.md` — covers correct `/v1/responses` image generation request/stream behavior, including `image_generation_call` output handling
- `docs/solutions/workflow-issues/stale-new-api-test-frontend-hides-tiered-billing-ui-2026-04-24.md` — documents a different UI confusion pattern caused by stale deployed frontend assets rather than a missing configuration surface
