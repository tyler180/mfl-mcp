# mfl-mcp

A read-only [Model Context Protocol](https://modelcontextprotocol.io/) server for the [MyFantasyLeague API](https://api.myfantasyleague.com/2026/api_info), written in Go.

The server exposes read-only tools over stdio. General league tools:

- `get_league`
- `get_rosters`
- `get_standings`
- `get_transactions`
- `get_players`

Draft-model data tools:

- `get_rules` and `get_all_rules`
- `get_free_agents`
- `get_rookie_adp`
- `get_assets` and `get_future_draft_picks`
- `get_draft_results` and `get_live_draft_results`
- `get_player_scores` and `get_projected_scores`
- `get_player_profiles`
- `get_salary_adjustments`

`get_rookie_adp` reads MFL's aggregate ADP from completed real rookie-only
drafts. It defaults to recent 12-team drafts across scoring formats and a 5%
minimum selection rate, which keeps MFL's documented lower-bound behavior
predictable while covering roughly six rookie rounds during draft season.

MFL import and other mutating operations are intentionally not exposed.

## Requirements

- Go 1.25 or newer
- An MFL league ID
- For private league data, either an MFL owner API key or an `MFL_USER_ID` cookie value

## Configure

Set configuration in the MCP client's environment:

```sh
export MFL_YEAR=2026
export MFL_LEAGUE_ID=12345
export MFL_API_KEY='your-owner-api-key'
export MFL_FRANCHISE_ID=0005 # consumed by dynasty-ff-draft-model, not mfl-mcp
```

Instead of `MFL_API_KEY`, you can set `MFL_USER_COOKIE` to the value of the `MFL_USER_ID` cookie. Do not commit either secret. The API key is safer for read-only owner access; MFL documents that it cannot authorize import requests or commissioner-only exports.

Optional settings:

| Variable | Default | Purpose |
| --- | --- | --- |
| `MFL_BASE_URL` | `https://api.myfantasyleague.com` | MFL API origin or a test server |
| `MFL_USER_AGENT` | `mfl-mcp/0.2.0` | Registered MFL API client user agent |
| `MFL_TIMEOUT` | `20s` | HTTP request timeout |
| `MFL_MIN_INTERVAL` | `1s` | Minimum start-to-start spacing between MFL requests; set `0s` only for a test server |

MFL seasons do not always match the calendar year in January, so set `MFL_YEAR` explicitly when working with an earlier season.

## Build and test

```sh
make test
make build
```

The binary is written to `bin/mfl-mcp`.

## MCP client configuration

Build the binary, then add a stdio server entry to your MCP client. For example:

```json
{
  "mcpServers": {
    "mfl": {
      "command": "/absolute/path/to/mfl-mcp/bin/mfl-mcp",
      "env": {
        "MFL_YEAR": "2026",
        "MFL_LEAGUE_ID": "12345",
        "MFL_API_KEY": "your-owner-api-key"
      }
    }
  }
}
```

Keep credentials in your client's secret/environment configuration rather than source control.

## Draft-model workflow

The MCP server is the live MFL acquisition layer for the adjacent
`dynasty-ff-draft-model` project. It intentionally returns MFL's original JSON
instead of embedding the optimizer's schema. An MCP client or sync adapter can
therefore refresh league data without coupling this server to one version of
the optimizer.

For a draft snapshot, collect these exports:

| Draft input | MCP tools |
| --- | --- |
| League limits, franchises, and lineup slots | `get_league` |
| Scoring configuration | `get_rules`; `get_all_rules` for abbreviation definitions |
| Current roster placement, salary, and contracts | `get_rosters` |
| Player names and positions | `get_players` |
| Birthdates and ADP, in batches of at most 100 IDs | `get_player_profiles` |
| Available player pool | `get_free_agents` |
| Current and future pick ownership | `get_assets`, `get_future_draft_picks` |
| Players already selected before or after the draft | `get_draft_results` |
| Near-real-time picks and on-the-clock state during a draft | `get_live_draft_results` |
| Historical league-scored production | `get_player_scores` with `week: "YTD"` or `week: "AVG"` and the appropriate season |
| Current weekly projections | `get_projected_scores` |
| Non-player cap charges or credits | `get_salary_adjustments` |

Example MCP arguments:

```json
{"week":"YTD","player_ids":["15751","15418"]}
```

```json
{"position":"WR"}
```

```json
{"player_ids":["15751","15418"]}
```

Those examples correspond to `get_player_scores`, `get_free_agents`, and
`get_player_profiles`, respectively. Player IDs remain strings so leading
zeroes are not lost.

`get_live_draft_results` follows MFL's documented dynamic XML feed. On its
first call for a league and season, the server resolves and remembers the
league's actual MFL host because this feed does not support the redirects used
by regular export requests. `unit` defaults to `LEAGUE`; Deluxe leagues can
pass their configured unit, such as `CONFERENCE00`.

MFL's `projectedScores` export is weekly. It does not replace the multi-season
rookie and veteran projections required for a defensible dynasty optimization.
The adapter must merge those projections and any league rules maintained
outside MFL before invoking the draft model. Use `get_live_draft_results`
during the draft; MFL says the regular `get_draft_results` export may lag by up
to 15 minutes.

## API behavior

Regular export tools request JSON from MFL and return both MCP text content and structured content. The live-draft tool returns the documented XML feed as text plus a structured wrapper containing `format` and `xml`. Export calls follow MFL redirects, and the resolved league host is remembered for live-draft requests.

MFL asks API clients to cache slow-changing data, space requests apart, and handle HTTP 429 responses without immediate retries. The server spaces request starts by `MFL_MIN_INTERVAL` and does not automatically retry failed or throttled calls. In particular, clients should cache `get_players`, `get_all_rules`, and other slow-changing league configuration rather than requesting them for every model run. Server-side response caching remains planned work.
