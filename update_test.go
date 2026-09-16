package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompareAppVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "newer patch", a: "0.2.8", b: "0.2.7", want: 1},
		{name: "older minor", a: "0.2.8", b: "0.3.0", want: -1},
		{name: "same with v prefix", a: "v0.2.8", b: "0.2.8", want: 0},
		{name: "missing patch", a: "0.3", b: "0.2.9", want: 1},
		{name: "ignores build metadata", a: "0.2.8+signed", b: "0.2.8", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareAppVersions(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("compareAppVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestWindowsReleaseExecutableName(t *testing.T) {
	if windowsReleaseExecutableName != "content-list-generator.exe" {
		t.Fatalf("windows release executable name = %q, want content-list-generator.exe", windowsReleaseExecutableName)
	}
}

func TestPrepareExecutableUpdateStagesHigherVersion(t *testing.T) {
	workspace := t.TempDir()
	currentExe := filepath.Join(workspace, "installed", windowsReleaseExecutableName)
	if err := os.MkdirAll(filepath.Dir(currentExe), 0o755); err != nil {
		t.Fatalf("mkdir installed: %v", err)
	}
	if err := os.WriteFile(currentExe, []byte("old exe"), 0o644); err != nil {
		t.Fatalf("write current exe: %v", err)
	}

	releaseFolder := filepath.Join(workspace, "release")
	candidate := filepath.Join(releaseFolder, windowsReleaseExecutableName)
	if err := os.MkdirAll(releaseFolder, 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	if err := os.WriteFile(candidate, []byte("new exe"), 0o644); err != nil {
		t.Fatalf("write candidate exe: %v", err)
	}

	var verifiedPaths []string
	prepared, status, err := prepareExecutableUpdate(currentExe, releaseFolder, "0.2.8", func(path string) (string, error) {
		if path != candidate {
			t.Fatalf("read version path = %q, want %q", path, candidate)
		}
		return "0.2.9", nil
	}, func(path, trustedPath string) error {
		if trustedPath != currentExe {
			t.Fatalf("trusted signature path = %q, want current executable %q", trustedPath, currentExe)
		}
		verifiedPaths = append(verifiedPaths, path)
		return nil
	})
	if err != nil {
		t.Fatalf("prepareExecutableUpdate: %v", err)
	}
	if prepared == nil {
		t.Fatalf("expected prepared update")
	}
	defer os.Remove(prepared.StagedPath)
	if !status.ReadyToRestart || !status.UpdateAvailable {
		t.Fatalf("expected update ready status, got %#v", status)
	}
	if len(verifiedPaths) != 2 {
		t.Fatalf("verified paths = %#v, want release and staged executable", verifiedPaths)
	}
	if verifiedPaths[0] != candidate {
		t.Fatalf("first verified path = %q, want release candidate %q", verifiedPaths[0], candidate)
	}
	if verifiedPaths[1] != prepared.StagedPath {
		t.Fatalf("second verified path = %q, want staged executable %q", verifiedPaths[1], prepared.StagedPath)
	}
	if status.LatestVersion != "0.2.9" || status.CurrentVersion != "0.2.8" {
		t.Fatalf("unexpected versions in status: %#v", status)
	}
	if status.ReleasePath != candidate {
		t.Fatalf("release path = %q, want %q", status.ReleasePath, candidate)
	}
	stagedBytes, err := os.ReadFile(prepared.StagedPath)
	if err != nil {
		t.Fatalf("read staged exe: %v", err)
	}
	if string(stagedBytes) != "new exe" {
		t.Fatalf("staged content = %q", stagedBytes)
	}
	if prepared.SHA256 == "" || status.SHA256 != prepared.SHA256 {
		t.Fatalf("expected staged hash in status and prepared update, got status=%#v prepared=%#v", status, prepared)
	}
}

func TestCheckForUpdatesClearsPreparedUpdateOnPrepareError(t *testing.T) {
	isolateUserConfig(t)

	workspace := t.TempDir()
	currentExe := filepath.Join(workspace, "installed", windowsReleaseExecutableName)
	if err := os.MkdirAll(filepath.Dir(currentExe), 0o755); err != nil {
		t.Fatalf("mkdir installed: %v", err)
	}
	if err := os.WriteFile(currentExe, []byte("old exe"), 0o644); err != nil {
		t.Fatalf("write current exe: %v", err)
	}

	releaseFolder := filepath.Join(workspace, "release")
	candidate := filepath.Join(releaseFolder, windowsReleaseExecutableName)
	if err := os.MkdirAll(releaseFolder, 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	if err := os.WriteFile(candidate, []byte("new exe"), 0o644); err != nil {
		t.Fatalf("write candidate exe: %v", err)
	}

	app := newApp("")
	if err := app.SaveReleaseFolder(releaseFolder); err != nil {
		t.Fatalf("save release folder: %v", err)
	}

	status, err := app.checkForUpdates(currentExe, func(string) (string, error) {
		// This path compares against the appVersion const, so the release
		// version has to stay ahead of whatever the app is bumped to.
		return "999.0.0", nil
	}, func(string, string) error {
		return nil
	})
	if err != nil {
		t.Fatalf("check updates: %v", err)
	}
	if !status.ReadyToRestart {
		t.Fatalf("expected update to be ready, got %#v", status)
	}
	app.updateMu.Lock()
	prepared := app.preparedUpdate
	app.updateMu.Unlock()
	if prepared == nil {
		t.Fatalf("expected prepared update")
	}
	stagedPath := prepared.StagedPath
	if _, err := os.Stat(stagedPath); err != nil {
		t.Fatalf("expected staged update to exist: %v", err)
	}

	_, err = app.checkForUpdates(currentExe, func(string) (string, error) {
		return "", errors.New("version metadata unreadable")
	}, func(string, string) error {
		return nil
	})
	if err == nil {
		t.Fatalf("expected update check error")
	}
	app.updateMu.Lock()
	prepared = app.preparedUpdate
	app.updateMu.Unlock()
	if prepared != nil {
		t.Fatalf("expected prepared update to be cleared, got %#v", prepared)
	}
	if _, statErr := os.Stat(stagedPath); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected stale staged update to be removed, stat err=%v", statErr)
	}
}

func TestPrepareExecutableUpdateSkipsCurrentVersion(t *testing.T) {
	workspace := t.TempDir()
	currentExe := filepath.Join(workspace, "installed", windowsReleaseExecutableName)
	if err := os.MkdirAll(filepath.Dir(currentExe), 0o755); err != nil {
		t.Fatalf("mkdir installed: %v", err)
	}
	if err := os.WriteFile(currentExe, []byte("old exe"), 0o644); err != nil {
		t.Fatalf("write current exe: %v", err)
	}
	releaseFolder := filepath.Join(workspace, "release")
	candidate := filepath.Join(releaseFolder, windowsReleaseExecutableName)
	if err := os.MkdirAll(releaseFolder, 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	if err := os.WriteFile(candidate, []byte("same exe"), 0o644); err != nil {
		t.Fatalf("write candidate exe: %v", err)
	}

	prepared, status, err := prepareExecutableUpdate(currentExe, releaseFolder, "0.2.8", func(string) (string, error) {
		return "0.2.8", nil
	}, func(path, trustedPath string) error {
		if trustedPath != currentExe {
			t.Fatalf("trusted signature path = %q, want current executable %q", trustedPath, currentExe)
		}
		t.Fatalf("signature verifier should not run for current version, got %q", path)
		return nil
	})
	if err != nil {
		t.Fatalf("prepareExecutableUpdate: %v", err)
	}
	if prepared != nil {
		t.Fatalf("expected no prepared update, got %#v", prepared)
	}
	if status.UpdateAvailable || status.ReadyToRestart {
		t.Fatalf("expected no update status, got %#v", status)
	}
}

func TestPrepareExecutableUpdateRejectsInvalidSignature(t *testing.T) {
	workspace := t.TempDir()
	currentExe := filepath.Join(workspace, "installed", windowsReleaseExecutableName)
	if err := os.MkdirAll(filepath.Dir(currentExe), 0o755); err != nil {
		t.Fatalf("mkdir installed: %v", err)
	}
	if err := os.WriteFile(currentExe, []byte("old exe"), 0o644); err != nil {
		t.Fatalf("write current exe: %v", err)
	}

	releaseFolder := filepath.Join(workspace, "release")
	candidate := filepath.Join(releaseFolder, windowsReleaseExecutableName)
	if err := os.MkdirAll(releaseFolder, 0o755); err != nil {
		t.Fatalf("mkdir release: %v", err)
	}
	if err := os.WriteFile(candidate, []byte("unsigned exe"), 0o644); err != nil {
		t.Fatalf("write candidate exe: %v", err)
	}

	prepared, status, err := prepareExecutableUpdate(currentExe, releaseFolder, "0.2.8", func(string) (string, error) {
		return "0.2.9", nil
	}, func(path, trustedPath string) error {
		if path != candidate {
			t.Fatalf("signature verifier path = %q, want %q", path, candidate)
		}
		if trustedPath != currentExe {
			t.Fatalf("trusted signature path = %q, want current executable %q", trustedPath, currentExe)
		}
		return errors.New("signature is not trusted")
	})
	if err == nil {
		t.Fatalf("expected invalid signature error")
	}
	if !strings.Contains(err.Error(), "verify release executable signature") {
		t.Fatalf("error = %q, want release signature context", err)
	}
	if prepared != nil {
		t.Fatalf("expected no prepared update, got %#v", prepared)
	}
	if status.ReadyToRestart || status.UpdateAvailable {
		t.Fatalf("expected no ready update after signature failure, got %#v", status)
	}
}
