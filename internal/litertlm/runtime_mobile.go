//go:build ios || android

package litertlm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type processState struct {
	fingerprint string
	endpoint    string
	modelID     string
}

func (m *Manager) ensurePlatform(ctx context.Context, config Config) (string, string, error) {
	endpoint := endpointForPort(config.Port)
	modelID := modelIDForPath(config.ModelPath)
	fingerprint := fmt.Sprintf("%s\x00%d", config.ModelPath, config.Port)
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/v1/models", nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				if m.process.fingerprint == fingerprint {
					return endpoint, m.process.modelID, nil
				}
				if config.ModelPath != "" {
					payload, _ := json.Marshal(map[string]string{"modelPath": config.ModelPath})
					configureReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/configure", bytes.NewReader(payload))
					configureReq.Header.Set("Content-Type", "application/json")
					configureResp, configureErr := http.DefaultClient.Do(configureReq)
					if configureErr == nil {
						_ = configureResp.Body.Close()
					}
				}
				m.process = processState{fingerprint: fingerprint, endpoint: endpoint, modelID: modelID}
				return endpoint, modelID, nil
			}
		}
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
	return "", "", fmt.Errorf("the bundled mobile LiteRT-LM SDK adapter did not start at %s", endpoint)
}

func (m *Manager) closePlatform() error {
	m.process = processState{}
	return nil
}
