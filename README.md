# google-maps-mcp

<p align="center">
  <img src="assets/banner.svg" alt="google-maps-mcp — share link to canonical URL, plus place and route ETA" width="100%">
</p>

Stdio MCP server for Google Maps share links, places, and driving ETAs.

This is **not** a poller and **not** a long-running server. Request/response only. Maps Platform is an API key, not Workspace OAuth — do not fold this into `google-mcp`.

Naming contract: [ai-gantry `docs/mcp-naming.md`](https://github.com/shotah/ai-gantry/blob/main/docs/mcp-naming.md).

<p align="center">
  <a href="https://github.com/shotah/google-maps-mcp/actions/workflows/ci.yml"><img src="https://github.com/shotah/google-maps-mcp/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/shotah/google-maps-mcp/actions/workflows/release.yml"><img src="https://github.com/shotah/google-maps-mcp/actions/workflows/release.yml/badge.svg" alt="Release"></a>
  <a href="https://github.com/shotah/google-maps-mcp/actions/workflows/ci.yml"><img src="https://github.com/shotah/google-maps-mcp/raw/gh-pages/badges/coverage.svg" alt="Coverage"></a>
  <a href="https://pkg.go.dev/github.com/shotah/google-maps-mcp"><img src="https://pkg.go.dev/badge/github.com/shotah/google-maps-mcp.svg" alt="Go Reference"></a>
  <img src="https://img.shields.io/github/go-mod/go-version/shotah/google-maps-mcp" alt="Go version">
  <a href="LICENSE"><img src="https://img.shields.io/github/license/shotah/google-maps-mcp" alt="License"></a>
</p>

## Tools

| Tool | Host name | Auth | What it does |
| --- | --- | --- | --- |
| `link_resolve` | `maps__link_resolve` | none | `urls` (1–8) → canonical `google.com/maps/…` plus name / coords / dir endpoints for each |
| `place_resolve` | `maps__place_resolve` | `GOOGLE_MAPS_API_KEY` | `queries` (1–8 names, addresses, or share URLs) → coords, rating, a few reviews, Maps URL for each |
| `place_search` | `maps__place_search` | `GOOGLE_MAPS_API_KEY` | `queries` (1–8) → a few rated places + Maps links each. Optional shared `near`, `limit` (default 5, max 8 hits per query) |
| `route_eta` | `maps__route_eta` | `GOOGLE_MAPS_API_KEY` | `routes` (1–8 `{origin, destination}`) → duration, distance, and a tap-to-open Maps URL each. Optional `mode` and `departure_time` on each route |

`link_resolve` accepts `maps.app.goo.gl`, `goo.gl/maps`, `g.co/maps`, and already-long `google.com/maps` URLs (parsed, no fetch). Redirects stay on Maps hosts only.

Not in v1: Street View, unbounded nearby dump, traffic-alert watches, `route_steps`.

## Auth

Maps Platform **API key** on this process. The kernel never sees it. `link_resolve` works with no key.

```bash
export GOOGLE_MAPS_API_KEY="…"   # console.cloud.google.com → APIs & Services → Credentials
```

A key alone is not enough. In [GCP Console](https://console.cloud.google.com/):

1. **APIs & Services → Library** — enable these three:

   | API | Used by |
   | --- | --- |
   | [Geocoding API](https://console.cloud.google.com/apis/library/geocoding-backend.googleapis.com) | `place_resolve`, `place_search` (`near`) |
   | [Places API](https://console.cloud.google.com/apis/library/places-backend.googleapis.com) | `place_search`; reviews on `place_resolve` |
   | [Directions API](https://console.cloud.google.com/apis/library/directions-backend.googleapis.com) | `route_eta` |

2. **APIs & Services → Credentials** — open the key. Under **API restrictions**, leave unrestricted **or** allow those three.

Do not enable Distance Matrix — `route_eta` already returns distance from Directions. `link_resolve` needs no API.

`REQUEST_DENIED` usually means an API is off or the key’s restriction list omitted it.

## Install

**Pre-built binary** — grab the archive for your platform from [Releases](https://github.com/shotah/google-maps-mcp/releases):

```bash
tar xzf google-maps-mcp_*_linux_amd64.tar.gz
chmod +x google-maps-mcp
mv google-maps-mcp ~/.local/bin/
```

**Or with Go** (1.26+):

```bash
go install github.com/shotah/google-maps-mcp@latest
```

## Gantry `mcp.toml`

Server id must be **`maps`** so hosts expose `maps__link_resolve` (do not put `maps` on the tool name).

```toml
[[server]]
name = "maps"
command = "google-maps-mcp"
download_tag = "latest"
download_url = "https://github.com/shotah/google-maps-mcp/releases/download/{tag}/google-maps-mcp_{version}_{os}_{arch}.tar.gz"
# GOOGLE_MAPS_API_KEY lives in the gantry process env (child inherits it)
```

## Development

```bash
make test
make lint
make coverage
make cli
make version          # dry-run: current tag → next patch
make release          # bump VERSION, tag v* + latest, push (BUMP=patch|minor|major)
```

`CGO_ENABLED=0`. Tests use `httptest` / stub `Fetcher` — no live Google.

## License

[MIT](LICENSE)
