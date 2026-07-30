package update

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/jungdosa/QuotaDock/internal/security"
)

const stagedInstallerName = "QuotaDock-update-Setup.exe"

var ErrHashMismatch = errors.New("update file SHA-256 mismatch")

type Stage int

const (
	StageDownloading Stage = iota
	StageVerifying
	StageInstalling
)

type Progress struct {
	Stage   Stage
	Percent int
}

type Launcher interface {
	Launch(installerPath, executablePath string) error
}

type Installer struct {
	Client         *http.Client
	DownloadDir    string
	Version        string
	ExecutablePath func() (string, error)
	Launcher       Launcher
}

func DefaultDownloadDir() (string, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate local application data: %w", err)
	}
	return filepath.Join(cacheDir, "QuotaDock", "updates"), nil
}

func (i *Installer) Install(ctx context.Context, asset Asset, report func(Progress)) error {
	if i == nil || i.Launcher == nil {
		return errors.New("installer launcher is unavailable")
	}
	expected, valid := ParseSHA256Digest(asset.Digest)
	if !valid {
		return errors.New("release asset has no valid SHA-256 digest")
	}
	if !isSafeSetupName(asset.Name) {
		return errors.New("release asset is not a setup executable")
	}
	if asset.Size <= 0 || asset.Size > int64(^uint64(0)>>1)-64*1024-1 {
		return errors.New("release asset size is invalid")
	}
	if !securityURLAllowed(asset.BrowserDownloadURL) {
		return errors.New("release asset URL is not allowed")
	}

	downloadDir := i.DownloadDir
	if downloadDir == "" {
		var err error
		downloadDir, err = DefaultDownloadDir()
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(downloadDir, 0o700); err != nil {
		return fmt.Errorf("create update directory: %w", err)
	}
	temporary, err := os.CreateTemp(downloadDir, ".quotadock-update-*.download")
	if err != nil {
		return fmt.Errorf("create update download: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
	}()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, asset.BrowserDownloadURL, nil)
	if err != nil {
		return fmt.Errorf("create update download request: %w", err)
	}
	request.Header.Set("Accept", "application/octet-stream")
	request.Header.Set("User-Agent", "QuotaDock/"+i.Version)
	response, err := updateHTTPClient(i.Client).Do(request)
	if err != nil {
		return fmt.Errorf("download update: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("update download returned HTTP %d", response.StatusCode)
	}
	if response.ContentLength != asset.Size {
		return fmt.Errorf("update download length %d does not match asset size %d", response.ContentLength, asset.Size)
	}

	reportProgress(report, Progress{Stage: StageDownloading, Percent: 0})
	writer := &downloadProgressWriter{writer: temporary, size: asset.Size, report: report, lastPercent: -1}
	maximum := asset.Size + 64*1024
	written, err := io.Copy(writer, io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return fmt.Errorf("write update download: %w", err)
	}
	if written != asset.Size {
		return fmt.Errorf("received %d update bytes, expected %d", written, asset.Size)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close update download: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	reportProgress(report, Progress{Stage: StageVerifying})
	if err := verifyDownloadedHash(temporaryPath, expected); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	finalPath := filepath.Join(downloadDir, stagedInstallerName)
	if err := os.Remove(finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("replace previous update: %w", err)
	}
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return fmt.Errorf("finalize update download: %w", err)
	}
	removeFinal := true
	defer func() {
		if removeFinal {
			_ = os.Remove(finalPath)
		}
	}()

	executablePath := os.Executable
	if i.ExecutablePath != nil {
		executablePath = i.ExecutablePath
	}
	currentExecutable, err := executablePath()
	if err != nil {
		return fmt.Errorf("locate current executable: %w", err)
	}
	if !isSafeSetupName(filepath.Base(finalPath)) {
		return errors.New("installer filename is not a setup executable")
	}
	reportProgress(report, Progress{Stage: StageInstalling, Percent: 100})
	if err := ctx.Err(); err != nil {
		return err
	}
	// This second, deliberately separate verification is the TOCTOU boundary.
	// Launch follows immediately so no UI callback can widen the checked window.
	if err := verifyInstallerHash(finalPath, expected); err != nil {
		return err
	}
	if err := i.Launcher.Launch(finalPath, currentExecutable); err != nil {
		return fmt.Errorf("launch update installer: %w", err)
	}
	removeFinal = false
	return nil
}

func isSafeSetupName(name string) bool {
	return name != "" && name == filepath.Base(name) && strings.HasSuffix(strings.ToLower(name), "-setup.exe")
}

func securityURLAllowed(raw string) bool {
	return security.IsAllowedUpdateURL(raw)
}

func verifyDownloadedHash(path, expected string) error {
	return verifyFileSHA256(path, expected)
}

func verifyInstallerHash(path, expected string) error {
	return verifyFileSHA256(path, expected)
}

func verifyFileSHA256(path, expected string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open update for verification: %w", err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("hash update file: %w", err)
	}
	expectedBytes, err := hex.DecodeString(expected)
	if err != nil || len(expectedBytes) != sha256.Size {
		return errors.New("expected SHA-256 digest is invalid")
	}
	if subtle.ConstantTimeCompare(hash.Sum(nil), expectedBytes) != 1 {
		return ErrHashMismatch
	}
	return nil
}

type downloadProgressWriter struct {
	writer      io.Writer
	size        int64
	written     int64
	report      func(Progress)
	lastPercent int
}

func (w *downloadProgressWriter) Write(data []byte) (int, error) {
	count, err := w.writer.Write(data)
	w.written += int64(count)
	percent := int(w.written * 100 / w.size)
	if percent > 100 {
		percent = 100
	}
	if percent != w.lastPercent {
		w.lastPercent = percent
		reportProgress(w.report, Progress{Stage: StageDownloading, Percent: percent})
	}
	return count, err
}

func reportProgress(report func(Progress), progress Progress) {
	if report != nil {
		report(progress)
	}
}
