// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package pricing contains the server-owned model price catalogue. Usage
// payloads may name a model, but they never supply a price. A missing or
// unknown model therefore remains explicitly unpriced.
package pricing

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"strings"
)

const (
	SchemaVersion   = "timekeeper-model-prices/v1"
	EnvFile         = "TIMEKEEPER_MODEL_PRICES_FILE"
	maxFileBytes    = 128 << 10
	unitsPerMillion = int64(1_000_000)
)

// Rate stores micro-units of the catalogue currency per million tokens.
// Zero is a valid rate (for example, a local model), so model presence is the
// authority for whether pricing is trusted.
type Rate struct {
	InputPerMillionMicros         int64 `json:"input_per_million_micros"`
	OutputPerMillionMicros        int64 `json:"output_per_million_micros"`
	CacheCreationPerMillionMicros int64 `json:"cache_creation_per_million_micros"`
	CacheReadPerMillionMicros     int64 `json:"cache_read_per_million_micros"`
}

// Catalog is immutable after construction and safe for concurrent reads.
type Catalog struct {
	source   string
	currency string
	rates    map[string]Rate
}

// Estimate is the result of applying a trusted catalogue entry.
type Estimate struct {
	Status       string
	Currency     string
	AmountMicros int64
	Source       string
	Reason       string
}

// NewCatalog validates and copies a server-owned price map.
func NewCatalog(source, currency string, rates map[string]Rate) (Catalog, error) {
	source = strings.TrimSpace(source)
	currency = strings.ToUpper(strings.TrimSpace(currency))
	if source == "" {
		return Catalog{}, errors.New("pricing source is required")
	}
	if len(currency) != 3 {
		return Catalog{}, errors.New("pricing currency must be a three-letter code")
	}
	for _, r := range currency {
		if r < 'A' || r > 'Z' {
			return Catalog{}, errors.New("pricing currency must contain only ASCII letters")
		}
	}
	copied := make(map[string]Rate, len(rates))
	for model, rate := range rates {
		model = strings.TrimSpace(model)
		if model == "" {
			return Catalog{}, errors.New("pricing model name is required")
		}
		if rate.InputPerMillionMicros < 0 || rate.OutputPerMillionMicros < 0 || rate.CacheCreationPerMillionMicros < 0 || rate.CacheReadPerMillionMicros < 0 {
			return Catalog{}, fmt.Errorf("pricing rates for %q must be non-negative", model)
		}
		copied[model] = rate
	}
	return Catalog{source: source, currency: currency, rates: copied}, nil
}

// Empty returns the safe default when no trusted local source is configured.
func Empty() Catalog { return Catalog{rates: map[string]Rate{}} }

// LoadFromEnv loads the operator-provided catalogue, or returns an empty
// catalogue when TIMEKEEPER_MODEL_PRICES_FILE is not configured.
func LoadFromEnv() (Catalog, error) {
	path := strings.TrimSpace(os.Getenv(EnvFile))
	if path == "" {
		return Empty(), nil
	}
	return LoadFile(path)
}

type fileFormat struct {
	SchemaVersion string          `json:"schema_version"`
	Source        string          `json:"source"`
	Currency      string          `json:"currency"`
	Models        map[string]Rate `json:"models"`
}

// LoadFile loads a bounded JSON catalogue. Unknown fields and trailing data
// are rejected so an accidental or stale file cannot silently alter pricing.
func LoadFile(path string) (Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return Catalog{}, fmt.Errorf("open model price catalogue: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil {
		return Catalog{}, fmt.Errorf("read model price catalogue: %w", err)
	}
	if len(data) > maxFileBytes {
		return Catalog{}, fmt.Errorf("model price catalogue exceeds %d bytes", maxFileBytes)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var document fileFormat
	if err := decoder.Decode(&document); err != nil {
		return Catalog{}, fmt.Errorf("decode model price catalogue: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return Catalog{}, errors.New("model price catalogue must contain one JSON object")
	}
	if document.SchemaVersion != SchemaVersion {
		return Catalog{}, fmt.Errorf("unsupported model price schema %q", document.SchemaVersion)
	}
	return NewCatalog(document.Source, document.Currency, document.Models)
}

// Estimate prices one cumulative session total. A missing model is never
// assigned a guessed fallback.
func (c Catalog) Estimate(model string, input, output, cacheCreation, cacheRead int64) Estimate {
	if input < 0 || output < 0 || cacheCreation < 0 || cacheRead < 0 {
		return Estimate{Status: "unpriced", Reason: "usage counters must be non-negative"}
	}
	model = strings.TrimSpace(model)
	rate, ok := c.rates[model]
	if !ok {
		reason := "no trusted local model price is configured"
		if model != "" {
			reason = fmt.Sprintf("no trusted local model price is configured for %q", model)
		}
		return Estimate{Status: "unpriced", Reason: reason}
	}
	amount, err := sumRoundedCosts([]int64{input, output, cacheCreation, cacheRead}, []int64{
		rate.InputPerMillionMicros, rate.OutputPerMillionMicros, rate.CacheCreationPerMillionMicros, rate.CacheReadPerMillionMicros,
	})
	if err != nil {
		return Estimate{Status: "unpriced", Reason: "priced usage exceeds the supported amount range"}
	}
	return Estimate{Status: "priced", Currency: c.currency, AmountMicros: amount, Source: c.source}
}

func sumRoundedCosts(tokens, rates []int64) (int64, error) {
	total := big.NewInt(0)
	for i := range tokens {
		if tokens[i] == 0 || rates[i] == 0 {
			continue
		}
		numerator := new(big.Int).Mul(big.NewInt(tokens[i]), big.NewInt(rates[i]))
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(numerator, big.NewInt(unitsPerMillion), remainder)
		if remainder.Sign() > 0 {
			quotient.Add(quotient, big.NewInt(1))
		}
		total.Add(total, quotient)
	}
	if !total.IsInt64() || total.Sign() < 0 || total.Cmp(big.NewInt(math.MaxInt64)) > 0 {
		return 0, errors.New("cost overflow")
	}
	return total.Int64(), nil
}
