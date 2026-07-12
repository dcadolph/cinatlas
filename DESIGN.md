# cinatlas Design

## Overview

cinatlas answers the quick questions people actually ask while watching something,
without opening IMDB, asking a voice assistant, or running a web search. It is a
small, fast reference and map for film. Point it at a movie or a person and it
returns the one fact you wanted.

The name reads as "cinema atlas": a reference compendium and a literal map. The map
half is real. Filming locations resolve to coordinates, link straight to Google Maps
and Google Earth, and render on an interactive globe.

## Core questions

1. What else was this person in? (person to filmography)
2. Who is that, and where do I know them from? (name to person and notable roles)
3. Where was this filmed? (title to real-world locations on a map)
4. Where can I watch this right now? (title to the services carrying it in your
   region, split into what a subscription already covers and what costs extra)

Plus the natural adjacent: what else did this director make, who directed this, and
what is worth watching next.

## Non-goals

- Not a tracker, watchlist, or social feed. Those exist and are crowded.
- Not a review aggregator. It reports facts and links out for the rest.

## Data sources

| Source | Role | Notes |
|---|---|---|
| TMDB | Cast, crew, filmography, posters, backdrops, trending, IMDB ids | Free key required |
| TMDB watch/providers | Streaming availability by region and access kind, watch-page link | JustWatch-backed, appended to the movie call |
| Wikidata | Filming locations (P915), settings (P840), countries (P495), coordinates (P625), Wikipedia sitelink | One SPARQL query per film |
| Wikipedia | Street-level filming places mined from the article's filming section | Links with coordinates only |
| IMDB | Outbound deep links: title, name, locations, full credits | No API used |

## Location pipeline

Locations are the moat, so they merge from tiers in `internal/locate`:

1. Wikidata P915 filming locations, the structured base.
2. Wikipedia mining: find the film's article via the Wikidata sitelink, find its
   filming or production section, collect the place articles linked there, resolve
   their coordinates in one batch call. A mined place with the same name upgrades an
   unresolved base entry; new places append. Links without coordinates drop, which
   filters people and films out naturally.
3. Production countries as the coarse fallback when nothing else exists.

Every location carries its source, shown in the UI. Zero-pin films link the IMDB
locations page instead of dead-ending. Settings (P840) display separately as
"Set in", never mixed with where the film shot.

## Availability

Availability answers "where can I watch this, truly, right now." TMDB's
watch/providers data is JustWatch-backed and rides along on the movie call
through append_to_response, so it costs no extra request. The response is keyed
by country; the client keeps a region, default US, and reads only that entry.

Each service is grouped by access kind, ordered best-for-the-viewer first:
stream (included in a subscription), free, ads, rent, buy. The split between
included and pay-extra is the point. A web search tells you a title is "on"
a service; it rarely tells you whether your subscription already covers it.

Personalization stays local. The viewer names the services they subscribe to
and each matching provider is tagged as already theirs. A service token matches
a provider name as a case-insensitive substring, so "prime" matches "Amazon
Prime Video." On the CLI the list comes from a flag or environment variable; on
the web it lives in browser localStorage and never leaves the device. Nothing
about a viewer's subscriptions is stored server-side or sent anywhere.

Availability rotates as licensing windows open and close, so it is the most
perishable data cinatlas holds. The response cache TTL governs freshness; the
region is always shown so a stale or wrong-country answer is never silent.

## Architecture

The CLI is a thin shell over small packages a web server reuses without change.

- `cmd/` holds command dispatch and flag handling.
- `internal/tmdb` is the TMDB client: search, details, filmography, trending,
  now-playing, upcoming, recommendations.
- `internal/wikidata` resolves place facts and the Wikipedia article in one query.
- `internal/wikipedia` mines the filming section for coordinate-bearing places.
- `internal/locate` merges the tiers into one answer for both CLI and web.
- `internal/imdb` builds outbound links. Pure functions, no network.
- `internal/httpcache` is a caching HTTP transport shared by every client.
  Successful GET responses land on disk and answer repeat questions without the
  network. Cache keys strip the API key.
- `internal/web` is the site: templates and static assets embedded, handlers over
  the same packages, plus the interactive globe.
- `internal/model` holds the shared types; `internal/render` the human CLI output;
  `internal/jsonutil` and `internal/logutil` centralize output and logging.

Logging goes to stderr. Data goes to stdout, human-readable by default and JSON
under `--json`. Standard library only; the web map uses MapLibre GL from a CDN in
the browser, not a Go dependency.

## Command surface

| Command | Answers |
|---|---|
| `cinatlas where <title>` | Where it filmed and where it is set |
| `cinatlas at <place>` | Which films shot at a place, reverse lookup |
| `cinatlas cast <title>` | Who is in it and who directed |
| `cinatlas watch <title>` | Where it streams now, split into included and pay-extra |
| `cinatlas films <person>` | What else they made |
| `cinatlas who <name>` | Who that is and their notable roles |
| `cinatlas serve` | The website, locally |
| `cinatlas version` | Build version |

Global flags: `--json`, `--pretty`, `--refresh` to bypass the cache,
`--region` for watch availability, `--services` to tag what you already have,
`--log-level`.

## Website

`cinatlas serve` on 127.0.0.1:8878. Cinema-styled: Bebas Neue poster titles,
Playfair italic taglines, marquee gold on deep black, film grain, marquee light
strip, dark and light modes. Home is three poster walls: trending, in theaters,
coming soon. Movie pages: backdrop hero, chips, top-billed cast shelf, locations
with source badges and an OpenStreetMap embed, a full-page interactive globe with
every pin, set-in chips, more-like-this row. A where-to-watch block leads the
body: provider chips grouped into included and pay-extra, a "your services" box
that greens the ones you already have, and a link to all watch options.
Person pages: portrait hero and a poster-shelf filmography with merged roles.

## Configuration

| Setting | Environment variable | Required |
|---|---|---|
| TMDB API key | `CINATLAS_TMDB_KEY` | Yes |
| Output mode | `CINATLAS_JSON` | No |
| Output format | `CINATLAS_PRETTY` | No |
| Watch region | `CINATLAS_REGION` | No |
| Your services | `CINATLAS_SERVICES` | No |
| Log level | `CINATLAS_LOG_LEVEL` | No |
| Cache directory | `CINATLAS_CACHE_DIR` | No |
| Cache freshness | `CINATLAS_CACHE_TTL` | No |

## Roadmap

Done: CLI, disk cache, website with discovery walls, tiered location pipeline,
interactive globe, watch availability with local personalization.

Next: a phrase-to-title search layer is possible with an LLM at real cost. Public
hosting waits until the data and look earn it; the domain cinatlas.com is already
secured. Mobile wraps the same backend later.

## Open questions

- Face and voice lookup for "who is that" is out of reach without an image
  pipeline. Name and title lookup stand in.
- Whether the full flag, environment, file, and default config layer is worth
  building before the data path is proven end to end.
