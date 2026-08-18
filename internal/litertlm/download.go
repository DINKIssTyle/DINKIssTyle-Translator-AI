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

// GetModelsDir returns the persistent /models folder in Documents or App Support
func GetModelsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		docsModels := filepath.Join(home, "Documents", "models")
		if err := os.MkdirAll(docsModels, 0755); err == nil {
			return docsModels, nil
		}
	}
	configDir, err := os.UserConfigDir()
	if err == nil && configDir != "" {
		appModels := filepath.Join(configDir, "DKST-Translator-AI", "models")
		if err := os.MkdirAll(appModels, 0755); err == nil {
			return appModels, nil
		}
	}
	tmpModels := filepath.Join(os.TempDir(), "dkst-translator-models")
	_ = os.MkdirAll(tmpModels, 0755)
	return tmpModels, nil
}

// ListLocalModels returns all .litertlm files found in the models directory
func ListLocalModels() ([]LocalModelInfo, error) {
	dir, err := GetModelsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var list []LocalModelInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.EqualFold(filepath.Ext(entry.Name()), ".litertlm") {
			info, err := entry.Info()
			if err == nil {
				list = append(list, LocalModelInfo{
					Name:      entry.Name(),
					Path:      filepath.Join(dir, entry.Name()),
					SizeBytes: info.Size(),
					ModTime:   info.ModTime(),
				})
			}
		}
	}
	return list, nil
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

// DownloadModel downloads a .litertlm model into the models directory with progress reporting
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

	if onProgress != nil {
		onProgress(DownloadProgress{
			ModelName:  filename,
			Downloaded: 0,
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

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		if onProgress != nil {
			onProgress(DownloadProgress{ModelName: filename, Status: "error", Error: err.Error()})
		}
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("download failed with HTTP %d %s", resp.StatusCode, resp.Status)
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			err = fmt.Errorf("access denied (HTTP %d). This model might be gated; please provide a valid Hugging Face Access Token", resp.StatusCode)
		}
		if onProgress != nil {
			onProgress(DownloadProgress{ModelName: filename, Status: "error", Error: err.Error()})
		}
		return "", err
	}

	totalBytes := resp.ContentLength

	file, err := os.Create(partPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 64*1024)
	var downloaded int64
	lastEmit := time.Now()

	for {
		select {
		case <-ctx.Done():
			_ = file.Close()
			_ = os.Remove(partPath)
			return "", ctx.Err()
		default:
		}

		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := file.Write(buffer[:n]); writeErr != nil {
				_ = file.Close()
				_ = os.Remove(partPath)
				return "", writeErr
			}
			downloaded += int64(n)

			if onProgress != nil && time.Since(lastEmit) > 200*time.Millisecond {
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
			_ = os.Remove(partPath)
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
