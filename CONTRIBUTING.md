# Contributing

Openhandle SDK behavior is defined by the pinned OpenAPI contract and the
language-neutral contract in [`../../docs/sdk-contract.md`](../../docs/sdk-contract.md).

Before opening a pull request, run:

```bash
gofmt -w .
go generate ./...
go run ./internal/cmd/generate --check
go vet ./...
go test -race ./...
```

Use Conventional Commits for commit subjects, such as `feat: add an endpoint`
or `fix(runtime): honor request cancellation`.
