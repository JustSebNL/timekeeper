// Copyright (c) 2026 https://github.com/JustSebNL. All rights reserved.

// Package semantic contains the dependency-free associative vector primitive.
// It is intentionally deterministic and rebuildable: SQLite remains the
// memory authority, while these vectors are only a local retrieval accelerator.
package semantic

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

const Dimension = 256

// Embed returns a normalized feature-hashed vector. Word features provide
// precision; character trigrams give related spelling/phrase surfaces without
// claiming to be a neural embedding model.
func Embed(text string) []float32 {
	vector := make([]float32, Dimension)
	words := tokenize(text)
	for _, word := range words {
		addFeature(vector, "w:"+word, 2)
		if len(word) >= 3 {
			for i := 0; i+3 <= len(word); i++ {
				addFeature(vector, "c:"+word[i:i+3], 1)
			}
		}
	}
	normalize(vector)
	return vector
}

func tokenize(text string) []string {
	var builder strings.Builder
	words := make([]string, 0)
	flush := func() {
		if builder.Len() > 0 {
			words = append(words, builder.String())
			builder.Reset()
		}
	}
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			builder.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return words
}

func addFeature(vector []float32, feature string, weight float32) {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(feature))
	h := hash.Sum32()
	index := int(h % uint32(len(vector)))
	if h&1 == 1 {
		vector[index] += weight
		return
	}
	vector[index] -= weight
}

func normalize(vector []float32) {
	var sum float64
	for _, value := range vector {
		sum += float64(value * value)
	}
	if sum == 0 {
		return
	}
	norm := float32(1 / math.Sqrt(sum))
	for i := range vector {
		vector[i] *= norm
	}
}

// Cosine returns the normalized dot product. Embed outputs are already
// normalized, but this remains defensive for decoded or test vectors.
func Cosine(left, right []float32) float64 {
	if len(left) == 0 || len(left) != len(right) {
		return 0
	}
	var dot, leftNorm, rightNorm float64
	for i := range left {
		dot += float64(left[i] * right[i])
		leftNorm += float64(left[i] * left[i])
		rightNorm += float64(right[i] * right[i])
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0
	}
	return dot / math.Sqrt(leftNorm*rightNorm)
}

func Encode(vector []float32) []byte {
	data := make([]byte, len(vector)*4)
	for i, value := range vector {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(value))
	}
	return data
}

func Decode(data []byte, dimension int) ([]float32, error) {
	if dimension <= 0 || len(data) != dimension*4 {
		return nil, errors.New("invalid semantic vector encoding")
	}
	vector := make([]float32, dimension)
	for i := range vector {
		vector[i] = math.Float32frombits(binary.LittleEndian.Uint32(data[i*4:]))
	}
	return vector, nil
}
