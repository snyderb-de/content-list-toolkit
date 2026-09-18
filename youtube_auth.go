package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/oauth2"
)

// Uploading to YouTube needs a Google account's permission, granted once and
// then remembered.
//
// The credentials belong to the organisation, not to this application. A state
// agency's channel, its daily quota, and the API audit that decides whether
// uploaded videos may be public are all properties of a Google Cloud project
// somebody there owns. So the app reads a client_secret.json that the operator
// supplies rather than shipping credentials of its own — which also keeps
// secrets out of a public repository.
//
// Only the upload scope is requested. It permits inserting a video and nothing
// else: no reading the channel, no editing or deleting what is already there.

const (
	// youtubeUploadScope is the narrowest scope that can insert a video.
	youtubeUploadScope = "https://www.googleapis.com/auth/youtube.upload"

	googleAuthURL  = "https://accounts.google.com/o/oauth2/auth"
	googleTokenURL = "https://oauth2.googleapis.com/token"

	youtubeTokenFileName = "youtube-token.json"
)

// installedAppCredentials is the shape Google Cloud writes when an OAuth client
// of type "Desktop app" is downloaded. Both spellings of the top-level key have
// been used over the years, so both are read.
type installedAppCredentials struct {
	Installed *oauthClientFields `json:"installed"`
	Web       *oauthClientFields `json:"web"`
}

type oauthClientFields struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	AuthURI      string `json:"auth_uri"`
	TokenURI     string `json:"token_uri"`
}

// loadYouTubeConfig reads the operator's OAuth client and returns something
// that can start a sign-in.
//
// The errors here are deliberately specific. "Invalid credentials" sends
// somebody to the wrong place; naming the actual problem — wrong file, wrong
// client type, missing field — is what lets them fix it without a support call.
func loadYouTubeConfig(path string) (*oauth2.Config, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("choose the client_secret.json downloaded from your Google Cloud project")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", filepath.Base(path), err)
	}

	var credentials installedAppCredentials
	if err := json.Unmarshal(raw, &credentials); err != nil {
		return nil, fmt.Errorf("%s is not a Google credentials file", filepath.Base(path))
	}

	fields := credentials.Installed
	if fields == nil {
		if credentials.Web != nil {
			return nil, fmt.Errorf("%s is a Web application client; YouTube upload from a desktop app needs a client of type Desktop app", filepath.Base(path))
		}
		return nil, fmt.Errorf("%s has no OAuth client in it; download the JSON for a Desktop app client", filepath.Base(path))
	}
	if fields.ClientID == "" || fields.ClientSecret == "" {
		return nil, fmt.Errorf("%s is missing its client id or secret", filepath.Base(path))
	}

	authURL := fields.AuthURI
	if authURL == "" {
		authURL = googleAuthURL
	}
	tokenURL := fields.TokenURI
	if tokenURL == "" {
		tokenURL = googleTokenURL
	}

	return &oauth2.Config{
		ClientID:     fields.ClientID,
		ClientSecret: fields.ClientSecret,
		Scopes:       []string{youtubeUploadScope},
		Endpoint:     oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL},
	}, nil
}

// youtubeTokenPath keeps the granted token beside the application's other
// settings, so signing in is a once-per-machine act rather than once per
// upload.
func (a *App) youtubeTokenPath() (string, error) {
	settings, err := a.settingsPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(settings), youtubeTokenFileName), nil
}

func (a *App) loadYouTubeToken() (*oauth2.Token, error) {
	path, err := a.youtubeTokenPath()
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal(raw, &token); err != nil {
		return nil, fmt.Errorf("the saved YouTube sign-in could not be read; sign in again")
	}
	return &token, nil
}

// saveYouTubeToken writes the token readable only by its owner. It grants the
// ability to upload to somebody's channel, which is not something to leave
// world-readable in a shared profile.
func (a *App) saveYouTubeToken(token *oauth2.Token) error {
	path, err := a.youtubeTokenPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

// forgetYouTubeToken removes the saved sign-in, which is what "sign out" has
// to mean for the operator to be able to switch channels.
func (a *App) forgetYouTubeToken() error {
	path, err := a.youtubeTokenPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// youtubeClientFor returns an HTTP client that carries the granted token and
// refreshes it when it expires, or reports that nobody has signed in yet.
func (a *App) youtubeClientFor(ctx context.Context, config *oauth2.Config) (*httpDoer, error) {
	token, err := a.loadYouTubeToken()
	if err != nil {
		return nil, fmt.Errorf("no YouTube account is connected yet")
	}
	// TokenSource refreshes in the background; persisting the refreshed token
	// keeps a long-lived sign-in from expiring between sessions.
	source := config.TokenSource(ctx, token)
	refreshed, err := source.Token()
	if err != nil {
		return nil, fmt.Errorf("the YouTube sign-in has expired; connect the account again")
	}
	if refreshed.AccessToken != token.AccessToken {
		_ = a.saveYouTubeToken(refreshed)
	}
	return &httpDoer{client: secureClient(oauth2.NewClient(ctx, source))}, nil
}
