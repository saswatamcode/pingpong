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

## Pong

```bash
Start the pong HTTP server that responds to /ping requests with configurable latency and success probability.

Usage:
  pingpong pong [flags]

Flags:
      --db-enabled              Enable database simulation metrics
      --db-error-types string   Distribution of error types when DB queries fail in format: <probability>%<error_type>,... (default "50%timeout,30%connection,20%deadlock")
      --db-latency string       Encoded latency and probability for simulated DB queries in format: <probability>%<duration>,<probability>%<duration>.... (default "90%10ms,10%50ms")
      --db-success-prob float   The probability (in %) of a successful simulated DB query (default 95)
  -h, --help                    help for pong
      --latency string          Encoded latency and probability of the response in format as: <probability>%<duration>,<probability>%<duration>.... (default "90%500ms,10%200ms")
      --listen-address string   The address to listen on for HTTP requests. (default ":8080")
      --set-version string      Injected version to be presented via metrics. (default "first")
      --success-prob float      The probability (in %) of getting a successful response (default 100)

Global Flags:
      --log.format string   Output format of log messages. One of: [logfmt, json] (default "logfmt")
      --log.level string    Only log messages with the given severity or above. One of: [debug, info, warn, error] (default "info")
```

## Ping

```bash
Start the ping client that continuously sends HTTP requests to a pong server endpoint with configurable rate.

Usage:
  pingpong ping [flags]

Flags:
      --endpoint string         The address of pong app we can connect to and send requests. (default "http://localhost:8080/ping")
  -h, --help                    help for ping
      --listen-address string   The address to listen on for HTTP requests. (default ":8080")
      --pings-per-second int    How many pings per second we should request (default 10)

Global Flags:
      --log.format string   Output format of log messages. One of: [logfmt, json] (default "logfmt")
      --log.level string    Only log messages with the given severity or above. One of: [debug, info, warn, error] (default "info")
```

(adapted from https://github.com/AnaisUrlichs/observe-argo-rollout/tree/main/app)