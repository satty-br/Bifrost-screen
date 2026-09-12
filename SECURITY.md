# Security Policy

Bifrost is a small, actively maintained hobby project. It runs entirely on
`127.0.0.1` with no external accounts, telemetry, or cloud services, but it
does talk to a USB device, the Steam Web API (optional) and, on Windows, runs
with access to your media session and running processes — so security reports
are taken seriously.

## Supported versions

Only the **latest release** on the [Releases page](https://github.com/satty-br/Bifrost-screen/releases)
is supported. There is no long-term support for older versions; please update
before reporting an issue.

## Reporting a vulnerability

**Please do not open a public issue for security vulnerabilities.**

Preferred way: use GitHub's private reporting, via the
**[Security tab → "Report a vulnerability"](https://github.com/satty-br/Bifrost-screen/security/advisories/new)**
button on this repository. This opens a private advisory only visible to the
maintainer until it's resolved.

If that's not available to you, email **ricardo@satty.com.br** with:

- A description of the vulnerability and its potential impact.
- Steps to reproduce (a minimal repro is very helpful).
- The affected version/commit and OS.

This is a one-person project maintained in spare time, so there's no formal
SLA — but security reports get priority over other issues. Expect an initial
response within a few days.

## Scope

In scope:

- The Bifrost application itself (`cmd/`, `internal/`), including the local
  web panel server (`internal/web`) and its interaction with the USB screen,
  Steam, and OS APIs.
- The build/release GitHub Actions workflows in `.github/workflows/`.

Out of scope (please report upstream instead):

- Vulnerabilities in third-party dependencies (`go.mod`) that don't have a
  Bifrost-specific exploitation path — report those to the upstream project.
  If you believe Bifrost's use of a dependency makes an otherwise-unexploitable
  issue exploitable, that *is* in scope here.
- The official Turing/UsbMonitor app or the physical LCD screen's firmware.

## Disclosure

Please give a reasonable amount of time to fix a confirmed vulnerability
before any public disclosure. Credit will be given in the release notes
unless you'd prefer to stay anonymous.
