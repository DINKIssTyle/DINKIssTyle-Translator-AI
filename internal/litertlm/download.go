package litertlm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type LocalModelInfo struct {
	Name      string    `json:"name"`
	Path      string    `json:"path"`
	SizeBytes int64     `json:"sizeBytes"`
	ModTime   time.Time `json:"modTime"`
}

type DownloadProgress struct {
	ModelName  string  `json:"modelName"`
	Downloaded int64   `json:"downloaded"`
	Total      int64   `json:"total"`
	Percentage float64 `json:"percentage"`
	Status     string  `json:"status"` // "connecting", "downloading", "completed", "error"
	Error      string  `json:"error,omitempty"`
	Path       string  `json:"path,omitempty"`
}

// GetModelsDir returns the persistent /models folder in OS Application Support or Documents
func GetModelsDir() (string, error) {
	home, err := os.UserHomeDir()
	if runtime.GOOS == "darwin" && err == nil && home != "" {
		appSupportModels := filepath.Join(home, "Library", "Application Support", "DKST Translator AI", "models")
		if err := os.MkdirAll(appSupportModels, 0o755); err == nil {
			return appSupportModels, nil
		}
	}
	if runtime.GOOS == "windows" {
		appData := os.Getenv("APPDATA")
		if appData != "" {
			winModels := filepath.Join(appData, "DKST Translator AI", "models")
			if err := os.MkdirAll(winModels, 0o755); err == nil {
				return winModels, nil
			}
		}
	}
	if runtime.GOOS == "linux" && err == nil && home != "" {
		linuxModels := filepath.Join(home, ".local", "share", "DKST Translator AI", "models")
		if err := os.MkdirAll(linuxModels, 0o755); err == nil {
			return linuxModels, nil
		}
	}
	// Fallback to Documents/models (especially on iOS sandbox or desktop fallback)
	if err == nil && home != "" {
		docsModels := filepath.Join(home, "Documents", "models")
		if err := os.MkdirAll(docsModels, 0o755); err == nil {
			return docsModels, nil
		}
	}
	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		appModels := filepath.Join(configDir, "DKST-Translator-AI", "models")
		if err := os.MkdirAll(appModels, 0o755); err == nil {
			return appModels, nil
		}
	}
	tmpModels := filepath.Join(os.TempDir(), "dkst-translator-models")
	_ = os.MkdirAll(tmpModels, 0o755)
	return tmpModels, nil
}

// getSearchDirs returns all directories where models might be stored or bundled
func getSearchDirs() []string {
	var dirs []string
	if primary, err := GetModelsDir(); err == nil && primary != "" {
		dirs = append(dirs, primary)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, "Library", "Application Support", "DKST Translator AI", "models"),
			filepath.Join(home, "Documents", "models"),
		)
	}
	if executable, err := os.Executable(); err == nil {
		base := filepath.Dir(executable)
		dirs = append(dirs,
			filepath.Join(base, "models"),
			filepath.Join(base, "..", "Resources", "models"),
		)
	}
	dirs = append(dirs, filepath.Join("bin", "models"))
	// Deduplicate existing directories
	seen := make(map[string]bool)
	var valid []string
	for _, d := range dirs {
		clean := filepath.Clean(d)
		if !seen[clean] {
			seen[clean] = true
			if fi, err := os.Stat(clean); err == nil && fi.IsDir() {
				valid = append(valid, clean)
			}
		}
	}
	return valid
}

// ListLocalModels returns all .litertlm files found across search directories
func ListLocalModels() ([]LocalModelInfo, error) {
	searchDirs := getSearchDirs()
	seenNames := make(map[string]bool)
	var list []LocalModelInfo
	for _, dir := range searchDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if strings.EqualFold(filepath.Ext(name), ".litertlm") {
				if seenNames[name] {
					continue
				}
				info, err := entry.Info()
				if err == nil {
					seenNames[name] = true
					list = append(list, LocalModelInfo{
						Name:      name,
						Path:      filepath.Join(dir, name),
						SizeBytes: info.Size(),
						ModTime:   info.ModTime(),
					})
				}
			}
		}
	}
	return list, nil
}

// ImportModelFile copies a .litertlm file into the primary models directory
func ImportModelFile(sourcePath string) (LocalModelInfo, error) {
	cleanSource := strings.TrimSpace(sourcePath)
	if cleanSource == "" {
		return LocalModelInfo{}, fmt.Errorf("model path cannot be empty")
	}
	if err := validateModelPath(cleanSource); err != nil {
		return LocalModelInfo{}, err
	}
	srcInfo, err := os.Stat(cleanSource)
	if err != nil {
		return LocalModelInfo{}, fmt.Errorf("failed to open source model file: %w", err)
	}
	if srcInfo.IsDir() {
		return LocalModelInfo{}, fmt.Errorf("source path is a directory, not a .litertlm file")
	}

	targetDir, err := GetModelsDir()
	if err != nil {
		return LocalModelInfo{}, fmt.Errorf("failed to locate models directory: %w", err)
	}
	filename := filepath.Base(cleanSource)
	targetPath := filepath.Join(targetDir, filename)

	if filepath.Clean(cleanSource) != filepath.Clean(targetPath) {
		srcFile, err := os.Open(cleanSource)
		if err != nil {
			return LocalModelInfo{}, fmt.Errorf("failed to read source model: %w", err)
		}
		defer srcFile.Close()

		dstFile, err := os.Create(targetPath)
		if err != nil {
			return LocalModelInfo{}, fmt.Errorf("failed to create target file in %s: %w", targetDir, err)
		}
		defer dstFile.Close()

		if _, err := io.Copy(dstFile, srcFile); err != nil {
			_ = os.Remove(targetPath)
			return LocalModelInfo{}, fmt.Errorf("failed to copy model file: %w", err)
		}
	}

	targetInfo, err := os.Stat(targetPath)
	if err != nil {
		targetInfo = srcInfo
	}

	return LocalModelInfo{
		Name:      filename,
		Path:      targetPath,
		SizeBytes: targetInfo.Size(),
		ModTime:   targetInfo.ModTime(),
	}, nil
}

// DeleteModelFile removes a model file from storage
func DeleteModelFile(modelPathOrName string) error {
	clean := strings.TrimSpace(modelPathOrName)
	if clean == "" {
		return fmt.Errorf("model path or name is empty")
	}

	// 1. Direct path check
	if filepath.IsAbs(clean) {
		if err := validateModelPath(clean); err != nil {
			return err
		}
		if _, err := os.Stat(clean); err == nil {
			if removeErr := os.Remove(clean); removeErr != nil {
				return fmt.Errorf("failed to delete model at %s: %w", clean, removeErr)
			}
			return nil
		}
	}

	// 2. Search in all search dirs by name or path
	models, err := ListLocalModels()
	if err != nil {
		return err
	}
	for _, m := range models {
		if m.Path == clean || m.Name == clean || strings.TrimSuffix(m.Name, filepath.Ext(m.Name)) == clean {
			if removeErr := os.Remove(m.Path); removeErr != nil {
				return fmt.Errorf("failed to delete model %s: %w", m.Name, removeErr)
			}
			return nil
		}
	}

	return fmt.Errorf("model %q was not found in storage", clean)
}

type hfSibling struct {
	Rfilename string `json:"rfilename"`
}

type hfModelInfo struct {
	Siblings []hfSibling `json:"siblings"`
}

// ResolveDownloadURL determines the final download URL and destination filename
func ResolveDownloadURL(repoOrURL string, token string) (downloadURL string, filename string, err error) {
	raw := strings.TrimSpace(repoOrURL)
	if raw == "" {
		raw = "litert-community/gemma-4-E2B-it-litert-lm"
	}

	// Case 1: Direct URL
	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		parsed, err := url.Parse(raw)
		if err != nil {
			return "", "", err
		}
		// If it's a huggingface tree/main or blob/main URL, convert to resolve/main
		if strings.Contains(parsed.Host, "huggingface.co") {
			path := parsed.Path
			if strings.Contains(path, "/tree/main") {
				repoPath := strings.Split(path, "/tree/main")[0]
				repoPath = strings.TrimPrefix(repoPath, "/")
				return resolveFromHFRepo(repoPath, token)
			}
			if strings.Contains(path, "/blob/main/") {
				path = strings.Replace(path, "/blob/main/", "/resolve/main/", 1)
				parsed.Path = path
				filename = filepath.Base(path)
				return parsed.String(), filename, nil
			}
		}
		filename = filepath.Base(parsed.Path)
		if !strings.HasSuffix(strings.ToLower(filename), ".litertlm") {
			filename += ".litertlm"
		}
		return parsed.String(), filename, nil
	}

	// Case 2: HuggingFace repo path (e.g. "litert-community/gemma-4-E2B-it-litert-lm")
	return resolveFromHFRepo(raw, token)
}

func resolveFromHFRepo(repo string, token string) (string, string, error) {
	repo = strings.TrimPrefix(strings.TrimSuffix(repo, "/"), "/")
	apiURL := fmt.Sprintf("https://huggingface.co/api/models/%s", repo)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", "", err
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		// Fallback to default filename if API unreachable
		fallbackFile := "gemma-4-E2B-it.litertlm"
		return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, fallbackFile), fallbackFile, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		var info hfModelInfo
		if err := json.NewDecoder(resp.Body).Decode(&info); err == nil && len(info.Siblings) > 0 {
			// First pass: prefer standard CPU model (without -gpu) for desktop compatibility
			for _, s := range info.Siblings {
				if strings.EqualFold(filepath.Ext(s.Rfilename), ".litertlm") && !strings.Contains(strings.ToLower(s.Rfilename), "-gpu") {
					return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, s.Rfilename), filepath.Base(s.Rfilename), nil
				}
			}
			for _, s := range info.Siblings {
				if strings.EqualFold(filepath.Ext(s.Rfilename), ".litertlm") {
					return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, s.Rfilename), filepath.Base(s.Rfilename), nil
				}
			}
		}
	}

	fallbackFile := "gemma-4-E2B-it.litertlm"
	return fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, fallbackFile), fallbackFile, nil
}

// DownloadModel downloads a .litertlm model into the models directory with progress reporting and resume capability
func DownloadModel(ctx context.Context, repoOrURL string, token string, onProgress func(DownloadProgress)) (string, error) {
	modelsDir, err := GetModelsDir()
	if err != nil {
		return "", err
	}

	downloadURL, filename, err := ResolveDownloadURL(repoOrURL, token)
	if err != nil {
		return "", err
	}

	targetPath := filepath.Join(modelsDir, filename)
	partPath := targetPath + ".part"

	var existingBytes int64
	if info, err := os.Stat(partPath); err == nil && !info.IsDir() {
		existingBytes = info.Size()
	}

	if onProgress != nil {
		onProgress(DownloadProgress{
			ModelName:  filename,
			Downloaded: existingBytes,
			Total:      0,
			Percentage: 0,
			Status:     "connecting",
		})
	}

	req, err := http.NewRequestWithContext(ctx, "GET", downloadURL, nil)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	if existingBytes > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", existingBytes))
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			if onProgress != nil {
				onProgress(DownloadProgress{ModelName: filename, Downloaded: existingBytes, Status: "cancelled"})
			}
			return "", ctx.Err()
		}
		if onProgress != nil {
			onProgress(DownloadProgress{ModelName: filename, Downloaded: existingBytes, Status: "error", Error: err.Error()})
		}
		return "", err
	}
	defer resp.Body.Close()

	var file *os.File
	var downloaded int64
	var totalBytes int64

	if resp.StatusCode == http.StatusPartialContent {
		// Server accepted range -> append to .part
		file, err = os.OpenFile(partPath, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return "", err
		}
		downloaded = existingBytes
		if resp.ContentLength > 0 {
			totalBytes = existingBytes + resp.ContentLength
		}
	} else if resp.StatusCode == http.StatusOK {
		// Server returned full file from offset 0
		file, err = os.Create(partPath)
		if err != nil {
			return "", err
		}
		downloaded = 0
		totalBytes = resp.ContentLength
	} else if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		// Existing part file was equal or greater than remote file, retry from 0
		_ = os.Remove(partPath)
		file, err = os.Create(partPath)
		if err != nil {
			return "", err
		}
		downloaded = 0
		totalBytes = resp.ContentLength
	} else {
		err := fmt.Errorf("download failed with HTTP %d %s", resp.StatusCode, resp.Status)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			err = fmt.Errorf("access denied (HTTP %d). This model might be gated; please provide a valid Hugging Face Access Token", resp.StatusCode)
		}
		if onProgress != nil {
			onProgress(DownloadProgress{ModelName: filename, Status: "error", Error: err.Error()})
		}
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 64*1024)
	lastEmit := time.Now()

	for {
		select {
		case <-ctx.Done():
			_ = file.Close()
			// Keep .part file for resume on next attempt
			if onProgress != nil {
				percent := float64(0)
				if totalBytes > 0 {
					percent = float64(downloaded) / float64(totalBytes) * 100.0
				}
				onProgress(DownloadProgress{
					ModelName:  filename,
					Downloaded: downloaded,
					Total:      totalBytes,
					Percentage: percent,
					Status:     "cancelled",
				})
			}
			return "", ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
				_ = file.Close()
				return "", writeErr
			}
			downloaded += int64(n)

			if onProgress != nil && time.Since(lastEmit) > 150*time.Millisecond {
				lastEmit = time.Now()
				percent := float64(0)
				if totalBytes > 0 {
					percent = float64(downloaded) / float64(totalBytes) * 100.0
				}
				onProgress(DownloadProgress{
					ModelName:  filename,
					Downloaded: downloaded,
					Total:      totalBytes,
					Percentage: percent,
					Status:     "downloading",
				})
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			_ = file.Close()
			if errors.Is(ctx.Err(), context.Canceled) {
				if onProgress != nil {
					percent := float64(0)
					if totalBytes > 0 {
						percent = float64(downloaded) / float64(totalBytes) * 100.0
					}
					onProgress(DownloadProgress{
						ModelName:  filename,
						Downloaded: downloaded,
						Total:      totalBytes,
						Percentage: percent,
						Status:     "cancelled",
					})
				}
				return "", ctx.Err()
			}
			if onProgress != nil {
				onProgress(DownloadProgress{
					ModelName:  filename,
					Downloaded: downloaded,
					Total:      totalBytes,
					Status:     "error",
					Error:      readErr.Error(),
				})
			}
			return "", readErr
		}
	}

	_ = file.Close()

	// Rename part to final target
	if err := os.Rename(partPath, targetPath); err != nil {
		return "", err
	}

	if onProgress != nil {
		onProgress(DownloadProgress{
			ModelName:  filename,
			Downloaded: downloaded,
			Total:      downloaded,
			Percentage: 100.0,
			Status:     "completed",
			Path:       targetPath,
		})
	}

	return targetPath, nil
}
