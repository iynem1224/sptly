package lyrics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

type Track struct {
	Name       string
	Album      string
	Artists    []string
	ProgressMs int
	DurationMs int
}

type LRCLine struct {
	TimeMs int
	Text   string
}

type TokenResponse struct {
	AccessToken string `json:"access_token"`
}

// Creates a new HTTP client for making requests.
func NewHTTPClient() *http.Client {
	return &http.Client{}
}

// Exchanges a refresh token for a new access token.
func GetAccessToken(client *http.Client, clientID, clientSecret, refreshToken string) (string, error) {
	body := bytes.NewBufferString(
		"grant_type=refresh_token&refresh_token=" + refreshToken +
			"&client_id=" + clientID +
			"&client_secret=" + clientSecret)
	req, _ := http.NewRequest("POST", "https://accounts.spotify.com/api/token", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var tokenRes TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenRes); err != nil {
		return "", err
	}
	if tokenRes.AccessToken == "" {
		return "", fmt.Errorf("no access token returned")
	}
	return tokenRes.AccessToken, nil
}

// Retrieves the currently playing track from Spotify.
func GetCurrentlyPlaying(client *http.Client, accessToken string) (Track, error) {
	var t Track
	req, _ := http.NewRequest("GET", "https://api.spotify.com/v1/me/player/currently-playing", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return t, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		io.ReadAll(resp.Body)
		return t, nil
	}

	var data struct {
		ProgressMs int64 `json:"progress_ms"`
		Item       struct {
			Name     string `json:"name"`
			Duration int64  `json:"duration_ms"`
			Album    struct {
				Name string `json:"name"`
			} `json:"album"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
	}
	json.NewDecoder(resp.Body).Decode(&data)

	if data.Item.Name == "" || len(data.Item.Artists) == 0 {
		return t, nil
	}

	t.Name = data.Item.Name
	t.Album = data.Item.Album.Name
	t.Artists = []string{data.Item.Artists[0].Name}
	t.ProgressMs = int(data.ProgressMs)
	t.DurationMs = int(data.Item.Duration)

	return t, nil
}

// Searches for lyrics online and returns either synced or plain lyrics.
func FetchLyrics(track, artist, album string, durationSec int) ([]LRCLine, string, error) {
	client := &http.Client{}

	trackEnc := url.QueryEscape(track)
	artistEnc := url.QueryEscape(artist)
	albumEnc := url.QueryEscape(album)

	searchURL := fmt.Sprintf(
		"https://lrc.davidcheng.ca/api/search?track_name=%s&artist_name=%s&album_name=%s",
		trackEnc, artistEnc, albumEnc,
	)

	searchReq, _ := http.NewRequest("GET", searchURL, nil)
	searchReq.Header.Set("User-Agent", "Mozilla/5.0")
	searchReq.Header.Set("Accept", "application/json")

	searchResp, err := client.Do(searchReq)
	if err != nil {
		return nil, searchURL, err
	}
	defer searchResp.Body.Close()

	searchBody, _ := io.ReadAll(searchResp.Body)

	var results []struct {
		TrackName    string  `json:"trackName"`
		ArtistName   string  `json:"artistName"`
		AlbumName    string  `json:"albumName"`
		Duration     float64 `json:"duration"`
		SyncedLyrics string  `json:"syncedLyrics"`
		PlainLyrics  string  `json:"plainLyrics"`
	}
	_ = json.Unmarshal(searchBody, &results)

	const tol = 2 // duration tolerance

	// Return synced lyrics if they match track info.
	for _, r := range results {
		if r.SyncedLyrics != "" &&
			strings.EqualFold(r.TrackName, track) &&
			strings.EqualFold(r.ArtistName, artist) &&
			(abs(int(r.Duration)-durationSec) <= tol) {

			if strings.Contains(strings.ToLower(r.SyncedLyrics), "rickrolling") {
				continue
			}

			parsed := ParseSyncedLyrics(r.SyncedLyrics)

			// Replace empty lines between lyrics with "♪"
			for i := 1; i < len(parsed)-1; i++ {
				if strings.TrimSpace(parsed[i].Text) == "" &&
					strings.TrimSpace(parsed[i-1].Text) != "" &&
					strings.TrimSpace(parsed[i+1].Text) != "" {
					parsed[i].Text = "♪"
				}
			}

			return parsed, searchURL, nil
		}
	}

	// Otherwise, return plain lyrics if available.
	for _, r := range results {
		if r.PlainLyrics != "" &&
			strings.EqualFold(r.TrackName, track) &&
			strings.EqualFold(r.ArtistName, artist) {

			rawLines := strings.Split(r.PlainLyrics, "\n")
			lines := []LRCLine{}

			for i, line := range rawLines {
				if strings.TrimSpace(line) == "" && i > 0 && i < len(rawLines)-1 &&
					strings.TrimSpace(rawLines[i-1]) != "" && strings.TrimSpace(rawLines[i+1]) != "" {
					line = "♪"
				}
				lines = append(lines, LRCLine{Text: line})
			}

			return lines, searchURL, nil
		}
	}

	return nil, searchURL, fmt.Errorf("no lyrics found for %s - %s", track, artist)
}

// Returns absolute value of an integer.
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Parses synced lyrics from LRC format into structured lines.
func ParseSyncedLyrics(raw string) []LRCLine {
	lines := []LRCLine{}
	re := regexp.MustCompile(`\[(\d+):(\d+)(?:\.(\d+))?\](.*)`)

	for _, l := range strings.Split(raw, "\n") {
		m := re.FindStringSubmatch(l)
		if len(m) == 0 {
			continue
		}
		min, _ := strconv.Atoi(m[1])
		sec, _ := strconv.Atoi(m[2])
		ms := 0
		if m[3] != "" {
			if len(m[3]) == 2 {
				ms, _ = strconv.Atoi(m[3])
				ms *= 10
			} else if len(m[3]) == 3 {
				ms, _ = strconv.Atoi(m[3])
			}
		}
		lines = append(lines, LRCLine{
			TimeMs: min*60*1000 + sec*1000 + ms,
			Text:   m[4],
		})
	}

	return lines
}
