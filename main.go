package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	edgetts "github.com/bytectlgo/edge-tts/pkg/edge_tts"
	"gopkg.in/yaml.v3"
)

const (
	defaultVoice      = "zh-CN-XiaoxiaoNeural"
	defaultPort       = 8080
	maxTextLen        = 1000
	defaultConfigPath = "config.yaml"
	defaultLogPath    = "tts.log"
)

type Config struct {
	Port        int    `yaml:"port"`
	Title       string `yaml:"title"`
	EnableCache bool   `yaml:"enable_cache"`
	CacheDir    string `yaml:"cache_dir"`
}

type App struct {
	config    Config
	startedAt time.Time
}

type statusItem struct {
	Name   string
	Value  string
	Detail string
	OK     bool
}

type pageData struct {
	Title         string
	Status        string
	Voice         string
	Addr          string
	StartedAt     string
	Uptime        string
	Example       string
	CurrentTime   string
	CurrentPath   string
	CurrentQuery  string
	CurrentMethod string
	ClientIP      string
	StatusItems   []statusItem
}

func main() {
	if err := setupLogging(); err != nil {
		log.Fatalf("setup logging failed: %v", err)
	}

	cfg, err := loadConfig(resolveConfigPath())
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	app := &App{
		config:    cfg,
		startedAt: time.Now(),
	}

	if err := app.ensureCacheDir(); err != nil {
		log.Fatalf("prepare cache failed: %v", err)
	}

	addr := app.listenAddr()
	log.Printf("service starting title=%q addr=%s voice=%s cache_enabled=%t cache_dir=%s", app.config.Title, addr, app.voice(), app.config.EnableCache, app.cacheDir())

	http.HandleFunc("/", app.handleRoot)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

func (a *App) handleRoot(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Printf("request start method=%s path=%s query=%q remote=%s", r.Method, r.URL.Path, r.URL.RawQuery, clientIP(r))

	if matchesStrictTTS(r) {
		a.handleTTS(w, r, start)
		return
	}

	a.renderHTML(w, r)
	log.Printf("request done method=%s path=%s type=html duration=%s", r.Method, r.URL.Path, time.Since(start).Round(time.Millisecond))
}

func (a *App) handleTTS(w http.ResponseWriter, r *http.Request, start time.Time) {
	text := strings.TrimSpace(r.URL.Query().Get("t"))
	if text == "" {
		log.Printf("tts reject empty text remote=%s", clientIP(r))
		writeJSONError(w, http.StatusBadRequest, "query parameter t is empty")
		return
	}
	if len([]rune(text)) > maxTextLen {
		log.Printf("tts reject too long chars=%d remote=%s", len([]rune(text)), clientIP(r))
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("query parameter t exceeds %d characters", maxTextLen))
		return
	}

	log.Printf("tts synth start voice=%s chars=%d text=%q", a.voice(), len([]rune(text)), text)

	if a.config.EnableCache {
		cachedAudio, cachePath, ok, err := a.readCache(text)
		if err != nil {
			log.Printf("tts cache read failed path=%s err=%v", cachePath, err)
		} else if ok {
			log.Printf("tts cache hit path=%s bytes=%d", cachePath, len(cachedAudio))
			writeMP3Response(w, cachedAudio)
			log.Printf("tts response done source=cache bytes=%d duration=%s", len(cachedAudio), time.Since(start).Round(time.Millisecond))
			return
		} else {
			log.Printf("tts cache miss path=%s", cachePath)
		}
	}

	audio, err := synthesizeSpeech(r.Context(), text, a.voice())
	if err != nil {
		log.Printf("tts synth failed voice=%s err=%v", a.voice(), err)
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(audio) == 0 {
		log.Printf("tts synth empty voice=%s", a.voice())
		writeJSONError(w, http.StatusInternalServerError, "empty audio returned")
		return
	}

	if a.config.EnableCache {
		if cachePath, err := a.writeCache(text, audio); err != nil {
			log.Printf("tts cache write failed err=%v", err)
		} else {
			log.Printf("tts cache saved path=%s bytes=%d", cachePath, len(audio))
		}
	}

	writeMP3Response(w, audio)

	log.Printf("tts synth done voice=%s bytes=%d duration=%s", a.voice(), len(audio), time.Since(start).Round(time.Millisecond))
}

func synthesizeSpeech(parent context.Context, text, voice string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	comm := edgetts.NewCommunicate(
		text,
		voice,
		edgetts.WithRate("+0%"),
		edgetts.WithVolume("+0%"),
		edgetts.WithPitch("+0Hz"),
	)

	stream, err := comm.Stream(ctx)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	for chunk := range stream {
		switch strings.ToLower(chunk.Type) {
		case "audio":
			buf.Write(chunk.Data)
		case "error":
			return nil, fmt.Errorf("%s", strings.TrimSpace(string(chunk.Data)))
		}
	}

	return buf.Bytes(), nil
}

func matchesStrictTTS(r *http.Request) bool {
	if r.Method != http.MethodGet || r.URL.Path != "/" {
		return false
	}

	query := r.URL.Query()
	if len(query) != 1 {
		return false
	}
	values, ok := query["t"]
	if !ok || len(values) != 1 {
		return false
	}
	return strings.TrimSpace(values[0]) != ""
}

func (a *App) renderHTML(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	data := pageData{
		Title:         a.config.Title,
		Status:        "Running",
		Voice:         a.voice(),
		Addr:          a.listenAddr(),
		StartedAt:     a.startedAt.Format(time.RFC3339),
		Uptime:        time.Since(a.startedAt).Round(time.Second).String(),
		Example:       "/?t=你好，欢迎使用TTS服务",
		CurrentTime:   time.Now().Format(time.RFC3339),
		CurrentPath:   r.URL.Path,
		CurrentQuery:  r.URL.RawQuery,
		CurrentMethod: r.Method,
		ClientIP:      clientIP(r),
		StatusItems:   a.serviceStatus(),
	}

	_ = htmlPage.Execute(w, data)
}

func (a *App) serviceStatus() []statusItem {
	cacheValue := "disabled"
	cacheDetail := "cache is turned off in config"
	if a.config.EnableCache {
		cacheValue = "enabled"
		cacheDetail = a.cacheDir()
	}

	return []statusItem{
		{Name: "HTTP Service", Value: "Running", Detail: "server is accepting requests", OK: true},
		{Name: "Route Rule", Value: "strict", Detail: "only GET /?t={msg} returns mp3", OK: true},
		{Name: "Output Format", Value: "audio/mpeg", Detail: "response returns mp3 bytes", OK: true},
		{Name: "Voice", Value: a.voice(), Detail: "supports Chinese input and voice", OK: true},
		{Name: "Config File", Value: "config.yaml", Detail: "port and title are loaded from yaml", OK: true},
		{Name: "Cache", Value: cacheValue, Detail: cacheDetail, OK: true},
	}
}

func (a *App) voice() string {
	return envOrDefault("EDGE_TTS_VOICE", defaultVoice)
}

func (a *App) cacheDir() string {
	cacheDir := strings.TrimSpace(a.config.CacheDir)
	if cacheDir == "" {
		cacheDir = "cache"
	}
	if filepath.IsAbs(cacheDir) {
		return cacheDir
	}
	if exePath, err := os.Executable(); err == nil {
		return filepath.Join(filepath.Dir(exePath), cacheDir)
	}
	return cacheDir
}

func (a *App) ensureCacheDir() error {
	if !a.config.EnableCache {
		return nil
	}
	return os.MkdirAll(a.cacheDir(), 0755)
}

func (a *App) cachePath(text string) string {
	sum := sha256.Sum256([]byte(text))
	fileName := hex.EncodeToString(sum[:]) + ".mp3"
	return filepath.Join(a.cacheDir(), fileName)
}

func (a *App) readCache(text string) ([]byte, string, bool, error) {
	path := a.cachePath(text)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, path, false, nil
		}
		return nil, path, false, err
	}
	return data, path, true, nil
}

func (a *App) writeCache(text string, audio []byte) (string, error) {
	path := a.cachePath(text)
	if err := os.WriteFile(path, audio, 0644); err != nil {
		return path, err
	}
	return path, nil
}

func (a *App) listenAddr() string {
	return ":" + strconv.Itoa(a.config.Port)
}

func loadConfig(path string) (Config, error) {
	cfg := Config{
		Port:        defaultPort,
		Title:       "Go Edge TTS Server",
		EnableCache: true,
		CacheDir:    "cache",
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Port <= 0 {
		cfg.Port = defaultPort
	}
	if strings.TrimSpace(cfg.Title) == "" {
		cfg.Title = "Go Edge TTS Server"
	}
	return cfg, nil
}

func resolveConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("CONFIG_PATH")); path != "" {
		return path
	}

	exePath, err := os.Executable()
	if err == nil {
		exeDir := filepath.Dir(exePath)
		candidate := filepath.Join(exeDir, defaultConfigPath)
		if _, statErr := os.Stat(candidate); statErr == nil {
			return candidate
		}
	}

	return defaultConfigPath
}

func setupLogging() error {
	logPath := defaultLogPath
	if exePath, err := os.Executable(); err == nil {
		logPath = filepath.Join(filepath.Dir(exePath), defaultLogPath)
	}

	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}

	log.SetOutput(io.MultiWriter(os.Stdout, file))
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	return r.RemoteAddr
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `{"error":%q}`, message)
}

func writeMP3Response(w http.ResponseWriter, audio []byte) {
	w.Header().Set("Content-Type", "audio/mpeg")
	w.Header().Set("Content-Disposition", `inline; filename="tts.mp3"`)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(audio)
}

var htmlPage = template.Must(template.New("index").Parse(`<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}}</title>
  <style>
    body { font-family: "Segoe UI", Arial, sans-serif; margin: 0; background: #f7f8fb; color: #1f2937; }
    .wrap { max-width: 980px; margin: 0 auto; padding: 32px 20px; }
    .card { background: #ffffff; border: 1px solid #e5e7eb; border-radius: 16px; padding: 20px; margin-bottom: 18px; box-shadow: 0 6px 18px rgba(15, 23, 42, 0.05); }
    h1,h2,p,li,code { margin-top: 0; }
    h1,h2 { color: #111827; }
    .ok { color: #059669; font-weight: 600; }
    .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px; }
    .item { background: #f9fafb; border: 1px solid #e5e7eb; border-radius: 12px; padding: 14px; }
    a { color: #2563eb; text-decoration: none; }
    a:hover { text-decoration: underline; }
    code { word-break: break-all; background: #f3f4f6; padding: 2px 6px; border-radius: 6px; }
    ul { padding-left: 18px; margin-bottom: 0; }
  </style>
</head>
<body>
  <div class="wrap">
    <div class="card">
      <h1>{{.Title}}</h1>
      <p>服务状态：<strong class="ok">{{.Status}}</strong></p>
      <p>仅严格匹配 <code>?t={msg}</code> 时返回生成的 <code>.mp3</code> 文件，其他访问一律返回本页面。</p>
      <p><a href="{{.Example}}">测试示例：{{.Example}}</a></p>
    </div>

    <div class="card">
      <h2>基础信息</h2>
      <div class="grid">
        <div class="item"><strong>监听地址</strong><br><code>{{.Addr}}</code></div>
        <div class="item"><strong>默认语音</strong><br><code>{{.Voice}}</code></div>
        <div class="item"><strong>启动时间</strong><br><code>{{.StartedAt}}</code></div>
        <div class="item"><strong>运行时长</strong><br><code>{{.Uptime}}</code></div>
        <div class="item"><strong>当前时间</strong><br><code>{{.CurrentTime}}</code></div>
        <div class="item"><strong>客户端</strong><br><code>{{.ClientIP}}</code></div>
      </div>
    </div>

    <div class="card">
      <h2>服务状态</h2>
      <div class="grid">
        {{range .StatusItems}}
        <div class="item">
          <strong>{{.Name}}</strong><br>
          <span class="{{if .OK}}ok{{end}}">{{.Value}}</span><br>
          <small>{{.Detail}}</small>
        </div>
        {{end}}
      </div>
    </div>

    <div class="card">
      <h2>当前请求</h2>
      <div class="grid">
        <div class="item"><strong>Method</strong><br><code>{{.CurrentMethod}}</code></div>
        <div class="item"><strong>Path</strong><br><code>{{.CurrentPath}}</code></div>
        <div class="item"><strong>Query</strong><br><code>{{.CurrentQuery}}</code></div>
      </div>
      <ul>
        <li><code>/?t=你好</code> -> 返回 mp3</li>
        <li><code>/</code> -> 返回 html</li>
        <li><code>/abc</code> -> 返回 html</li>
        <li><code>/?t=你好&amp;x=1</code> -> 返回 html</li>
      </ul>
    </div>
  </div>
</body>
</html>`))
