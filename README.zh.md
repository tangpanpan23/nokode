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

```javascript
// 整个后端
const result = await generateText({
  model,
  tools: {
    database,      // 执行 SQL 查询
    webResponse,   // 返回 HTML/JSON
    updateMemory   // 保存用户反馈
  },
  prompt: `处理这个 HTTP 请求: ${method} ${path}`,
});
```

三个工具：
- **`database`** - 在 SQLite 上执行 SQL。AI 设计模式。
- **`webResponse`** - 返回任何 HTTP 响应。AI 生成 HTML、JavaScript、JSON 或任何合适的内容。
- **`updateMemory`** - 将反馈持久化到 markdown。AI 在下次请求时读取它。

AI 仅从路径推断要返回什么。访问 `/contacts` 你会得到一个 HTML 页面。访问 `/api/contacts` 你会得到 JSON：

```javascript
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

```bash
npm install
```

`.env`:
```env
LLM_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-...
ANTHROPIC_MODEL=claude-3-haiku-20240307
```

```bash
npm start
```

访问 `http://localhost:3001`。首次请求：30-60 秒。

**可以尝试的：**

查看 `prompt.md` 并自定义它。更改它构建的应用，添加功能，修改行为。这就是整个界面。

开箱即用，它构建一个联系人管理器。但可以尝试：
- `/game` - 也许你会得到一个游戏？
- `/dashboard` - 可能是任何东西
- `/api/stats` - 可能会发明一个 API
- 输入反馈："把这个变成紫色"或"添加一个搜索框"

⚠️ **成本警告**：每个请求根据模型花费 $0.001-0.05。请相应预算。

MIT License

