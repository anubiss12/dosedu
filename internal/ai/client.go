package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Client wraps the Anthropic Messages API for one narrow use: explaining
// why a quiz answer was wrong, in plain Kazakh, for a language-learner
// audience. It is deliberately optional — until a real API key/
// subscription is configured, Enabled() is false and callers get a
// friendly placeholder instead of an error.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

func NewClient(apiKey string) *Client {
	return &Client{apiKey: apiKey, httpClient: &http.Client{Timeout: 20 * time.Second}}
}

func (c *Client) Enabled() bool { return c.apiKey != "" }

const comingSoonMessage = "AI түсіндірмесі жақында қосылады — жазылым белсендірілгенде осы жерден қолжетімді болады."

// explainLanguageName maps a UI language code to the name the prompt
// asks the model to answer in. Any student can pick any explanation
// language regardless of which language they're learning — defaults
// to Kazakh for an unrecognized/empty code.
func explainLanguageName(code string) string {
	switch code {
	case "ru":
		return "Russian"
	case "en":
		return "English"
	case "zh":
		return "Chinese"
	default:
		return "Kazakh"
	}
}

// ExplainMistake returns a short, encouraging explanation — in
// explainLanguage, chosen by the student, independent of the language
// they're learning — of why the student's answer was wrong. When no
// API key is configured it returns the "coming soon" placeholder
// instead of an error, so the quiz UI never breaks waiting on this.
func (c *Client) ExplainMistake(ctx context.Context, question, chosenAnswer, correctAnswer, learningLanguage, explainLanguage string) (string, error) {
	if !c.Enabled() {
		return comingSoonMessage, nil
	}

	prompt := fmt.Sprintf(
		"Question: %s\nStudent's answer: %s\nCorrect answer: %s\nLanguage being learned: %s\n\n"+
			"Explain briefly (2-3 sentences, in %s) why the correct answer is right and the "+
			"student's choice is wrong. Keep it encouraging and simple for a language learner.",
		question, chosenAnswer, correctAnswer, learningLanguage, explainLanguageName(explainLanguage),
	)

	payload := map[string]any{
		"model":      "claude-haiku-4-5-20251001",
		"max_tokens": 300,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai api error: status %d", resp.StatusCode)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Content) == 0 {
		return "", errors.New("empty ai response")
	}
	return result.Content[0].Text, nil
}
