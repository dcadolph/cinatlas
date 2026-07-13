package ddd

// Topic is one curated content trigger offered as a hard veto.
type Topic struct {
	// Key is the stable identifier stored in profiles.
	Key string
	// Label is the display name shown in the profile builder.
	Label string
}

// Topics lists the curated vetoes in display order. The list is deliberately short:
// common kid triggers, not the full DoesTheDogDie taxonomy.
var Topics = []Topic{
	{Key: "animal-death", Label: "An animal dies"},
	{Key: "child-death", Label: "A child dies"},
	{Key: "parent-death", Label: "A parent dies"},
	{Key: "jump-scares", Label: "Jump scares"},
	{Key: "blood-gore", Label: "Blood or gore"},
	{Key: "monsters", Label: "Scary monsters"},
	{Key: "clowns", Label: "Clowns"},
	{Key: "spiders", Label: "Spiders"},
	{Key: "snakes", Label: "Snakes"},
	{Key: "needles", Label: "Needles"},
	{Key: "vomit", Label: "Vomiting"},
	{Key: "thunder", Label: "Thunderstorms"},
}

// topicKeys maps lowercased DoesTheDogDie topic names to curated topic keys. Several
// upstream topics can fold into one key; a yes on any of them flags the key. The
// upstream names come from observed API responses and must be re-checked against the
// live API whenever the curated list changes.
var topicKeys = map[string]string{
	"a dog dies":                "animal-death",
	"a cat dies":                "animal-death",
	"a horse dies":              "animal-death",
	"an animal dies":            "animal-death",
	"a pet dies":                "animal-death",
	"a kid dies":                "child-death",
	"a baby dies":               "child-death",
	"a parent dies":             "parent-death",
	"there are jump scares":     "jump-scares",
	"there's blood/gore":        "blood-gore",
	"there's body horror":       "monsters",
	"there's a clown":           "clowns",
	"there are spiders":         "spiders",
	"there are snakes":          "snakes",
	"needles/syringes are used": "needles",
	"someone vomits":            "vomit",
	"there's a thunderstorm":    "thunder",
}
