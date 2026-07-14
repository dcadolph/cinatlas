// Package taste turns a plain-language mood into a set of discovery filters.
// Someone types "a feel-good movie with animals that go on an adventure" and
// gets back the genres, keyword themes, era, and quality floor that describe
// it, ready to hand to TMDB's discover endpoint. The lexicon does this with no
// model behind it; an optional Enhancer refines the read when one is wired.
package taste

import (
	"context"
	"sort"
	"strconv"
	"strings"
)

// TMDB movie genre ids, the stable identifiers the discover endpoint filters
// by. They never change, so hard-coding them keeps the lexicon self-contained.
const (
	genreAction      = 28
	genreAdventure   = 12
	genreAnimation   = 16
	genreComedy      = 35
	genreCrime       = 80
	genreDocumentary = 99
	genreDrama       = 18
	genreFamily      = 10751
	genreFantasy     = 14
	genreHistory     = 36
	genreHorror      = 27
	genreMusic       = 10402
	genreMystery     = 9648
	genreRomance     = 10749
	genreSciFi       = 878
	genreThriller    = 53
	genreWar         = 10752
	genreWestern     = 37
)

// Sort orders TMDB understands, named so the lexicon reads clearly.
const (
	sortPopular = "popularity.desc"
	sortVoted   = "vote_count.desc"
	sortRated   = "vote_average.desc"
	sortRecent  = "primary_release_date.desc"
)

// Intent is a structured reading of what a viewer asked for. Its zero value is
// a valid "anything popular" search; each matched phrase fills more of it in.
type Intent struct {
	// Genres are the TMDB genre ids the mood implies, combined as AND.
	Genres []int
	// ExcludeGenres are genre ids to filter out, such as horror on a
	// wholesome request.
	ExcludeGenres []int
	// Keywords are theme phrases to resolve to TMDB keyword ids, combined as
	// OR so any one of them qualifies a film.
	Keywords []string
	// MinRating is the lowest acceptable TMDB vote average, zero for any.
	MinRating float64
	// MinVotes is the lowest acceptable vote count, a proxy for how known and
	// vetted a film is.
	MinVotes int
	// YearFrom bounds the earliest release year, zero for no bound.
	YearFrom int
	// YearTo bounds the latest release year, zero for no bound.
	YearTo int
	// Sort is the TMDB ordering to request.
	Sort string
}

// rule maps trigger phrases to the intent fragment they contribute. A rule
// fires when any of its phrases appears in the lowercased query.
type rule struct {
	// phrases are the substrings that trigger the rule.
	phrases []string
	// genres are added to the intent when the rule fires.
	genres []int
	// exclude are added to the excluded genres when the rule fires.
	exclude []int
	// keywords are theme terms added when the rule fires.
	keywords []string
	// minRating raises the rating floor when higher than the current one.
	minRating float64
	// minVotes raises the vote-count floor when higher than the current one.
	minVotes int
	// yearFrom sets the earliest year when the rule fires.
	yearFrom int
	// yearTo sets the latest year when the rule fires.
	yearTo int
	// sort overrides the ordering when set.
	sort string
}

// lexicon is the hand-built vocabulary. Order does not matter: every matching
// rule contributes, so blended moods like "funny and scary" stack genres.
var lexicon = []rule{
	// Tone and feel. Feel-good leans comedy with warm-theme keywords rather than
	// the Family genre, which skews the results toward children's animation.
	{phrases: []string{"feel good", "feel-good", "feelgood", "heartwarming", "wholesome", "uplifting", "cozy", "cosy", "comforting", "comfort", "warm", "rainy day", "rainy-day", "curl up", "curled up"},
		genres: []int{genreComedy}, exclude: []int{genreHorror}, keywords: []string{"feel-good", "heartwarming"}},
	{phrases: []string{"sad", "tearjerker", "tear jerker", "cry", "emotional", "heartbreaking", "moving"},
		genres: []int{genreDrama}, keywords: []string{"tragedy", "melancholy"}},
	{phrases: []string{"funny", "hilarious", "laugh", "comedy", "goofy", "silly", "lighthearted", "light-hearted"},
		genres: []int{genreComedy}},
	{phrases: []string{"dark", "gritty", "bleak", "grim"}, keywords: []string{"dark", "gritty"}},
	{phrases: []string{"feel-bad", "disturbing", "unsettling"}, keywords: []string{"disturbing"}},

	// Energy and action.
	{phrases: []string{"action", "explosions", "adrenaline", "high octane", "high-octane", "fast paced", "fast-paced"},
		genres: []int{genreAction}},
	{phrases: []string{"thriller", "suspense", "suspenseful", "edge of your seat", "tense", "gripping"},
		genres: []int{genreThriller}},
	{phrases: []string{"adventure", "journey", "quest", "epic", "expedition"}, genres: []int{genreAdventure}, keywords: []string{"epic"}},
	{phrases: []string{"heist", "robbery"}, genres: []int{genreCrime}, keywords: []string{"heist"}},
	{phrases: []string{"revenge", "vengeance"}, keywords: []string{"revenge"}},
	{phrases: []string{"survival", "stranded"}, keywords: []string{"survival"}},

	// Romance and intimacy.
	{phrases: []string{"romantic", "romance", "love story", "date night"}, genres: []int{genreRomance}},
	{phrases: []string{"sexy", "steamy", "erotic", "sensual", "seductive", "sultry", "raunchy"},
		genres: []int{genreRomance}, keywords: []string{"erotic", "sensuality", "seduction"}},

	// Fear and the strange.
	{phrases: []string{"scary", "horror", "terrifying", "creepy", "frightening", "spooky", "haunting"},
		genres: []int{genreHorror}},
	{phrases: []string{"slasher", "gory", "gruesome", "bloody"}, genres: []int{genreHorror}, keywords: []string{"slasher", "gore"}},
	{phrases: []string{"supernatural", "ghost", "haunted", "paranormal"}, genres: []int{genreHorror}, keywords: []string{"supernatural", "ghost"}},

	// Worlds and ideas.
	{phrases: []string{"sci-fi", "scifi", "science fiction", "space", "aliens", "futuristic", "dystopian", "cyberpunk"},
		genres: []int{genreSciFi}},
	{phrases: []string{"fantasy", "magic", "wizards", "dragons", "mythical"}, genres: []int{genreFantasy}},
	{phrases: []string{"mind bending", "mind-bending", "cerebral", "twist", "twisty", "psychological"},
		genres: []int{genreMystery}, keywords: []string{"plot twist", "psychological"}},
	{phrases: []string{"mystery", "whodunit", "detective", "noir"}, genres: []int{genreMystery}},

	// People and places.
	{phrases: []string{"animals", "animal", "dogs", "dog", "cats", "pets", "wildlife"}, keywords: []string{"animal", "dog"}},
	{phrases: []string{"family", "kids", "children", "for the whole family"}, genres: []int{genreFamily}, exclude: []int{genreHorror}},
	{phrases: []string{"coming of age", "coming-of-age", "growing up", "teen"}, genres: []int{genreDrama}, keywords: []string{"coming-of-age"}},
	{phrases: []string{"superhero", "comic book", "comic-book"}, genres: []int{genreAction}, keywords: []string{"superhero", "based on comic"}},
	{phrases: []string{"war", "wartime", "soldiers", "battlefield"}, genres: []int{genreWar}},
	{phrases: []string{"western", "cowboys", "wild west"}, genres: []int{genreWestern}},
	{phrases: []string{"musical", "music", "singing", "band"}, genres: []int{genreMusic}},
	{phrases: []string{"true story", "based on a true story", "real events", "biopic", "biographical"},
		genres: []int{genreHistory}, keywords: []string{"based on true story", "biography"}},
	{phrases: []string{"documentary", "docu", "real life"}, genres: []int{genreDocumentary}},
	{phrases: []string{"crime", "gangster", "mafia", "mobster"}, genres: []int{genreCrime}},

	// Memorability and quality.
	{phrases: []string{"memorable", "iconic", "unforgettable", "classic", "timeless", "legendary"},
		minVotes: 3000, minRating: 7, sort: sortRated},
	{phrases: []string{"acclaimed", "award winning", "award-winning", "critically acclaimed", "masterpiece", "best"},
		minRating: 7.5, minVotes: 1500, sort: sortRated},
	{phrases: []string{"underrated", "hidden gem", "overlooked", "underappreciated"}, minRating: 7, minVotes: 200},
	{phrases: []string{"popular", "blockbuster", "crowd pleaser", "crowd-pleaser", "mainstream"}, minVotes: 2000, sort: sortPopular},

	// Era.
	{phrases: []string{"classic", "old", "golden age", "black and white", "black-and-white"}, yearTo: 1979},
	{phrases: []string{"recent", "new", "modern", "latest"}, yearFrom: 2018, sort: sortRecent},
	{phrases: []string{"90s", "nineties"}, yearFrom: 1990, yearTo: 1999},
	{phrases: []string{"80s", "eighties"}, yearFrom: 1980, yearTo: 1989},
	{phrases: []string{"2000s"}, yearFrom: 2000, yearTo: 2009},
}

// baselineVotes keeps discovery from surfacing near-unrated obscurities when
// the query implies nothing about how known a film should be.
const baselineVotes = 200

// genreBreadth ranks how broad a genre is so a narrowing fallback keeps the
// most specific genre. Lower is more specific; genres absent here rank zero.
var genreBreadth = map[int]int{
	genreComedy: 3, genreDrama: 3,
	genreAction: 2, genreAdventure: 2, genreFamily: 2, genreThriller: 2,
}

// genreNames labels the genre ids the lexicon uses for display.
var genreNames = map[int]string{
	genreAction: "Action", genreAdventure: "Adventure", genreAnimation: "Animation",
	genreComedy: "Comedy", genreCrime: "Crime", genreDocumentary: "Documentary",
	genreDrama: "Drama", genreFamily: "Family", genreFantasy: "Fantasy",
	genreHistory: "History", genreHorror: "Horror", genreMusic: "Music",
	genreMystery: "Mystery", genreRomance: "Romance", genreSciFi: "Sci-Fi",
	genreThriller: "Thriller", genreWar: "War", genreWestern: "Western",
}

// Labels renders the intent as human-readable chips describing what the search
// looked for, so the viewer can see how their words were read.
func (i Intent) Labels() []string {
	var chips []string
	for _, g := range i.Genres {
		if name, ok := genreNames[g]; ok {
			chips = append(chips, name)
		}
	}
	chips = append(chips, i.Keywords...)
	switch {
	case i.YearFrom > 0 && i.YearTo > 0:
		chips = append(chips, era(i.YearFrom)+"–"+era(i.YearTo))
	case i.YearFrom > 0:
		chips = append(chips, era(i.YearFrom)+" and newer")
	case i.YearTo > 0:
		chips = append(chips, era(i.YearTo)+" and older")
	}
	if i.MinRating > 0 {
		chips = append(chips, "highly rated")
	}
	return chips
}

// era renders a year for a chip label.
func era(year int) string {
	return strconv.Itoa(year)
}

// Enhancer refines a lexicon Intent from the raw query, typically by asking a
// model to read phrasing the vocabulary missed. Implementations must be safe
// to call with any query and should return the input Intent unchanged when
// they have nothing to add.
type Enhancer interface {
	// Enhance returns a possibly-refined Intent for the query.
	Enhance(ctx context.Context, query string, base Intent) (Intent, error)
}

// AIReading is a model's structured read of a mood query, in display terms.
// Genres carry names rather than ids so the model works in vocabulary it knows;
// MergeAI resolves them.
type AIReading struct {
	// Genres are display genre names the film should match.
	Genres []string
	// ExcludeGenres are display genre names to filter out.
	ExcludeGenres []string
	// Keywords are theme terms to resolve to TMDB keyword ids.
	Keywords []string
	// MinRating is the lowest acceptable vote average, zero for any.
	MinRating float64
	// MinVotes is the lowest acceptable vote count, zero for any.
	MinVotes int
	// YearFrom bounds the earliest release year, zero for no bound.
	YearFrom int
	// YearTo bounds the latest release year, zero for no bound.
	YearTo int
	// Sort is a friendly ordering: popularity, rating, or recent.
	Sort string
}

// GenreNames returns the display genre names the lexicon recognizes, sorted, so
// an enhancer can offer them to a model as the allowed set.
func GenreNames() []string {
	names := make([]string, 0, len(genreNames))
	for _, name := range genreNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// MergeAI folds a model's reading into the lexicon Intent. A model that named
// genres replaces the lexicon's genre guess outright, since it reads nuance the
// vocabulary cannot; everything else combines conservatively so the model only
// ever adds signal. The result is renormalized and ready to query.
func (base Intent) MergeAI(r AIReading) Intent {
	out := base
	if genres := genreIDs(r.Genres); len(genres) > 0 {
		out.Genres = genres
	}
	out.ExcludeGenres = append(out.ExcludeGenres, genreIDs(r.ExcludeGenres)...)
	out.Keywords = append(out.Keywords, r.Keywords...)
	if r.MinRating > out.MinRating {
		out.MinRating = r.MinRating
	}
	if r.MinVotes > out.MinVotes {
		out.MinVotes = r.MinVotes
	}
	if r.YearFrom != 0 {
		out.YearFrom = r.YearFrom
	}
	if r.YearTo != 0 {
		out.YearTo = r.YearTo
	}
	if sort := aiSort(r.Sort); sort != "" {
		out.Sort = sort
	}
	return normalize(out)
}

// genreIDs maps display genre names to ids, dropping names it does not know.
func genreIDs(names []string) []int {
	var ids []int
	for _, name := range names {
		if id, ok := genreIDByName(name); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// genreIDByName resolves a display genre name to its id, case-insensitively.
func genreIDByName(name string) (int, bool) {
	want := strings.TrimSpace(strings.ToLower(name))
	for id, display := range genreNames {
		if strings.ToLower(display) == want {
			return id, true
		}
	}
	return 0, false
}

// aiSort maps a friendly ordering word to a TMDB sort, empty when unrecognized.
func aiSort(word string) string {
	switch strings.ToLower(strings.TrimSpace(word)) {
	case "popularity", "popular":
		return sortPopular
	case "rating", "rated", "quality":
		return sortRated
	case "recent", "new", "newest":
		return sortRecent
	default:
		return ""
	}
}

// EnhancerFunc adapts a function to the Enhancer interface.
type EnhancerFunc func(ctx context.Context, query string, base Intent) (Intent, error)

// Enhance calls the underlying function.
func (f EnhancerFunc) Enhance(ctx context.Context, query string, base Intent) (Intent, error) {
	return f(ctx, query, base)
}

// Parse reads a mood query into an Intent using the lexicon alone. It always
// returns a usable Intent; an empty or unrecognized query yields a popular
// baseline rather than nothing.
func Parse(query string) Intent {
	q := normalizeText(query)
	var intent Intent
	for _, r := range lexicon {
		if !matchesAny(q, r.phrases) {
			continue
		}
		intent.Genres = append(intent.Genres, r.genres...)
		intent.ExcludeGenres = append(intent.ExcludeGenres, r.exclude...)
		intent.Keywords = append(intent.Keywords, r.keywords...)
		if r.minRating > intent.MinRating {
			intent.MinRating = r.minRating
		}
		if r.minVotes > intent.MinVotes {
			intent.MinVotes = r.minVotes
		}
		if r.yearFrom != 0 {
			intent.YearFrom = r.yearFrom
		}
		if r.yearTo != 0 {
			intent.YearTo = r.yearTo
		}
		if r.sort != "" {
			intent.Sort = r.sort
		}
	}
	return normalize(intent)
}

// normalize dedupes genre and keyword lists, drops excluded genres from the
// included set, and fills sensible defaults so the Intent is ready to query.
func normalize(intent Intent) Intent {
	excluded := make(map[int]bool, len(intent.ExcludeGenres))
	for _, g := range intent.ExcludeGenres {
		excluded[g] = true
	}
	intent.Genres = dedupeInts(intent.Genres, excluded)
	// Order most-specific first so a narrowing fallback keeps the distinctive
	// genre (Romance) over a broad one (Comedy).
	sort.SliceStable(intent.Genres, func(i, j int) bool {
		return genreBreadth[intent.Genres[i]] < genreBreadth[intent.Genres[j]]
	})
	intent.ExcludeGenres = dedupeInts(intent.ExcludeGenres, nil)
	intent.Keywords = dedupeStrings(intent.Keywords)
	// Default to the most-voted films so a mood returns titles people actually
	// know and watched, not whatever spiked in popularity this week.
	if intent.Sort == "" {
		intent.Sort = sortVoted
	}
	if intent.MinVotes < baselineVotes {
		intent.MinVotes = baselineVotes
	}
	return intent
}

// matchesAny reports whether any phrase appears in the normalized query as a
// whole word or phrase, so "war" does not fire inside "warm".
func matchesAny(query string, phrases []string) bool {
	padded := " " + query + " "
	for _, p := range phrases {
		if strings.Contains(padded, " "+normalizeText(p)+" ") {
			return true
		}
	}
	return false
}

// normalizeText lowercases and reduces a string to space-separated words,
// turning punctuation and hyphens into spaces so "feel-good", "feel good", and
// "action, sexy" all match cleanly.
func normalizeText(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// dedupeInts returns the unique values in order, skipping any in drop.
func dedupeInts(in []int, drop map[int]bool) []int {
	seen := make(map[int]bool, len(in))
	var out []int
	for _, v := range in {
		if seen[v] || drop[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// dedupeStrings returns the unique values in sorted order for a stable query.
func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	var out []string
	for _, v := range in {
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
