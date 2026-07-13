// Package ddd looks up crowdsourced content trigger flags on DoesTheDogDie.
package ddd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/dcadolph/cinatlas/internal/family"
)

// defaultBaseURL is the DoesTheDogDie site root used for all API calls.
const defaultBaseURL = "https://www.doesthedogdie.com"

// TriggerSource looks up content trigger flags for a film by title and year.
type TriggerSource interface {
	TriggersFor(ctx context.Context, title string, year int) (map[string]family.Trigger, error)
}

// TriggerSourceFunc adapts a function to the TriggerSource interface.
type TriggerSourceFunc func(ctx context.Context, title string, year int) (map[string]family.Trigger, error)

// TriggersFor calls the wrapped function.
func (f TriggerSourceFunc) TriggersFor(ctx context.Context, title string, year int) (map[string]family.Trigger, error) {
	return f(ctx, title, year)
}

// Media is one search result on DoesTheDogDie.
type Media struct {
	// ID is the DoesTheDogDie media identifier.
	ID int
	// Name is the display title.
	Name string
	// Year is the release year, zero when unknown.
	Year int
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets the HTTP client, letting callers wire caching transports.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) { c.http = h }
}

// WithBaseURL overrides the API root, used by tests.
func WithBaseURL(base string) Option {
	return func(c *Client) { c.base = strings.TrimRight(base, "/") }
}

// Client calls the DoesTheDogDie API.
type Client struct {
	// key is the API key sent in the X-API-KEY header.
	key string
	// base is the API root without a trailing slash.
	base string
	// http performs the requests.
	http *http.Client
}

// New returns a Client for the given API key.
func New(key string, opts ...Option) (*Client, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, ErrNoKey
	}
	c := &Client{key: key, base: defaultBaseURL, http: http.DefaultClient}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// Search returns media matching the query.
func (c *Client) Search(ctx context.Context, query string) ([]Media, error) {
	var out searchDTO
	q := url.Values{"q": []string{query}}
	if err := c.get(ctx, "/dddsearch", q, &out); err != nil {
		return nil, err
	}
	media := make([]Media, 0, len(out.Items))
	for _, item := range out.Items {
		media = append(media, item.toMedia())
	}
	return media, nil
}

// Triggers returns the trigger flags for a media id, keyed by cinatlas topic key.
// Only topics in the curated mapping appear; a present flag wins over an absent one
// when several DoesTheDogDie topics map to the same key.
func (c *Client) Triggers(ctx context.Context, id int) (map[string]family.Trigger, error) {
	var out mediaDTO
	if err := c.get(ctx, "/media/"+strconv.Itoa(id), nil, &out); err != nil {
		return nil, err
	}
	flags := map[string]family.Trigger{}
	for _, stat := range out.TopicItemStats {
		key, ok := topicKeys[strings.ToLower(strings.TrimSpace(stat.Topic.Name))]
		if !ok {
			continue
		}
		state := stat.trigger()
		if state == family.TriggerUnknown {
			continue
		}
		if flags[key] != family.TriggerYes {
			flags[key] = state
		}
	}
	return flags, nil
}

// TriggersFor searches for the film and returns its trigger flags. The result with a
// matching title and year wins, then a matching title, then the first result. A film
// with no search hit returns an empty map, which the fit engine reports as unverified.
func (c *Client) TriggersFor(ctx context.Context, title string, year int) (map[string]family.Trigger, error) {
	media, err := c.Search(ctx, title)
	if err != nil {
		return nil, err
	}
	if len(media) == 0 {
		return map[string]family.Trigger{}, nil
	}
	best := media[0]
	for _, m := range media {
		if !strings.EqualFold(m.Name, title) {
			continue
		}
		if m.Year == year {
			best = m
			break
		}
		if !strings.EqualFold(best.Name, title) {
			best = m
		}
	}
	return c.Triggers(ctx, best.ID)
}

// get performs one authenticated GET and decodes the JSON body into out.
func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRequest, err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-API-KEY", c.key)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrRequest, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.Copy(io.Discard, resp.Body) //nolint:errcheck // Drain for connection reuse.
		return fmt.Errorf("%w: %s: %s", ErrStatus, path, resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("%w: %w", ErrDecodeBody, err)
	}
	return nil
}

// searchDTO is the wire shape of a search response.
type searchDTO struct {
	// Items lists the matching media.
	Items []searchItemDTO `json:"items"`
}

// searchItemDTO is one media entry in a search response.
type searchItemDTO struct {
	// ID is the media identifier.
	ID int `json:"id"`
	// Name is the display title.
	Name string `json:"name"`
	// ReleaseYear is the release year as a string, sometimes empty.
	ReleaseYear string `json:"releaseYear"`
}

// toMedia converts the wire entry to the exported type.
func (s searchItemDTO) toMedia() Media {
	year, _ := strconv.Atoi(strings.TrimSpace(s.ReleaseYear)) //nolint:errcheck // Zero on bad year.
	return Media{ID: s.ID, Name: s.Name, Year: year}
}

// mediaDTO is the wire shape of a media detail response.
type mediaDTO struct {
	// TopicItemStats lists the vote tallies per content topic.
	TopicItemStats []topicStatDTO `json:"topicItemStats"`
}

// topicStatDTO is the vote tally for one topic on one media item.
type topicStatDTO struct {
	// YesSum counts votes saying the topic occurs.
	YesSum int `json:"yesSum"`
	// NoSum counts votes saying the topic does not occur.
	NoSum int `json:"noSum"`
	// Topic describes the content topic being voted on.
	Topic topicDTO `json:"topic"`
}

// topicDTO is the content topic metadata.
type topicDTO struct {
	// Name is the topic phrase, such as "a dog dies".
	Name string `json:"name"`
}

// trigger converts vote tallies to a trigger state by simple majority.
func (t topicStatDTO) trigger() family.Trigger {
	switch {
	case t.YesSum == 0 && t.NoSum == 0:
		return family.TriggerUnknown
	case t.YesSum > t.NoSum:
		return family.TriggerYes
	default:
		return family.TriggerNo
	}
}
