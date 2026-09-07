package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

const apiBase = "https://api.telegram.org/bot"

// Client is a minimal wrapper around the Telegram Bot API — just enough
// for the notifications and mini-app flows the spec calls for
// (temporary passwords, QR check-in pings, balance checks, PDF reports).
type Client struct {
	botToken   string
	httpClient *http.Client
}

func NewClient(botToken string) *Client {
	return &Client{
		botToken:   botToken,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// Enabled reports whether a bot token was configured. Callers should
// no-op (not error) when it's false, so the platform still works in
// environments where the Telegram integration hasn't been set up yet.
func (c *Client) Enabled() bool {
	return c.botToken != ""
}

// SetWebhook registers the URL Telegram should POST updates to, along
// with a secret token that gets echoed back in every request's
// X-Telegram-Bot-Api-Secret-Token header so the handler can verify the
// call actually came from Telegram. Run this once after deploying (or
// whenever the public URL changes) — e.g. from a small init script or
// an admin CLI command, not on every server boot.
func (c *Client) SetWebhook(ctx context.Context, url, secretToken string) error {
	if !c.Enabled() {
		return nil
	}
	payload := map[string]string{"url": url, "secret_token": secretToken}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+c.botToken+"/setWebhook", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram setWebhook failed: status %d", resp.StatusCode)
	}
	return nil
}

type sendMessagePayload struct {
	ChatID    string `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

// SendMessage sends a plain-text (or Markdown, if parseMode is set)
// message to a chat. chatID is the parent's telegram_chat_id captured
// during the /start linking flow.
func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	if !c.Enabled() {
		return nil
	}
	payload := sendMessagePayload{ChatID: chatID, Text: text, ParseMode: "Markdown"}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+c.botToken+"/sendMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram sendMessage failed: status %d", resp.StatusCode)
	}
	return nil
}

// SendDocument uploads a file (e.g. the monthly progress PDF) to a chat.
func (c *Client) SendDocument(ctx context.Context, chatID, filename string, fileBytes []byte, caption string) error {
	if !c.Enabled() {
		return nil
	}

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	if err := writer.WriteField("chat_id", chatID); err != nil {
		return err
	}
	if caption != "" {
		if err := writer.WriteField("caption", caption); err != nil {
			return err
		}
	}
	part, err := writer.CreateFormFile("document", filename)
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, bytes.NewReader(fileBytes)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		apiBase+c.botToken+"/sendDocument", &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram sendDocument failed: status %d", resp.StatusCode)
	}
	return nil
}
