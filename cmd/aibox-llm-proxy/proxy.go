package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// LLMProxy is the core reverse proxy that intercepts LLM API calls,
// injects credentials, enforces rate limits, and logs payloads.
type LLMProxy struct {
	config      Config
	credentials *CredentialManager
	logger      *PayloadLogger
	limiter     *RateLimiter
	proxy       *httputil.ReverseProxy
	server      *http.Server
}

// NewLLMProxy creates a fully wired LLM proxy.
func NewLLMProxy(cfg Config) (*LLMProxy, error) {
	creds, err := NewCredentialManager(cfg.SecretPath)
	if err != nil {
		return nil, fmt.Errorf("credential manager: %w", err)
	}

	plog, err := NewPayloadLogger(cfg.LogPath, cfg.MaxRequestSize)
	if err != nil {
		return nil, fmt.Errorf("payload logger: %w", err)
	}

	upstream, err := url.Parse(cfg.Upstream)
	if err != nil {
		return nil, fmt.Errorf("parsing upstream URL: %w", err)
	}

	p := &LLMProxy{
		config:      cfg,
		credentials: creds,
		logger:      plog,
		limiter:     NewRateLimiter(cfg.RateLimitRPM, cfg.RateLimitTPM),
	}

	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = upstream.Scheme
			req.URL.Host = upstream.Host
			req.Host = upstream.Host

			// Strip any client-supplied auth and inject ours
			req.Header.Del("Authorization")
			req.Header.Set("Authorization", "Bearer "+creds.GetAPIKey())

			// Tag the request
			if cfg.SandboxID != "" {
				req.Header.Set("X-AIBox-Sandbox-ID", cfg.SandboxID)
			}
			if cfg.User != "" {
				req.Header.Set("X-AIBox-User", cfg.User)
			}
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("proxy error", "err", err, "path", r.URL.Path)
			http.Error(w, `{"error":"bad_gateway","message":"upstream unreachable"}`, http.StatusBadGateway)
		},
	}

	p.proxy = rp
	return p, nil
}

// ServeHTTP is the main request handler.
func (p *LLMProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Health check endpoint
	if r.URL.Path == "/health" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
		return
	}

	// Rate limit check (RPM)
	if !p.limiter.AllowRequest() {
		slog.Warn("rate limit exceeded (RPM)", "path", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limit_exceeded","message":"requests per minute limit reached"}`))
		return
	}

	// Read and buffer request body for logging (limited to MaxRequestSize)
	var reqBody []byte
	if r.Body != nil {
		limited := io.LimitReader(r.Body, p.config.MaxRequestSize+1)
		var err error
		reqBody, err = io.ReadAll(limited)
		if err != nil {
			http.Error(w, `{"error":"bad_request","message":"failed to read request body"}`, http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()

		if int64(len(reqBody)) > p.config.MaxRequestSize {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte(`{"error":"request_too_large","message":"request body exceeds maximum size"}`))
			return
		}

		// Restore body for the reverse proxy
		r.Body = io.NopCloser(bytes.NewReader(reqBody))
		r.ContentLength = int64(len(reqBody))
	}

	// Estimate tokens from request for pre-check
	estReqTokens := EstimateTokens(len(reqBody), 0)
	if !p.limiter.AllowTokens(estReqTokens) {
		slog.Warn("rate limit exceeded (TPM)", "path", r.URL.Path, "est_tokens", estReqTokens)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate_limit_exceeded","message":"tokens per minute limit reached"}`))
		return
	}

	model := ExtractModel(reqBody)
	start := time.Now()

	// Check if this is likely a streaming request
	isStreaming := isSSERequest(r, reqBody)

	if isStreaming {
		p.serveStreaming(w, r, reqBody, model, start)
	} else {
		p.serveBuffered(w, r, reqBody, model, start)
	}
}

// serveBuffered handles non-streaming requests by capturing the full response.
func (p *LLMProxy) serveBuffered(w http.ResponseWriter, r *http.Request, reqBody []byte, model string, start time.Time) {
	rec := &responseRecorder{
		header: http.Header{},
		body:   &bytes.Buffer{},
		code:   http.StatusOK,
	}

	p.proxy.ServeHTTP(rec, r)

	// Copy captured response to real client
	for k, vv := range rec.header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(rec.code)
	respBytes := rec.body.Bytes()
	_, _ = w.Write(respBytes)

	// Log asynchronously
	duration := time.Since(start)
	estTokens := EstimateTokens(len(reqBody), len(respBytes))
	p.limiter.RecordTokens(estTokens)

	go p.logger.Log(PayloadEntry{
		Timestamp:    start.UTC().Format(time.RFC3339Nano),
		SandboxID:    p.config.SandboxID,
		User:         p.config.User,
		Method:       r.Method,
		Path:         r.URL.Path,
		Model:        model,
		RequestSize:  len(reqBody),
		ResponseSize: len(respBytes),
		EstTokens:    estTokens,
		DurationMS:   duration.Milliseconds(),
		StatusCode:   rec.code,
		RequestBody:  string(reqBody),
		ResponseBody: string(respBytes),
	})
}

// serveStreaming handles SSE streaming responses by teeing writes to the client
// in real time while capturing them for logging.
func (p *LLMProxy) serveStreaming(w http.ResponseWriter, r *http.Request, reqBody []byte, model string, start time.Time) {
	sw := &streamingWriter{
		inner:   w,
		flusher: tryFlusher(w),
		capture: &bytes.Buffer{},
	}

	p.proxy.ServeHTTP(sw, r)

	// Log after stream completes
	duration := time.Since(start)
	respBytes := sw.capture.Bytes()
	estTokens := EstimateTokens(len(reqBody), len(respBytes))
	p.limiter.RecordTokens(estTokens)

	go p.logger.Log(PayloadEntry{
		Timestamp:    start.UTC().Format(time.RFC3339Nano),
		SandboxID:    p.config.SandboxID,
		User:         p.config.User,
		Method:       r.Method,
		Path:         r.URL.Path,
		Model:        model,
		RequestSize:  len(reqBody),
		ResponseSize: len(respBytes),
		EstTokens:    estTokens,
		DurationMS:   duration.Milliseconds(),
		StatusCode:   sw.code,
		RequestBody:  string(reqBody),
		ResponseBody: string(respBytes),
	})
}

// Start begins listening on the configured address.
func (p *LLMProxy) Start() error {
	p.server = &http.Server{
		Addr:              p.config.ListenAddr,
		Handler:           p,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return context.Background()
		},
	}
	return p.server.ListenAndServe()
}

// Shutdown gracefully stops the proxy server and closes the logger.
func (p *LLMProxy) Shutdown(ctx context.Context) error {
	err := p.server.Shutdown(ctx)
	_ = p.logger.Close()
	return err
}

// responseRecorder captures the full response for non-streaming requests.
type responseRecorder struct {
	header http.Header
	body   *bytes.Buffer
	code   int
}

func (r *responseRecorder) Header() http.Header {
	return r.header
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	return r.body.Write(b)
}

func (r *responseRecorder) WriteHeader(code int) {
	r.code = code
}

// streamingWriter tees all writes to the real client (flushing immediately for
// SSE) while capturing a copy for audit logging.
type streamingWriter struct {
	inner       http.ResponseWriter
	flusher     http.Flusher
	capture     *bytes.Buffer
	code        int
	wroteHeader bool
}

func (sw *streamingWriter) Header() http.Header {
	return sw.inner.Header()
}

func (sw *streamingWriter) WriteHeader(code int) {
	sw.code = code
	sw.wroteHeader = true
	sw.inner.WriteHeader(code)
}

func (sw *streamingWriter) Write(b []byte) (int, error) {
	if !sw.wroteHeader {
		sw.code = http.StatusOK
		sw.wroteHeader = true
	}
	_, _ = sw.capture.Write(b)
	n, err := sw.inner.Write(b)
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
	return n, err
}

// Flush implements http.Flusher for streaming.
func (sw *streamingWriter) Flush() {
	if sw.flusher != nil {
		sw.flusher.Flush()
	}
}

func tryFlusher(w http.ResponseWriter) http.Flusher {
	if f, ok := w.(http.Flusher); ok {
		return f
	}
	return nil
}

// isSSERequest detects streaming requests by looking for stream:true in the
// JSON body or text/event-stream in the Accept header.
func isSSERequest(r *http.Request, body []byte) bool {
	if strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		return true
	}
	// Many LLM APIs use {"stream": true} in the request body
	var partial struct {
		Stream bool `json:"stream"`
	}
	if json.Unmarshal(body, &partial) == nil && partial.Stream {
		return true
	}
	return false
}
