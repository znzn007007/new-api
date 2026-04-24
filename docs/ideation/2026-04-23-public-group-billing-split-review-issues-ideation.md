---
date: 2026-04-23
topic: public-group-billing-split-review-issues
focus: review issues in docs/reports/2026-04-23-public-group-billing-split-review-report.md
---

# Ideation: public-group-billing-split review issues

## Codebase Context

- This branch introduces public-group billing/tag resolution in `service/group_tag_resolver.go` and feeds the result into channel selection through `service/channel_select.go`.
- Current code now uses `resolution.MatchedTag` as both:
  - a **billing attribution signal**
  - and a **hard channel pool filter**
- In the current implementation, `ResolveGroupBilling` returns an error for multi-tag ambiguity without an override (`service/group_tag_resolver.go:80-87`). That is a behavior tightening compared with the previous fallback-oriented routing path.
- `model.GetRandomSatisfiedChannel(group, model, tag, retry)` filters to the tag sub-pool whenever `tag != ""`; only when tag is empty does it fall back to the broader group/model pool that still includes untagged fallback channels (`model/channel_cache.go:244-260`).
- Retry pricing is initialized once via `helper.ModelPriceHelper` before the first downstream call (`controller/relay.go:152-163`). During retry, code refreshes only `info.PriceData.GroupRatioInfo = helper.HandleGroupRatio(...)` (`controller/relay.go:300-302`), so pre-consume admission is not re-aligned if the effective billed path changes.
- Realtime quota code currently treats `GroupRatioInfo.GroupRatio == 0` as "not resolved" and recomputes from default group ratios (`service/quota.go:109-123`), which conflicts with the new meaning of `0` as a legitimate free ratio.
- Existing learning `docs/solutions/best-practices/public-group-based-billing-split-requires-code-changes-2026-04-22.md` remains valid: this area needs explicit code-level semantics rather than config-only tweaks.

## Ranked Ideas

### 1. Split **billing tag resolution** from **routing tag restriction** (recommended)
**Description:**  
Introduce two separate outcomes in the resolver:
- `BillingTag` / `BillingAttribution` for pricing and logs
- `RouteTagMode` for channel filtering, with explicit modes such as `none`, `prefer_tag`, `strict_tag`

Default behavior should be conservative:
- explicit admin override -> `strict_tag`
- inferred single tag -> `prefer_tag`
- ambiguous multi-tag -> `none`

**Rationale:**  
This directly addresses the root modeling error behind issues 1 and 2: one field currently means both “who should be billed” and “which channels are legal.” Separating those restores old fallback behavior while preserving the new billing attribution feature.

**Downsides:**  
Requires touching resolver output, channel selection, and likely some logging structs. Slightly larger than a one-line hotfix.

**Confidence:** 95%
**Complexity:** Medium
**Status:** Unexplored

### 2. Add **soft tag preference with untagged fallback** for inferred tags
**Description:**  
For automatically inferred tags, try the tagged channel pool first, then fall back to the untagged group/model pool if no tagged channel is available at the current retry/priority level.

**Rationale:**  
This is the smallest behavior-preserving route fix for issue 2. It keeps the “smart billing tag” feature, but avoids turning a single inferred tag into a hard exclusion of healthy untagged fallback channels.

**Downsides:**  
By itself, it does not fully solve issue 1 unless ambiguity also stops erroring. It also leaves the dual-meaning resolver API in place.

**Confidence:** 92%
**Complexity:** Medium
**Status:** Unexplored

### 3. Make ambiguity **non-fatal by default**; only explicit overrides can hard-fail
**Description:**  
Change `ResolveGroupBilling` so that multi-tag ambiguity without an operator-configured override becomes:
- `MatchedTag = ""`
- billing falls back to the public-group ratio
- channel selection keeps the old broad pool

Only explicit override mismatch should remain a hard error.

**Rationale:**  
This restores pre-change availability expectations and fixes issue 1 with minimal risk. It follows the report's recommendation: ambiguity means “cannot narrow early,” not “request invalid.”

**Downsides:**  
Operators lose the current “force cleanup by erroring” behavior unless that becomes a separate strict mode. Billing may be less granular in ambiguous cases until configuration is improved.

**Confidence:** 94%
**Complexity:** Low
**Status:** Unexplored

### 4. Add a **retry repricing reconciliation** step before each retry attempt
**Description:**  
Whenever retry changes the effective public group or billing tag, re-run pricing resolution and reconcile the pre-consume decision before issuing the next upstream attempt. If the new route is more expensive, perform a delta admission check and top up the pre-consume amount; if it is cheaper, keep the higher hold or optionally refund the delta at the end.

**Rationale:**  
This is the cleanest direct fix for issue 3. It aligns admission, pre-consume, and final settlement around the same effective billing decision.

**Downsides:**  
Touches quota/pre-consume semantics and has the highest correctness burden. Needs careful handling to avoid double-charging or refund gaps under repeated retries.

**Confidence:** 90%
**Complexity:** High
**Status:** Unexplored

### 5. Replace the “0 means unresolved” pattern with an explicit **ratio-resolution state**
**Description:**  
Extend `GroupRatioInfo` with an explicit boolean or enum such as `HasResolvedRatio` / `RatioSourceState`. Realtime and other consumers should branch on that state instead of `GroupRatio == 0`.

**Rationale:**  
This directly fixes issue 4 and also hardens future logic against the same class of bug. Free routes become first-class supported outcomes instead of overloading zero as both data and sentinel.

**Downsides:**  
Requires a small but cross-cutting type update, plus careful audit of every `GroupRatio == 0` assumption.

**Confidence:** 96%
**Complexity:** Medium
**Status:** Unexplored

### 6. Introduce a single **effective billing decision** object reused by pre-consume, retry, and final settlement
**Description:**  
Create one structured object that captures the effective billing outcome for an attempt:
- public group
- matched/inferred tag
- route restriction mode
- resolved ratio
- source metadata

Use this same object for:
- first-pass `ModelPriceHelper`
- retry recalculation
- final quota consumption/logging

**Rationale:**  
This is the strongest long-term move for issues 3 and 4 and prevents further drift between price computation, quota admission, and consume logging.

**Downsides:**  
More invasive than the narrow hotfixes. Best when paired with idea 1 rather than attempted as a stand-alone refactor.

**Confidence:** 88%
**Complexity:** High
**Status:** Unexplored

### 7. Add a targeted regression matrix before further behavior changes
**Description:**  
Lock in tests for the four reported cases:
- multi-tag + no override -> no hard failure
- inferred single tag + untagged fallback channel -> fallback still reachable
- retry to more expensive group/tag -> pre-consume/admission stays consistent
- realtime free route with ratio 0 -> remains free

**Rationale:**  
These bugs sit at the intersection of routing, pricing, and retry. Tests are unusually valuable here because future refactors will otherwise re-break invisible edge cases.

**Downsides:**  
Does not fix prod behavior by itself. Needs discipline to model the retry/realtime cases accurately.

**Confidence:** 97%
**Complexity:** Medium
**Status:** Unexplored

## Rejection Summary

| # | Idea | Reason Rejected |
|---|------|-----------------|
| 1 | Revert the whole public-group billing split feature | Too blunt; throws away useful billing attribution work instead of correcting the unsafe semantics |
| 2 | Keep multi-tag ambiguity as a hard error to force operators to clean config | Too expensive in availability terms and breaks pre-existing fallback behavior |
| 3 | Use channel affinity to choose the right billing tag/group | Not grounded in current repo responsibilities; affinity is for in-group stickiness, not group decision semantics |
| 4 | Treat inferred single tag as always authoritative and document the new behavior | Too risky relative to value; preserves the exact fallback regression reported in issue 2 |
| 5 | Fix only the realtime zero-ratio bug first | Valid hotfix but too narrow as an overall direction because P1 routing regressions would remain |
| 6 | Solve everything with more admin config knobs and strict-mode options only | Adds operator burden without first restoring safe defaults |
| 7 | Move billing entirely to channel-level profiles immediately | Interesting architecture direction, but too large and under-grounded for the current review-fix scope |

## Session Log
- 2026-04-23: Initial issue-focused ideation — 13 candidates/themes considered, 7 survived
