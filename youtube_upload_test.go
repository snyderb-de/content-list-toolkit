package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A stand-in for the two-step resumable protocol: a POST carrying metadata
// that answers with a Location, then a PUT carrying the bytes.
func fakeYouTube(t *testing.T, onUpload func(body []byte)) (*httpDoer, *videoResource) {
	t.Helper()
	received := &videoResource{}

	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			if err := json.NewDecoder(r.Body).Decode(received); err != nil {
				t.Errorf("decode metadata: %v", err)
			}
			w.Header().Set("Location", server.URL+"/bytes")
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	mux.HandleFunc("/bytes", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if onUpload != nil {
			onUpload(body)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"abc123","status":{"privacyStatus":"private"}}`))
	})

	return &httpDoer{client: server.Client(), endpoint: server.URL + "/upload"}, received
}

func videoFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "meeting.mp4")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write video: %v", err)
	}
	return path
}

func TestUploadVideoSendsMetadataThenBytes(t *testing.T) {
	var uploaded []byte
	doer, received := fakeYouTube(t, func(body []byte) { uploaded = body })

	result, err := uploadVideo(context.Background(), doer, videoFile(t, "video-bytes"), YouTubeVideo{
		Title:         "Wilmington City Council Committee Meeting",
		Description:   "Resource ID: 9200-B35-002",
		PrivacyStatus: "private",
	}, nil)
	if err != nil {
		t.Fatalf("uploadVideo: %v", err)
	}

	if result.ID != "abc123" {
		t.Fatalf("ID = %q", result.ID)
	}
	if received.Snippet.Title != "Wilmington City Council Committee Meeting" {
		t.Fatalf("title not sent: %q", received.Snippet.Title)
	}
	if received.Snippet.Description != "Resource ID: 9200-B35-002" {
		t.Fatalf("description not sent: %q", received.Snippet.Description)
	}
	if received.Status.PrivacyStatus != "private" {
		t.Fatalf("privacy not sent: %q", received.Status.PrivacyStatus)
	}
	if string(uploaded) != "video-bytes" {
		t.Fatalf("file contents not sent: %q", string(uploaded))
	}
}

// Metadata goes first precisely so it can be wrong cheaply. A rejected title
// must not cost a gigabyte of transfer.
func TestUploadStopsBeforeSendingBytesWhenMetadataIsRejected(t *testing.T) {
	sentBytes := false
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid title","errors":[{"reason":"invalidTitle"}]}}`))
	})
	mux.HandleFunc("/bytes", func(w http.ResponseWriter, r *http.Request) { sentBytes = true })

	doer := &httpDoer{client: server.Client(), endpoint: server.URL + "/upload"}
	_, err := uploadVideo(context.Background(), doer, videoFile(t, "x"), YouTubeVideo{Title: ""}, nil)
	if err == nil {
		t.Fatal("expected an error")
	}
	if sentBytes {
		t.Fatal("the file was sent despite the metadata being rejected")
	}
	if !strings.Contains(err.Error(), "Invalid title") {
		t.Fatalf("error should carry what YouTube said, got %q", err)
	}
}

func TestUploadReportsProgress(t *testing.T) {
	doer, _ := fakeYouTube(t, nil)

	var lastSent, lastTotal int64
	calls := 0
	_, err := uploadVideo(context.Background(), doer, videoFile(t, strings.Repeat("x", 4096)), YouTubeVideo{Title: "t"},
		func(sent, total int64) { lastSent, lastTotal, calls = sent, total, calls+1 })
	if err != nil {
		t.Fatalf("uploadVideo: %v", err)
	}
	if calls == 0 {
		t.Fatal("expected progress to be reported")
	}
	if lastTotal != 4096 {
		t.Fatalf("total = %d, want 4096", lastTotal)
	}
	if lastSent != lastTotal {
		t.Fatalf("finished at %d of %d", lastSent, lastTotal)
	}
}

// The two errors that will actually happen say what to do, because neither is
// fixed by trying again.
func TestUploadNamesQuotaAndAuthFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		want   string
	}{
		{
			name:   "quota",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"quota","errors":[{"reason":"quotaExceeded"}]}}`,
			want:   "used its uploads for today",
		},
		{
			name:   "expired sign-in",
			status: http.StatusUnauthorized,
			body:   `{"error":{"message":"Invalid Credentials"}}`,
			want:   "connect the account again",
		},
		{
			name:   "refused",
			status: http.StatusForbidden,
			body:   `{"error":{"message":"The user is not enabled for video upload.","errors":[{"reason":"forbidden"}]}}`,
			want:   "not enabled for video upload",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)

			doer := &httpDoer{client: server.Client(), endpoint: server.URL}
			_, err := uploadVideo(context.Background(), doer, videoFile(t, "x"), YouTubeVideo{Title: "t"}, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestUploadRejectsAnEmptyOrMissingFile(t *testing.T) {
	doer, _ := fakeYouTube(t, nil)

	if _, err := uploadVideo(context.Background(), doer, videoFile(t, ""), YouTubeVideo{Title: "t"}, nil); err == nil ||
		!strings.Contains(err.Error(), "is empty") {
		t.Fatalf("expected an empty-file error, got %v", err)
	}
	if _, err := uploadVideo(context.Background(), doer, filepath.Join(t.TempDir(), "gone.mp4"), YouTubeVideo{Title: "t"}, nil); err == nil ||
		!strings.Contains(err.Error(), "could not open") {
		t.Fatalf("expected a missing-file error, got %v", err)
	}
}

// Without a Location there is nowhere to send the bytes, and saying so beats a
// confusing failure on the next request.
func TestUploadReportsAMissingUploadLocation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)

	doer := &httpDoer{client: server.Client(), endpoint: server.URL}
	_, err := uploadVideo(context.Background(), doer, videoFile(t, "x"), YouTubeVideo{Title: "t"}, nil)
	if err == nil || !strings.Contains(err.Error(), "did not say where to send") {
		t.Fatalf("error = %v", err)
	}
}

func TestYouTubeWatchURL(t *testing.T) {
	if got := youtubeWatchURL("abc123"); got != "https://www.youtube.com/watch?v=abc123" {
		t.Fatalf("got %q", got)
	}
	if got := youtubeWatchURL(""); got != "" {
		t.Fatalf("an empty id should produce no link, got %q", got)
	}
}

var _ = fmt.Sprint
