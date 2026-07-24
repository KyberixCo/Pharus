# Pharus

![Pharus cover](docs/portada.jpg)

[Versión en español](README.md)

Pharus is a local deep-research engine written in Go. It plans a research run,
queries SearXNG, extracts evidence from web pages, indexes the corpus in a
vector store, and generates a Markdown report with an LLM.

It can run directly from the CLI or as a daemon exposing a Unix socket, an HTTP
API, and a Model Context Protocol (MCP)-compatible endpoint. The research
workflow includes Co-STORM-inspired expert perspectives, TAXMORPH outline
refinement, gap analysis, section-by-section synthesis, citation validation,
and a configurable critic loop.

> **Project status:** some integrations are experimental. Read
> [Current limitations](#current-limitations) before using Pharus in
> production.

> **Security:** configuration can contain credentials. Do not publish `.env` or
> `~/.pharus/config.yaml`.

## Features

- Query planning and `quick`, `balanced`, and `deep` effort profiles.
- Search through a SearXNG instance with JSON output enabled.
- HTML extraction with a 5 MiB limit, private/loopback address blocking,
  redirect validation, and DNS pinning.
- LLM providers: MiniMax, OpenAI, Anthropic, Ollama, and llama.cpp.
- Embedding providers: OMLX, MLX, Ollama, and OpenAI-compatible external APIs.
- Persistent `chromem-go` vector storage with per-research result isolation.
- Optional native LanceDB integration through `lancedb/lancedb-go`.
- Planning and synthesis checkpoints for resuming research.
- `success`, `degraded`, and `failed` report states; Pharus does not produce a
  report when it cannot collect enough evidence.
- HTTP/UDS daemon and MCP server with synchronous and asynchronous research.
- RACE evaluation and sample benchmark suites.

## Requirements

- Go 1.25 or later.
- A SearXNG instance with JSON output enabled.
- A configured LLM provider.
- A configured embedding service.
- macOS to manage the daemon through `launchd`.
- Optionally, Docker to deploy SearXNG and use containerized code execution.

The engine can compile on other Go-supported systems, but automatic setup,
managed OMLX, and `launchd` are macOS-oriented; managed OMLX requires macOS on
Apple Silicon.

## Quick start

```bash
git clone https://github.com/KyberixCo/Pharus.git
cd Pharus
go build -o bin/pharus ./cmd/pharus
./bin/pharus config
./bin/pharus doctor
```

The `config` wizard configures providers, can prepare local services, and saves
settings to `~/.pharus/config.yaml` and `.env`.

Alternatively, `setup` creates the working directories, saves the
configuration, and tries to deploy a managed SearXNG instance when Docker is
available:

```bash
./bin/pharus setup
./bin/pharus doctor --fix
```

`doctor --fix` only attempts to repair the local OMLX service when OMLX is the
selected embedding provider.

## CLI usage

Run research in the current process:

```bash
./bin/pharus research --direct --language en \
  "Recent advances in quantum computing"
```

By default, the report is saved as a `.md` file in the current directory. You
can select an output path and also print the report:

```bash
./bin/pharus research --direct --output report.md --stdout \
  --language en "Quantum computing"
```

The global `--language` and `--lang` flags accept `auto`, `en`, or `es`.
`--profile` accepts:

| Profile | Intended use |
| --- | --- |
| `quick` | Fewer queries and sections; no critic iteration. |
| `balanced` | Default configuration. |
| `deep` | More queries, sections, context, and critic iterations. |

Examples:

```bash
./bin/pharus research --direct --profile quick "Executive overview"
./bin/pharus research --direct --profile balanced "Technical analysis"
./bin/pharus research --direct --profile deep "Exhaustive review"
```

Direct runs resume compatible checkpoints by default. Use `--no-resume` to
ignore them or `--restart` to delete the checkpoint for the same topic,
language, and profile:

```bash
./bin/pharus research --direct --no-resume "Topic"
./bin/pharus research --direct --restart "Topic"
```

Without `--direct`, the CLI first uses the daemon's Unix socket and falls back
to in-process execution if it cannot connect.

### Daemon

On macOS:

```bash
./bin/pharus daemon start
./bin/pharus daemon status
./bin/pharus research "The impact of AI on healthcare"
./bin/pharus daemon stop
```

Run it in the foreground, including on other systems:

```bash
./bin/pharus daemon run
```

Default listeners:

- Unix socket: `~/.pharus/pharus.sock`
- HTTP: `http://127.0.0.1:8765`
- MCP Streamable HTTP: `http://127.0.0.1:8765/mcp`
- Task-event SSE: `http://127.0.0.1:8765/mcp/subscriptions/listen`

## HTTP API

| Method and path | Authentication | Description |
| --- | --- | --- |
| `GET /health` | No | Basic health and uptime. |
| `GET /status` | No | Engine status and configuration summary. |
| `POST /research` | Yes over TCP | Runs synchronous research. |

Example:

```bash
curl -X POST http://127.0.0.1:8765/research \
  -H 'Content-Type: application/json' \
  -H 'X-Pharus-Token: <daemon_token>' \
  -d '{"topic":"Quantum computing","language":"en","profile":"balanced"}'
```

The token is stored in the `daemon_token` field of
`~/.pharus/config.yaml`. Requests over the Unix socket bypass token validation;
TCP requests to `/research` and `/mcp` must send `X-Pharus-Token`. A client may
also send `X-Pharus-User-ID`; loopback connections default to
`user_local_default` when it is omitted.

## MCP

The daemon registers a stateless Streamable HTTP handler at `/mcp` and exposes:

| Tool or resource | Status | Description |
| --- | --- | --- |
| `deep_research` | Functional | Runs synchronous research. |
| `start_async_research` | Functional | Starts an in-memory task and returns its URI. |
| `resource://tasks/{id}` | Functional | Reads task progress and its result. |
| `resource://tasks/list` | Functional | Lists tasks visible to the user. |
| `ask_user_input` | Experimental | Returns a custom `input_required` structure; it does not automatically suspend or resume a call. |
| `execute_code_script` | Experimental | Runs Python or Bash through the native or Docker backend. |
| `vector_search` | Functional | Queries real evidence, isolated by authenticated user and optionally by `research_id`. |

Asynchronous tasks and events are held in memory and are lost when the daemon
restarts. The `/mcp/subscriptions/listen` SSE channel is a custom HTTP
extension, not a standard MCP subscription.

## Configuration

The main configuration file is `~/.pharus/config.yaml`. Pharus also loads
`.env` from the current directory, beside the executable, from its parent
directory, and from `~/.pharus/.env`. Process environment variables take
precedence over YAML.

Relevant defaults:

| Option | Default |
| --- | --- |
| LLM | MiniMax with `MiniMax-M3` |
| LLM retries | 5 attempts, 2 s initial and 30 s maximum backoff |
| Embeddings | OMLX at `http://localhost:8000`, model `mlx-community/Qwen3-Embedding-0.6B-8bit` |
| SearXNG | `http://localhost:8090` |
| Vectors | `chromem`, collection `pharus_research` |
| Profile | `balanced` |
| Checkpoints | enabled, with a 72-hour maximum age |

Supported environment variables:

| Variable | Description |
| --- | --- |
| `PHARUS_LANGUAGE` | `auto`, `en`, or `es`. |
| `PHARUS_LLM_PROVIDER` | `minimax`, `openai`, `anthropic`, `ollama`, or `llamacpp`. |
| `PHARUS_LLM_RETRY_MAX_ATTEMPTS` | Maximum number of attempts. |
| `PHARUS_LLM_RETRY_INITIAL_BACKOFF_SECONDS` | Initial retry delay. |
| `PHARUS_LLM_RETRY_MAX_BACKOFF_SECONDS` | Maximum retry delay. |
| `PHARUS_MINIMAX_API_KEY`, `PHARUS_MINIMAX_GROUP_ID` | MiniMax credentials. |
| `PHARUS_MINIMAX_MODEL`, `PHARUS_MINIMAX_BASE_URL`, `PHARUS_MINIMAX_MAX_TOKENS` | MiniMax settings. |
| `PHARUS_OPENAI_API_KEY`, `PHARUS_OPENAI_MODEL`, `PHARUS_OPENAI_BASE_URL` | OpenAI settings. |
| `PHARUS_ANTHROPIC_API_KEY`, `PHARUS_ANTHROPIC_MODEL`, `PHARUS_ANTHROPIC_BASE_URL` | Anthropic settings. |
| `PHARUS_OLLAMA_LLM_URL`, `PHARUS_OLLAMA_LLM_MODEL` | Ollama chat settings. |
| `PHARUS_EMBEDDING_PROVIDER` | `omlx`, `mlx`, `ollama`, `openai`, `external`, or `custom`. |
| `PHARUS_EMBEDDING_URL`, `PHARUS_EMBEDDING_MODEL`, `PHARUS_EMBEDDING_API_KEY` | Embedding settings. |
| `PHARUS_SEARXNG_URL`, `PHARUS_SEARXNG_IMAGE` | SearXNG service and image. |
| `PHARUS_VECTOR_PROVIDER`, `PHARUS_VECTOR_COLLECTION` | `chromem` or `lancedb`, plus the base collection name. |
| `PHARUS_RESEARCH_PROFILE` | `quick`, `balanced`, or `deep`. |
| `PHARUS_RESEARCH_RESUME_SESSIONS` | Enables or disables checkpoints. |
| `PHARUS_RESEARCH_CHECKPOINT_MAX_AGE_HOURS` | Checkpoint maximum age. |
| `PHARUS_LLAMACPP_URL`, `PHARUS_LLAMACPP_MODEL`, `PHARUS_LLAMACPP_GRAMMAR_ENABLE` | llama.cpp provider and GBNF support. |
| `PHARUS_DATASTORM_SOURCES` | YAML/JSON map from table name to CSV file, for example `{research_metrics: /data/metrics.csv}`. |

HTTP limits, sandbox backends, and other options are configured only through
the YAML file.

## Vector storage

| Provider | Actual implementation |
| --- | --- |
| `chromem` | Persistent `chromem-go` storage under `~/.pharus/vectors`. This is the default. |
| `lancedb` | Upstream `lancedb-go` SDK backed by LanceDB's native engine and format. Build with `-tags lancedb_native` and link the SDK's native artifacts. |

Collection names include the embedding provider and model to prevent mixing
incompatible vector spaces. A build without native support explicitly rejects
`provider: lancedb`; it never creates a JSON file with a misleading extension.

DataSTORM is enabled for both the CLI and daemon by configuring one or more
sources:

```yaml
datastorm:
  sources:
    research_metrics: /absolute/path/metrics.csv
```

The first CSV row defines the columns. An invalid source fails engine startup
instead of silently disabling DataSTORM.

## Benchmarks

The built-in suites are small smoke-test samples, not official copies of the
datasets they are named after:

```bash
./bin/pharus benchmark list
./bin/pharus benchmark run --suite synthetic --limit 1
./bin/pharus benchmark run --file dataset.jsonl --output results
```

`benchmark run` executes the real engine and uses the configured LLM as the
RACE judge, so it consumes the configured services and credentials.

## Development and tests

```bash
go test -short ./...
./dev.sh
```

`dev.sh` installs `air` when it is unavailable and starts the daemon with live
reload. Without `-short`, `go test ./...` may also enable live tests when it
finds credentials. Integration tests require services and credentials and can
be run explicitly with:

```bash
./test_llm.sh
```

The `postman/` directory contains a sample collection and environment for the
local API.

## Isolation and optional dependencies

The native sandbox uses `sandbox-exec` on macOS and `bubblewrap` + `prlimit` on
Linux. It enforces filesystem/network policy and timeout; Linux also enforces
CPU/memory limits. Because this macOS backend cannot provide a reliable
per-process memory limit, it rejects `memory_mb > 0` and requires Docker.
Docker additionally uses a read-only filesystem, a process limit, dropped
capabilities, CPU/memory limits, and network policy. Both backends fail closed:
Docker never executes a script locally when the container cannot start.

LanceDB requires the native headers and binaries published by
[`lancedb-go`](https://github.com/lancedb/lancedb-go). Standard builds keep
`chromem` as the default and do not require additional CGO setup.

## License

Pharus is distributed under the
[GNU General Public License v3.0](LICENSE).
