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
cinatlas cast Heat
cinatlas films "Denis Villeneuve"
cinatlas who "Stephen Tobolowsky"
```

Output is human-readable text by default. Add `--json` for machine output, and
`--pretty` to indent that JSON. Logs go to stderr, so data and diagnostics never mix.

## Caching

Successful responses are cached on disk for 24 hours, so repeat questions answer
instantly and stay clear of API rate limits. The cache lives in the OS user cache
directory under `cinatlas`. Use `--refresh` to bypass it, `CINATLAS_CACHE_DIR` to
move it, and `CINATLAS_CACHE_TTL` (a Go duration like `1h` or `72h`) to change
freshness.

## Data sources

TMDB supplies cast, crew, filmography, and IMDB ids. Wikidata supplies filming
locations with coordinates through property P915. IMDB is a link target only.

## Status

Early. The command surface and data path are in place. Response caching, richer
location coverage, and the web interface are next. See the design plan for the
roadmap.
