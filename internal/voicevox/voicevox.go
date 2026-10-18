package voicevox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	base, cache string
	ttl         time.Duration
	log         *slog.Logger
	http        *http.Client
	mu          sync.Mutex
}
type Speaker struct {
	Name   string  `json:"name"`
	Styles []Style `json:"styles"`
}
type Style struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func New(base, cache string, ttl time.Duration, log *slog.Logger) *Engine {
	return &Engine{strings.TrimRight(base, "/"), cache, ttl, log, &http.Client{Timeout: 30 * time.Second}, sync.Mutex{}}
}

func (e *Engine) Synthesize(ctx context.Context, text string, speakerID int) ([]byte, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("文章が空です")
	}
	if len([]rune(text)) > 200 {
		return nil, fmt.Errorf("文章は200文字以内で指定してください")
	}
	key := sha256.Sum256([]byte(strconv.Itoa(speakerID) + ":" + text))
	name := hex.EncodeToString(key[:])
	opus := filepath.Join(e.cache, name+".opus")
	if b, ok := e.readCache(opus); ok {
		return b, nil
	}
	q, err := e.post(ctx, "/audio_query?text="+url.QueryEscape(text)+"&speaker="+strconv.Itoa(speakerID), nil)
	if err != nil {
		return nil, fmt.Errorf("audio_queryに失敗しました: %w", err)
	}
	wav, err := e.post(ctx, "/synthesis?speaker="+strconv.Itoa(speakerID), q)
	if err != nil {
		return nil, fmt.Errorf("音声合成に失敗しました: %w", err)
	}
	out, err := transcodeWAVToPCM(ctx, wav)
	if err != nil {
		return nil, err
	}
	e.writeCache(opus, out)
	return out, nil
}
func (e *Engine) Speakers(ctx context.Context) ([]Speaker, error) {
	b, err := e.get(ctx, "/speakers")
	if err != nil {
		return nil, err
	}
	var s []Speaker
	return s, json.Unmarshal(b, &s)
}
func (e *Engine) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return e.do(req)
}
func (e *Engine) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.base+path, nil)
	if err != nil {
		return nil, err
	}
	return e.do(req)
}
func (e *Engine) do(req *http.Request) ([]byte, error) {
	r, err := e.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	if r.StatusCode/100 != 2 {
		return nil, fmt.Errorf("VOICEVOXが%sを返しました: %s", r.Status, string(b))
	}
	return b, nil
}
func transcodeWAVToPCM(ctx context.Context, wav []byte) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "ffmpeg", "-loglevel", "error", "-i", "pipe:0", "-f", "s16le", "-acodec", "pcm_s16le", "-ac", "2", "-ar", "48000", "pipe:1")
	cmd.Stdin = bytes.NewReader(wav)
	return cmd.Output()
}
func (e *Engine) readCache(path string) ([]byte, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := os.Stat(path)
	if err != nil || time.Since(st.ModTime()) > e.ttl {
		return nil, false
	}
	b, err := os.ReadFile(path)
	return b, err == nil
}
func (e *Engine) writeCache(path string, b []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := os.MkdirAll(e.cache, 0755); err == nil {
		_ = os.WriteFile(path, b, 0644)
	}
}
