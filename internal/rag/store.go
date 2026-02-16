// Package rag provides a knowledge store with semantic search capabilities.
// It uses DashScope's text-embedding-v3 API for embedding generation and
// stores vectors in ChromaDB (HTTP client mode).
// Mirrors survival/nanobot/rag_store.py.
package rag

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Config holds RAGStore configuration.
type Config struct {
	Workspace      string
	CollectionName string // ChromaDB collection (default: "knowledge")
	EmbeddingModel string // DashScope model (default: "text-embedding-v3")
	EmbeddingAPIKey string
	EmbeddingBaseURL string
	ChromaURL      string // ChromaDB HTTP URL (default: "http://localhost:8000")
	ChunkSize      int    // chars per chunk (default: 500)
	ChunkOverlap   int    // overlap chars (default: 50)
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		CollectionName: "knowledge",
		EmbeddingModel: "text-embedding-v3",
		EmbeddingBaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		ChromaURL:      "http://localhost:8000",
		ChunkSize:      500,
		ChunkOverlap:   50,
	}
}

// Store provides document ingestion and semantic search.
type Store struct {
	cfg          Config
	initialized  bool
	httpClient   *http.Client
}

// NewStore creates a new RAGStore.
func NewStore(cfg Config) *Store {
	if cfg.CollectionName == "" {
		cfg.CollectionName = "knowledge"
	}
	if cfg.EmbeddingModel == "" {
		cfg.EmbeddingModel = "text-embedding-v3"
	}
	if cfg.EmbeddingBaseURL == "" {
		cfg.EmbeddingBaseURL = "https://dashscope.aliyuncs.com/compatible-mode/v1"
	}
	if cfg.ChromaURL == "" {
		cfg.ChromaURL = "http://localhost:8000"
	}
	if cfg.ChunkSize == 0 {
		cfg.ChunkSize = 500
	}
	if cfg.ChunkOverlap == 0 {
		cfg.ChunkOverlap = 50
	}

	return &Store{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Embed generates embeddings for the given texts using DashScope API.
func (s *Store) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if s.cfg.EmbeddingAPIKey == "" {
		return nil, fmt.Errorf("embedding API key not configured")
	}

	var allEmbeddings [][]float64
	batchSize := 25 // DashScope max batch size

	for i := 0; i < len(texts); i += batchSize {
		end := i + batchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]

		body, _ := json.Marshal(map[string]any{
			"model": s.cfg.EmbeddingModel,
			"input": batch,
		})

		req, err := http.NewRequestWithContext(ctx, "POST",
			strings.TrimRight(s.cfg.EmbeddingBaseURL, "/")+"/embeddings",
			bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+s.cfg.EmbeddingAPIKey)

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("embedding API call: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("embedding API error %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var result struct {
			Data []struct {
				Embedding []float64 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("parse embedding response: %w", err)
		}

		for _, d := range result.Data {
			allEmbeddings = append(allEmbeddings, d.Embedding)
		}
	}

	return allEmbeddings, nil
}

// Query performs semantic search against ChromaDB.
func (s *Store) Query(ctx context.Context, text string, topK int) ([]SearchResult, error) {
	if topK == 0 {
		topK = 5
	}

	// Generate query embedding
	embeddings, err := s.Embed(ctx, []string{text})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	if len(embeddings) == 0 {
		return nil, fmt.Errorf("no embedding generated")
	}

	// Query ChromaDB
	body, _ := json.Marshal(map[string]any{
		"query_embeddings": [][]float64{embeddings[0]},
		"n_results":        topK,
		"include":          []string{"documents", "metadatas", "distances"},
	})

	endpoint := fmt.Sprintf("%s/api/v1/collections/%s/query",
		strings.TrimRight(s.cfg.ChromaURL, "/"), s.cfg.CollectionName)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("chromaDB query: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("chromaDB error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		Documents [][]string           `json:"documents"`
		Metadatas [][]map[string]any   `json:"metadatas"`
		Distances [][]float64          `json:"distances"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("parse chromaDB response: %w", err)
	}

	var results []SearchResult
	if len(result.Documents) > 0 {
		for i, doc := range result.Documents[0] {
			r := SearchResult{Text: doc}
			if i < len(result.Distances[0]) {
				r.Distance = result.Distances[0][i]
			}
			if i < len(result.Metadatas[0]) {
				r.Metadata = result.Metadatas[0][i]
				if src, ok := r.Metadata["source"].(string); ok {
					r.Source = src
				}
			}
			results = append(results, r)
		}
	}

	return results, nil
}

// SearchResult holds a single search result.
type SearchResult struct {
	Text     string         `json:"text"`
	Source   string         `json:"source"`
	Distance float64       `json:"distance"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// IngestText directly ingests text into the knowledge store.
// Used for Memory Flush — saves conversation context to RAG.
func (s *Store) IngestText(ctx context.Context, text, source string) (int, error) {
	chunks := ChunkText(text, s.cfg.ChunkSize, s.cfg.ChunkOverlap, source)
	if len(chunks) == 0 {
		return 0, nil
	}

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Text
	}

	embeddings, err := s.Embed(ctx, texts)
	if err != nil {
		return 0, fmt.Errorf("embed chunks: %w", err)
	}

	// Add to ChromaDB
	ids := make([]string, len(chunks))
	metadatas := make([]map[string]any, len(chunks))
	for i, c := range chunks {
		ids[i] = fmt.Sprintf("%x", md5.Sum([]byte(c.Text)))[:16]
		metadatas[i] = map[string]any{
			"source":      c.Source,
			"chunk_index": c.ChunkIndex,
		}
	}

	body, _ := json.Marshal(map[string]any{
		"ids":        ids,
		"embeddings": embeddings,
		"documents":  texts,
		"metadatas":  metadatas,
	})

	endpoint := fmt.Sprintf("%s/api/v1/collections/%s/add",
		strings.TrimRight(s.cfg.ChromaURL, "/"), s.cfg.CollectionName)

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("chromaDB add: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("chromaDB add error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	log.Printf("[RAG] Ingested %d chunks from %s", len(chunks), source)
	return len(chunks), nil
}

// IngestDir ingests all files in a directory.
func (s *Store) IngestDir(ctx context.Context, dir string) (int, error) {
	total := 0
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".md" && ext != ".txt" && ext != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			log.Printf("[RAG] ⚠️ Read failed: %s: %v", path, err)
			continue
		}

		n, err := s.IngestText(ctx, string(data), entry.Name())
		if err != nil {
			log.Printf("[RAG] ⚠️ Ingest failed: %s: %v", path, err)
			continue
		}
		total += n
	}

	return total, nil
}

// Chunk holds a text chunk with metadata.
type Chunk struct {
	Text       string `json:"text"`
	Source     string `json:"source"`
	ChunkIndex int    `json:"chunk_index"`
}

// ChunkText splits text into overlapping chunks.
func ChunkText(text string, chunkSize, chunkOverlap int, source string) []Chunk {
	if len(text) == 0 {
		return nil
	}

	// Split by paragraphs first
	paragraphs := strings.Split(text, "\n\n")
	var chunks []Chunk
	var current strings.Builder
	idx := 0

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}

		if current.Len()+len(para) > chunkSize && current.Len() > 0 {
			chunks = append(chunks, Chunk{
				Text:       current.String(),
				Source:     source,
				ChunkIndex: idx,
			})
			idx++

			// Keep overlap
			text := current.String()
			current.Reset()
			if len(text) > chunkOverlap {
				current.WriteString(text[len(text)-chunkOverlap:])
			}
		}

		if current.Len() > 0 {
			current.WriteString("\n\n")
		}
		current.WriteString(para)
	}

	// Remaining text
	if current.Len() > 0 {
		chunks = append(chunks, Chunk{
			Text:       current.String(),
			Source:     source,
			ChunkIndex: idx,
		})
	}

	return chunks
}
