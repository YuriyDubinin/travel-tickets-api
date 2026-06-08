// Package telegram is an outbound client for the Telegram Bot API, used to post
// messages to a configured channel.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	defaultBaseURL   = "https://api.telegram.org"
	defaultTimeout   = 10 * time.Second
	maxResponseBytes = 1 << 20 // 1 MiB cap on the response body
)

// Config configures the client. Kept local so this package does not depend on
// the application's config package.
type Config struct {
	BaseURL               string
	BotToken              string
	ChannelID             string
	ParseMode             string
	DisableWebPagePreview bool
	Timeout               time.Duration
}

// Client talks to the Telegram Bot API.
type Client struct {
	httpClient            *http.Client
	baseURL               string
	botToken              string
	channelID             string
	parseMode             string
	disableWebPagePreview bool
}

// NewClient builds a Client, applying sensible defaults for empty settings.
func NewClient(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		httpClient:            &http.Client{Timeout: timeout},
		baseURL:               strings.TrimRight(baseURL, "/"),
		botToken:              cfg.BotToken,
		channelID:             cfg.ChannelID,
		parseMode:             cfg.ParseMode,
		disableWebPagePreview: cfg.DisableWebPagePreview,
	}
}

// SendMessage posts text to the configured channel.
func (c *Client) SendMessage(ctx context.Context, text string) error {
	return c.SendMessageTo(ctx, c.channelID, text)
}

type sendMessageRequest struct {
	ChatID                string `json:"chat_id"`
	Text                  string `json:"text"`
	ParseMode             string `json:"parse_mode,omitempty"`
	DisableWebPagePreview bool   `json:"disable_web_page_preview,omitempty"`
}

// SendMessageTo posts text to a specific chat/channel. chatID can be a channel
// username ("@channel") or a numeric id ("-100...").
func (c *Client) SendMessageTo(ctx context.Context, chatID, text string) error {
	if c.botToken == "" {
		return fmt.Errorf("telegram: bot token is not configured")
	}
	if chatID == "" {
		return fmt.Errorf("telegram: chat id is not configured")
	}

	body, err := json.Marshal(sendMessageRequest{
		ChatID:                chatID,
		Text:                  text,
		ParseMode:             c.parseMode,
		DisableWebPagePreview: c.disableWebPagePreview,
	})
	if err != nil {
		return fmt.Errorf("telegram: marshal request: %w", err)
	}

	respBody, err := c.do(ctx, "sendMessage", body)
	if err != nil {
		return err
	}

	var r struct {
		OK          bool   `json:"ok"`
		ErrorCode   int    `json:"error_code"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return fmt.Errorf("telegram: decode response: %w", err)
	}
	if !r.OK {
		return fmt.Errorf("telegram: sendMessage failed (%d): %s", r.ErrorCode, r.Description)
	}
	return nil
}

// BotInfo is a subset of the getMe response.
type BotInfo struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"first_name"`
}

// GetMe validates the bot token and returns basic bot info. Useful at startup to
// confirm the token works without posting anything.
func (c *Client) GetMe(ctx context.Context) (BotInfo, error) {
	if c.botToken == "" {
		return BotInfo{}, fmt.Errorf("telegram: bot token is not configured")
	}

	respBody, err := c.do(ctx, "getMe", nil)
	if err != nil {
		return BotInfo{}, err
	}

	var r struct {
		OK          bool    `json:"ok"`
		ErrorCode   int     `json:"error_code"`
		Description string  `json:"description"`
		Result      BotInfo `json:"result"`
	}
	if err := json.Unmarshal(respBody, &r); err != nil {
		return BotInfo{}, fmt.Errorf("telegram: decode response: %w", err)
	}
	if !r.OK {
		return BotInfo{}, fmt.Errorf("telegram: getMe failed (%d): %s", r.ErrorCode, r.Description)
	}
	return r.Result, nil
}

// do performs a POST to /bot<token>/<method>. A nil body sends no payload.
func (c *Client) do(ctx context.Context, method string, body []byte) ([]byte, error) {
	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.botToken, method)

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("telegram: new request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("telegram: read body: %w", err)
	}
	return respBody, nil
}
