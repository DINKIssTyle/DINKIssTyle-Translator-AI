//go:build !ios && !android

package litertlm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type processState struct {
	cmd         *exec.Cmd
	fingerprint string
	endpoint    string
	modelID     string
}

func (m *Manager) ensurePlatform(ctx context.Context, config Config) (string, string, error) {
	if config.ModelPath == "" {
		config.ModelPath = findBundledModel()
	}
	endpoint := endpointForPort(config.Port)
	modelID := modelIDForPath(config.ModelPath)
	fingerprint := strings.Join([]string{config.ModelPath, config.RuntimePath, config.RuntimeMode, strconv.Itoa(config.Port)}, "\x00")
	if m.process.cmd != nil && m.process.fingerprint == fingerprint {
		if err := probeServer(ctx, endpoint); err == nil {
			return endpoint, m.process.modelID, nil
		}
		_ = stopCommand(m.process.cmd)
		m.process = processState{}
	}

	runtimePath, err := findRuntimeExecutable(config.RuntimePath)
	if err != nil {
		return "", "", err
	}
	if config.ModelPath != "" {
		if err := importModel(ctx, runtimePath, config.ModelPath, modelID); err != nil {
			return "", "", err
		}
	}

	host := "127.0.0.1"
	if config.RuntimeMode == "server" {
		host = "0.0.0.0"
	}
	cmd := exec.Command(runtimePath, "serve", "--host", host, "--port", strconv.Itoa(config.Port))
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("start bundled LiteRT-LM runtime: %w", err)
	}
	if err := waitForServer(ctx, endpoint, cmd); err != nil {
		_ = stopCommand(cmd)
		return "", "", err
	}

	m.process = processState{cmd: cmd, fingerprint: fingerprint, endpoint: endpoint, modelID: modelID}
	return endpoint, modelID, nil
}

func findBundledModel() string {
	executable, _ := os.Executable()
	base := filepath.Dir(executable)
	for _, candidate := range []string{
		filepath.Join(base, "models", "gemma-2b-it.litertlm"),
		filepath.Join(base, "..", "Resources", "models", "gemma-2b-it.litertlm"),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func (m *Manager) closePlatform() error {
	if m.process.cmd == nil {
		return nil
	}
	err := stopCommand(m.process.cmd)
	m.process = processState{}
	return err
}

func findRuntimeExecutable(configured string) (string, error) {
	if configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil
		}
		return "", fmt.Errorf("LiteRT-LM runtime was not found at %s", configured)
	}
	if fromEnv := strings.TrimSpace(os.Getenv("DKST_LITERTLM_RUNTIME")); fromEnv != "" {
		return findRuntimeExecutable(fromEnv)
	}

	name := "litert-lm"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	executable, _ := os.Executable()
	base := filepath.Dir(executable)
	platform := runtime.GOOS + "-" + runtime.GOARCH
	candidates := []string{
		filepath.Join(base, "litertlm", platform, name),
		filepath.Join(base, "runtime", "litertlm", platform, name),
		filepath.Join(base, "..", "Resources", "litertlm", platform, name),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath("litert-lm"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("LiteRT-LM runtime is not bundled; set DKST_LITERTLM_RUNTIME or run the litertlm:bundle task for %s", platform)
}

func importModel(ctx context.Context, runtimePath, modelPath, modelID string) error {
	fingerprint, err := modelFingerprint(modelPath)
	if err != nil {
		return err
	}
	markerPath := importMarkerPath(modelID)
	if markerPath != "" {
		if existing, readErr := os.ReadFile(markerPath); readErr == nil && string(existing) == fingerprint {
			return nil
		}
	}
	cmd := exec.CommandContext(ctx, runtimePath, "import", modelPath, modelID)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("import LiteRT-LM model: %w: %s", err, strings.TrimSpace(string(output)))
	}
	if markerPath != "" {
		if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err == nil {
			_ = os.WriteFile(markerPath, []byte(fingerprint), 0o600)
		}
	}
	return nil
}

func modelFingerprint(modelPath string) (string, error) {
	absolute, err := filepath.Abs(modelPath)
	if err != nil {
		return "", fmt.Errorf("resolve LiteRT-LM model path: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return "", fmt.Errorf("inspect LiteRT-LM model: %w", err)
	}
	return strings.Join([]string{absolute, strconv.FormatInt(info.Size(), 10), strconv.FormatInt(info.ModTime().UnixNano(), 10)}, "\x00"), nil
}

func importMarkerPath(modelID string) string {
	cacheDir, err := os.UserCacheDir()
	if err != nil || cacheDir == "" {
		return ""
	}
	return filepath.Join(cacheDir, "dkst-translator-ai", "litertlm", modelID+".imported")
}

func waitForServer(ctx context.Context, endpoint string, cmd *exec.Cmd) error {
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := probeServer(ctx, endpoint); err == nil {
			return nil
		}
		if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
			return fmt.Errorf("LiteRT-LM runtime exited before it became ready")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("timed out waiting for LiteRT-LM runtime at %s", endpoint)
		case <-ticker.C:
		}
	}
}

func probeServer(ctx context.Context, endpoint string) error {
	probeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, endpoint+"/v1/models", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("LiteRT-LM health check returned %s", resp.Status)
	}
	return nil
}

func stopCommand(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	err := cmd.Process.Kill()
	waitErr := cmd.Wait()
	if err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) {
			return waitErr
		}
	}
	return nil
}
