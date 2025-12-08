# nokode

**一个没有应用逻辑的 Web 服务器。只有一个 LLM 和三个工具。**

[English](README.md) | [中文版](README.zh.md)

## 灵光一现

有一天，我们将不再需要代码。LLM 将以 120fps 输出视频，实时采样输入，然后... 成为我们的计算机。没有应用，没有代码，只有意图和执行。

那是科幻小说。

但我很好奇：用这个周末的几个小时和今天的技术水平，我们能走多远？

## 假设

我原本以为这会彻底失败。

每个人都在关注编写代码的 AI。你知道那些常见的工具，Claude Code、Cursor、Copilot 等等。但这感觉像是错过了更大的图景。所以我构建了一些东西来测试一个不同的问题：如果你完全跳过代码生成会怎样？一个零应用代码的 Web 服务器。没有路由，没有控制器，没有业务逻辑。只是一个 HTTP 服务器，对每个请求都询问 LLM"我应该做什么？"。

目标：证明我们离那个未来还有多远。

## 目标

联系人管理器。基本的 CRUD：表单、数据库、列表视图、持久化。

为什么？因为大多数软件只是换了个样子的 CRUD。如果这能工作，那就会是某种成就。

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

AI 仅从路径推断要返回什么。访问 `/contacts` 你会得到一个 HTML 页面。访问 `/api/contacts` 你会得到 JSON：

```json
// AI 为 /api/contacts 生成的内容
{
  "contacts": [
    { "id": 1, "name": "Alice", "email": "alice@example.com" },
    { "id": 2, "name": "Bob", "email": "bob@example.com" }
  ]
}
```

每个页面都有一个反馈小部件。用户输入"让按钮更大"或"使用深色主题"，AI 就会实现它。

## 结果

它有效。这很烦人。

每次点击或表单提交需要 30-60 秒。传统的 Web 应用在 10-100 毫秒内响应。这慢了 300-6000 倍。每个请求在 API token 上花费 $0.01-0.05——比传统计算贵 100-1000 倍。AI 花费 75-85% 的时间进行推理，忘记了 5 秒前生成的 UI，当它产生错误的 SQL 时，立即出现 500 错误。颜色在请求之间漂移。布局改变。我尝试了提示工程技巧，比如"⚡ 快速思考"，但它让事情变得更慢，因为模型花了更多时间思考如何快速。

但尽管如此，表单实际上正确提交了。数据在重启后持续存在。UI 可用。API 返回有效的 JSON。用户反馈得到了实现。AI 在没有示例的情况下发明了合理的数据库模式，具有适当的类型和索引，参数化的 SQL 查询免受注入攻击，REST 风格的 API 约定，响应式 Bootstrap 布局，表单验证，以及边缘情况的错误处理。所有这些都是从给它三个工具和一个提示中涌现的行为。

所以是的，能力存在。AI 可以处理应用逻辑。它只是灾难性地慢，荒谬地昂贵，并且有金鱼般的记忆。

## 截图

<table>
  <tr>
    <td><img src="screenshots/1.png" alt="全新的空主页" width="300"/></td>
    <td><img src="screenshots/2.png" alt="填写联系人表单" width="300"/></td>
    <td><img src="screenshots/3.png" alt="联系人详情视图" width="300"/></td>
  </tr>
  <tr>
    <td><img src="screenshots/4.png" alt="有三个联系人的主页" width="300"/></td>
    <td><img src="screenshots/5.png" alt="另一个联系人详情" width="300"/></td>
    <td><img src="screenshots/6.png" alt="有十个联系人的主页" width="300"/></td>
  </tr>
  <tr>
    <td><img src="screenshots/7.png" alt="删除联系人后" width="300"/></td>
    <td><img src="screenshots/8.png" alt="删除后的主页" width="300"/></td>
    <td><img src="screenshots/9.png" alt="进化后的联系人应用" width="300"/></td>
  </tr>
</table>

## 结论

能力存在。AI 可以处理应用逻辑。

问题都在性能方面：速度（慢 300-6000 倍）、成本（贵 100-1000 倍）、一致性（没有设计记忆）、可靠性（幻觉 → 错误）。

但这些感觉像是程度问题，而不是类型问题：
- 推理：每年改进约 10 倍
- 成本：趋向于零
- 上下文：增长（最终的设计记忆？）
- 错误：下降

但事实上，我构建了一个零应用代码的工作 CRUD 应用，尽管它很慢且昂贵，这表明我们可能更接近"AI 只是做这件事"而不是"AI 帮助编写代码"。

在这个项目中，剩下的是基础设施：HTTP 设置、工具定义、数据库连接。应用逻辑消失了。但真正的愿景？每秒 120 次推理，以恒定的实时输入采样渲染显示。那成为计算机。没有 HTTP 服务器，没有数据库，根本没有基础设施层。只有意图和执行。

我认为我们没有意识到代码，作为一个东西，主要是过渡性的。


---

## 安装

### 前置要求

- Go 1.22 或更高版本
- MySQL 5.7+ 或 MariaDB 10.3+

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

3. 创建 MySQL 数据库：
```sql
CREATE DATABASE nokode CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

4. 创建 `.env` 文件：
```env
LLM_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-...
ANTHROPIC_MODEL=claude-3-haiku-20240307

# 或使用 OpenAI:
# LLM_PROVIDER=openai
# OPENAI_API_KEY=sk-...
# OPENAI_MODEL=gpt-4-turbo-preview

# 数据库配置
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password
DB_NAME=nokode

PORT=3001
```

5. 运行服务器：
```bash
go run main.go -f etc/nokode-api.yaml
```

或编译后运行：
```bash
go build -o nokode
./nokode -f etc/nokode-api.yaml
```

访问 `http://localhost:3001`。首次请求：30-60 秒。

## 项目结构

```
nokode/
├── main.go                 # 应用程序入口点
├── go.mod                  # Go 模块定义
├── etc/
│   └── nokode-api.yaml    # go-zero 配置文件
├── internal/
│   ├── config/            # 配置管理
│   │   └── config.go
│   ├── handler/           # HTTP 处理器
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
└── prompt.zh.md           # LLM 系统提示（中文）
```

## 特性

- **零应用代码**：所有应用逻辑由 LLM 处理
- **基于 go-zero**：高性能微服务框架
- **多 LLM 提供商**：支持 Anthropic Claude 和 OpenAI GPT 模型
- **三个简单工具**：数据库、Web 响应和内存持久化
- **自我演化**：用户可以提供反馈来塑造应用程序
- **快速启动**：Go 的编译特性提供快速服务器启动
- **类型安全**：Go 的静态类型在编译时捕获错误

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

**可以尝试的：**

开箱即用，它构建一个联系人管理器。但可以尝试：
- `/game` - 也许你会得到一个游戏？
- `/dashboard` - 可能是任何东西
- `/api/stats` - 可能会发明一个 API
- 输入反馈："把这个变成紫色"或"添加一个搜索框"

## 配置

### 配置文件

服务器使用 go-zero 的配置系统。编辑 `etc/nokode-api.yaml`：

```yaml
Name: nokode-api
Host: 0.0.0.0
Port: 3001
Timeout: 300000
MaxConns: 1000
MaxBytes: 1048576

Database:
  Host: localhost
  Port: 3306
  User: root
  Password: ""
  Database: nokode
```

### 环境变量

**服务器:**
- `PORT` - 服务器端口（默认：3001）

**LLM 提供商:**
- `LLM_PROVIDER` - "anthropic" 或 "openai"（默认：anthropic）
- `ANTHROPIC_API_KEY` - 你的 Anthropic API 密钥
- `ANTHROPIC_MODEL` - Anthropic 模型名称（默认：claude-3-haiku-20240307）
- `OPENAI_API_KEY` - 你的 OpenAI API 密钥
- `OPENAI_MODEL` - OpenAI 模型名称（默认：gpt-4-turbo-preview）

**数据库:**
- `DB_HOST` - MySQL 主机（默认：localhost）
- `DB_PORT` - MySQL 端口（默认：3306）
- `DB_USER` - MySQL 用户（默认：root）
- `DB_PASSWORD` - MySQL 密码（默认：空）
- `DB_NAME` - MySQL 数据库名称（默认：nokode）

**调试:**
- `DEBUG` - 设置为 "true" 以启用调试日志

## 性能

Go 实现配合 go-zero 提供：
- **更快的启动**：编译的二进制文件立即启动
- **更低的内存使用**：Go 的高效运行时
- **更好的并发性**：原生 goroutine 支持处理多个请求
- **生产就绪**：go-zero 提供内置监控、追踪和服务发现

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

### 运行

```bash
# 开发模式
go run main.go -f etc/nokode-api.yaml

# 生产模式
./nokode -f etc/nokode-api.yaml
```

## 关于 go-zero

本项目使用 [go-zero](https://go-zero.dev)，一个内置大量工程实践的 web 和 rpc 框架。它旨在简化微服务的开发，并提供：

- 内置服务发现、负载均衡、追踪、监控等
- 高性能，开销最小
- 简单的 API 定义和代码生成
- 开箱即用的生产就绪功能

## 故障排除

### 数据库问题

**连接错误:**
- 确保 MySQL 正在运行：`mysql -u root -p`
- 验证数据库是否存在：`SHOW DATABASES;`
- 检查 `.env` 或 `etc/nokode-api.yaml` 中的凭据

**模式问题:**
- AI 会在首次使用时自动创建表
- 如果需要重置，删除并重新创建数据库：
  ```sql
  DROP DATABASE nokode;
  CREATE DATABASE nokode CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
  ```

### API 密钥问题

确保你的 `.env` 文件包含所选提供商的有效 API 密钥。

### 端口已被使用

更改 `PORT` 环境变量或编辑 `etc/nokode-api.yaml` 以使用不同的端口。

### 配置文件未找到

确保 `-f` 标志指向正确的配置文件路径，或者如果不存在则创建 `etc/nokode-api.yaml`。

⚠️ **成本警告**：每个请求根据模型花费 $0.001-0.05。请相应预算。

MIT License
