# ForgeCLI Agent MVP

一个使用 Go 编写的小型 Coding Agent MVP，用来模拟 OpenCode、Claude Code 这类工具优先型产品的核心工作循环。

- 支持配置兼容 OpenAI 的模型接口
- 在内存中保留多轮对话上下文
- 向模型暴露本地编码工具
- 在写文件或执行命令前请求用户授权
- 支持简单的 HTML 生成功能，可将可运行页面直接写入磁盘

## MVP 包含的能力

- `forgecli chat`：带上下文记忆的交互式 REPL 会话
- 对话模式：
  - `analyze`：只读分析模式，支持 `list_files`、`read_file`、`search`
  - `full`：在此基础上增加 `write_file` 和白名单内的 `run_command`
- `forgecli generate`：生成一个自包含的 HTML 页面并保存到本地
  - 在已配置模型时可调用模型生成
  - 对于“番茄钟页面”这类请求，在无模型时可回退到内置本地模板
- `forgecli run`：用于执行一次单任务闭环代码修改
- 兼容 OpenAI `/chat/completions` 接口的客户端
- 工作区边界检查，以及基础危险命令拒绝列表

## 快速开始

直接生成一个番茄钟网页并保存到本地文件：

```powershell
go run ./cmd/forgecli generate --prompt '我想做一个番茄钟网页' --output pomodoro.html
```

以只读分析模式启动：

```powershell
go run ./cmd/forgecli chat --repo . --config forgecli.json --mode analyze
```

如果需要编辑文件或执行验证命令，可以切换到 `full` 模式：

```powershell
go run ./cmd/forgecli chat --repo . --config forgecli.json --mode full
```

## 对话命令

- `/help`
- `/tools`
- `/reset`
- `/exit`

## 说明

- 对话历史仅保存在当前会话的内存中
- `write_file` 需要传入完整文件内容，而不是补丁
- `generate` 会写出一个完整的 `.html` 文件，可直接在浏览器中打开
- 默认会在仓库扫描时忽略 `.gocache` 和 `.gotmp`
