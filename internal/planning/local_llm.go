// Copyright (c) 2026 Seb. All rights reserved.

// Package planning talks to explicitly configured local LLM servers. It returns
// drafts only; applying a draft to Time Keeper is a separate approval action.
package planning

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/model"
)

const maxPromptBytes = 100_000
const maxResponseBytes = 1 << 20

// Client invokes a configured local inference server with bounded requests.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a client with a conservative inference timeout.
func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	} else {
		clientCopy := *httpClient
		httpClient = &clientCopy
	}
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return &Client{httpClient: httpClient}
}

// GenerateDraft returns model text for review; it never applies hierarchy changes.
func (c *Client) GenerateDraft(ctx context.Context, pipeline model.LLMPipeline, prompt string) (string, error) {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" || len(prompt) > maxPromptBytes {
		return "", errors.New("planning prompt must contain at most 100000 bytes")
	}
	var endpoint string
	var body any
	switch pipeline.Provider {
	case "ollama":
		endpoint = pipeline.BaseURL + "/api/chat"
		body = map[string]any{"model": pipeline.Model, "stream": false, "messages": messages(pipeline.SystemPrompt, prompt), "options": map[string]any{"temperature": 0.2, "num_predict": 2048}}
	case "openai-compatible":
		endpoint = pipeline.BaseURL + "/v1/chat/completions"
		body = map[string]any{"model": pipeline.Model, "messages": messages(pipeline.SystemPrompt, prompt), "temperature": 0.2, "max_tokens": 2048}
	default:
		return "", fmt.Errorf("unsupported local LLM provider %q", pipeline.Provider)
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("encode planning request: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(encoded))
	if err != nil {
		return "", fmt.Errorf("create planning request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := c.httpClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("call local LLM: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return "", fmt.Errorf("read local LLM response: %w", err)
	}
	if len(payload) > maxResponseBytes {
		return "", errors.New("local LLM response exceeds 1 MiB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", fmt.Errorf("local LLM returned %s: %s", response.Status, strings.TrimSpace(string(payload)))
	}
	var text string
	if pipeline.Provider == "ollama" {
		var result struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(payload, &result); err != nil {
			return "", fmt.Errorf("decode Ollama response: %w", err)
		}
		text = result.Message.Content
	} else {
		var result struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(payload, &result); err != nil {
			return "", fmt.Errorf("decode OpenAI-compatible response: %w", err)
		}
		if len(result.Choices) > 0 {
			text = result.Choices[0].Message.Content
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("local LLM returned no planning text")
	}
	return text, nil
}

func messages(systemPrompt, prompt string) []map[string]string {
	items := make([]map[string]string, 0, 2)
	if systemPrompt = strings.TrimSpace(systemPrompt); systemPrompt != "" {
		items = append(items, map[string]string{"role": "system", "content": systemPrompt})
	}
	return append(items, map[string]string{"role": "user", "content": prompt})
}
