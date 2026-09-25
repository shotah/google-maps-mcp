# Agent / contributor notes

## Tool naming (required)

Canonical: [ai-gantry `docs/mcp-naming.md`](https://github.com/shotah/ai-gantry/blob/main/docs/mcp-naming.md).
Package-specific checklist: [TODO.md](TODO.md).

Every MCP tool name is **service-first**:

```text
{service}_{verb}_{object}[_{qualifier}]
```

This server:

| Layer | Value |
| --- | --- |
| Server id (`ServerName`, `mcp.toml` `name`) | `maps` |
| Tools | `link_resolve`, `place_resolve`, `place_search`, `route_eta` |
| Host-facing | `maps__link_resolve`, `maps__place_resolve`, `maps__place_search`, `maps__route_eta` |

Rules:

1. **Service first** — `link_resolve`, not `resolve_link` / `get_eta`.
2. **No server id on the tool** — never `maps_place_resolve` (host already prefixes → `maps__maps_place_resolve`).
3. **Stable verbs** — `resolve` / `search`. `route_eta` = service `route`, object `eta`. Do not invent `fetch` / `expand` / `unshorten`.
4. **No dual aliases.** No `maps_*` synonym. No `short_url_*`.
5. Tests: every name matches `^[a-z]+_[a-z]+` and does **not** start with `maps`.

Descriptions lead with agent intent. Args are snake_case lists (`urls`, `queries`, `routes`). Teach-in errors name the next call (`Next: link_resolve(urls=["https://maps.app.goo.gl/…"])`). Do not accept a singular `url`, `query`, `origin`, or `destination`.

## GCP APIs to enable

Same key as `GOOGLE_MAPS_API_KEY`. Console: **APIs & Services → Library**, then allow them on the key under **Credentials → API restrictions**.

| API | Tools |
| --- | --- |
| Geocoding API | `place_resolve`, `place_search` (`near`) |
| Places API | `place_search`; reviews on `place_resolve` |
| Directions API | `route_eta` |

`link_resolve` needs none. Do not enable Distance Matrix. Full steps: [README.md](README.md#auth).

## Sisters

| What | Path |
| --- | --- |
| Host + naming | `/home/christopher/ai-gantry` (`docs/mcp-naming.md`) |
| Stdio MCP scaffold | `/home/christopher/feeds-mcp` |
| One env secret | `/home/christopher/twitter-mcp` (`X_BEARER_TOKEN` → `GOOGLE_MAPS_API_KEY`) |
| Workspace OAuth (not this binary) | `/home/christopher/google-mcp` |
