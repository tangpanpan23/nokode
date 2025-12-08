# nokode

**A web server with no application logic. Just an LLM with three tools.**

[中文版](README.zh.md) | [English](README.md)

## The Shower Thought

One day we won't need code. LLMs will output video at 120fps, sample inputs in realtime, and just... be our computers. No apps, no code, just intent and execution.

That's science fiction.

But I got curious: with a few hours this weekend and today's level of tech, how far can we get?

## The Hypothesis

I expected this to fail spectacularly.

Everyone's focused on AI that writes code. You know the usual suspects, Claude Code, Cursor, Copilot, all that. But that felt like missing the bigger picture. So I built something to test a different question: what if you skip code generation entirely? A web server with zero application code. No routes, no controllers, no business logic. Just an HTTP server that asks an LLM "what should I do?" for every request.

The goal: prove how far away we really are from that future.

## The Target

Contact manager. Basic CRUD: forms, database, list views, persistence.

Why? Because most software is just CRUD dressed up differently. If this works at all, it would be something.

## The Experiment

```go
// The entire backend (simplified)
result := callLLM(cfg, prompt, tools)
// Tools: database, webResponse, updateMemory
```

Three tools:
- **`database`** - Execute SQL on SQLite. AI designs the schema.
- **`webResponse`** - Return any HTTP response. AI generates the HTML, JavaScript, JSON or whatever fits.
- **`updateMemory`** - Persist feedback to markdown. AI reads it on next request.

The AI infers what to return from the path alone. Hit `/contacts` and you get an HTML page. Hit `/api/contacts` and you get JSON:

```json
// What the AI generates for /api/contacts
{
  "contacts": [
    { "id": 1, "name": "Alice", "email": "alice@example.com" },
    { "id": 2, "name": "Bob", "email": "bob@example.com" }
  ]
}
```

Every page has a feedback widget. Users type "make buttons bigger" or "use dark theme" and the AI implements it.

## The Results

It works. That's annoying.

Every click or form submission took 30-60 seconds. Traditional web apps respond in 10-100 milliseconds. That's 300-6000x slower. Each request cost $0.01-0.05 in API tokens—100-1000x more expensive than traditional compute. The AI spent 75-85% of its time reasoning, forgot what UI it generated 5 seconds ago, and when it hallucinated broken SQL that was an immediate 500 error. Colors drifted between requests. Layouts changed. I tried prompt engineering tricks like "⚡ THINK QUICKLY" and it made things slower because the model spent more time reasoning about how to be fast.

But despite all that, forms actually submitted correctly. Data persisted across restarts. The UI was usable. APIs returned valid JSON. User feedback got implemented. The AI invented, without any examples, sensible database schemas with proper types and indexes, parameterized SQL queries that were safe from injection, REST-ish API conventions, responsive Bootstrap layouts, form validation, and error handling for edge cases. All emergent behavior from giving it three tools and a prompt.

So yes, the capability exists. The AI can handle application logic. It's just catastrophically slow, absurdly expensive, and has the memory of a goldfish.

## Screenshots

<table>
  <tr>
    <td><img src="screenshots/1.png" alt="Fresh empty home" width="300"/></td>
    <td><img src="screenshots/2.png" alt="Filling out a contact form" width="300"/></td>
    <td><img src="screenshots/3.png" alt="Contact detail view" width="300"/></td>
  </tr>
  <tr>
    <td><img src="screenshots/4.png" alt="Home with three contacts" width="300"/></td>
    <td><img src="screenshots/5.png" alt="Another contact detail" width="300"/></td>
    <td><img src="screenshots/6.png" alt="Home with ten contacts" width="300"/></td>
  </tr>
  <tr>
    <td><img src="screenshots/7.png" alt="After deleting a contact" width="300"/></td>
    <td><img src="screenshots/8.png" alt="Home after delete" width="300"/></td>
    <td><img src="screenshots/9.png" alt="Evolved contact app" width="300"/></td>
  </tr>
</table>

## The Conclusion

The capability exists. The AI can handle application logic.

The problems are all performance: speed (300-6000x slower), cost (100-1000x more expensive), consistency (no design memory), reliability (hallucinations → errors).

But these feel like problems of degree, not kind:
- Inference: improving ~10x/year
- Cost: heading toward zero
- Context: growing (eventual design memory?)
- Errors: dropping

But the fact that I built a working CRUD app with zero application code, despite it being slow and expensive, suggests we might be closer to "AI just does the thing" than "AI helps write code."

In this project, what's left is infrastructure: HTTP setup, tool definitions, database connections. The application logic is gone. But the real vision? 120 inferences per second rendering displays with constant realtime input sampling. That becomes the computer. No HTTP servers, no databases, no infrastructure layer at all. Just intent and execution.

I think we don't realize how much code, as a thing, is mostly transitional.


---

## Installation

### Prerequisites

- Go 1.22 or later
- MySQL 5.7+ or MariaDB 10.3+

### Setup

1. Clone the repository:
```bash
git clone <repository-url>
cd nokode
```

2. Install dependencies:
```bash
go mod download
```

3. Create a MySQL database:
```sql
CREATE DATABASE nokode CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

4. Create a `.env` file:
```env
LLM_PROVIDER=qwen
QWEN_API_KEY=sk-...
QWEN_MODEL=qwen-turbo

# Or use other providers:
# LLM_PROVIDER=anthropic
# ANTHROPIC_API_KEY=sk-ant-...
# ANTHROPIC_MODEL=claude-3-haiku-20240307
# 
# LLM_PROVIDER=openai
# OPENAI_API_KEY=sk-...
# OPENAI_MODEL=gpt-4-turbo-preview

# Database configuration
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_password
DB_NAME=nokode

PORT=3001
```

5. Run the server:
```bash
go run main.go -f etc/nokode-api.yaml
```

Or build and run:
```bash
go build -o nokode
./nokode -f etc/nokode-api.yaml
```

Visit `http://localhost:3001`. First request: 30-60s.

## Project Structure

```
nokode/
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── etc/
│   └── nokode-api.yaml    # go-zero configuration file
├── internal/
│   ├── config/            # Configuration management
│   │   └── config.go
│   ├── handler/           # HTTP handlers
│   │   └── llm_handler.go # LLM request handler
│   ├── tools/             # LLM tools
│   │   ├── database.go    # SQLite database tool
│   │   ├── web_response.go # HTTP response tool
│   │   └── memory.go      # Memory persistence tool
│   └── utils/             # Utility functions
│       ├── logger.go      # Logging utility
│       ├── prompt_loader.go # Load prompt.md
│       └── memory_loader.go # Load memory.md
├── prompt.md              # LLM system prompt
├── prompt.zh.md           # LLM system prompt (Chinese)
└── memory.md              # User feedback memory (auto-generated)
```

## Features

- **Zero Application Code**: All application logic is handled by the LLM
- **Built with go-zero**: High-performance microservices framework
- **Multiple LLM Providers**: Supports Alibaba Cloud Qwen (Tongyi Qianwen), Anthropic Claude, and OpenAI GPT models
- **Three Simple Tools**: Database, web response, and memory persistence
- **Self-Evolving**: Users can provide feedback that shapes the application
- **Fast Startup**: Go's compiled nature provides quick server startup
- **Type Safe**: Go's static typing catches errors at compile time

## Usage

### Basic Usage

The server handles all HTTP requests through the LLM. Simply make requests:

```bash
# Get the home page
curl http://localhost:3001/

# Create a contact (POST request)
curl -X POST http://localhost:3001/contacts \
  -H "Content-Type: application/json" \
  -d '{"name":"Alice","email":"alice@example.com"}'

# Get API response
curl http://localhost:3001/api/contacts
```

### Customization

Edit `prompt.md` to change what application the LLM builds. The prompt defines the behavior, features, and style of the generated application.

**What to try:**

Out of the box it builds a contact manager. But try:
- `/game` - Maybe you get a game?
- `/dashboard` - Could be anything
- `/api/stats` - Might invent an API
- Type feedback: "make this purple" or "add a search box"

## Configuration

### Configuration File

The server uses go-zero's configuration system. Edit `etc/nokode-api.yaml`:

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

### Environment Variables

**Server:**
- `PORT` - Server port (default: 3001)

**LLM Provider:**
- `LLM_PROVIDER` - "qwen", "anthropic", or "openai" (default: qwen)
- `QWEN_API_KEY` or `DASHSCOPE_API_KEY` - Alibaba Cloud DashScope API key
- `QWEN_MODEL` - Qwen model name (default: qwen-turbo, options: qwen-plus, qwen-max, etc.)
- `ANTHROPIC_API_KEY` - Your Anthropic API key
- `ANTHROPIC_MODEL` - Anthropic model name (default: claude-3-haiku-20240307)
- `OPENAI_API_KEY` - Your OpenAI API key
- `OPENAI_MODEL` - OpenAI model name (default: gpt-4-turbo-preview)

**Database:**
- `DB_HOST` - MySQL host (default: localhost)
- `DB_PORT` - MySQL port (default: 3306)
- `DB_USER` - MySQL user (default: root)
- `DB_PASSWORD` - MySQL password (default: empty)
- `DB_NAME` - MySQL database name (default: nokode)

**Debug:**
- `DEBUG` - Set to "true" for debug logging

## Performance

The Go implementation with go-zero provides:
- **Faster startup**: Compiled binary starts instantly
- **Lower memory usage**: Go's efficient runtime
- **Better concurrency**: Native goroutine support for handling multiple requests
- **Production ready**: go-zero provides built-in monitoring, tracing, and service discovery

However, the LLM processing time remains the same (30-60 seconds per request) as it depends on the API provider, not the server implementation.

## Development

### Building

```bash
# Build for current platform
go build -o nokode

# Build for Linux
GOOS=linux GOARCH=amd64 go build -o nokode-linux

# Build for macOS
GOOS=darwin GOARCH=amd64 go build -o nokode-macos

# Build for Windows
GOOS=windows GOARCH=amd64 go build -o nokode.exe
```

### Running

```bash
# Development mode
go run main.go -f etc/nokode-api.yaml

# Production mode
./nokode -f etc/nokode-api.yaml
```

## About go-zero

This project uses [go-zero](https://go-zero.dev), a web and rpc framework with lots of built-in engineering practices. It's designed to simplify the development of microservices and provides:

- Built-in service discovery, load balancing, tracing, monitoring, and more
- High performance with minimal overhead
- Simple API definition and code generation
- Production-ready features out of the box

## Troubleshooting

### Database Issues

**Connection Errors:**
- Make sure MySQL is running: `mysql -u root -p`
- Verify database exists: `SHOW DATABASES;`
- Check credentials in `.env` or `etc/nokode-api.yaml`

**Schema Issues:**
- The AI will create tables automatically on first use
- If you need to reset, drop and recreate the database:
  ```sql
  DROP DATABASE nokode;
  CREATE DATABASE nokode CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
  ```

### API Key Issues

Make sure your `.env` file contains valid API keys for your chosen provider.

### Port Already in Use

Change the `PORT` environment variable or edit `etc/nokode-api.yaml` to use a different port.

### Configuration File Not Found

Make sure the `-f` flag points to the correct configuration file path, or create `etc/nokode-api.yaml` if it doesn't exist.

⚠️ **Cost warning**: Each request costs $0.001-0.05 depending on model. Budget accordingly.

MIT License
