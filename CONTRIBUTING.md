# Contributing to Bifrost

Thanks for considering contributing! This is a small hobby project, so keep
expectations proportionate, but PRs, bug reports and ideas are all welcome.

## Before you start

- For anything beyond a trivial fix, please open an issue first to discuss the
  approach — this avoids wasted work on both sides.
- Found a **security vulnerability**? Don't open a public issue — see
  [SECURITY.md](SECURITY.md) instead.

## Development setup

Requires [Go 1.24+](https://go.dev/dl/).

```sh
git clone https://github.com/satty-br/Bifrost-screen.git
cd Bifrost-screen
go test ./...
```

Quick local build on Windows: `build.bat 1.0.0`. Building for all three
platforms (Windows/Linux/macOS, amd64/arm64, no CGO): `./build.sh 1.0.0`. See
[README.md#building](README.md#building) for details.

If you don't have the physical USB screen, set `BIFROST_DEMO=1` before running
the app to inject fake media/game data, so you can work on the UI without
hardware. `internal/render/render_test.go`'s `TestRenderAll` (with
`BIFROST_PREVIEW_DIR` set) renders every screen to PNG files, which is handy
for reviewing layout changes without running the whole app.

## Project layout

See the [Structure section in README.md](README.md#structure) for what lives
where. A few conventions worth knowing:

- Platform-specific code is split by `_windows.go` / `_linux.go` / `_darwin.go`
  / `_other.go` build tags (not `runtime.GOOS` checks inside shared files),
  matching the existing pattern in `internal/lcd`, `internal/media`,
  `internal/sysinfo` and `internal/winutil`.
- Code comments and internal identifiers are mostly in **Portuguese**
  (the maintainer's first language); user-facing docs (README, this file) are
  in **English**. Please keep new code comments in Portuguese for consistency,
  unless you're not comfortable with that — an English comment is much better
  than no comment.
- Keep diffs focused. Prefer several small PRs over one large one.

## Making changes

1. Fork the repo and create a branch off `main` (e.g. `fix/steam-detection`,
   `feat/linux-tray-icon`).
2. Make your change. Add/update tests where it makes sense — the project has
   decent coverage (`go test ./... -cover`) and CI will run it.
3. Before opening a PR, please run:
   ```sh
   go build ./...
   go vet ./...
   go test ./...
   gofmt -l .   # should print nothing you added
   ```
4. If you touched code that's shared across platforms, sanity-check it also
   builds elsewhere (CI does this automatically, but you can check locally
   too — no cross-toolchain or CGO needed):
   ```sh
   $env:GOOS="linux";  $env:GOARCH="amd64"; $env:CGO_ENABLED="0"; go build ./...
   $env:GOOS="darwin"; $env:GOARCH="arm64"; $env:CGO_ENABLED="0"; go build ./...
   ```
5. Open a PR against `main`. Describe *what* changed and *why*; link the
   related issue if there is one. CI (`.github/workflows/ci.yml`) runs
   `go vet`, `go build`, `go test`, and cross-compile checks for Linux/macOS.

### Commit messages

Short, imperative, [Conventional Commits](https://www.conventionalcommits.org/)-style
prefixes are used throughout the history (`feat:`, `fix:`, `docs:`, `chore:`,
`build:`) — not strictly enforced, but appreciated.

## Reporting bugs / requesting features

Please use the issue templates (bug report / feature request) — they ask for
the info that's usually needed to act on a report (OS, version, logs,
`bifrost --diagnostico` output for bugs).

## License

By contributing, you agree your contribution is licensed under this
project's [GPL-3.0-or-later](LICENSE) license.
