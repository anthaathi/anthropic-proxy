# Anthropic Proxy

A smart proxy for Claude Code that automatically routes requests to different providers when speed slows down or fails. Supports both **Anthropic** and **OpenAI** API providers with automatic format conversion.

## Features

- 🔄 **Automatic Failover**: Routes to alternative providers when primary is slow or unavailable
- 🌐 **Multi-Provider Support**: Works with Anthropic, OpenAI, OpenRouter, Groq, and more
- 🔀 **Format Conversion**: Automatically converts between Anthropic and OpenAI API formats
- 📊 **Performance-Based Routing**: Selects providers based on TPS (tokens per second) metrics
- 🚦 **Failover-First Routing**: Immediately moves to the next provider, with optional same-provider retries
- 📈 **TUI Monitoring**: Real-time dashboard with live metrics
- ⚡ **Streaming Support**: Full support for streaming responses with format conversion

## Quick Start

1. Copy the example configuration:
```bash
cp example.config.yaml config.yaml
```

2. Edit `config.yaml` with your API keys and provider preferences

3. Run the proxy:
```bash
go run main.go
```

Or with monitoring UI:
```bash
go run main.go -tui
```

## Configure Claude Code

Set these environment variables to use the proxy with Claude Code:

```bash
export ANTHROPIC_BASE_URL=http://localhost:8080
export ANTHROPIC_AUTH_TOKEN=sk-proxy-custom-key-123  # Use your API key from config.yaml
export API_TIMEOUT_MS=600000
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

Now Claude Code will automatically failover to alternative providers when the primary is slow or unavailable.

## Provider Support

The proxy supports two types of providers:

### Anthropic Providers
Providers using Anthropic's API format (`/v1/messages`):
- Anthropic official API
- OpenRouter (Anthropic mode)
- Chutes AI
- Z.ai
- DeepSeek
- Custom Anthropic-compatible endpoints

### OpenAI Providers
Providers using OpenAI's API format (`/v1/chat/completions`):
- OpenAI official API
- Azure OpenAI
- Groq
- Perplexity AI
- Together AI
- OpenRouter (OpenAI mode)
- Custom OpenAI-compatible endpoints

**Important**: The proxy always accepts requests in Anthropic format from clients. When routing to OpenAI providers, it automatically converts the request/response formats transparently.

## Configuration

See `example.config.yaml` for detailed configuration options including:

### Provider Configuration
```yaml
providers:
  # Anthropic provider
  anthropic:
    type: anthropic  # Optional: defaults to "anthropic"
    endpoint: https://api.anthropic.com
    apiKey: env.ANTHROPIC_API_KEY

  # OpenAI provider
  openai:
    type: openai
    endpoint: https://api.openai.com
    apiKey: env.OPENAI_API_KEY
```

### Model Configuration
```yaml
models:
  # Claude model via Anthropic
  - name: "claude-sonnet-4-5-20250929"
    context: 200000
    alias: "claude-sonnet*"
    provider: anthropic
    weight: 5

  # GPT-4 model via OpenAI (format converted automatically)
  - name: "gpt-4o"
    context: 128000
    alias: "gpt-4o*"
    provider: openai
    weight: 5
```

### Features
- **Provider Types**: Specify `anthropic` or `openai` for each provider
- **Model Routing**: Use wildcards for flexible model matching
- **Weights**: Prioritize providers with higher weights
- **Failover Settings**: Optional same-provider retries with exponential backoff controls
- **Performance Thresholds**: Set minimum TPS requirements

## How It Works

### Request Flow

1. **Client Request**: Client sends request in Anthropic format to `/v1/messages`
2. **Provider Selection**: Proxy selects best provider based on:
   - TPS (tokens per second) performance
   - Model weight configuration
   - Provider availability
3. **Format Conversion** (if needed):
   - For Anthropic providers: Request forwarded as-is
   - For OpenAI providers: Request converted to OpenAI format
4. **Response Conversion** (if needed):
   - OpenAI responses converted back to Anthropic format
   - Streaming responses converted in real-time
5. **Client Response**: Client receives response in Anthropic format

### Format Conversion

The proxy handles automatic conversion between Anthropic and OpenAI formats:

**Request Conversion** (Anthropic → OpenAI):
- `messages` array with content blocks → `messages` with string content
- `system` field → system message in messages array
- `max_tokens` preserved
- `temperature`, `top_p` preserved
- `stop_sequences` → `stop`

**Response Conversion** (OpenAI → Anthropic):
- `choices[0].message.content` → `content` array with text blocks
- `usage.prompt_tokens` → `usage.input_tokens`
- `usage.completion_tokens` → `usage.output_tokens`
- `finish_reason` mapped to Anthropic equivalents

**Streaming Conversion**:
- OpenAI deltas → Anthropic content_block_delta events
- Maintains Anthropic SSE format for clients
- Real-time token counting and metrics

## Advanced Features

### Command-Line Flags

```bash
# Run with TUI monitoring
go run main.go -tui

# Watch for config changes (prompts to reload)
go run main.go -watch

# Log all requests and responses to file
go run main.go -log-file requests.jsonl

# Set minimum TPS threshold for provider selection
go run main.go -tps-threshold 50.0

# Combine flags
go run main.go -tui -watch -log-file requests.jsonl -tps-threshold 45.0
```

### Validate Command

Use the `validate` subcommand to verify your config and refresh model context windows directly from each provider's `/v1/models` endpoint:

```bash
# Dry run (prints validation summary)
go run main.go validate --config config.yaml

# Persist refreshed contexts (writes to the supplied path)
go run main.go validate --config config.yaml --write config.yaml

# Print raw /v1/models payloads for specific providers
go run main.go validate --config config.yaml --dump-provider zai --dump-provider google
```

When `--write` is omitted the command only reports differences. With `--write` it serializes the updated config (without comments) to the target path. Add one or more `--dump-provider <name>` flags to print the raw `/v1/models` JSON returned by those providers (useful when a provider omits metadata such as context window sizes). The validator works with both Anthropic-style and OpenAI-style providers.

### Configuration Hot Reloading

Enable config watching to reload configuration without restarting:

```bash
# Server mode with config watching
go run main.go -watch

# TUI mode (config watching enabled by default)
go run main.go -tui
```

When config file changes, you'll be prompted to review and apply changes.

### Request Logging

Log all requests and responses for debugging or analysis:

```bash
go run main.go -log-file requests.jsonl
```

Creates a JSON Lines file with full request/response data, including:
- Timestamps and durations
- Provider and model information
- Request/response headers and bodies
- Token counts and performance metrics

## Troubleshooting

### OpenAI Provider Issues

**Problem**: OpenAI provider returns errors
- **Check**: Verify `type: openai` is set in provider config
- **Check**: Ensure endpoint is correct (usually `https://api.openai.com`)
- **Check**: Verify API key has correct permissions

**Problem**: Responses don't match expected format
- **Solution**: The proxy automatically converts formats - check logs for conversion errors

### Format Conversion Issues

**Problem**: Request to OpenAI provider fails with format errors
- **Check**: Enable debug logging: `export LOG_LEVEL=DEBUG`
- **Check**: Review conversion logs for details
- **Solution**: Some Anthropic features (like `thinking` parameter) aren't supported by OpenAI

## License

MIT
