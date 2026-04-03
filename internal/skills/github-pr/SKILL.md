---
name: github-pr
description: |
  面向 PR 评审上下文，优先梳理 diff、评论和潜在回归风险。
when_to_use: 用户要求分析 PR、处理 review comments、评估合并风险。
---

# github-pr

## Workflow

1. 明确比较基线（base ref / feature ref）。
2. 汇总主要改动文件与模块。
3. 标注风险点（行为变化、边界条件、兼容性）。
4. 汇总 review comments 对应的处理建议。
5. 给出合并前验证清单。

## Output Contract

- Diff Summary
- Key Risks
- Review Comment Response Plan
- Pre-merge Verification Checklist

