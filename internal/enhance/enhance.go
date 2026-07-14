// Package enhance implements a Claude-backed taste.Enhancer. It reads a mood
// query the lexicon could not fully parse and returns a refined intent, so a
// phrasing like "something cozy for a rainy day" resolves to the genres,
// themes, and era a person means rather than the literal words. The enhancer
// is optional: when no API key is configured the site runs on the lexicon
// alone.
package enhance

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/dcadolph/cinatlas/internal/taste"
)

// DefaultModel is the model used when none is configured.
const DefaultModel = "claude-opus-4-8"

// maxTokens caps the model's response. The reply is one small tool call, so a
// modest ceiling is plenty.
const maxTokens = 1024

// toolName is the single tool the model must call to return its reading.
const toolName = "set_intent"

// Client is a Claude-backed taste.Enhancer.
type Client struct {
	// api is the Anthropic client.
	api anthropic.Client
	// model is the model id to call.
	model string
	// log receives diagnostics.
	log *slog.Logger
}

// New returns a Claude enhancer for the given API key. An empty model falls
// back to DefaultModel. It panics on an empty key or nil logger, which are
// developer errors, since callers gate construction on a configured key.
func New(apiKey, model string, log *slog.Logger) *Client {
	if apiKey == "" {
		panic("enhance.New: api key required")
	}
	if log == nil {
		panic("enhance.New: logger required")
	}
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		api:   anthropic.NewClient(option.WithAPIKey(apiKey)),
		model: model,
		log:   log,
	}
}

// systemPrompt tells the model how to read a mood into discovery filters.
const systemPrompt = `You translate a moviegoer's plain-language mood into movie discovery filters.
Read what they actually want to watch, not the literal words. "cozy rainy day" is warm low-stakes drama and comedy, not children's animation. "sexy" is adult romance and eroticism. "mind-bending" is psychological thrillers and twists.
Call the set_intent tool exactly once with your reading. Pick genres only from the allowed list. Use keywords for concrete themes the genres do not capture (animals, heist, time travel, dark, feel-good). Set min_rating or min_votes only when the request implies quality or fame ("acclaimed", "memorable", "hidden gem"). Set years only when an era is implied. Choose sort by what matters most: rating for quality requests, recent for new releases, otherwise popularity.`

// Enhance asks the model to refine the lexicon intent and merges its reading.
// On any error it returns the base intent unchanged so discovery still runs.
func (c *Client) Enhance(ctx context.Context, query string, base taste.Intent) (taste.Intent, error) {
	msg, err := c.api.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: maxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock("Mood: " + query)),
		},
		Tools: []anthropic.ToolUnionParam{{OfTool: &anthropic.ToolParam{
			Name:        toolName,
			Description: anthropic.String("Record the discovery filters that match the mood."),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: schemaProperties()},
		}}},
		ToolChoice: anthropic.ToolChoiceUnionParam{
			OfTool: &anthropic.ToolChoiceToolParam{Name: toolName},
		},
	})
	if err != nil {
		return base, fmt.Errorf("enhance: message: %w", err)
	}

	reading, ok := c.readTool(msg)
	if !ok {
		return base, fmt.Errorf("enhance: no tool call in response")
	}
	return base.MergeAI(reading), nil
}

// readTool pulls the set_intent call out of the response and decodes it.
func (c *Client) readTool(msg *anthropic.Message) (taste.AIReading, bool) {
	for _, block := range msg.Content {
		use, ok := block.AsAny().(anthropic.ToolUseBlock)
		if !ok || use.Name != toolName {
			continue
		}
		var dto intentDTO
		if err := json.Unmarshal([]byte(use.JSON.Input.Raw()), &dto); err != nil {
			c.log.Error("enhance decode failed", "err", err)
			return taste.AIReading{}, false
		}
		return dto.reading(), true
	}
	return taste.AIReading{}, false
}

// intentDTO is the tool input shape the model fills in.
type intentDTO struct {
	Genres        []string `json:"genres"`
	ExcludeGenres []string `json:"exclude_genres"`
	Keywords      []string `json:"keywords"`
	MinRating     float64  `json:"min_rating"`
	MinVotes      int      `json:"min_votes"`
	YearFrom      int      `json:"year_from"`
	YearTo        int      `json:"year_to"`
	Sort          string   `json:"sort"`
}

// reading converts the decoded tool input to a taste.AIReading.
func (d intentDTO) reading() taste.AIReading {
	return taste.AIReading{
		Genres:        d.Genres,
		ExcludeGenres: d.ExcludeGenres,
		Keywords:      d.Keywords,
		MinRating:     d.MinRating,
		MinVotes:      d.MinVotes,
		YearFrom:      d.YearFrom,
		YearTo:        d.YearTo,
		Sort:          d.Sort,
	}
}

// schemaProperties builds the tool input schema, offering only genre names the
// lexicon can resolve.
func schemaProperties() map[string]any {
	genreEnum := make([]any, 0)
	for _, name := range taste.GenreNames() {
		genreEnum = append(genreEnum, name)
	}
	genreArray := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string", "enum": genreEnum},
	}
	stringArray := map[string]any{
		"type":  "array",
		"items": map[string]any{"type": "string"},
	}
	return map[string]any{
		"genres":         genreArray,
		"exclude_genres": genreArray,
		"keywords":       stringArray,
		"min_rating":     map[string]any{"type": "number", "description": "0-10, only when quality is implied"},
		"min_votes":      map[string]any{"type": "integer", "description": "vote-count floor, only when fame is implied"},
		"year_from":      map[string]any{"type": "integer", "description": "earliest release year"},
		"year_to":        map[string]any{"type": "integer", "description": "latest release year"},
		"sort":           map[string]any{"type": "string", "enum": []any{"popularity", "rating", "recent"}},
	}
}
