<img src="internal/web/static/banner.png" width="100%" alt="cinatlas projector banner">

# cinatlas

Quick movie facts from the command line. Point it at a movie or a person and get
the one answer you wanted, without opening IMDB or asking a voice assistant.

It reads as "cinema atlas": a reference and a literal map. Filming locations resolve
to coordinates and link straight to Google Maps and Google Earth.

## What it answers

- Where a movie was filmed, on a map.
- Who is in a movie, and who directed it.
- What else a person was in or directed.
- Who someone is and their notable roles.

## Install

```
go install github.com/dcadolph/cinatlas@latest
```

Or build from a clone:

```
go build -o cinatlas .
```

## Configure

Data comes from TMDB, which needs a free key. Create one at themoviedb.org under
Settings, then API. A v3 key or a v4 read access token both work.

```
export CINATLAS_TMDB_KEY=your_key_here
```

## Use

```
cinatlas where "No Country for Old Men"
cinatlas at "Monument Valley"
cinatlas cast Heat
cinatlas films "Denis Villeneuve"
cinatlas who "Stephen Tobolowsky"
cinatlas serve
```

`at` searches the other direction: name a place, get the films with recorded
locations there. Coverage mirrors Wikidata, so film hubs are rich and small
towns can be empty.

`serve` runs the cinatlas website locally on 127.0.0.1:8878 and opens it in your
browser: one search box, poster and cast cards, filming locations on an embedded
map. Same data, same cache, zero setup beyond the key.

Output is human-readable text by default. Add `--json` for machine output, and
`--pretty` to indent that JSON. Logs go to stderr, so data and diagnostics never mix.

## Caching

Successful responses are cached on disk for 24 hours, so repeat questions answer
instantly and stay clear of API rate limits. The cache lives in the OS user cache
directory under `cinatlas`. Use `--refresh` to bypass it, `CINATLAS_CACHE_DIR` to
move it, and `CINATLAS_CACHE_TTL` (a Go duration like `1h` or `72h`) to change
freshness.

## Where the locations come from

Locations merge from tiers, each labeled by source in the output: Wikidata
filming locations (structured), Wikipedia mining (street-level places linked
from the article's filming section), and production countries as the coarse
fallback. Films with zero pins link the IMDB locations page instead. Where the
story is *set* shows separately from where it *filmed*.

On the website every movie gets an interactive globe of its pins: the
"Open the globe" button, or `/globe?id=<tmdb id>`.

## Data sources

TMDB supplies cast, crew, filmography, images, and IMDB ids. Wikidata supplies
filming locations, settings, and coordinates. Wikipedia supplies street-level
filming places. IMDB is a link target only.

## Status

Early. The command surface and data path are in place. Response caching, richer
location coverage, and the web interface are next. See the design plan for the
roadmap.
