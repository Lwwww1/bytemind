# ByteMind

一个用 Go 实现的 AI Coding CLI，目标是提供更接近 OpenCode / ClaudeCode 的工作流能力。

当前版本已经具备：

- 多轮会话与会话持久化
- 纯 CLI 聊天交互
- 流式终端输出
- OpenAI-compatible 与 Anthropic 双 provider 适配层
- 工具调用循环
- 工作区文件浏览、读取、搜索、写入、精确替换
- Shell 命令执行与审批策略
- `-max-iterations` 执行预算覆盖
- 到达预算时自动返回阶段性总结，而不是直接报错
- 简单的重复工具调用检测，避免死循环

## 目录结构

```text
cmd/bytemind            CLI 入口
internal/agent          对话循环、系统提示词模板、流式输出
internal/config         配置加载与环境变量覆盖
internal/llm            通用消息与工具类型
internal/provider       多 provider 适配层
internal/session        会话持久化
internal/tools          文件工具、patch 工具、shell 工具
```

## 快速开始

先按下方“配置文件”章节准备好 `config.json`，再在仓库根目录运行：

聊天模式：

```powershell
go run ./cmd/bytemind chat
```

单次任务：

```powershell
go run ./cmd/bytemind run -prompt "分析当前项目并生成改进建议"
```

需要更大的执行预算时：

```powershell
go run ./cmd/bytemind chat -max-iterations 64
go run ./cmd/bytemind run -prompt "refactor this module" -max-iterations 64
```

## 首次运行自动配置

新环境首次执行 `go run ./cmd/bytemind chat` 或 `go run ./cmd/bytemind tui` 时，程序会先检查 API 配置。

- 如果已存在可用配置：直接进入程序。
- 如果缺少配置：会提示并按 OpenAI-compatible 格式输入三项：

```text
url:
key:
model:
```

输入完成后会自动写入配置文件：

- 指定了 `-config`：写入该路径。
- 未指定 `-config`：写入当前 workspace 的 `config.json`。

## 配置文件

在工作区根目录下寻找配置文件 `config.json`，直接从仓库根目录复制示例模板开始：

```powershell
Copy-Item config.example.json config.json
```

然后把 `api_key` 等字段改成你自己的配置。

配置示例：

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4-mini",
    "api_key": "your-api-key-here"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "stream": true
}
```

Anthropic 示例：

```json
{
  "provider": {
    "type": "anthropic",
    "base_url": "https://api.anthropic.com",
    "model": "claude-sonnet-4-20250514",
    "api_key": "your-api-key-here",
    "anthropic_version": "2023-06-01"
  }
}
```
安全行为：

- `config.json` 不再保存明文 `api_key`。
- 配置会保存 `provider.api_key_env`（默认 `BYTEMIND_API_KEY`）。
- 输入的 key 只注入当前进程环境变量，用于本次启动。
- 检测到旧配置中有明文 `provider.api_key` 时，会自动迁移并移除明文。


## 交互命令

- `/help`
- `/session`
- `/sessions`
- `/quit`

## 已实现工具

- `list_files`
- `read_file`
- `search_text`
- `write_file`
- `replace_in_file`
- `apply_patch`
- `run_shell`

## 系统提示词维护

系统提示词已抽离为独立模板文档维护：

- `internal/agent/prompts/system.md`

运行时由 `internal/agent/prompt.go` 通过 `go:embed` 内嵌 Markdown 文档，并替换 `{{WORKSPACE}}`、`{{APPROVAL_POLICY}}` 占位符，因此修改提示词时不需要再直接编辑 Go 字符串常量。

## 开源

本项目采用MIT开源协议。