# nokode (Go Implementation)

**A web server with no application logic. Just an LLM with three tools.**

[中文版](README-GO.zh.md) | [English](README-GO.md)

## Overview

This is the Go language implementation of nokode - a web server where every HTTP request is handled by an LLM with three simple tools: database, webResponse, and updateMemory.

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

The AI infers what to return from the path alone. Hit `/contacts` and you get an HTML page. Hit `/api/contacts` and you get JSON.

## Installation

### Prerequisites

- Go 1.21 or later
- SQLite3 (usually comes with the system)

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

3. Create a `.env` file:
```env
LLM_PROVIDER=anthropic
ANTHROPIC_API_KEY=sk-ant-...
ANTHROPIC_MODEL=claude-3-haiku-20240307

# Or use OpenAI:
# LLM_PROVIDER=openai
# OPENAI_API_KEY=sk-...
# OPENAI_MODEL=gpt-4-turbo-preview

PORT=3001
```

4. Run the server:
```bash
go run main.go
```

Or build and run:
```bash
go build -o nokode
./nokode
```

Visit `http://localhost:3001`. First request: 30-60s.

## Project Structure

```
nokode/
├── main.go                 # Application entry point
├── go.mod                  # Go module definition
├── internal/
│   ├── config/            # Configuration management
│   │   └── config.go
│   ├── middleware/        # HTTP middleware
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
├── memory.md              # User feedback memory (auto-generated)
└── database.db            # SQLite database (auto-generated)
```

## Features

- **Zero Application Code**: All application logic is handled by the LLM
- **Multiple LLM Providers**: Supports both Anthropic Claude and OpenAI GPT models
- **Three Simple Tools**: Database, web response, and memory persistence
- **Self-Evolving**: Users can provide feedback that shapes the application
- **Fast Startup**: Go's compiled nature provides quick server startup

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

### What to Try

Out of the box it builds a contact manager. But try:
- `/game` - Maybe you get a game?
- `/dashboard` - Could be anything
- `/api/stats` - Might invent an API
- Type feedback: "make this purple" or "add a search box"

## Configuration

### Environment Variables

- `PORT` - Server port (default: 3001)
- `LLM_PROVIDER` - Either "anthropic" or "openai" (default: anthropic)
- `ANTHROPIC_API_KEY` - Your Anthropic API key
- `ANTHROPIC_MODEL` - Anthropic model name (default: claude-3-haiku-20240307)
- `OPENAI_API_KEY` - Your OpenAI API key
- `OPENAI_MODEL` - OpenAI model name (default: gpt-4-turbo-preview)
- `DEBUG` - Set to "true" for debug logging

## Performance

The Go implementation provides:
- **Faster startup**: Compiled binary starts instantly
- **Lower memory usage**: Go's efficient runtime
- **Better concurrency**: Native goroutine support for handling multiple requests

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

### Testing

```bash
# Run tests (when implemented)
go test ./...

# Run with race detector
go test -race ./...
```

## Differences from Node.js Version

1. **Compiled Binary**: Go produces a single executable file
2. **Type Safety**: Go's static typing catches errors at compile time
3. **Concurrency**: Native goroutines for better concurrent request handling
4. **Memory**: More efficient memory usage
5. **Dependencies**: Fewer runtime dependencies (just the binary)

## Troubleshooting

### Database Issues

If you encounter database errors, delete `database.db` and restart the server. The LLM will recreate the schema.

### API Key Issues

Make sure your `.env` file contains valid API keys for your chosen provider.

### Port Already in Use

Change the `PORT` environment variable or kill the process using port 3001.

## License

MIT License

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

