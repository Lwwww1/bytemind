# AICoding

一个用 Go 实现的 AI Coding CLI，目标是提供更接近 OpenCode / ClaudeCode 的工作流能力。

当前版本已经具备：

- 多轮会话与会话持久化
- 纯 CLI 聊天交互
- 流式终端输出
- OpenAI-compatible 与 Anthropic 双 provider 适配层
- 工具调用循环
- 工作区文件浏览、读取、搜索、写入、精确替换
- `apply_patch` 补丁编辑器
- `update_plan` 任务计划器
- Shell 命令执行与审批策略
- `-workspace` 跨目录工作区切换
- `-max-iterations` 执行预算覆盖
- 到达预算时自动返回阶段性总结，而不是直接报错
- 简单的重复工具调用检测，避免死循环

## 目录结构

```text
cmd/aicoding            CLI 入口
internal/agent          对话循环、系统提示词、流式输出
internal/config         配置加载与环境变量覆盖
internal/llm            通用消息与工具类型
internal/provider       多 provider 适配层
internal/session        会话和计划持久化
internal/tools          文件工具、patch 工具、计划工具、shell 工具
scripts                 安装与卸载脚本
```

## 安装

推荐在仓库根目录执行：

```powershell
.\scripts\install.ps1
```

或者：

```cmd
scripts\install.cmd
```

安装脚本会：

- 构建 `aicoding.exe`
- 安装到 `%LOCALAPPDATA%\Programs\AICoding\bin`
- 自动把该目录加入当前用户的 `PATH`

安装完成后，打开一个新的终端，就可以在任意目录直接执行：

```powershell
aicoding chat
aicoding run -prompt "analyze this repo"
aicoding chat -workspace E:\experiments
```

卸载：

```powershell
.\scripts\uninstall.ps1
```

## 快速开始

如果你暂时不想安装，也可以直接在仓库根目录运行：

```powershell
$env:AICODING_API_KEY = "your-api-key"
$env:AICODING_MODEL = "gpt-4.1-mini"
go run ./cmd/aicoding chat
```

聊天模式：

```powershell
aicoding chat
```

单次任务：

```powershell
go run ./cmd/aicoding run -prompt "分析当前项目并生成改进建议"
```

需要更大的执行预算时：

```powershell
aicoding chat -max-iterations 64
aicoding run -prompt "refactor this module" -max-iterations 64
```

## 跨目录运行

`go run ./cmd/aicoding ...` 只能在本仓库模块根目录内执行，因为 Go 需要从当前目录向上找到 `go.mod`。

如果你还没安装，但想在别的目录里启动并让程序作用在那个目录，可以这样：

```powershell
go -C E:\AICoding run ./cmd/aicoding chat -workspace E:\experiments
```

如果已经安装，则更简单：

```powershell
aicoding chat -workspace E:\experiments
```

## 配置文件

支持工作区 `.aicoding/config.json` 或用户目录 `.aicoding/config.json`：

```json
{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-4.1-mini",
    "api_key_env": "AICODING_API_KEY"
  },
  "approval_policy": "on-request",
  "max_iterations": 32,
  "session_dir": ".aicoding/sessions",
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
    "api_key_env": "AICODING_API_KEY",
    "anthropic_version": "2023-06-01"
  }
}
```

## 交互命令

- `/help`
- `/session`
- `/sessions`
- `/plan`
- `/exit`

## 已实现工具

- `list_files`
- `read_file`
- `search_text`
- `write_file`
- `replace_in_file`
- `apply_patch`
- `update_plan`
- `run_shell`

## apply_patch 改进

当前 `apply_patch` 已经新增：

- hunk header 行号与计数校验
- 有 header 时按期望 old line 精确定位
- 重复代码块只有在 header 明确指向时才允许命中
- fuzz test 覆盖 header 驱动的替换路径

## 下一步建议

- 增加更好的 diff 预览和撤销机制
- 增加真正的 Anthropic SSE 流式支持
- 增加任务检查点和自动恢复
