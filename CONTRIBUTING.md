# Contributing

Thanks for improving Agent Workforce.

## Before opening a change

1. Search existing issues and pull requests.
2. For a large behavioral or architectural change, open an issue first so the scope can be agreed on.
3. Keep pull requests focused. Avoid mixing unrelated cleanup with a feature or fix.

## Development setup

Requirements:

- Go 1.25.13 or newer
- `make`
- ForgeCode and a `forge` command on `PATH` for live integration testing

```sh
git clone https://github.com/donske07/agent-workforce.git
cd agent-workforce
make verify
```

Build both commands locally:

```sh
make build
./bin/agent-workforce --help
```

## Pull requests

- Add a focused regression test for bug fixes and observable behavior changes.
- Run `make verify` before submitting.
- Exercise the changed command or web flow manually.
- Update `README.md`, `SECURITY.md`, or `CHANGELOG.md` when their documented contract changes.
- Do not include credentials, local agent transcripts, `.omo/`, `.playwright-mcp/`, or generated binaries.

Security vulnerabilities must follow `SECURITY.md`, not the public issue tracker.
