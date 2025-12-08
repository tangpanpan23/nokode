# nokode (Go 语言实现)

**一个没有应用逻辑的 Web 服务器。只有一个 LLM 和三个工具。**

[English](README-GO.md) | [中文版](README-GO.zh.md)

## 概述

这是 nokode 的 Go 语言实现 - 一个 Web 服务器，其中每个 HTTP 请求都由一个带有三个简单工具的 LLM 处理：database、webResponse 和 updateMemory。

## 实验

```go
// 整个后端（简化版）
result := callLLM(cfg, prompt, tools)
// 工具: database, webResponse, updateMemory
```

三个工具：
- **`database`** - 在 SQLite 上执行 SQL。AI 设计模式。
- **`webResponse`** - 返回任何 HTTP 响应。AI 生成 HTML、JavaScript、JSON 或任何合适的内容。
- **`updateMemory`** - 将反馈持久化到 markdown。AI 在下次请求时读取它。

AI 仅从路径推断要返回什么。访问 `/contacts` 你会得到一个 HTML 页面。访问 `/api/contacts` 你会得到 JSON。

## 安装

### 前置要求

- Go 1.21 或更高版本
- SQLite3（通常系统自带）

### 设置

1. 克隆仓库：
```bash
git clone <repository-url>
cd nokode
```

2. 安装依赖：
```bash
go mod download
```

3. 创建 `.env` 文件：
```env
LLM_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-...
ANTHROPIC_MODEL=claude-3-haiku-20240307

# 或使用 OpenAI:
# LLM_PROVIDER=openai
# OPENAI_API_KEY=sk-...
# OPENAI_MODEL=gpt-4-turbo-preview

PORT=3001
```

4. 运行服务器：
```bash
go run main.go
```

或编译后运行：
```bash
go build -o nokode
./nokode
```

访问 `http://localhost:3001`。首次请求：30-60 秒。

## 项目结构

```
nokode/
├── main.go                 # 应用程序入口点
├── go.mod                  # Go 模块定义
├── internal/
│   ├── config/            # 配置管理
│   │   └── config.go
│   ├── middleware/        # HTTP 中间件
│   │   └── llm_handler.go # LLM 请求处理器
│   ├── tools/             # LLM 工具
│   │   ├── database.go    # SQLite 数据库工具
│   │   ├── web_response.go # HTTP 响应工具
│   │   └── memory.go      # 内存持久化工具
│   └── utils/             # 工具函数
│       ├── logger.go      # 日志工具
│       ├── prompt_loader.go # 加载 prompt.md
│       └── memory_loader.go # 加载 memory.md
├── prompt.md              # LLM 系统提示
├── memory.md              # 用户反馈内存（自动生成）
└── database.db            # SQLite 数据库（自动生成）
```

## 特性

- **零应用代码**：所有应用逻辑由 LLM 处理
- **多 LLM 提供商**：支持 Anthropic Claude 和 OpenAI GPT 模型
- **三个简单工具**：数据库、Web 响应和内存持久化
- **自我演化**：用户可以提供反馈来塑造应用程序
- **快速启动**：Go 的编译特性提供快速服务器启动

## 使用方法

### 基本使用

服务器通过 LLM 处理所有 HTTP 请求。只需发送请求：

```bash
# 获取主页
curl http://localhost:3001/

# 创建联系人（POST 请求）
curl -X POST http://localhost:3001/contacts \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","email":"alice@example.com"}'

# 获取 API 响应
curl http://localhost:3001/api/contacts
```

### 自定义

编辑 `prompt.md` 来更改 LLM 构建的应用程序。提示定义了生成应用程序的行为、功能和样式。

### 可以尝试的

开箱即用，它构建一个联系人管理器。但可以尝试：
- `/game` - 也许你会得到一个游戏？
- `/dashboard` - 可能是任何东西
- `/api/stats` - 可能会发明一个 API
- 输入反馈："把这个变成紫色"或"添加一个搜索框"

## 配置

### 环境变量

- `PORT` - 服务器端口（默认：3001）
- `LLM_PROVIDER` - "anthropic" 或 "openai"（默认：anthropic）
- `ANTHROPIC_API_KEY` - 你的 Anthropic API 密钥
- `ANTHROPIC_MODEL` - Anthropic 模型名称（默认：claude-3-haiku-20240307）
- `OPENAI_API_KEY` - 你的 OpenAI API 密钥
- `OPENAI_MODEL` - OpenAI 模型名称（默认：gpt-4-turbo-preview）
- `DEBUG` - 设置为 "true" 以启用调试日志

## 性能

Go 实现提供：
- **更快的启动**：编译的二进制文件立即启动
- **更低的内存使用**：Go 的高效运行时
- **更好的并发性**：原生 goroutine 支持处理多个请求

但是，LLM 处理时间保持不变（每个请求 30-60 秒），因为它取决于 API 提供商，而不是服务器实现。

## 开发

### 构建

```bash
# 为当前平台构建
go build -o nokode

# 为 Linux 构建
GOOS=linux GOARCH=amd64 go build -o nokode-linux

# 为 macOS 构建
GOOS=darwin GOARCH=amd64 go build -o nokode-macos

# 为 Windows 构建
GOOS=windows GOARCH=amd64 go build -o nokode.exe
```

### 测试

```bash
# 运行测试（实现后）
go test ./...

# 使用竞态检测器运行
go test -race ./...
```

## 与 Node.js 版本的区别

1. **编译的二进制文件**：Go 生成单个可执行文件
2. **类型安全**：Go 的静态类型在编译时捕获错误
3. **并发性**：原生 goroutine 用于更好的并发请求处理
4. **内存**：更高效的内存使用
5. **依赖项**：更少的运行时依赖（只有二进制文件）

## 故障排除

### 数据库问题

如果遇到数据库错误，删除 `database.db` 并重启服务器。LLM 将重新创建模式。

### API 密钥问题

确保你的 `.env` 文件包含所选提供商的有效 API 密钥。

### 端口已被使用

更改 `PORT` 环境变量或终止使用端口 3001 的进程。

## 许可证

MIT License

## 贡献

欢迎贡献！请随时提交 Pull Request。

