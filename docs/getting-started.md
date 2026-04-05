# Getting Started

This guide walks you through running Visoto locally for the first time.

## Prerequisites

- **Go 1.21 or later** — [download from go.dev](https://go.dev/dl/)
- **A SPARQL endpoint** — the public [LINDAS endpoint](https://ld.admin.ch/query/) works out of the box with no account
- **Optional: Google Gemini API key** — only required for the AI chat feature; get one at [aistudio.google.com](https://aistudio.google.com/app/apikey)

## 1. Clone and Install Dependencies

```sh
git clone https://github.com/your-org/visoto.git
cd visoto
go mod download
```

## 2. Create a Config File

```sh
cp visoto.config.example visoto.config
```

The minimal config needed to start:

```toml
[application]
port = 8060
sparqlEndpoint = "https://ld.admin.ch/query/"
timeout = 30

[logging]
level = "INFO"
format = "text"
output = "stdout"
```

Edit `visoto.config` to point at your own SPARQL endpoint if needed. See [docs/configuration.md](configuration.md) for the full field reference.

> **Note:** The config file must exist in the working directory where you run the server. If it is missing, the server starts with a code default port of `8080` instead of `8060`.

## 3. Run the Server

```sh
go run ./cmd/visoto/
```

You should see log output confirming the server started on the configured port.

## 4. Open the App

| URL | Description |
|---|---|
| `http://localhost:8060` | Web UI |
| `http://localhost:8060/ping` | Health check — returns `pong` |
| `http://localhost:8060/mcp` | MCP server for AI tool integration |

## 5. Browse a Resource

Navigate to a resource by IRI. The IRI is percent-encoded in the URL path:

```
http://localhost:8060/resource/skos%3AConceptScheme
```

You can also use the full IRI:

```
http://localhost:8060/resource/http%3A%2F%2Fwww.w3.org%2F2004%2F02%2Fskos%2Fcore%23ConceptScheme
```

Or use the search page at `http://localhost:8060/search` to find resources by keyword.

## Docker Alternative

If you prefer Docker over a local Go installation:

```sh
docker build -t visoto .
docker run \
  -e GIN_MODE=release \
  -p 8060:8060 \
  -v ./visoto.config:/app/visoto.config:ro \
  visoto
```

`GIN_MODE=release` suppresses Gin framework debug output. The config file is mounted read-only.

For a full production deployment with HTTPS and Caddy, see [deployment.md](deployment.md).

## Troubleshooting

**Server fails to start / "no such file" errors**
Run the server from the project root directory. The server expects `./templates/` and `./static/` to exist relative to the working directory.

**SPARQL errors on every page**
Check that your `sparqlEndpoint` URL is reachable and accepts unauthenticated queries. Test it directly:
```sh
curl "https://ld.admin.ch/query/?query=SELECT+1"
```

**Port already in use**
Change `application.port` in `visoto.config` and restart.

**Gemini chat not working**
The AI chat feature requires a valid `gemini_api_key` in the config. The rest of the app works without it.

## Next Steps

- [Configuration Reference](configuration.md) — all `visoto.config` fields explained
- [Architecture Overview](architecture.md) — how the system fits together
- [Template Authoring Guide](templating.md) — add custom templates for your RDF types
- [Deployment Guide](deployment.md) — Docker + Caddy production setup
