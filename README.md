# Anthropic Proxy

A smart proxy for Claude Code that automatically routes requests to different providers when speed slows down or fails.

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

## Configuration

See `example.config.yaml` for detailed configuration options including:
- Provider endpoints and API keys
- Model routing and weights
- Retry settings
- Performance thresholds

## License

MIT
