package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	token      string
	baseURL    string
	httpClient *http.Client
}

func NewClient(token string) *Client {
	return &Client{
		token:   token,
		baseURL: fmt.Sprintf("https://api.telegram.org/bot%s", token),
		httpClient: &http.Client{
			Timeout: 45 * time.Second,
		},
	}
}

func (c *Client) GetMe() (*User, error) {
	url := fmt.Sprintf("%s/getMe", c.baseURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to call getMe: %w", err)
	}
	defer resp.Body.Close()

	var apiResp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      User   `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		return nil, fmt.Errorf("failed to decode getMe response: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("getMe returned not ok: %s", apiResp.Description)
	}

	return &apiResp.Result, nil
}

func (c *Client) SetMyCommands(commands []BotCommand) error {
	payload := SetMyCommandsPayload{Commands: commands}
	return c.post("setMyCommands", payload, nil)
}

func (c *Client) SendMessage(chatID interface{}, text string, replyMarkup interface{}) error {
	payload := SendMessagePayload{
		ChatID:      chatID,
		Text:        text,
		ParseMode:   "HTML",
		ReplyMarkup: replyMarkup,
	}
	return c.post("sendMessage", payload, nil)
}

func (c *Client) AnswerCallbackQuery(callbackID string, text string, showAlert bool) error {
	payload := AnswerCallbackQueryPayload{
		CallbackQueryID: callbackID,
		Text:            text,
		ShowAlert:       showAlert,
	}
	return c.post("answerCallbackQuery", payload, nil)
}

func (c *Client) GetUpdates(offset int64, timeout int) ([]Update, error) {
	url := fmt.Sprintf("%s/getUpdates?offset=%d&timeout=%d", c.baseURL, offset, timeout)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to get updates: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiResp struct {
		OK          bool     `json:"ok"`
		Description string   `json:"description"`
		Result      []Update `json:"result"`
	}

	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal updates: %w", err)
	}

	if !apiResp.OK {
		return nil, fmt.Errorf("telegram API error: %s", apiResp.Description)
	}

	return apiResp.Result, nil
}

func (c *Client) post(method string, payload interface{}, result interface{}) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/%s", c.baseURL, method)
	resp, err := c.httpClient.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to send request to %s: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read body from %s: %w", method, err)
	}

	var baseResp APIResponse
	if err := json.Unmarshal(respBody, &baseResp); err != nil {
		return fmt.Errorf("failed to unmarshal api response: %w", err)
	}

	if !baseResp.OK {
		return fmt.Errorf("telegram api error (%s): %s", method, baseResp.Description)
	}

	if result != nil {
		return json.Unmarshal(respBody, result)
	}

	return nil
}
