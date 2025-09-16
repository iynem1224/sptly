package main

import (
	"fmt"
	"os"
	"time"

	"sptly/config"
	"sptly/lyrics"
	"sptly/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/liuzl/gocc"
)

func main() {
	// Load configuration, set up defaults if needed
	cfg, err := config.Main()
	if err != nil {
		fmt.Println("Failed to load config:", err)
		return
	}

	client := lyrics.NewHTTPClient()
	var lastTrack string

	// Initialize UI
	model := ui.New(nil)
	model.UpdateChan = make(chan struct{}, 1)
	prog := tea.NewProgram(model, tea.WithAltScreen())

	// Set up Traditional → Simplified converter
	conv, err := gocc.New("t2s")
	if err != nil {
		fmt.Println("Failed to initialize converter:", err)
		return
	}

	token := cfg.AccessToken
	tokenExpiry := time.Now().Add(10 * time.Minute) // refresh every 10 min

	go func() {
		lastIndex := -1
		for {
			// Refresh token if it's time
			if time.Now().After(tokenExpiry) {
				t, err := lyrics.GetAccessToken(client, cfg.ClientID, cfg.ClientSecret, cfg.RefreshToken)
				if err != nil {
					time.Sleep(500 * time.Millisecond)
					continue
				}
				token = t
				tokenExpiry = time.Now().Add(10 * time.Minute) // reset 10-min interval
			}

			track, err := lyrics.GetCurrentlyPlaying(client, token)
			if err != nil || track.Name == "" || len(track.Artists) == 0 {
				time.Sleep(time.Second)
				continue
			}

			currentTrack := fmt.Sprintf("%s - %s", track.Name, track.Artists[0])
			if currentTrack != lastTrack {
				lastTrack = currentTrack

				lines, _, err := lyrics.FetchLyrics(track.Name, track.Artists[0], track.Album, track.DurationMs/1000)
				if err != nil {
					lines = []lyrics.LRCLine{}
				} else {
					for i := range lines {
						lines[i].Text, _ = conv.Convert(lines[i].Text)
					}
				}

				model.Lines = lines
				model.Index = -1
				lastIndex = -1

				select {
				case model.UpdateChan <- struct{}{}:
				default:
				}
			}

			// Update highlighted line
			if len(model.Lines) > 0 {
				curIdx := -1
				for i := len(model.Lines) - 1; i >= 0; i-- {
					if track.ProgressMs >= model.Lines[i].TimeMs {
						curIdx = i
						break
					}
				}
				if curIdx != -1 && curIdx != lastIndex {
					lastIndex = curIdx
					model.Index = curIdx
					select {
					case model.UpdateChan <- struct{}{}:
					default:
					}
				}
			}

			time.Sleep(500 * time.Millisecond)
		}
	}()

	// Launch the UI
	if err := prog.Start(); err != nil {
		fmt.Println("UI error:", err)
		os.Exit(1)
	}
}
