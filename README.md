# pingpong

```bash
Pingpong provides ping and pong commands for testing HTTP request/response with configurable latency and metrics.

Usage:
  pingpong [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  help        Help about any command
  ping        Start the ping client that sends requests to a pong server
  pong        Start the pong HTTP server

Flags:
  -h, --help                help for pingpong
      --log.format string   Output format of log messages. One of: [logfmt, json] (default "logfmt")
      --log.level string    Only log messages with the given severity or above. One of: [debug, info, warn, error] (default "info")

Use "pingpong [command] --help" for more information about a command.
```

(adapted from https://github.com/AnaisUrlichs/observe-argo-rollout/tree/main/app)