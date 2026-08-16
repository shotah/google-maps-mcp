# google-maps-mcp — build this package

Open this folder in Cursor and **build the repo here**. This file is the brief.
Nothing else is on disk yet.

**Do not `git init` / `gh` — the human will create the repo and push.**

---

## Sister libraries (read these first)

Absolute paths on this machine. Copy patterns; do not invent a third dialect.

| What | Path | Steal from it |
| --- | --- | --- |
| **Host + naming (canonical)** | `/home/christopher/ai-gantry` | [docs/mcp-naming.md](https://github.com/shotah/ai-gantry/blob/main/docs/mcp-naming.md), `docs/mcp.md`, `docs/watch.md`. Gantry `future_todo.md` § Tooling — maps. |
| **Stdio MCP + Makefile + GoReleaser + CI + banner + badges** | `/home/christopher/feeds-mcp` | `Makefile`, `.github/workflows/ci.yml` + `release.yml`, `.goreleaser.yaml`, `.golangci.yml`, `cmd/release`, `scripts/pre-commit`, `scripts/coverage-badge.sh`, `assets/banner.svg`, README badge block, `LICENSE` (MIT shotah), `VERSION`, `.gitignore`, `tools/http.go` (size-capped GET + redirect cap), `tools/names_test.go` |
| **Same scaffold, one env secret** | `/home/christopher/twitter-mcp` | `X_BEARER_TOKEN` → same shape for `GOOGLE_MAPS_API_KEY` (child inherits gantry process env). Lean README. |
| **Workspace OAuth (not this binary)** | `/home/christopher/google-mcp` | Calendar / Gmail / Drive. **Do not fold Maps into google-mcp.** Maps Platform is an API key, not the Workspace OAuth dance. |

Copy the feeds-mcp Makefile / CI / release / banner / badge pipeline and **rename** `feeds-mcp` → `google-maps-mcp`, server id `feeds` → `maps`. Draw a maps-themed `assets/banner.svg` (pin + share-link → canonical URL), not an RSS rail.

Host contract: gantry `mcp.toml` `name = "maps"` → tools published as `maps__{tool}`.

This is **not** a watch poller and **not** a long-running server. Stdio request/response only (`mark3labs/mcp-go`, same as the sisters).

---

## Naming (locked)

Canonical: ai-gantry `docs/mcp-naming.md`.

| Layer | Value |
| --- | --- |
| Server id (`mcp.toml` `name`, `server.ServerName`) | `maps` |
| Binary / module | `google-maps-mcp` / `github.com/shotah/google-maps-mcp` |
| Expand a share / short URL | `link_resolve` → host `maps__link_resolve` |
| Text / address → place | `place_resolve` → host `maps__place_resolve` |
| Origin + dest → leave-by / duration | `route_eta` → host `maps__route_eta` |

Rules:

- `{service}_{verb}_{object}` — **not** `resolve_link`, **not** `get_eta`, **not** `maps_place_resolve` (double prefix → `maps__maps_place_resolve`).
- Do **not** put the server id on the tool. Tests: every name matches `^[a-z]+_[a-z]+` and does **not** start with `maps`.
- Stable verbs: `resolve` (already used: `rentals__areas_resolve`, `feeds__source_resolve`). `route_eta` = service `route`, object `eta`. Do not invent `fetch` / `expand` / `unshorten`.
- Descriptions lead with agent intent. Args snake_case (`url`, `query`, `origin`, `destination`). Teach-in errors (`Next: link_resolve(url="https://maps.app.goo.gl/…")`).
- No dual aliases. No `maps_*` synonym. No `short_url_*`.
- Lean catalog: **≤3 tools** in v1. Do not add `route_steps` unless a daily ask appears.

Gantry `mcp.toml` snippet (do **not** wire into ai-gantry until a release exists):

```toml
[[server]]
name = "maps"
command = "google-maps-mcp"
download_tag = "latest"
download_url = "https://github.com/shotah/google-maps-mcp/releases/download/{tag}/google-maps-mcp_{version}_{os}_{arch}.tar.gz"
# GOOGLE_MAPS_API_KEY lives in the gantry process env (child inherits it)
```

---

## Product (locked)

Calendar answers “what’s next.” Maps answers **“when do I leave?”** and **“what is this share link?”**

The live agent already fails the second one: a human pastes `https://maps.app.goo.gl/…` (or older `goo.gl/maps/…`) and the model cannot turn it into a place or a route. That hop is **HTTP redirects**, not the Places API. Ship `link_resolve` first.

| Tool | Auth | Job |
| --- | --- | --- |
| `link_resolve` | **none** | Follow a Maps share / short URL → canonical `google.com/maps/…` + parsed name / coords / dir endpoints |
| `place_resolve` | `GOOGLE_MAPS_API_KEY` | Query or address → place_id, lat/lng, name. If the input is a share URL, call the same helper as `link_resolve` first |
| `route_eta` | `GOOGLE_MAPS_API_KEY` | Origin + destination (names, coords, or share URLs) → duration in traffic / leave-by |

### `link_resolve` (required — this is why the agent complains)

Accept:

- `https://maps.app.goo.gl/{id}` (current share sheet)
- `https://goo.gl/maps/{id}` (older)
- `https://g.co/maps/…`
- Already-long `https://www.google.com/maps/…` / `maps.google.com` (parse, no fetch)

Return JSON text:

```json
{
  "url": "https://maps.app.goo.gl/xxxx",
  "canonical": "https://www.google.com/maps/place/Space+Needle/@47.6205,-122.3493,17z",
  "kind": "place",
  "name": "Space Needle",
  "lat": 47.6205,
  "lng": -122.3493
}
```

`kind` is `place` | `directions` | `search` | `view` | `unknown`. Directions fill `origin` / `destination` when the path is `/maps/dir/…`.

SSRF: only follow redirects to Maps hosts (`maps.app.goo.gl`, `goo.gl`, `g.co`, `google.com` / `*.google.com`, `www.google.*`, `maps.google.*`). Tests use a stub `Fetcher` / `httptest` — **no live Google**.

Copy the GET + redirect-cap shape from `feeds-mcp/tools/http.go`. Do not use a fat Maps SDK for this tool.

### Places / Routes

Official Google Maps Platform only. **No scrape. No undocumented InnerTube. No page HTML as a geocoder.**

| Question | Answer |
| --- | --- |
| Key | `GOOGLE_MAPS_API_KEY` on this process. Kernel never sees it. Same inheritance as `X_BEARER_TOKEN` on twitter-mcp. |
| Missing key | Teach-in error, not a panic. `link_resolve` must still work with no key. |
| Not in v1 | Workspace OAuth, Street View, nearby search dump, traffic-alert watches, `route_steps` |
| On-demand | “I have climbing at 6 — when do I leave?” → `route_eta`. “What is this pin?” → `link_resolve` then maybe `place_resolve`. |

---

## Scaffold to copy (from feeds-mcp, then rename)

- [ ] `go.mod` — `github.com/shotah/google-maps-mcp`, Go 1.26, `CGO_ENABLED=0`, `mark3labs/mcp-go`
- [ ] `server/server.go` — `ServerName = "maps"`
- [ ] `main.go` — stdio only
- [ ] `Makefile` / `.golangci.yml` / `.goreleaser.yaml` / `.github/workflows/ci.yml` + `release.yml`
- [ ] `cmd/release` + `make release` / `make version` (same as feeds-mcp)
- [ ] `scripts/pre-commit`, `scripts/coverage-badge.sh` (CI pushes coverage.svg to `gh-pages`)
- [ ] `LICENSE` MIT shotah, `VERSION` `v0.0.1`, `.gitignore`
- [ ] `assets/banner.svg` + README badge row (CI, Release, coverage, pkg.go.dev, Go version, license) — badges 404 until the GitHub repo exists; that is fine
- [ ] `AGENTS.md` — short naming reminder + link to this TODO + sister paths
- [ ] `README.md` — lean: tools table, no-key `link_resolve`, key env for the other two, gantry `mcp.toml` snippet (`name = "maps"`), link mcp-naming

## Tools

- [ ] `link_resolve(url)` — share / short URL expander (no auth)
- [ ] `place_resolve(query)` — Geocoding / Places
- [ ] `route_eta(origin, destination, departure_time?)` — Routes / Directions; reuse `ResolveLink` when an arg is a share URL
- [ ] Name-lock tests (`^[a-z]+_[a-z]+`, no `maps` prefix)
- [ ] `golangci-lint run ./...` and `go test ./...` green

## Do not

- [ ] `git init` / `gh`
- [ ] Wire `mcp.toml` into ai-gantry until a release exists
- [ ] Document live-agent enablement in this repo

---

## Out of scope

- Polling / webhooks / Push (kernel)
- Folding into `google-mcp` (wrong auth story, fat catalog)
- Scraping / unofficial endpoints
- Street View, nearby dump, traffic watches
