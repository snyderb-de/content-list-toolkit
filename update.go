package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultReleaseFolder         = `X:\Apps`
	windowsReleaseExecutableName = "content-list-generator.exe"
)

type executableVersionReader func(path string) (string, error)
type executableSignatureVerifier func(path, trustedPath string) error

type preparedUpdate struct {
	StagedPath    string
	TargetPath    string
	ReleasePath   string
	SHA256        string
	LatestVersion string
}

func (a *App) SaveReleaseFolder(path string) error {
	settings, err := a.loadSettings()
	if err != nil {
		settings = AppSettings{
			HashAlgorithm: string(defaultHashAlgorithm()),
			ExcludeHidden: true,
			ExcludeSystem: true,
			CreateXLSX:    true,
			PreserveZeros: true,
			DeleteCSV:     true,
			AgencyFields:  defaultAgencyTemplateFields(),
			ReleaseFolder: defaultReleaseFolder,
		}
	}
	settings.ReleaseFolder = strings.TrimSpace(path)
	return a.writeSettings(settings)
}

func (a *App) CheckForUpdates() (UpdateStatus, error) {
	status := UpdateStatus{
		Supported:      runtime.GOOS == "windows",
		CurrentVersion: appVersion,
	}
	if runtime.GOOS != "windows" {
		status.Message = "Windows executable updates are only available on Windows."
		return status, nil
	}

	settings, err := a.loadSettings()
	status.ReleaseFolder = defaultReleaseFolder
	if err == nil && strings.TrimSpace(settings.ReleaseFolder) != "" {
		status.ReleaseFolder = settings.ReleaseFolder
	}

	currentExe, err := os.Executable()
	if err != nil {
		a.clearPreparedUpdate()
		return status, err
	}

	return a.checkForUpdates(currentExe, readExecutableVersion, verifyExecutableSignature)
}

func (a *App) checkForUpdates(currentExe string, readVersion executableVersionReader, verifySignature executableSignatureVerifier) (UpdateStatus, error) {
	status := UpdateStatus{
		Supported:      true,
		CurrentVersion: appVersion,
	}

	settings, err := a.loadSettings()
	status.ReleaseFolder = defaultReleaseFolder
	if err == nil && strings.TrimSpace(settings.ReleaseFolder) != "" {
		status.ReleaseFolder = settings.ReleaseFolder
	}

	prepared, status, err := prepareExecutableUpdate(currentExe, status.ReleaseFolder, appVersion, readVersion, verifySignature)
	if err != nil {
		a.clearPreparedUpdate()
		return status, err
	}

	a.replacePreparedUpdate(prepared)
	return status, nil
}

func (a *App) clearPreparedUpdate() {
	a.replacePreparedUpdate(nil)
}

func (a *App) replacePreparedUpdate(prepared *preparedUpdate) {
	var oldPrepared *preparedUpdate
	a.updateMu.Lock()
	if a.preparedUpdate != nil && (prepared == nil || a.preparedUpdate.StagedPath != prepared.StagedPath) {
		oldPrepared = a.preparedUpdate
	}
	a.preparedUpdate = prepared
	a.updateMu.Unlock()
	if oldPrepared != nil {
		_ = os.Remove(oldPrepared.StagedPath)
	}
}

func (a *App) RestartToApplyUpdate() error {
	if runtime.GOOS != "windows" {
		return errors.New("Windows executable updates are only available on Windows")
	}

	a.updateMu.Lock()
	prepared := a.preparedUpdate
	a.updateMu.Unlock()
	if prepared == nil {
		return errors.New("no update is ready to apply")
	}
	if err := launchPreparedUpdate(*prepared, os.Getpid()); err != nil {
		return err
	}
	if a.ctx != nil {
		wailsRuntime.Quit(a.ctx)
		return nil
	}
	os.Exit(0)
	return nil
}

func (a *App) writeSettings(settings AppSettings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	path, err := a.settingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func prepareExecutableUpdate(currentExe, releaseFolder, currentVersion string, readVersion executableVersionReader, verifySignature executableSignatureVerifier) (*preparedUpdate, UpdateStatus, error) {
	releaseFolder = strings.TrimSpace(releaseFolder)
	status := UpdateStatus{
		Supported:      runtime.GOOS == "windows",
		CurrentVersion: currentVersion,
		ReleaseFolder:  releaseFolder,
	}
	if releaseFolder == "" {
		status.Message = "No release folder is configured."
		return nil, status, nil
	}

	releasePath := filepath.Join(releaseFolder, windowsReleaseExecutableName)
	status.ReleasePath = releasePath
	info, err := os.Stat(releasePath)
	if errors.Is(err, os.ErrNotExist) {
		status.Message = fmt.Sprintf("%s was not found in the release folder.", windowsReleaseExecutableName)
		return nil, status, nil
	}
	if err != nil {
		return nil, status, err
	}
	if info.IsDir() {
		return nil, status, fmt.Errorf("%s is a folder, not an executable", releasePath)
	}

	latestVersion, err := readVersion(releasePath)
	if err != nil {
		return nil, status, fmt.Errorf("read release executable version: %w", err)
	}
	status.LatestVersion = latestVersion
	if compareAppVersions(latestVersion, currentVersion) <= 0 {
		status.Message = fmt.Sprintf("Content List Toolkit %s is current.", currentVersion)
		return nil, status, nil
	}

	if err := verifySignature(releasePath, currentExe); err != nil {
		return nil, status, fmt.Errorf("verify release executable signature: %w", err)
	}

	sourceHash, err := sha256File(releasePath)
	if err != nil {
		return nil, status, err
	}
	stagedPath, err := copyExecutableToTemp(releasePath)
	if err != nil {
		return nil, status, err
	}
	stagedHash, err := sha256File(stagedPath)
	if err != nil {
		_ = os.Remove(stagedPath)
		return nil, status, err
	}
	if stagedHash != sourceHash {
		_ = os.Remove(stagedPath)
		return nil, status, errors.New("staged update hash does not match release executable")
	}
	if err := verifySignature(stagedPath, currentExe); err != nil {
		_ = os.Remove(stagedPath)
		return nil, status, fmt.Errorf("verify staged executable signature: %w", err)
	}

	prepared := &preparedUpdate{
		StagedPath:    stagedPath,
		TargetPath:    currentExe,
		ReleasePath:   releasePath,
		SHA256:        stagedHash,
		LatestVersion: latestVersion,
	}
	status.UpdateAvailable = true
	status.ReadyToRestart = true
	status.SHA256 = stagedHash
	status.Message = fmt.Sprintf("Content List Toolkit %s is ready to install.", latestVersion)
	return prepared, status, nil
}

func copyExecutableToTemp(source string) (string, error) {
	tempFile, err := os.CreateTemp("", "content-list-generator-update-*.exe")
	if err != nil {
		return "", err
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return "", err
	}
	if err := copyUpdateFile(source, tempPath); err != nil {
		_ = os.Remove(tempPath)
		return "", err
	}
	return tempPath, nil
}

func copyUpdateFile(source, dest string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func compareAppVersions(a, b string) int {
	left := versionParts(a)
	right := versionParts(b)
	for i := 0; i < len(left) || i < len(right); i++ {
		var lv, rv int
		if i < len(left) {
			lv = left[i]
		}
		if i < len(right) {
			rv = right[i]
		}
		if lv > rv {
			return 1
		}
		if lv < rv {
			return -1
		}
	}
	return 0
}

func versionParts(value string) []int {
	value = strings.TrimSpace(strings.TrimPrefix(value, "v"))
	if cut := strings.IndexAny(value, "+-"); cut >= 0 {
		value = value[:cut]
	}
	if value == "" {
		return []int{0}
	}
	rawParts := strings.Split(value, ".")
	parts := make([]int, 0, len(rawParts))
	for _, raw := range rawParts {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			parts = append(parts, 0)
			continue
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			parts = append(parts, 0)
			continue
		}
		parts = append(parts, n)
	}
	return parts
}

func updateScriptName() string {
	return fmt.Sprintf("content-list-generator-apply-update-%d.ps1", time.Now().UnixNano())
}
