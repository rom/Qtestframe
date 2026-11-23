// Package ai is the optional AI assistance: drafts and suggestions from a
// language model service that an administrator configures (off by
// default). It holds the providers (the Anthropic Messages API and
// OpenAI-compatible chat completions, both streamed), the versioned prompt
// templates in English and Swedish, and the strict JSON schemas of the
// answers, validated on the server (one retry on an invalid answer).
//
// The model never acts: there are no tools, nothing is executed, and the
// text of the items is sent as clearly delimited data. Every suggestion
// is returned to the user, who reviews, edits and accepts it; nothing is
// saved by this package.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Message is one turn of the conversation sent to the service.
type Message struct {
	// Role is "user" or "assistant".
	Role string
	Text string
}

// Request is one completion request.
type Request struct {
	System   string
	Messages []Message
	// MaxTokens bounds the answer (0: 4096).
	MaxTokens int
	// Temperature is sent when set.
	Temperature *float64
}

// Usage counts the tokens of a request.
type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// Add adds u2 to u.
func (u *Usage) Add(u2 Usage) {
	u.InputTokens += u2.InputTokens
	u.OutputTokens += u2.OutputTokens
}

// Response is the answer of the service.
type Response struct {
	Text string
	// Model is the model that answered (as reported by the service).
	Model string
	Usage Usage
	// Truncated is set when the answer hit the token limit.
	Truncated bool
}

// Provider is a language model service. Complete streams the answer:
// onDelta (may be nil) receives each piece of text as it arrives.
type Provider interface {
	Complete(ctx context.Context, req Request, onDelta func(string)) (*Response, error)
	// Name is the provider type ("anthropic", "openai").
	Name() string
	// ModelName is the model asked for.
	ModelName() string
}

// Errors of the providers.
var (
	// ErrRefused: the service declined to answer.
	ErrRefused = errors.New("ai: the service declined to answer")
	// ErrService: the service answered with an error status.
	ErrService = errors.New("ai: the service failed")
)

// maxResponse bounds the body of a response.
const maxResponse = 16 << 20

const defaultMaxTokens = 4096

// DefaultAnthropicURL is the base URL of the public Anthropic API.
const DefaultAnthropicURL = "https://api.anthropic.com"

// anthropicVersion is the API version header of the Messages API.
const anthropicVersion = "2023-06-01"

func httpClient(c *http.Client) *http.Client {
	if c == nil {
		return &http.Client{Timeout: 2 * time.Minute}
	}
	return c
}

// serviceError reads the error message of a failed response without the
// request (it never contains the key).
func serviceError(resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var e struct {
		Error any `json:"error"`
	}
	msg := ""
	if json.Unmarshal(data, &e) == nil {
		switch v := e.Error.(type) {
		case string:
			msg = v
		case map[string]any:
			if m, ok := v["message"].(string); ok {
				msg = m
			}
		}
	}
	msg = strings.TrimSpace(msg)
	if len(msg) > 300 {
		msg = msg[:300] + "…"
	}
	if msg != "" {
		return fmt.Errorf("%w: %d %s", ErrService, resp.StatusCode, msg)
	}
	return fmt.Errorf("%w: %d", ErrService, resp.StatusCode)
}

// post sends a JSON body and returns the response (2xx only).
func post(ctx context.Context, hc *http.Client, endpoint string, headers map[string]string, body any) (*http.Response, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := httpClient(hc).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		// the URL may carry a query (Azure api-version) but never the key
		return nil, fmt.Errorf("ai: %w", stripURL(err))
	}
	if resp.StatusCode/100 != 2 {
		defer resp.Body.Close()
		return nil, serviceError(resp)
	}
	return resp, nil
}

func stripURL(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return ue.Err
	}
	return err
}

// sseEvents calls fn with the event name and data of each server-sent
// event of r (a plain JSON body is passed as one event without a name).
func sseEvents(r io.Reader, fn func(event, data string) error) error {
	br := bufio.NewReaderSize(io.LimitReader(r, maxResponse), 64<<10)
	first, _ := br.Peek(1)
	if len(first) == 1 && first[0] == '{' {
		data, err := io.ReadAll(br)
		if err != nil {
			return err
		}
		return fn("", string(data))
	}
	sc := bufio.NewScanner(br)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	event, data := "", []string{}
	flush := func() error {
		if len(data) == 0 {
			event = ""
			return nil
		}
		err := fn(event, strings.Join(data, "\n"))
		event, data = "", data[:0]
		return err
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, ":"):
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(line[5:], " "))
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return flush()
}

// ---------------------------------------------------------------- Anthropic

// Anthropic is the Anthropic Messages API (POST {URL}/v1/messages).
type Anthropic struct {
	// URL is the base URL ("" = DefaultAnthropicURL).
	URL   string
	Key   string
	Model string
	HTTP  *http.Client
}

// Name implements Provider.
func (a *Anthropic) Name() string { return "anthropic" }

// ModelName implements Provider.
func (a *Anthropic) ModelName() string { return a.Model }

func (a *Anthropic) endpoint() string {
	base := strings.TrimSuffix(a.URL, "/")
	if base == "" {
		base = DefaultAnthropicURL
	}
	if strings.HasSuffix(base, "/v1/messages") {
		return base
	}
	if strings.HasSuffix(base, "/v1") {
		return base + "/messages"
	}
	return base + "/v1/messages"
}

// Complete implements Provider.
func (a *Anthropic) Complete(ctx context.Context, req Request, onDelta func(string)) (*Response, error) {
	max := req.MaxTokens
	if max <= 0 {
		max = defaultMaxTokens
	}
	msgs := make([]map[string]string, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Text})
	}
	body := map[string]any{"model": a.Model, "max_tokens": max, "system": req.System, "messages": msgs, "stream": true}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	headers := map[string]string{"anthropic-version": anthropicVersion}
	if a.Key != "" {
		headers["x-api-key"] = a.Key
	}
	resp, err := post(ctx, a.HTTP, a.endpoint(), headers, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &Response{Model: a.Model}
	var text strings.Builder
	stop := ""
	err = sseEvents(resp.Body, func(event, data string) error {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Model string `json:"model"`
				Usage Usage  `json:"usage"`
				// a plain (not streamed) answer
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
				StopReason string `json:"stop_reason"`
			} `json:"message"`
			Delta struct {
				Type       string `json:"type"`
				Text       string `json:"text"`
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
			Usage Usage `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			// plain answer fields
			Model      string `json:"model"`
			StopReason string `json:"stop_reason"`
			Content    []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			return fmt.Errorf("ai: unreadable answer: %w", err)
		}
		switch ev.Type {
		case "message_start":
			if ev.Message.Model != "" {
				out.Model = ev.Message.Model
			}
			out.Usage.InputTokens = ev.Message.Usage.InputTokens
			if ev.Message.Usage.OutputTokens > 0 {
				out.Usage.OutputTokens = ev.Message.Usage.OutputTokens
			}
		case "content_block_delta":
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				text.WriteString(ev.Delta.Text)
				if onDelta != nil {
					onDelta(ev.Delta.Text)
				}
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stop = ev.Delta.StopReason
			}
			if ev.Usage.OutputTokens > 0 {
				out.Usage.OutputTokens = ev.Usage.OutputTokens
			}
			if ev.Usage.InputTokens > 0 {
				out.Usage.InputTokens = ev.Usage.InputTokens
			}
		case "error":
			return fmt.Errorf("%w: %s", ErrService, ev.Error.Message)
		case "message":
			// not streamed
			if ev.Model != "" {
				out.Model = ev.Model
			}
			for _, c := range ev.Content {
				if c.Type == "text" {
					text.WriteString(c.Text)
				}
			}
			stop = ev.StopReason
			var u struct {
				Usage Usage `json:"usage"`
			}
			_ = json.Unmarshal([]byte(data), &u)
			out.Usage = u.Usage
			if onDelta != nil && text.Len() > 0 {
				onDelta(text.String())
			}
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	switch stop {
	case "refusal":
		return nil, ErrRefused
	case "max_tokens":
		out.Truncated = true
	}
	out.Text = text.String()
	return out, nil
}

// ---------------------------------------------------------------- OpenAI-compatible

// OpenAI is an OpenAI-compatible chat completions endpoint (POST
// {URL}/chat/completions): OpenAI, Azure OpenAI (a deployment URL with
// ?api-version=…; the key is sent as api-key), local servers such as
// Ollama or vLLM (key optional).
type OpenAI struct {
	// URL is the base URL, e.g. https://llm.example.com/v1 (a URL that
	// already ends in /chat/completions is used as it is).
	URL   string
	Key   string
	Model string
	HTTP  *http.Client
}

// Name implements Provider.
func (o *OpenAI) Name() string { return "openai" }

// ModelName implements Provider.
func (o *OpenAI) ModelName() string { return o.Model }

func (o *OpenAI) endpoint() (string, bool) {
	u, err := url.Parse(o.URL)
	if err != nil {
		return o.URL, false
	}
	if !strings.HasSuffix(u.Path, "/chat/completions") {
		u.Path = strings.TrimSuffix(u.Path, "/") + "/chat/completions"
	}
	azure := strings.HasSuffix(u.Hostname(), ".openai.azure.com") || u.Query().Get("api-version") != ""
	return u.String(), azure
}

// Complete implements Provider.
func (o *OpenAI) Complete(ctx context.Context, req Request, onDelta func(string)) (*Response, error) {
	max := req.MaxTokens
	if max <= 0 {
		max = defaultMaxTokens
	}
	msgs := []map[string]string{{"role": "system", "content": req.System}}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Text})
	}
	body := map[string]any{"messages": msgs, "max_tokens": max, "stream": true,
		"stream_options": map[string]bool{"include_usage": true}}
	if o.Model != "" {
		body["model"] = o.Model
	}
	if req.Temperature != nil {
		body["temperature"] = *req.Temperature
	}
	endpoint, azure := o.endpoint()
	headers := map[string]string{}
	if o.Key != "" {
		if azure {
			headers["api-key"] = o.Key
		} else {
			headers["Authorization"] = "Bearer " + o.Key
		}
	}
	resp, err := post(ctx, o.HTTP, endpoint, headers, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	out := &Response{Model: o.Model}
	var text strings.Builder
	finish := ""
	refusal := ""
	err = sseEvents(resp.Body, func(event, data string) error {
		if strings.TrimSpace(data) == "[DONE]" {
			return nil
		}
		var ch struct {
			Model   string `json:"model"`
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
					Refusal string `json:"refusal"`
				} `json:"delta"`
				Message struct {
					Content string `json:"content"`
					Refusal string `json:"refusal"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(data), &ch); err != nil {
			return fmt.Errorf("ai: unreadable answer: %w", err)
		}
		if ch.Error != nil {
			return fmt.Errorf("%w: %s", ErrService, ch.Error.Message)
		}
		if ch.Model != "" {
			out.Model = ch.Model
		}
		for _, c := range ch.Choices {
			piece := c.Delta.Content + c.Message.Content
			if piece != "" {
				text.WriteString(piece)
				if onDelta != nil {
					onDelta(piece)
				}
			}
			refusal += c.Delta.Refusal + c.Message.Refusal
			if c.FinishReason != "" {
				finish = c.FinishReason
			}
		}
		if ch.Usage != nil {
			out.Usage = Usage{InputTokens: ch.Usage.PromptTokens, OutputTokens: ch.Usage.CompletionTokens}
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if refusal != "" || finish == "content_filter" {
		return nil, ErrRefused
	}
	if finish == "length" {
		out.Truncated = true
	}
	out.Text = text.String()
	return out, nil
}
