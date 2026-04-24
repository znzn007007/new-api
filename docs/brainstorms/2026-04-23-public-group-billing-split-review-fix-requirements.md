---
date: 2026-04-23
topic: public-group-billing-split-review-fix
---

# Public Group Billing Split Review Fix Requirements

## Problem Frame

`public-group-billing-split` 当前实现把“标签命中”直接带入了渠道选择收窄逻辑，导致两个直接可见的可用性回归：

- 对同一个 `public_group + model`，如果存在多个非空标签且没有显式 override，系统直接报错；
- 对同一个 `public_group + model`，如果自动推导出唯一标签，系统会排除无标签兜底渠道，导致旧有 fallback 能力消失。

在当前运行约束下：

- `RetryTimes = 0`
- 不使用 `auto` 分组
- 不需要支持免费通道 / `0` 倍率免费路由

因此本次修复只聚焦**请求可用性**与**旧有 fallback 行为**，不把 retry 重新计费或免费倍率支持纳入当前范围。

## Goal

在保留当前 public-group billing split 主体能力的前提下，修复标签解析过度收窄路由范围的问题，使新逻辑不再把原来能成功的请求变成失败。

## Non-Goals

- N1. 本次不要求处理 `RetryTimes > 0` 时的重试重定价一致性。
- N2. 本次不要求支持实时 / WebSocket 免费倍率 (`GroupRatio == 0`) 语义。
- N3. 本次不重构整个 public group billing split 架构。
- N4. 本次不要求新增用户可见配置概念；优先沿用现有 public group / channel tag / override 机制。
- N5. 本次不调整个别客户的特殊计费口径说明方式；只修路由回归。

## Recommended Direction

采用“**显式 override 是强约束，自动标签推导是弱约束**”的修复方向：

- 显式 override 仍然可以硬锁标签；
- 自动推导出的标签只能作为优先候选，不能把无标签兜底渠道排除掉；
- 多标签歧义默认回退到原有 group/model 选择链路，而不是直接报错。

这意味着：

- **显式 override** = 强约束（可以硬锁标签）
- **自动推导标签** = 弱约束（只影响优先顺序）
- **多标签歧义** = 不收窄、不报错、走回退链路

## Requirements

### A. 标签解析行为

- R1. 当 `public_group + model` 存在多个非空标签，且未配置显式 override 时，系统不得直接报错。
- R2. 上述多标签歧义场景下，系统必须回退为：
  - 不锁定路由标签；
  - 继续使用原有 group/model 级渠道选择能力。
- R3. 当存在显式 `public_group + model -> tag` override 时，该 override 仍然有效，并继续作为最高优先级规则。
- R4. 只有当显式 override 指向的标签当前不存在于可用标签集合中时，系统才允许报错。

### B. 渠道选择行为

- R5. 自动推导出唯一标签时，系统不得把该标签解释为唯一合法渠道池。
- R6. 自动推导出唯一标签时，系统应优先尝试该标签对应渠道池；若当前层级下无可用渠道，必须允许回退到同组同模型下的无标签/通用渠道池。
- R7. 无显式 override 时，系统不得因为自动解析结果而永久排除无标签兜底渠道。
- R8. 显式 override 命中时，系统可以继续使用严格标签池选择，不要求保留无标签 fallback。

### C. 兼容性与回归保护

- R9. 本次修复后，旧有“同组同模型下有可用渠道就尽量成功”的行为必须恢复。
- R10. 本次修复不得要求运营方为所有多标签模型补齐 override 才能恢复可用性。
- R11. 本次必须补充回归测试覆盖以下场景：
  - 多标签 + 无 override -> 不报错，并走回退链路；
  - 唯一自动标签 + 存在无标签兜底渠道 -> 兜底渠道仍可被使用；
  - 显式 override -> 继续严格锁标签；
  - override 指向不存在标签 -> 明确报错。

## Success Criteria

- 多标签且无 override 的请求不再因为标签歧义直接失败。
- 自动命中单标签时，无标签兜底渠道仍可作为后备路径使用。
- 显式 override 仍然保持强约束能力。
- public-group billing split 的现有主体逻辑保留，但不再破坏旧有路由可用性。
- 新增测试能够稳定防止这两类回归再次出现。

## Scope Boundaries

- 本次修复只聚焦评审问题 1 和 2。
- 评审问题 3（retry 期间重新计费）记录为后续修复项，但不阻塞本次交付。
- 评审问题 4（免费倍率 `0` 语义）由于当前业务不使用免费通道，不纳入本次实现范围。

## Key Decisions

- 将“自动标签解析”定位为**弱约束**，恢复 fallback 优先于保持当前严格行为。
- 将“显式 override”定位为**强约束**，保留运营可控性。
- 多标签歧义默认回退而不是报错，优先保证请求成功率。
- 本次按最小风险修复，不顺带引入 retry / 免费倍率的扩展改造。

## Dependencies / Assumptions

- 同组同模型下的无标签渠道仍然是业务上允许的合法兜底路径。
- 显式 override 是运营侧刻意配置的强规则，允许比自动推导更强。
- 现有 channel cache / ability 数据足以区分 tagged pool 与 untagged pool。

## Deferred to Later Planning

- [Post-fix] 若未来启用 `RetryTimes > 0`，需补做 retry 期间价格/预扣费一致性修复。
- [Post-fix] 若未来引入免费通道或 `0` 倍率规则，需补做 `GroupRatio == 0` 的显式状态建模。
- [Optional] 是否引入更明确的“route_tag_mode”结构化对象，以替代当前的隐式上下文字段。

## Next Steps

-> /ce:plan for implementation planning focused on issues 1 and 2
