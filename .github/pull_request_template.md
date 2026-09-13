## What this changes

<!-- One or two sentences. Link the issue if there is one: Fixes #123 -->

## How to verify

<!-- The steps a reviewer follows to see it work: the page or route to open,
     the IRI or endpoint to try, what should appear. -->

## Checklist

- [ ] `gofmt -w .` — CI fails the build on unformatted files
- [ ] `go test ./...` passes
- [ ] The server boots (`go run ./cmd/visoto/`) — templates parse at runtime,
      so `go build` alone does not catch a broken template
- [ ] New UI strings added to all five `locales/*.toml` — a missing translation
      falls back to English silently rather than failing a test
