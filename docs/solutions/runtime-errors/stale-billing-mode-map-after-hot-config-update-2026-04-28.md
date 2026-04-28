---
title: Stale billing mode map can keep removed tiered_expr models active after hot config update
date: 2026-04-28
category: runtime-errors
module: billing_setting hot config reload
problem_type: runtime_error
component: service_object
symptoms:
  - "DB billing_setting.billing_mode no longer contains Claude keys, but /api/pricing still reports tiered_expr for those models"
  - "The pricing/settings UI continues showing expression/tiered billing after the operator switches the model back to ratio billing"
  - "Usage logs still show tiered expression details such as tier base for affected models"
  - "Runtime billing and UI state diverge from the persisted options table until the new-api process reloads cleanly"
root_cause: config_error
resolution_type: code_fix
severity: high
related_components:
  - "model/option.go option hot update path"
  - "setting/config/config.go map deserialization"
  - "setting/billing_setting/tiered_billing.go runtime billing mode map"
  - "model/pricing.go /api/pricing generation"
tags: [new-api, billing, tiered-expr, hot-config, optionmap, json-unmarshal, pricing-ui, runtime-cache]
---

# Stale billing mode map can keep removed tiered_expr models active after hot config update

## Problem

After an operator switched Claude models from expression/tiered billing back to default ratio billing, both the pricing UI and real request logs still behaved as if those models were configured with `tiered_expr`.

The persisted database configuration was already correct. The failure was in the hot runtime config path: map-based config was merged into an existing in-memory Go map, so keys removed from the saved JSON stayed active until the `new-api` process was restarted or the map was replaced by code.

## Symptoms

- Production DB showed `billing_setting.billing_mode` only contained Gemini tiered entries, not the affected Claude models.
- Production `/api/pricing` still returned `billing_mode: "tiered_expr"` for `claude-haiku-4-5-20251001` and `claude-sonnet-4-6`.
- The pricing/settings UI showed expression/tiered billing because it consumed backend pricing state derived from runtime config.
- Usage logs still showed tiered expression details, including tier `base`, because the real billing path used the same stale runtime mode map.

Concrete read-only production evidence from the investigation:

```text
billing_setting.billing_mode in DB:
  gemini-3-pro-preview -> tiered_expr
  gemini-3.1-pro-preview -> tiered_expr
  no claude-haiku-4-5-20251001 key
  no claude-sonnet-4-6 key

/api/pricing at runtime:
  claude-haiku-4-5-20251001 -> billing_mode=tiered_expr
  claude-sonnet-4-6 -> billing_mode=tiered_expr
```

That combination means the bug is not a failed DB save. It is runtime state drifting from persisted config.

## What Didn't Work

- **Checking only the database** was insufficient. The DB reflected the operator's intended ratio-billing state, while the running process still held stale in-memory keys.
- **Assuming the UI reads only `billing_setting.billing_mode` from DB** was misleading. The pricing display can also be driven by `/api/pricing`, which is generated from runtime config.
- **Looking for a hidden long-context billing rule** was a dead end. A prior session found GPT/Claude long-context billing does not automatically enable extra billing; `tiered_expr` is explicitly opt-in through `billing_setting.billing_mode` and `billing_setting.billing_expr` (session history).
- **Treating missing or unexpected tiered UI state as only stale frontend assets** was incomplete. A related earlier issue involved stale deployed frontend assets hiding the tiered option, but this incident was the inverse: the UI showed stale tiered runtime state even though persisted config removed it.

## Solution

### Immediate operational workaround

If this appears in production before the code fix is deployed, restart only the `new-api` application container so it reloads config from persisted options into fresh process memory.

Do not restart Postgres or Redis for this failure mode; the observed mismatch is inside the `new-api` process.

> Status note: during the 2026-04-28 investigation, the operator explicitly chose not to restart production yet. Document this workaround as recommended triage, not as an action already performed.

### Durable code fix

Map config values should be treated as complete JSON documents during hot reload. When applying `billing_setting.billing_mode = { ... }`, absence of a model key in JSON must mean absence from the runtime map.

The problematic pattern is unmarshalling directly into the existing map field:

```go
case reflect.Map, reflect.Slice, reflect.Struct:
    err := json.Unmarshal([]byte(strValue), field.Addr().Interface())
    if err != nil {
        continue
    }
```

For Go maps, unmarshalling into a non-nil map merges keys and updates values, but it does not delete keys that are absent from the incoming JSON.

Handle maps separately by decoding into a fresh map value and assigning the field:

```go
case reflect.Map:
    next := reflect.New(field.Type()).Interface()
    err := json.Unmarshal([]byte(strValue), next)
    if err != nil {
        continue
    }
    field.Set(reflect.ValueOf(next).Elem())
case reflect.Slice, reflect.Struct:
    err := json.Unmarshal([]byte(strValue), field.Addr().Interface())
    if err != nil {
        continue
    }
```

This keeps slice and struct behavior unchanged while making map config reload authoritative.

Add a regression test that proves stale keys are removed:

```go
func TestUpdateConfigFromMapReplacesMapValues(t *testing.T) {
    cfg := struct {
        BillingMode map[string]string `json:"billing_mode"`
    }{
        BillingMode: map[string]string{
            "stale-model": "tiered_expr",
        },
    }

    err := UpdateConfigFromMap(&cfg, map[string]string{
        "billing_mode": `{}`,
    })
    if err != nil {
        t.Fatalf("UpdateConfigFromMap returned error: %v", err)
    }

    if len(cfg.BillingMode) != 0 {
        t.Fatalf("expected stale map entries to be removed, got %#v", cfg.BillingMode)
    }
}
```

## Why This Works

Both the pricing UI and real billing path consult the same runtime billing-mode accessor:

```text
setting/billing_setting/tiered_billing.go
  GetBillingMode(model)
    -> reads billingSetting.BillingMode
```

That accessor feeds two visible paths:

```text
model/pricing.go
  updatePricing()
    -> GetBillingMode(model)
    -> /api/pricing includes BillingMode/BillingExpr
    -> UI shows tiered billing
```

```text
relay/helper/price.go
  ModelPriceHelper(...)
    -> GetBillingMode(info.OriginModelName)
    -> modelPriceHelperTiered when tiered_expr
    -> usage logs show tiered expression details
```

Replacing the map during hot config update restores the expected invariant:

```text
saved JSON document == runtime map contents
```

So when the saved JSON no longer contains `claude-haiku-4-5-20251001`, the runtime map no longer contains that key either. `GetBillingMode(model)` then falls back to the default `ratio` mode, and both `/api/pricing` and real billing stop taking the tiered path.

## Prevention

- Treat map-backed config fields as whole-document state unless the feature explicitly implements patch semantics.
- Add stale-key removal tests for any config map that controls routing, billing, permissions, or feature modes.
- When UI, logs, and DB disagree, check the live API layer (`/api/pricing` here) before assuming the frontend or database is wrong.
- For production triage, compare all three layers:
  1. options table / persisted JSON
  2. backend API output generated from runtime memory
  3. real request logs
- After changing pricing mode in production, verify a model that was removed from `billing_setting.billing_mode` no longer appears as `billing_mode=tiered_expr` in `/api/pricing`.

## Related Issues

- `docs/solutions/workflow-issues/stale-new-api-test-frontend-hides-tiered-billing-ui-2026-04-24.md` — related tiered-billing UI confusion, but the root cause was stale deployed frontend assets rather than stale runtime config.
- `docs/solutions/best-practices/fork-public-group-billing-vs-upstream-tiered-expr-maintenance-boundary-2026-04-24.md` — related billing architecture context; reinforces that pricing display, actual charge, and logs must stay on the same explanation path.
- `docs/solutions/documentation-gaps/responses-image-generation-call-pricing-is-backend-fixed-and-not-exposed-in-the-model-pricing-editor-2026-04-24.md` — another example where operator-visible billing UI and backend billing paths need to be reconciled explicitly.

## Verification Notes

At documentation time:

- The root cause was confirmed by comparing DB state to live `/api/pricing` output and by inspecting the hot config update path.
- A local code patch and regression test existed as pending, unapproved work, but production was not restarted and no deployed fix was verified.
- The expected verification after applying the fix is: save `billing_setting.billing_mode` without the affected model, then confirm `/api/pricing` and a real request both use default ratio billing instead of `tiered_expr`.
