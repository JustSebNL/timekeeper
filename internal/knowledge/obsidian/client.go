// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

package obsidian

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/JustSebNL/timekeeper/internal/knowledge"
)

const (
	MaxDescriptorBytes = 16 * 1024
	MaxTokenBytes      = 512
	MaxResponseBytes   = 128 * 1024
)

// EndpointDescriptor is the small, local-only discovery record written by the
// Obsidian plugin. The capability itself stays in token_file, never in Galaxy
// Markdown or a source-controlled configuration file.
type EndpointDescriptor struct {
	BaseURL         string `json:"base_url"`
	ProtocolVersion string `json:"protocol_version"`
	Provider        string `json:"provider"`
	VaultID         string `json:"vault_id"`
	TokenFile       string `json:"token_file"`
}

type Client struct {
	descriptorPath string
	httpClient     *http.Client
}

func NewClient(descriptorPath string) *Client {
	return &Client{
		descriptorPath: descriptorPath,
		httpClient: &http.Client{
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

func (c *Client) Status(ctx context.Context) (knowledge.ProviderStatus, error) {
	descriptor, token, err := c.load()
	if err != nil {
		return knowledge.ProviderStatus{Provider: knowledge.ProviderNameObsidian, Message: safeMessage(err)}, err
	}
	health, err := c.doJSON(ctx, descriptor, token, http.MethodGet, "/health", nil, false)
	if err != nil {
		return knowledge.ProviderStatus{Provider: descriptor.Provider, ProtocolVersion: descriptor.ProtocolVersion, VaultID: descriptor.VaultID, Message: safeMessage(err)}, err
	}
	var healthPayload struct {
		Status          string `json:"status"`
		ProtocolVersion string `json:"protocol_version"`
	}
	if err := json.Unmarshal(health, &healthPayload); err != nil {
		return knowledge.ProviderStatus{Provider: descriptor.Provider, ProtocolVersion: descriptor.ProtocolVersion, VaultID: descriptor.VaultID, Message: "invalid health response"}, &ProviderError{Code: "invalid_response", Message: "invalid health response", Retryable: true}
	}
	if healthPayload.Status != "ok" {
		return knowledge.ProviderStatus{Provider: descriptor.Provider, ProtocolVersion: descriptor.ProtocolVersion, VaultID: descriptor.VaultID, Message: "provider is not healthy"}, &ProviderError{Code: "provider_unavailable", Message: "provider is not healthy", Retryable: true}
	}
	statusBody, err := c.doJSON(ctx, descriptor, token, http.MethodGet, "/api/v1/galaxy/status", nil, true)
	if err != nil {
		return knowledge.ProviderStatus{Provider: descriptor.Provider, ProtocolVersion: descriptor.ProtocolVersion, VaultID: descriptor.VaultID, Message: safeMessage(err)}, err
	}
	var statusPayload struct {
		Available       bool     `json:"available"`
		ProtocolVersion string   `json:"protocol_version"`
		Capabilities    []string `json:"capabilities"`
		Message         string   `json:"message"`
	}
	if err := json.Unmarshal(statusBody, &statusPayload); err != nil {
		return knowledge.ProviderStatus{Provider: descriptor.Provider, ProtocolVersion: descriptor.ProtocolVersion, VaultID: descriptor.VaultID, Message: "invalid provider status response"}, &ProviderError{Code: "invalid_response", Message: "invalid provider status response", Retryable: true}
	}
	protocol := descriptor.ProtocolVersion
	if statusPayload.ProtocolVersion != "" {
		protocol = statusPayload.ProtocolVersion
	}
	if protocol != knowledge.ProtocolVersion {
		return knowledge.ProviderStatus{Provider: descriptor.Provider, ProtocolVersion: protocol, VaultID: descriptor.VaultID, Capabilities: statusPayload.Capabilities, Message: "provider protocol mismatch"}, &ProviderError{Code: "protocol_mismatch", Message: "provider protocol mismatch", Retryable: false}
	}
	return knowledge.ProviderStatus{Available: statusPayload.Available, Provider: descriptor.Provider, ProtocolVersion: protocol, VaultID: descriptor.VaultID, Capabilities: statusPayload.Capabilities, LastCheckedAt: time.Now().UTC(), Message: statusPayload.Message}, nil
}

func (c *Client) Search(ctx context.Context, query knowledge.KnowledgeQuery) ([]knowledge.KnowledgeResult, error) {
	descriptor, token, err := c.load()
	if err != nil {
		return nil, err
	}
	body, err := c.doJSON(ctx, descriptor, token, http.MethodPost, "/api/v1/galaxy/search", query, true)
	if err != nil {
		return nil, err
	}
	var response struct {
		Results []knowledge.KnowledgeResult `json:"results"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, &ProviderError{Code: "invalid_response", Message: "invalid search response", Retryable: true}
	}
	return response.Results, nil
}

func (c *Client) Get(ctx context.Context, id string) (knowledge.KnowledgeItem, error) {
	descriptor, token, err := c.load()
	if err != nil {
		return knowledge.KnowledgeItem{}, err
	}
	body, err := c.doJSON(ctx, descriptor, token, http.MethodGet, "/api/v1/galaxy/items/"+url.PathEscape(strings.TrimSpace(id)), nil, true)
	if err != nil {
		return knowledge.KnowledgeItem{}, err
	}
	var item knowledge.KnowledgeItem
	if err := json.Unmarshal(body, &item); err != nil {
		return knowledge.KnowledgeItem{}, &ProviderError{Code: "invalid_response", Message: "invalid Galaxy item response", Retryable: true}
	}
	return item, nil
}

func (c *Client) Create(ctx context.Context, candidate knowledge.KnowledgeCandidate, idempotencyKey string) (knowledge.KnowledgeItem, error) {
	descriptor, token, err := c.load()
	if err != nil {
		return knowledge.KnowledgeItem{}, err
	}
	payload := struct {
		knowledge.KnowledgeCandidate
		IdempotencyKey string `json:"idempotency_key"`
	}{KnowledgeCandidate: candidate, IdempotencyKey: strings.TrimSpace(idempotencyKey)}
	body, err := c.doJSON(ctx, descriptor, token, http.MethodPost, "/api/v1/galaxy/items", payload, true)
	if err != nil {
		return knowledge.KnowledgeItem{}, err
	}
	var item knowledge.KnowledgeItem
	if err := json.Unmarshal(body, &item); err != nil {
		return knowledge.KnowledgeItem{}, &ProviderError{Code: "invalid_response", Message: "invalid Galaxy create response", Retryable: true}
	}
	return item, nil
}

func (c *Client) load() (EndpointDescriptor, string, error) {
	if c == nil || strings.TrimSpace(c.descriptorPath) == "" {
		return EndpointDescriptor{}, "", &ProviderError{Code: "provider_unavailable", Message: "Obsidian endpoint is not configured", Retryable: false}
	}
	data, err := readBoundedFile(c.descriptorPath, MaxDescriptorBytes)
	if err != nil {
		return EndpointDescriptor{}, "", &ProviderError{Code: "provider_unavailable", Message: "Obsidian endpoint descriptor is unavailable", Retryable: true}
	}
	var descriptor EndpointDescriptor
	if err := json.Unmarshal(data, &descriptor); err != nil {
		return EndpointDescriptor{}, "", &ProviderError{Code: "invalid_descriptor", Message: "invalid Obsidian endpoint descriptor", Retryable: false}
	}
	if err := validateDescriptor(descriptor); err != nil {
		return EndpointDescriptor{}, "", err
	}
	tokenPath := descriptor.TokenFile
	if !filepath.IsAbs(tokenPath) {
		tokenPath = filepath.Join(filepath.Dir(c.descriptorPath), tokenPath)
	}
	tokenBytes, err := readBoundedFile(tokenPath, MaxTokenBytes)
	if err != nil {
		return EndpointDescriptor{}, "", &ProviderError{Code: "provider_unavailable", Message: "Obsidian capability is unavailable", Retryable: true}
	}
	token := strings.TrimSpace(string(tokenBytes))
	if token == "" || strings.ContainsAny(token, "\r\n") {
		return EndpointDescriptor{}, "", &ProviderError{Code: "invalid_capability", Message: "invalid Obsidian capability", Retryable: false}
	}
	return descriptor, token, nil
}

func validateDescriptor(descriptor EndpointDescriptor) error {
	if descriptor.Provider != knowledge.ProviderNameObsidian {
		return &ProviderError{Code: "provider_mismatch", Message: "unexpected knowledge provider", Retryable: false}
	}
	if descriptor.ProtocolVersion != knowledge.ProtocolVersion {
		return &ProviderError{Code: "protocol_mismatch", Message: "provider protocol mismatch", Retryable: false}
	}
	parsed, err := url.Parse(strings.TrimSpace(descriptor.BaseURL))
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return &ProviderError{Code: "invalid_endpoint", Message: "Obsidian endpoint must be plain loopback HTTP", Retryable: false}
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil || !ip.IsLoopback() || parsed.Port() == "" {
		return &ProviderError{Code: "invalid_endpoint", Message: "Obsidian endpoint must use a numeric loopback address and explicit port", Retryable: false}
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return &ProviderError{Code: "invalid_endpoint", Message: "Obsidian endpoint port is invalid", Retryable: false}
	}
	if strings.TrimSpace(descriptor.TokenFile) == "" || len(descriptor.TokenFile) > 1024 {
		return &ProviderError{Code: "invalid_descriptor", Message: "Obsidian capability reference is invalid", Retryable: false}
	}
	return nil
}

func (c *Client) doJSON(ctx context.Context, descriptor EndpointDescriptor, token, method, path string, input any, authenticated bool) ([]byte, error) {
	var body io.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			return nil, &ProviderError{Code: "invalid_request", Message: "could not encode provider request", Retryable: false}
		}
		if len(encoded) > knowledge.MaxCandidateContent+knowledge.MaxCandidateTitle+MaxDescriptorBytes {
			return nil, &ProviderError{Code: "request_too_large", Message: "provider request is too large", Retryable: false}
		}
		body = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(descriptor.BaseURL, "/")+path, body)
	if err != nil {
		return nil, &ProviderError{Code: "invalid_endpoint", Message: "could not create provider request", Retryable: false}
	}
	request.Header.Set("Accept", "application/json")
	if input != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, &ProviderError{Code: "provider_unavailable", Message: "Obsidian provider request failed", Retryable: true}
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, MaxResponseBytes+1)
	responseBody, err := io.ReadAll(limited)
	if err != nil || len(responseBody) > MaxResponseBytes {
		return nil, &ProviderError{Code: "response_too_large", Message: "Obsidian provider response is too large", Retryable: false}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, parseProviderError(response.StatusCode, responseBody)
	}
	return responseBody, nil
}

// ProviderError deliberately exposes only the typed provider envelope, never
// the endpoint, token reference, filesystem path, or raw response body.
//
// It aliases the provider-agnostic knowledge.ProviderError so the HTTP
// layer can classify a failure without importing this plugin.
type ProviderError = knowledge.ProviderError

func parseProviderError(status int, body []byte) error {
	var envelope struct {
		Error struct {
			Code      string `json:"code"`
			Message   string `json:"message"`
			Retryable bool   `json:"retryable"`
		} `json:"error"`
	}
	if json.Unmarshal(body, &envelope) == nil && envelope.Error.Code != "" {
		return &ProviderError{Code: envelope.Error.Code, Message: envelope.Error.Message, Retryable: envelope.Error.Retryable, Status: status}
	}
	return &ProviderError{Code: "provider_http_error", Message: "Obsidian provider returned an error", Retryable: status >= 500, Status: status}
}

func readBoundedFile(path string, limit int) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, errors.New("bounded file read failed")
	}
	return data, nil
}

func safeMessage(err error) string {
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Message
	}
	return "knowledge provider unavailable"
}

func contentHash(content []byte) string {
	hash := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(hash[:])
}

var _ knowledge.KnowledgeProvider = (*Client)(nil)
