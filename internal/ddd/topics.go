package ddd

import "strings"

// Topic is one content trigger offered as a hard veto.
type Topic struct {
	// Key is the stable identifier stored in profiles.
	Key string
	// Label is the display name shown in the profile builder.
	Label string
	// Names lists the DoesTheDogDie topic names that map to this key. A yes on
	// any of them flags the key.
	Names []string
}

// Category groups related topics under a heading in the profile builder.
type Category struct {
	// Name is the heading shown above the category's topics.
	Name string
	// Topics lists the vetoes in the category in display order.
	Topics []Topic
}

// Catalog is the curated set of hard vetoes, grouped by category. It is a broad
// but deliberate subset of the DoesTheDogDie taxonomy: concrete content triggers a
// family might avoid, not production notes, politics, or spoiler flags. Every name
// in Names was taken from live API responses on 2026-07-13.
var Catalog = []Category{
	{Name: "Animals", Topics: []Topic{
		{Key: "animal-death", Label: "An animal dies", Names: []string{"an animal dies", "there's a dead animal"}},
		{Key: "dog-death", Label: "A dog dies", Names: []string{"a dog dies"}},
		{Key: "cat-death", Label: "A cat dies", Names: []string{"a cat dies"}},
		{Key: "horse-death", Label: "A horse dies", Names: []string{"a horse dies"}},
		{Key: "pet-death", Label: "A pet dies", Names: []string{"a pet dies"}},
		{Key: "animal-abuse", Label: "Animals are abused", Names: []string{"animals are abused", "there's dog fighting"}},
	}},
	{Name: "Death & loss", Topics: []Topic{
		{Key: "parent-death", Label: "A parent dies", Names: []string{"a parent dies"}},
		{Key: "child-death", Label: "A child dies", Names: []string{"a kid dies", "a baby is stillborn"}},
		{Key: "family-death", Label: "A family member dies", Names: []string{"a family member dies"}},
		{Key: "major-death", Label: "A major character dies", Names: []string{"a major character dies"}},
		{Key: "toy-destroyed", Label: "A cherished toy is destroyed", Names: []string{"A child's dear toy is destroyed"}},
		{Key: "sad-ending", Label: "The ending is sad", Names: []string{"the ending is sad"}},
	}},
	{Name: "Scary & supernatural", Topics: []Topic{
		{Key: "jump-scares", Label: "Jump scares", Names: []string{"there are jump scares"}},
		{Key: "ghosts", Label: "Ghosts", Names: []string{"there's ghosts"}},
		{Key: "possession", Label: "Possession", Names: []string{"someone is possessed"}},
		{Key: "demons", Label: "Demons or Hell", Names: []string{"there's demons or Hell"}},
		{Key: "clowns", Label: "Clowns", Names: []string{"there are clowns"}},
		{Key: "body-horror", Label: "Body horror", Names: []string{"there's body horror"}},
		{Key: "claustrophobia", Label: "Claustrophobic scenes", Names: []string{"there's a claustrophobic scene"}},
	}},
	{Name: "Phobias", Topics: []Topic{
		{Key: "spiders", Label: "Spiders", Names: []string{"there are spiders"}},
		{Key: "snakes", Label: "Snakes", Names: []string{"there are snakes"}},
		{Key: "bugs", Label: "Bugs", Names: []string{"there are bugs"}},
		{Key: "sharks", Label: "Sharks", Names: []string{"there are sharks"}},
		{Key: "gators", Label: "Alligators or crocodiles", Names: []string{"there's an alligator or crocodile"}},
		{Key: "deep-water", Label: "Open water", Names: []string{"there's natural bodies of water"}},
		{Key: "trypophobia", Label: "Trypophobic content", Names: []string{"trypophobic content is shown"}},
	}},
	{Name: "Violence & gore", Topics: []Topic{
		{Key: "blood-gore", Label: "Blood or gore", Names: []string{"there's blood/gore", "there's excessive gore"}},
		{Key: "gun-violence", Label: "Gun violence", Names: []string{"there's gun violence"}},
		{Key: "torture", Label: "Torture", Names: []string{"there's torture"}},
		{Key: "stabbing", Label: "Someone is stabbed", Names: []string{"someone is stabbed"}},
		{Key: "decapitation", Label: "Decapitation", Names: []string{"there's decapitation"}},
		{Key: "burned-alive", Label: "Someone is burned alive", Names: []string{"someone is burned alive"}},
		{Key: "buried-alive", Label: "Someone is buried alive", Names: []string{"someone is buried alive"}},
	}},
	{Name: "Heavy themes", Topics: []Topic{
		{Key: "domestic-violence", Label: "Domestic violence", Names: []string{"there's domestic violence"}},
		{Key: "child-abuse", Label: "Child abuse", Names: []string{"there's child abuse"}},
		{Key: "kidnapping", Label: "Kidnapping", Names: []string{"someone is kidnapped", "an infant is abducted"}},
		{Key: "self-harm", Label: "Self harm", Names: []string{"someone self harms"}},
		{Key: "suicide", Label: "Suicide", Names: []string{"someone dies by suicide", "Someone attempts suicide"}},
		{Key: "sexual-assault", Label: "Sexual assault", Names: []string{"someone is sexually assaulted", "sexual assault is mentioned"}},
	}},
	{Name: "Medical", Topics: []Topic{
		{Key: "needles", Label: "Needles", Names: []string{"needles/syringes are used"}},
		{Key: "hospital", Label: "Hospital scenes", Names: []string{"there's a hospital scene"}},
		{Key: "cancer", Label: "Cancer", Names: []string{"someone has cancer"}},
		{Key: "seizure", Label: "Seizures", Names: []string{"someone has a seizure"}},
		{Key: "heart-attack", Label: "Heart attack", Names: []string{"someone has a heart attack"}},
	}},
	{Name: "Gross-out", Topics: []Topic{
		{Key: "vomit", Label: "Vomiting", Names: []string{"someone vomits"}},
		{Key: "defecation", Label: "Defecation", Names: []string{"someone defecates"}},
		{Key: "spitting", Label: "Spitting", Names: []string{"there's spitting"}},
	}},
	{Name: "Vehicles", Topics: []Topic{
		{Key: "car-crash", Label: "Car crash", Names: []string{"a car crashes", "a person is hit by a car"}},
		{Key: "plane-crash", Label: "Plane crash", Names: []string{"a plane crashes"}},
	}},
	{Name: "Mature content", Topics: []Topic{
		{Key: "sexual-content", Label: "Sexual content", Names: []string{"there is sexual content"}},
		{Key: "nudity", Label: "Nudity", Names: []string{"there are nude scenes"}},
		{Key: "drug-use", Label: "Drug use", Names: []string{"someone uses drugs", "someone overdoses"}},
		{Key: "alcohol-abuse", Label: "Alcohol abuse", Names: []string{"alcohol abuse"}},
	}},
}

// topicKeys maps lowercased DoesTheDogDie topic names to curated keys. Built once
// from Catalog so the grouped list stays the single source of truth.
var topicKeys = buildTopicKeys()

// buildTopicKeys inverts Catalog into the name-to-key lookup used at match time.
func buildTopicKeys() map[string]string {
	keys := make(map[string]string)
	for _, cat := range Catalog {
		for _, topic := range cat.Topics {
			for _, name := range topic.Names {
				keys[strings.ToLower(name)] = topic.Key
			}
		}
	}
	return keys
}
