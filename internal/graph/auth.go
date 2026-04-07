package graph

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	tokenCacheFile = "token_cache.json"
	redirectURI    = "http://localhost:8765"
	scope          = "https://graph.microsoft.com/Files.ReadWrite.All https://graph.microsoft.com/Sites.ReadWrite.All offline_access"
	tenantID       = "9188040d-6c67-4c5b-b112-36a304b66dad"
)

type tokenCache struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

func (tc *tokenCache) get() (string, bool) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.token != "" && time.Now().Before(tc.expiresAt) {
		return tc.token, true
	}
	return "", false
}

func (tc *tokenCache) set(token string, expiresIn int) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.token = token
	tc.expiresAt = time.Now().Add(time.Duration(expiresIn-60) * time.Second)
}

type savedToken struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Authenticator fetches Microsoft Graph tokens via Authorization Code Flow.
type Authenticator struct {
	clientID   string
	cache      tokenCache
	httpClient *http.Client
	saved      *savedToken
}

func NewAuthenticator(tenantIDParam, clientID, clientSecret string) *Authenticator {
	a := &Authenticator{
		clientID:   clientID,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
	a.loadSavedToken()
	return a
}

func (a *Authenticator) Token(ctx context.Context) (string, error) {
	// 1. In-memory cache
	if t, ok := a.cache.get(); ok {
		return t, nil
	}
	// 2. Saved token from file
	if a.saved != nil && time.Now().Before(a.saved.ExpiresAt) {
		a.cache.set(a.saved.AccessToken, int(time.Until(a.saved.ExpiresAt).Seconds()))
		return a.saved.AccessToken, nil
	}
	// 3. Refresh token
	if a.saved != nil && a.saved.RefreshToken != "" {
		if t, err := a.refreshToken(ctx); err == nil {
			return t, nil
		}
	}
	// 4. Authorization code flow
	return a.authCodeFlow(ctx)
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

func randomState() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}

func (a *Authenticator) authCodeFlow(ctx context.Context) (string, error) {
	state := randomState()
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	// Start local HTTP server to receive callback
	listener, err := net.Listen("tcp", "localhost:8765")
	if err != nil {
		return "", fmt.Errorf("could not start local server on port 8765: %w", err)
	}

	srv := &http.Server{}
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			errCh <- fmt.Errorf("state mismatch")
			fmt.Fprintf(w, "<html><body><h2>Error: state mismatch</h2></body></html>")
			return
		}
		if e := q.Get("error"); e != "" {
			errCh <- fmt.Errorf("%s: %s", e, q.Get("error_description"))
			fmt.Fprintf(w, "<html><body><h2>Authorization failed: %s</h2><p>You can close this window.</p></body></html>", e)
			return
		}
		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback")
			return
		}
		fmt.Fprintf(w, "<html><body><h2>✓ Authorization successful!</h2><p>You can close this window and return to the terminal.</p></body></html>")
		codeCh <- code
	})

	go srv.Serve(listener)
	defer srv.Close()

	// Build auth URL
	authURL := fmt.Sprintf(
		"https://login.microsoftonline.com/%s/oauth2/v2.0/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&state=%s&prompt=select_account",
		tenantID,
		url.QueryEscape(a.clientID),
		url.QueryEscape(redirectURI),
		url.QueryEscape(scope),
		url.QueryEscape(state),
	)

	fmt.Println("\n─────────────────────────────────────────────")
	fmt.Println("  Microsoft authorization required")
	fmt.Println("─────────────────────────────────────────────")
	fmt.Println("  Opening browser for sign in...")
	fmt.Println("  Sign in with: alx.polt@outlook.com")
	fmt.Println("─────────────────────────────────────────────")

	openBrowser(authURL)

	// Wait for code or error
	select {
	case code := <-codeCh:
		return a.exchangeCode(ctx, code)
	case err := <-errCh:
		return "", err
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("authorization timeout")
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (a *Authenticator) exchangeCode(ctx context.Context, code string) (string, error) {
	form := url.Values{}
	form.Set("client_id", a.clientID)
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("scope", scope)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://login.microsoftonline.com/"+tenantID+"/oauth2/v2.0/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var tr tokenResponse
	json.Unmarshal(body, &tr)
	if tr.Error != "" || tr.AccessToken == "" {
		return "", fmt.Errorf("token exchange failed: %s: %s", tr.Error, tr.ErrorDesc)
	}

	fmt.Println("  ✓ Authorization successful!")
	fmt.Println("─────────────────────────────────────────────")
	a.cache.set(tr.AccessToken, tr.ExpiresIn)
	a.saveToken(tr.AccessToken, tr.RefreshToken, tr.ExpiresIn)
	return tr.AccessToken, nil
}

func (a *Authenticator) refreshToken(ctx context.Context) (string, error) {
	form := url.Values{}
	form.Set("client_id", a.clientID)
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", a.saved.RefreshToken)
	form.Set("scope", scope)

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://login.microsoftonline.com/"+tenantID+"/oauth2/v2.0/token",
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var tr tokenResponse
	json.Unmarshal(body, &tr)
	if tr.Error != "" || tr.AccessToken == "" {
		return "", fmt.Errorf("refresh failed: %s", tr.Error)
	}

	a.cache.set(tr.AccessToken, tr.ExpiresIn)
	a.saveToken(tr.AccessToken, tr.RefreshToken, tr.ExpiresIn)
	return tr.AccessToken, nil
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Start()
}

func (a *Authenticator) saveToken(access, refresh string, expiresIn int) {
	saved := savedToken{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn-60) * time.Second),
	}
	a.saved = &saved
	data, _ := json.MarshalIndent(saved, "", "  ")
	os.WriteFile(tokenCacheFile, data, 0600)
}

func (a *Authenticator) loadSavedToken() {
	data, err := os.ReadFile(tokenCacheFile)
	if err != nil {
		return
	}
	var saved savedToken
	if err := json.Unmarshal(data, &saved); err != nil {
		return
	}
	a.saved = &saved
}
