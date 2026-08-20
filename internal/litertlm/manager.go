package litertlm

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

const (
	DefaultEndpoint = "http://127.0.0.1:9379"
	DefaultModelID  = "gemma-2b-it"
)

type Config struct {
	ModelPath   string
	RuntimePath string
	RuntimeMode string
	Port        int
}

type Manager struct {
	mu      sync.Mutex
	process processState
}

func NewManager() *Manager { return &Manager{} }

func (m *Manager) Ensure(ctx context.Context, config Config) (endpoint string, modelID string, err error) {
	config = normalizeConfig(config)
	if err := validateModelPath(config.ModelPath); err != nil {
		return "", "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ensurePlatform(ctx, config)
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closePlatform()
}

func normalizeConfig(config Config) Config {
	config.ModelPath = strings.TrimSpace(config.ModelPath)
	config.RuntimePath = strings.TrimSpace(config.RuntimePath)
	config.RuntimeMode = "ondevice"
	if config.Port <= 0 || config.Port > 65535 {
		config.Port = 9379
	}
	return config
}

func validateModelPath(modelPath string) error {
	if modelPath == "" {
		return nil
	}
	ext := strings.ToLower(filepath.Ext(modelPath))
	if ext == ".bin" {
		return fmt.Errorf("LiteRT-LM does not load legacy .bin models; convert gemma-2b-it.bin to a .litertlm package first")
	}
	if ext != ".litertlm" {
		return fmt.Errorf("LiteRT-LM model must use the .litertlm format: %s", modelPath)
	}
	return nil
}

func endpointForPort(port int) string {
	return fmt.Sprintf("http://127.0.0.1:%d", port)
}

func modelIDForPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return DefaultModelID
	}
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name = strings.TrimSpace(name)
	if name == "" {
		return DefaultModelID
	}
	return name
}
