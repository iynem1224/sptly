package config

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
}

const Scopes = "user-read-playback-state user-read-currently-playing"

// Load the config or run setup if it's missing
func Main() (*Config, error) {
	cfg, err := loadConfig()
	if err == nil {
		return cfg, nil
	}
	return setupConfig()
}

// Return the OS-specific config file path
func getConfigPath() string {
	var dir string
	if runtime.GOOS == "windows" {
		dir = os.Getenv("APPDATA")
	} else {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "sptly", "config.json")
}

func loadConfig() (*Config, error) {
	path := getConfigPath()
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var cfg Config
	if err := json.NewDecoder(f).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func saveConfig(cfg *Config) error {
	path := getConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(cfg)
}

// Guide and setup flow
func setupConfig() (*Config, error) {
	fmt.Println("Config not found. Opening Spotify Developer Dashboard in 5 seconds...")
	fmt.Println("Steps:")
	fmt.Println(" 1. Create a new app at https://developer.spotify.com/dashboard")
	fmt.Println(" 2. Copy Client ID, Client Secret, and set Redirect URI (example: http://127.0.0.1:8888/callback)")
	fmt.Println(" 3. Paste them here when prompted")

	time.Sleep(5 * time.Second)
	openBrowser("https://developer.spotify.com/dashboard")

	var clientID, clientSecret, redirectURI string
	fmt.Print("Client ID: ")
	fmt.Scanln(&clientID)
	fmt.Print("Client Secret: ")
	fmt.Scanln(&clientSecret)
	fmt.Print("Redirect URI: ")
	fmt.Scanln(&redirectURI)

	codeCh := make(chan string)
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			w.Write([]byte("No code received"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(`<html><body><p>Authorization complete. You can close this window.</p><script>window.close();</script></body></html>`))
		codeCh <- code
	})
	go http.ListenAndServe(":8888", nil)

	authURL := fmt.Sprintf(
		"https://accounts.spotify.com/authorize?client_id=%s&response_type=code&redirect_uri=%s&scope=%s&show_dialog=true",
		clientID, url.QueryEscape(redirectURI), url.QueryEscape(Scopes),
	)
	fmt.Println("Opening Spotify authorization page...")
	openBrowser(authURL)

	code := <-codeCh

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)

	resp, err := http.PostForm("https://accounts.spotify.com/api/token", data)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}
	_ = json.Unmarshal(body, &tokenResp)

	cfg := &Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURI:  redirectURI,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
	}

	if err := saveConfig(cfg); err != nil {
		return nil, err
	}

	fmt.Println("Config saved to", getConfigPath())
	return cfg, nil
}

// Launch the default browser
func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		fmt.Println("Open this URL manually:", url)
		return
	}
	_ = cmd.Start()
}
