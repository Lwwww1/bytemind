---
name: review
description: |
  对当前代码改动做审查，聚焦正确性、回归风险和测试缺口。
when_to_use: 用户要求 code review、pre-merge review、风险评估时。
---

# review

## Workflow

1. 明确审查范围（分支、目录、文件）。
2. 先看行为变化，再看实现细节。
3. 按严重级别输出问题，必须包含定位信息。
4. 单独列出测试缺口和未验证假设。

## Must Check

- correctness 与边界条件
- 潜在回归与兼容性
- 错误处理与异常路径
- 测试覆盖是否匹配变更

## Output Contract

- Findings: 严重度排序，给出文件与原因
- Risks: 非阻断但需要关注的点
- Verification: 已执行与建议执行的验证

