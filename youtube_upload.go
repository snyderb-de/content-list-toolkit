package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// The resumable upload protocol, hand-written against Google's documentation
// rather than pulled in with the full API client. Exactly one endpoint is
// needed, and the generated client brings gRPC and protobuf with it for a
// desktop application that will never speak either.
//
// The protocol has two steps. A POST carrying the video's metadata returns a
// URL in the Location header; the bytes are then sent to that URL. Sending
// metadata first is what makes an interrupted transfer resumable, and it is
// also what lets the title and description be checked before a gigabyte of
// council meeting goes over the wire.

const (
	youtubeUploadEndpoint = "https://www.googleapis.com/upload/youtube/v3/videos"
	youtubeVideoURLPrefix = "https://www.youtube.com/watch?v="
)

// httpDoer is the seam the tests substitute. The upload code never constructs
// a client of its own, so a test can serve the whole protocol locally.
type httpDoer struct {
	client   *http.Client
	endpoint string
}

func (d *httpDoer) uploadURL() string {
	if d.endpoint != "" {
		return d.endpoint
	}
	return youtubeUploadEndpoint
}

// YouTubeVideo is what will be sent, assembled before anything is uploaded so
// it can be shown to the operator first.
type YouTubeVideo struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	// PrivacyStatus is requested, not guaranteed: an API project that has not
	// passed Google's audit has its uploads forced to private whatever this
	// says.
	PrivacyStatus string   `json:"privacyStatus"`
	Tags          []string `json:"tags,omitempty"`
	CategoryID    string   `json:"categoryId,omitempty"`
}

type videoResource struct {
	Snippet struct {
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Tags        []string `json:"tags,omitempty"`
		CategoryID  string   `json:"categoryId,omitempty"`
	} `json:"snippet"`
	Status struct {
		PrivacyStatus string `json:"privacyStatus"`
	} `json:"status"`
}

type uploadedVideo struct {
	ID     string `json:"id"`
	Status struct {
		PrivacyStatus string `json:"privacyStatus"`
	} `json:"status"`
}

// uploadVideo sends one file and returns the resulting video.
//
// progress is called with bytes sent and the total, so a long upload can show
// something moving. It may be nil.
func uploadVideo(ctx context.Context, doer *httpDoer, videoPath string, video YouTubeVideo, progress func(sent, total int64)) (uploadedVideo, error) {
	file, err := os.Open(videoPath)
	if err != nil {
		return uploadedVideo{}, fmt.Errorf("could not open %s: %w", filepath.Base(videoPath), err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return uploadedVideo{}, err
	}
	if info.Size() == 0 {
		return uploadedVideo{}, fmt.Errorf("%s is empty", filepath.Base(videoPath))
	}

	location, err := startResumableUpload(ctx, doer, video, info.Size())
	if err != nil {
		return uploadedVideo{}, err
	}
	return sendVideoBytes(ctx, doer, location, file, info.Size(), progress)
}

// startResumableUpload sends the metadata and returns the URL the bytes go to.
func startResumableUpload(ctx context.Context, doer *httpDoer, video YouTubeVideo, size int64) (string, error) {
	var resource videoResource
	resource.Snippet.Title = video.Title
	resource.Snippet.Description = video.Description
	resource.Snippet.Tags = video.Tags
	resource.Snippet.CategoryID = video.CategoryID
	resource.Status.PrivacyStatus = video.PrivacyStatus

	body, err := json.Marshal(resource)
	if err != nil {
		return "", err
	}

	url := doer.uploadURL() + "?uploadType=resumable&part=snippet,status"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json; charset=UTF-8")
	request.Header.Set("X-Upload-Content-Type", "video/*")
	request.Header.Set("X-Upload-Content-Length", fmt.Sprint(size))

	response, err := doer.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("could not reach YouTube: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", describeYouTubeError(response)
	}
	location := response.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("YouTube accepted the details but did not say where to send the video")
	}
	return location, nil
}

// sendVideoBytes streams the file to the upload URL, reporting progress.
func sendVideoBytes(ctx context.Context, doer *httpDoer, location string, file io.Reader, size int64, progress func(sent, total int64)) (uploadedVideo, error) {
	body := io.Reader(file)
	if progress != nil {
		body = &countingReader{inner: file, total: size, report: progress}
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPut, location, body)
	if err != nil {
		return uploadedVideo{}, err
	}
	request.Header.Set("Content-Type", "video/*")
	request.ContentLength = size

	response, err := doer.client.Do(request)
	if err != nil {
		return uploadedVideo{}, fmt.Errorf("the upload was interrupted: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated {
		return uploadedVideo{}, describeYouTubeError(response)
	}

	var uploaded uploadedVideo
	if err := json.NewDecoder(response.Body).Decode(&uploaded); err != nil {
		return uploadedVideo{}, fmt.Errorf("the video was sent but YouTube's reply could not be read: %w", err)
	}
	if uploaded.ID == "" {
		return uploadedVideo{}, fmt.Errorf("the video was sent but YouTube did not return a video id")
	}
	return uploaded, nil
}

// countingReader reports progress as the body is consumed.
type countingReader struct {
	inner  io.Reader
	sent   int64
	total  int64
	report func(sent, total int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.inner.Read(p)
	if n > 0 {
		c.sent += int64(n)
		c.report(c.sent, c.total)
	}
	return n, err
}

// describeYouTubeError turns an API error into something actionable.
//
// The quota and audit cases are named specifically because they are the two
// that will actually happen, and neither is fixed by trying again: one waits
// for tomorrow, the other for Google to finish a review.
func describeYouTubeError(response *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<16))

	var parsed struct {
		Error struct {
			Message string `json:"message"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)

	reason := ""
	if len(parsed.Error.Errors) > 0 {
		reason = parsed.Error.Errors[0].Reason
	}

	switch {
	case reason == "quotaExceeded" || reason == "dailyLimitExceeded":
		return fmt.Errorf("this Google project has used its uploads for today; the allowance resets at midnight Pacific time")
	case reason == "forbidden" || response.StatusCode == http.StatusForbidden:
		return fmt.Errorf("YouTube refused the upload: %s", firstLine(valueOrDefault(parsed.Error.Message, string(body))))
	case response.StatusCode == http.StatusUnauthorized:
		return fmt.Errorf("the YouTube sign-in is no longer valid; connect the account again")
	case parsed.Error.Message != "":
		return fmt.Errorf("YouTube returned an error: %s", firstLine(parsed.Error.Message))
	default:
		return fmt.Errorf("YouTube returned HTTP %d: %s", response.StatusCode, firstLine(string(body)))
	}
}

func youtubeWatchURL(id string) string {
	if id == "" {
		return ""
	}
	return youtubeVideoURLPrefix + strings.TrimSpace(id)
}
