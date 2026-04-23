---
date: 2026-04-22
topic: public-group-billing-split
focus: public-group-based-billing-split-requires-code-changes-2026-04-22.md
---

# Ideation: Public Group Billing Split

## Codebase Context

- 当前仓库里的 `group` 不是单一概念，它同时承担了 **用户可见分组**、**模型可见性**、**渠道选择**、**计费倍率** 这几件事。
- `middleware/auth.go` 会先把 `token.Group` / `user.Group` 解析成 `ContextKeyUsingGroup`；后续 `middleware/distributor.go`、`relay/helper/price.go`、`service/quota.go` 都继续依赖这个值。
- `setting/ratio_setting/group_ratio.go` 的分组倍率是 **按 group 配置**；`setting/ratio_setting/model_ratio.go` 的模型倍率是 **全局按 model 配置**。也就是说，当前系统没有“同一个公开组里，不同模型命中不同计费倍率”的原生表达方式。
- `controller/group.go` + `web/src/helpers/api.js` + `processGroupsData` 说明：前端下拉里实际用的还是 group key，本质上没有“一个公开名称背后挂多个内部组”的正式机制；最多只能给 group 配个描述。
- `controller/model.go`、`controller/pricing.go` 也都默认把“用户看到的组”和“后台计费使用的组”视为同一个值。
- 已有文档 `docs/solutions/best-practices/public-group-based-billing-split-requires-code-changes-2026-04-22.md` 的核心判断是对的：**这个需求合理，而且在当前架构下确实不能只靠 Nginx 或现有分组配置解决。**

## Ranked Ideas

### 1. 给公开组增加“计费覆盖规则”层（推荐）
**Description:**  
保留用户可见分组不变，例如：
- `ask-第三方渠道`
- `ask-CC专用`

新增一层规则，例如：
- `ask-第三方渠道` + `gpt-*` → `billing_group=ask-gpt-internal`
- `ask-第三方渠道` + `claude-*` → `billing_group=ask-claude-3p-internal`
- `ask-CC专用` + `claude-*` → `billing_group=ask-cc-internal`

运行时仍保留公开组用于用户视角；真正算倍率/日志归档/财务归因时使用 `billing_group`。

**Rationale:**  
这是最贴合你当前诉求的做法：  
用户仍只看到两个组，但后台可以按 GPT / Claude 第三方 / Claude Code 三个桶结算。

**Downsides:**  
- 需要新增一份规则配置与解析逻辑  
- `/api/pricing`、日志、后台展示也要同步吃这套规则，否则前台看到的价格会不准

**Confidence:** 92%
**Complexity:** Medium
**Status:** Unexplored

### 2. 正式拆成三种组语义：public group / route group / billing group
**Description:**  
把现在一个 `group` 拆成三个概念：
- `public_group`：用户看到和选择的组
- `route_group`：渠道选择真正使用的组
- `billing_group`：扣费倍率和财务归档使用的组

默认三者相等；只有命中规则时才分离。

**Rationale:**  
这是长期最干净的架构。因为你这次碰到的问题，本质上就是一个字段承载了太多职责。

**Downsides:**  
- 改动面会明显大于方案 1  
- 需要检查模型列表、定价页、日志、重试、affinity、task billing 等多处逻辑

**Confidence:** 88%
**Complexity:** High
**Status:** Unexplored

### 3. 不再用“内部组”表达成本，而是给渠道/标签挂 billing profile
**Description:**  
把“倍率不同”从 group 维度挪到 channel/tag 维度。  
例如给 GPT 渠道打 `billing_profile=gpt_3p`，Claude 第三方渠道打 `billing_profile=claude_3p`，Claude Code 渠道打 `billing_profile=claude_cc`。  
选到哪个渠道，就吃哪个 billing profile。

**Rationale:**  
从业务语义上说，成本差异很多时候确实是“渠道差异”而不是“用户分组差异”。这个建模更接近真实世界。

**Downsides:**  
- 当前代码的计费核心是先按 group 算，不是按 channel profile 算  
- 同一个公开组下如果有多个候选渠道，前台价格预览会更难解释

**Confidence:** 76%
**Complexity:** High
**Status:** Unexplored

### 4. 增加“公开别名 + 隐藏内部组”机制
**Description:**  
允许后台维护多个内部组，但给它们挂同一个公开别名，并支持隐藏。  
例如：
- `ask-gpt-internal` → alias=`ask-第三方渠道`
- `ask-claude-3p-internal` → alias=`ask-第三方渠道`
- `ask-cc-internal` → alias=`ask-CC专用`

用户只看到 alias，系统内部再做映射。

**Rationale:**  
这会让“用户视角只有两个组”变成一个明确的产品能力，而不是靠约定实现。

**Downsides:**  
- 单独做 alias 没用，底层仍然需要规则引擎决定到底映射到哪个内部组  
- 前端和 API 都要补“显示名 != 内部 key”的处理

**Confidence:** 72%
**Complexity:** Medium
**Status:** Unexplored

### 5. 给定价接口和后台加“有效计费预览”
**Description:**  
把 `/api/pricing`、用户分组信息、后台测试工具做成“按公开组 + 模型 → 有效计费结果”的展示。  
例如用户在 `ask-第三方渠道` 里看 `gpt-4o` 时，页面显示的是 GPT 内部结算倍率；看 `claude-3-7-sonnet` 时显示的是 Claude 第三方倍率。

**Rationale:**  
如果只改运行时扣费，不改展示层，用户/运营会看到“页面价格”和“实际扣费”不一致，后面一定会反复解释。

**Downsides:**  
- 属于配套工程，不能单独解决主问题  
- 需要把同一套规则复用到展示接口里，避免双份逻辑漂移

**Confidence:** 90%
**Complexity:** Medium
**Status:** Unexplored

### 6. 日志与对账分层：用户看到 public group，后台财务看到 billing group
**Description:**  
消费日志、使用记录、财务统计里区分两层：
- 用户视图继续展示 `public_group`
- 管理员/内部对账额外记录 `billing_group`

可以放到 `other`、内部字段，或后续单独字段里。

**Rationale:**  
这样既能满足“用户只感知两个组”，也能满足你内部核算 GPT / Claude 第三方 / Claude Code 三个成本桶。

**Downsides:**  
- 需要梳理哪些页面是用户可见，哪些页面是管理员可见  
- 如果未来要按 billing group 做复杂统计，最终可能还是要加正式字段

**Confidence:** 86%
**Complexity:** Medium
**Status:** Unexplored

## Rejection Summary

| # | Idea | Reason Rejected |
|---|------|-----------------|
| 1 | 纯 Nginx 前置分流 | Nginx 在 new-api 认证前，天然不知道 token 最终使用哪个 group，和当前需求不匹配 |
| 2 | 只用 `GroupGroupRatio` | 仍然是“用户组 → 使用组”的静态映射，不能把同一个公开组按模型拆成多个内部结算桶 |
| 3 | 只调模型倍率 | `GetModelRatio(name)` 是全局模型倍率，不带 group 维度，无法做到“只在这个公开组里生效” |
| 4 | 给用户发更多 key / 更多域名 | 技术上能做，但直接破坏“用户只看到两个组”的目标，体验变差 |
| 5 | 多个内部组共用同一个描述文案 | 前端和 API 仍然用内部 group key 作为真实值，不算真正隐藏，只会制造歧义 |

## Session Log

- 2026-04-22: Initial ideation — 11 candidates generated, 6 survived
