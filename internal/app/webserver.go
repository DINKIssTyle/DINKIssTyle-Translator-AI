package app

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"dinkisstyle-translator/internal/llm"

	"golang.org/x/crypto/bcrypt"
)

const (
	appConfigDirName          = "DKST Translator AI"
	webServerSettingsFileName = "webserver.json"
	defaultWebServerPort      = "8080"
	webSessionCookieName      = "dkst_translator_ai_session"
)

const webLoginPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1.0" />
  <title>DKST Translator AI Login</title>
  <style>
    :root {
      color-scheme: light;
      --bg: #f3f6fb;
      --card: rgba(255,255,255,0.96);
      --line: rgba(214,219,228,0.95);
      --text: #1f2937;
      --soft: #6b7280;
      --accent: #2563eb;
      --danger: #b91c1c;
    }
    * { box-sizing: border-box; }
    body {
      margin: 0;
      min-height: 100vh;
      display: grid;
      place-items: center;
      background:
        radial-gradient(circle at top left, rgba(37,99,235,0.14), transparent 32%),
        radial-gradient(circle at bottom right, rgba(15,23,42,0.08), transparent 30%),
        var(--bg);
      font-family: ui-rounded, "SF Pro Display", "Segoe UI", sans-serif;
      color: var(--text);
      padding: 24px;
    }
    .card {
      width: min(440px, 100%);
      background: var(--card);
      border: 1px solid var(--line);
      border-radius: 24px;
      box-shadow: 0 30px 80px rgba(15, 23, 42, 0.14);
      padding: 28px;
    }
    .mark {
      width: 52px;
      height: 52px;
      border-radius: 16px;
      display: grid;
      place-items: center;
      background: linear-gradient(135deg, #2563eb, #1d4ed8);
      color: white;
      font-weight: 900;
      letter-spacing: 0.06em;
      margin-bottom: 18px;
    }
    h1 {
      margin: 0;
      font-size: 1.4rem;
      font-weight: 900;
      letter-spacing: -0.03em;
    }
    p {
      margin: 8px 0 0;
      color: var(--soft);
      line-height: 1.5;
    }
    form {
      margin-top: 22px;
      display: grid;
      gap: 14px;
    }
    label {
      display: grid;
      gap: 8px;
      font-size: 0.78rem;
      font-weight: 800;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--soft);
    }
    input {
      width: 100%;
      border: 1px solid var(--line);
      background: #f8f9fc;
      border-radius: 14px;
      padding: 14px 15px;
      font-size: 1rem;
    }
    button {
      border: 0;
      border-radius: 14px;
      padding: 14px 16px;
      background: linear-gradient(135deg, #2563eb, #1d4ed8);
      color: white;
      font-size: 0.98rem;
      font-weight: 800;
      cursor: pointer;
    }
    .error {
      margin-top: 14px;
      color: var(--danger);
      font-size: 0.92rem;
      min-height: 1.2em;
    }
    .note {
      margin-top: 16px;
      padding: 12px 14px;
      border-radius: 14px;
      background: rgba(37,99,235,0.08);
      color: var(--soft);
      font-size: 0.9rem;
      line-height: 1.45;
    }
  </style>
</head>
<body>
  <div class="card">
    <div class="mark">DKST</div>
    <h1>DKST Translator AI</h1>
    <p>Enter the web access password to open the translator workspace.</p>
    <form id="login-form">
      <label>
        Password
        <input id="password" name="password" type="password" autocomplete="current-password" required />
      </label>
      <button type="submit">Open Workspace</button>
    </form>
    <div class="error" id="error"></div>
    <div class="note">Only a password is required. The desktop app controls the web server password and TLS settings.</div>
  </div>
  <script>
    const form = document.getElementById('login-form');
    const password = document.getElementById('password');
    const error = document.getElementById('error');
    form.addEventListener('submit', async (event) => {
      event.preventDefault();
      error.textContent = '';
      try {
        const response = await fetch('/login', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ password: password.value })
        });
        if (!response.ok) {
          error.textContent = (await response.text()) || 'Login failed.';
          return;
        }
        const next = new URLSearchParams(window.location.search).get('next') || '/';
        window.location.href = next;
      } catch (err) {
        error.textContent = String(err);
      }
    });
    password.focus();
  </script>
</body>
</html>`

type persistedWebServerSettings struct {
	Enabled      bool   `json:"enabled"`
	Port         string `json:"port"`
	PasswordHash string `json:"passwordHash,omitempty"`
	UseTLS       bool   `json:"useTls"`
	CertDomain   string `json:"certDomain,omitempty"`
	CertPath     string `json:"certPath,omitempty"`
	KeyPath      string `json:"keyPath,omitempty"`
}

type WebServerSettings struct {
	Enabled              bool   `json:"enabled"`
	Port                 string `json:"port"`
	UseTLS               bool   `json:"useTls"`
	CertDomain           string `json:"certDomain,omitempty"`
	CertPath             string `json:"certPath,omitempty"`
	KeyPath              string `json:"keyPath,omitempty"`
	CertificateDirectory string `json:"certificateDirectory"`
	ConfigDirectory      string `json:"configDirectory"`
	HasPassword          bool   `json:"hasPassword"`
	URL                  string `json:"url,omitempty"`
}

type WebServerSettingsInput struct {
	Enabled    bool   `json:"enabled"`
	Port       string `json:"port"`
	Password   string `json:"password,omitempty"`
	UseTLS     bool   `json:"useTls"`
	CertDomain string `json:"certDomain,omitempty"`
	CertPath   string `json:"certPath,omitempty"`
	KeyPath    string `json:"keyPath,omitempty"`
}

type webTranslateResponse struct {
	Text  string                      `json:"text"`
	Stats llm.TranslationStatsPayload `json:"stats"`
}

type sseEventSink struct {
	flusher http.Flusher
	writer  http.ResponseWriter
	mu      sync.Mutex
}

func getAppConfigDir() string {
	configDir, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(configDir) == "" {
		homeDir, homeErr := os.UserHomeDir()
		if homeErr != nil || strings.TrimSpace(homeDir) == "" {
			return "."
		}
		configDir = filepath.Join(homeDir, ".config")
	}
	dir := filepath.Join(configDir, appConfigDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return configDir
	}
	return dir
}

func getCertificateDir() string {
	dir := filepath.Join(getAppConfigDir(), "certs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return filepath.Join(getAppConfigDir(), "certs")
	}
	return dir
}

func getWebServerSettingsPath() string {
	return filepath.Join(getAppConfigDir(), webServerSettingsFileName)
}

func defaultWebServerSettings() persistedWebServerSettings {
	return persistedWebServerSettings{
		Enabled:    false,
		Port:       defaultWebServerPort,
		UseTLS:     false,
		CertDomain: "localhost",
	}
}

func loadPersistedWebServerSettings() persistedWebServerSettings {
	settings := defaultWebServerSettings()
	raw, err := os.ReadFile(getWebServerSettingsPath())
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return defaultWebServerSettings()
	}
	settings.Port = normalizeWebServerPort(settings.Port)
	settings.CertDomain = normalizeCertDomain(settings.CertDomain)
	settings.CertPath = strings.TrimSpace(settings.CertPath)
	settings.KeyPath = strings.TrimSpace(settings.KeyPath)
	return settings
}

func savePersistedWebServerSettings(settings persistedWebServerSettings) error {
	settings.Port = normalizeWebServerPort(settings.Port)
	settings.CertDomain = normalizeCertDomain(settings.CertDomain)
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(getWebServerSettingsPath(), data, 0o600)
}

func normalizeWebServerPort(port string) string {
	port = strings.TrimSpace(port)
	if port == "" {
		return defaultWebServerPort
	}
	for _, r := range port {
		if r < '0' || r > '9' {
			return defaultWebServerPort
		}
	}
	return port
}

func normalizeCertDomain(domain string) string {
	domain = strings.TrimSpace(domain)
	if domain == "" {
		return "localhost"
	}
	return domain
}

func (a *App) currentWebServerSettings() WebServerSettings {
	a.webServerMu.Lock()
	defer a.webServerMu.Unlock()
	return a.currentWebServerSettingsLocked()
}

func (a *App) currentWebServerSettingsLocked() WebServerSettings {
	urlScheme := "http"
	if a.webServerSettings.UseTLS {
		urlScheme = "https"
	}
	return WebServerSettings{
		Enabled:              a.webServerSettings.Enabled,
		Port:                 a.webServerSettings.Port,
		UseTLS:               a.webServerSettings.UseTLS,
		CertDomain:           a.webServerSettings.CertDomain,
		CertPath:             a.webServerSettings.CertPath,
		KeyPath:              a.webServerSettings.KeyPath,
		CertificateDirectory: getCertificateDir(),
		ConfigDirectory:      getAppConfigDir(),
		HasPassword:          strings.TrimSpace(a.webServerSettings.PasswordHash) != "",
		URL:                  fmt.Sprintf("%s://localhost:%s", urlScheme, a.webServerSettings.Port),
	}
}

func (a *App) GetWebServerSettings() (WebServerSettings, error) {
	return a.currentWebServerSettings(), nil
}

func (a *App) SaveWebServerSettings(input WebServerSettingsInput) (WebServerSettings, error) {
	a.webServerMu.Lock()
	defer a.webServerMu.Unlock()

	next := a.webServerSettings
	next.Enabled = input.Enabled
	next.Port = normalizeWebServerPort(input.Port)
	next.UseTLS = input.UseTLS
	next.CertDomain = normalizeCertDomain(input.CertDomain)
	next.CertPath = strings.TrimSpace(input.CertPath)
	next.KeyPath = strings.TrimSpace(input.KeyPath)

	if strings.TrimSpace(input.Password) != "" {
		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			return WebServerSettings{}, err
		}
		next.PasswordHash = string(hash)
	}

	if next.Enabled && strings.TrimSpace(next.PasswordHash) == "" {
		return WebServerSettings{}, fmt.Errorf("web server password is required")
	}

	if next.UseTLS {
		if _, _, err := resolveTLSFiles(next); err != nil {
			return WebServerSettings{}, err
		}
	}

	if err := savePersistedWebServerSettings(next); err != nil {
		return WebServerSettings{}, err
	}

	a.webServerSettings = next
	if err := a.applyWebServerSettingsLocked(); err != nil {
		return WebServerSettings{}, err
	}

	return a.currentWebServerSettingsLocked(), nil
}

func (a *App) OpenCertificateFolder() error {
	dir := getCertificateDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", dir)
	case "windows":
		cmd = exec.Command("explorer", dir)
	default:
		cmd = exec.Command("xdg-open", dir)
	}
	return cmd.Start()
}

func (a *App) applyWebServerSettings() error {
	a.webServerMu.Lock()
	defer a.webServerMu.Unlock()
	return a.applyWebServerSettingsLocked()
}

func (a *App) applyWebServerSettingsLocked() error {
	if a.webServerSettings.Enabled {
		return a.startWebServerLocked()
	}
	return a.stopWebServerLocked()
}

func (a *App) startWebServerLocked() error {
	if strings.TrimSpace(a.webServerSettings.PasswordHash) == "" {
		return fmt.Errorf("web server password is required")
	}

	if a.webServer != nil && a.webServerPort == a.webServerSettings.Port && a.webServerTLS == a.webServerSettings.UseTLS {
		return nil
	}
	if err := a.stopWebServerLocked(); err != nil {
		return err
	}

	handler, err := a.newWebServerHandler()
	if err != nil {
		return err
	}

	server := &http.Server{
		Addr:    ":" + a.webServerSettings.Port,
		Handler: handler,
	}
	a.webServer = server
	a.webServerPort = a.webServerSettings.Port
	a.webServerTLS = a.webServerSettings.UseTLS

	go func(settings persistedWebServerSettings, srv *http.Server) {
		var serveErr error
		if settings.UseTLS {
			certFile, keyFile, err := resolveTLSFiles(settings)
			if err != nil {
				serveErr = err
			} else {
				serveErr = srv.ListenAndServeTLS(certFile, keyFile)
			}
		} else {
			serveErr = srv.ListenAndServe()
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Printf("[web] server stopped: %v\n", serveErr)
		}
	}(a.webServerSettings, server)

	return nil
}

func (a *App) stopWebServerLocked() error {
	if a.webServer == nil {
		a.webServerPort = ""
		a.webServerTLS = false
		return nil
	}
	err := a.webServer.Close()
	a.webServer = nil
	a.webServerPort = ""
	a.webServerTLS = false
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func resolveTLSFiles(settings persistedWebServerSettings) (string, string, error) {
	certPath := strings.TrimSpace(settings.CertPath)
	keyPath := strings.TrimSpace(settings.KeyPath)
	if certPath != "" || keyPath != "" {
		if certPath == "" || keyPath == "" {
			return "", "", fmt.Errorf("both certificate and key files are required for TLS")
		}
		if _, err := os.Stat(certPath); err != nil {
			return "", "", fmt.Errorf("certificate file not found: %s", certPath)
		}
		if _, err := os.Stat(keyPath); err != nil {
			return "", "", fmt.Errorf("key file not found: %s", keyPath)
		}
		return certPath, keyPath, nil
	}

	certDir := getCertificateDir()
	domain := normalizeCertDomain(settings.CertDomain)
	candidates := [][2]string{
		{filepath.Join(certDir, "cert.pem"), filepath.Join(certDir, "key.pem")},
		{filepath.Join(certDir, domain+".crt"), filepath.Join(certDir, domain+".key")},
		{filepath.Join(certDir, domain+".pem"), filepath.Join(certDir, domain+".key")},
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate[0]); err == nil {
			if _, err := os.Stat(candidate[1]); err == nil {
				return candidate[0], candidate[1], nil
			}
		}
	}
	return "", "", fmt.Errorf("no TLS certificate pair found in %s", certDir)
}

func (a *App) newWebServerHandler() (http.Handler, error) {
	sub, err := fs.Sub(a.assets, "frontend/dist")
	if err != nil {
		return nil, err
	}
	fileServer := http.FileServer(http.FS(sub))
	indexBytes, err := fs.ReadFile(sub, "index.html")
	if err != nil {
		return nil, err
	}

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/api/client-config", a.handleWebClientConfig)
	apiMux.HandleFunc("/api/models", a.handleWebModels)
	apiMux.HandleFunc("/api/translate", a.handleWebTranslate)
	apiMux.HandleFunc("/api/translate/stream", a.handleWebTranslateStream)
	apiMux.HandleFunc("/api/cancel", a.handleWebCancel)
	apiMux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})

	rootMux := http.NewServeMux()
	rootMux.HandleFunc("/login", a.handleWebLogin)
	rootMux.HandleFunc("/logout", a.handleWebLogout)
	rootMux.Handle("/api/", a.requireWebSession(apiMux))
	rootMux.Handle("/", a.requireWebSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cleanPath := strings.TrimPrefix(pathClean(r.URL.Path), "/")
		if cleanPath == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(indexBytes)
			return
		}
		if _, err := fs.Stat(sub, cleanPath); err == nil {
			fileServer.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(indexBytes)
	})))

	return rootMux, nil
}

func pathClean(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	cleaned := filepath.ToSlash(filepath.Clean(path))
	if cleaned == "." {
		return "/"
	}
	return cleaned
}

func (a *App) requireWebSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a.isValidWebSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		target := r.URL.RequestURI()
		http.Redirect(w, r, "/login?next="+urlQueryEscape(target), http.StatusFound)
	})
}

func (a *App) handleWebLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(webLoginPageHTML))
	case http.MethodPost:
		var payload struct {
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "invalid login payload", http.StatusBadRequest)
			return
		}
		if !a.verifyWebPassword(payload.Password) {
			http.Error(w, "Incorrect password.", http.StatusUnauthorized)
			return
		}
		token, err := generateWebSessionToken()
		if err != nil {
			http.Error(w, "could not create session", http.StatusInternalServerError)
			return
		}
		a.storeWebSession(token)
		http.SetCookie(w, &http.Cookie{
			Name:     webSessionCookieName,
			Value:    token,
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   a.webServerSettings.UseTLS,
			MaxAge:   60 * 60 * 12,
		})
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (a *App) handleWebLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(webSessionCookieName); err == nil {
		a.deleteWebSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     webSessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   a.webServerSettings.UseTLS,
		MaxAge:   -1,
	})
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (a *App) verifyWebPassword(password string) bool {
	a.webServerMu.Lock()
	hash := strings.TrimSpace(a.webServerSettings.PasswordHash)
	a.webServerMu.Unlock()
	return hash != "" && bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func (a *App) isValidWebSession(r *http.Request) bool {
	cookie, err := r.Cookie(webSessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return false
	}
	a.webServerMu.Lock()
	defer a.webServerMu.Unlock()
	expiry, ok := a.webSessions[cookie.Value]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(a.webSessions, cookie.Value)
		return false
	}
	return true
}

func (a *App) storeWebSession(token string) {
	a.webServerMu.Lock()
	defer a.webServerMu.Unlock()
	a.webSessions[token] = time.Now().Add(12 * time.Hour)
}

func (a *App) deleteWebSession(token string) {
	a.webServerMu.Lock()
	defer a.webServerMu.Unlock()
	delete(a.webSessions, token)
}

func generateWebSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}

func urlQueryEscape(value string) string {
	replacer := strings.NewReplacer(
		"%", "%25",
		" ", "%20",
		"\"", "%22",
		"#", "%23",
		"&", "%26",
		"+", "%2B",
		"?", "%3F",
		"=", "%3D",
	)
	return replacer.Replace(value)
}

func (a *App) handleWebModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	settings := a.hostSettings
	models, err := a.llm.ListModels(settings)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, models)
}

func (a *App) handleWebTranslate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req llm.TranslationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hostSettings := a.hostSettings
	req.Settings.Mode = hostSettings.Mode
	req.Settings.Endpoint = hostSettings.Endpoint
	req.Settings.APIKey = hostSettings.APIKey
	if strings.TrimSpace(req.Settings.Model) == "" {
		req.Settings.Model = hostSettings.Model
	}
	text, stats, err := a.webLLM.TranslateText(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeJSON(w, http.StatusOK, webTranslateResponse{
		Text:  text,
		Stats: stats,
	})
}

func (a *App) handleWebTranslateStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	var req llm.TranslationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hostSettings := a.hostSettings
	req.Settings.Mode = hostSettings.Mode
	req.Settings.Endpoint = hostSettings.Endpoint
	req.Settings.APIKey = hostSettings.APIKey
	if strings.TrimSpace(req.Settings.Model) == "" {
		req.Settings.Model = hostSettings.Model
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sink := &sseEventSink{
		flusher: flusher,
		writer:  w,
	}

	if _, _, err := a.webLLM.TranslateTextStream(req, sink); err != nil {
		sink.send("error", map[string]any{
			"message": err.Error(),
		})
		return
	}
	sink.send("close", map[string]any{"ok": true})
}

func (a *App) handleWebCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	a.webLLM.CancelTranslation()
	w.WriteHeader(http.StatusNoContent)
}

func (a *App) handleWebClientConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, sanitizeProviderSettingsForWeb(a.hostSettings))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *sseEventSink) Token(token string) {
	s.send("token", map[string]any{"token": token})
}

func (s *sseEventSink) Chunk(payload llm.TranslationChunkPayload) {
	s.send("chunk", payload)
}

func (s *sseEventSink) Clear() {
	s.send("clear", map[string]any{"ok": true})
}

func (s *sseEventSink) Complete(payload llm.TranslationCompletePayload) {
	s.send("complete", payload)
}

func (s *sseEventSink) Progress(payload llm.TranslationProgressPayload) {
	s.send("progress", payload)
}

func (s *sseEventSink) Stats(payload llm.TranslationStatsPayload) {
	s.send("stats", payload)
}

func (s *sseEventSink) Debug(direction string, endpoint string, payload string) {
	s.send("debug", map[string]any{
		"direction": direction,
		"endpoint":  endpoint,
		"payload":   payload,
	})
}

func (s *sseEventSink) send(event string, payload any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(payload)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.writer, "event: %s\n", event)
	_, _ = fmt.Fprintf(s.writer, "data: %s\n\n", data)
	s.flusher.Flush()
}
