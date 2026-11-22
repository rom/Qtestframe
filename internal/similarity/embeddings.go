package similarity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"time"
)

// Optional embeddings: an OpenAI-compatible /v1/embeddings endpoint the
// administrator configures (off by default; it sends the text of the
// items to that service). Its vectors replace the TF-IDF vectors for the
// similarity; the conflict detection stays local.

// Embedder turns texts into vectors.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, error)
	// Model names the vectors (cached vectors of another model are not used).
	Model() string
}

// Client is an Embedder for an OpenAI-compatible embeddings endpoint.
type Client struct {
	// URL of the endpoint, e.g. https://host/v1/embeddings.
	URL string
	// Key is sent as a bearer token ("" none).
	Key string
	// ModelName is sent as "model" ("" lets the service choose).
	ModelName string
	HTTP      *http.Client
	// Batch is the most texts per request (default 64).
	Batch int
}

// Model implements Embedder.
func (c *Client) Model() string {
	if c.ModelName == "" {
		return "default"
	}
	return c.ModelName
}

// maxResponse bounds a response body.
const maxResponse = 64 << 20

// Embed implements Embedder.
func (c *Client) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	batch := c.Batch
	if batch <= 0 {
		batch = 64
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	out := make([][]float32, 0, len(texts))
	for start := 0; start < len(texts); start += batch {
		end := min(start+batch, len(texts))
		body := map[string]any{"input": texts[start:end]}
		if c.ModelName != "" {
			body["model"] = c.ModelName
		}
		raw, _ := json.Marshal(body)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.Key != "" {
			req.Header.Set("Authorization", "Bearer "+c.Key)
		}
		resp, err := hc.Do(req)
		if err != nil {
			return nil, fmt.Errorf("embeddings: %w", err)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponse))
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("embeddings: %w", err)
		}
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("embeddings: the service answered %d", resp.StatusCode)
		}
		var res struct {
			Data []struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.Unmarshal(data, &res); err != nil {
			return nil, fmt.Errorf("embeddings: unreadable answer: %w", err)
		}
		if len(res.Data) != end-start {
			return nil, fmt.Errorf("embeddings: %d vectors for %d texts", len(res.Data), end-start)
		}
		sort.SliceStable(res.Data, func(i, j int) bool { return res.Data[i].Index < res.Data[j].Index })
		for _, d := range res.Data {
			if len(d.Embedding) == 0 {
				return nil, errors.New("embeddings: an empty vector")
			}
			out = append(out, normalise(d.Embedding))
		}
	}
	return out, nil
}

func normalise(v []float32) []float32 {
	s := 0.0
	for _, x := range v {
		s += float64(x) * float64(x)
	}
	if s == 0 {
		return v
	}
	n := float32(math.Sqrt(s))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / n
	}
	return out
}

// TextHash identifies the text an embedding was made of.
func TextHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:16])
}

// EncodeVector stores a vector as base64 little-endian float32.
func EncodeVector(v []float32) string {
	b := make([]byte, 4*len(v))
	for i, x := range v {
		binary.LittleEndian.PutUint32(b[4*i:], math.Float32bits(x))
	}
	return base64.StdEncoding.EncodeToString(b)
}

// DecodeVector reverses EncodeVector.
func DecodeVector(s string) ([]float32, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b)%4 != 0 {
		return nil, errors.New("invalid vector")
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4*i:]))
	}
	return v, nil
}

// DenseIndex is a corpus of embedding vectors (normalised).
type DenseIndex struct {
	ids  []string
	vecs [][]float32
	pos  map[string]int
}

// NewDenseIndex builds an index of vectors (ids[i] has vecs[i]).
func NewDenseIndex(ids []string, vecs [][]float32) *DenseIndex {
	x := &DenseIndex{ids: ids, vecs: vecs, pos: map[string]int{}}
	for i, id := range ids {
		x.pos[id] = i
	}
	return x
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	s := 0.0
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return clamp(s)
}

// Score is the similarity of two documents of the index.
func (x *DenseIndex) Score(a, b string) float64 {
	i, ok1 := x.pos[a]
	j, ok2 := x.pos[b]
	if !ok1 || !ok2 {
		return 0
	}
	return cosine(x.vecs[i], x.vecs[j])
}

// Nearest returns the documents most similar to q (see Index.Similar).
func (x *DenseIndex) Nearest(q []float32, limit int, min float64, skip func(id string) bool) []Match {
	q = normalise(q)
	out := []Match{}
	for i, v := range x.vecs {
		s := cosine(q, v)
		if s < min || s <= 0 || (skip != nil && skip(x.ids[i])) {
			continue
		}
		out = append(out, Match{ID: x.ids[i], Score: s})
	}
	sortMatches(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Pairs returns every pair with a similarity of at least min.
func (x *DenseIndex) Pairs(min float64) []Pair {
	var out []Pair
	for i := range x.vecs {
		for j := i + 1; j < len(x.vecs); j++ {
			if s := cosine(x.vecs[i], x.vecs[j]); s >= min && s > 0 {
				out = append(out, Pair{A: x.ids[i], B: x.ids[j], Score: s})
			}
		}
	}
	sortPairs(out)
	return out
}
