package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"dinkisstyle-translator/internal/llm"
)

const hostProviderSettingsFileName = "provider-settings.json"

func defaultHostProviderSettings() llm.ProviderSettings {
	return llm.ProviderSettings{
		Mode:                             "lmstudio",
		Endpoint:                         "http://127.0.0.1:1234",
		APIKey:                           "",
		Model:                            "",
		Reasoning:                        "",
		Temperature:                      0,
		ForceShowReasoning:               true,
		ForceShowTemperature:             true,
		EnablePostEdit:                   true,
		EnableTopicAwarePostEdit:         true,
		EnableEnhancedContextTranslation: false,
		EnableSmartChunking:              true,
		SmartChunkSize:                   1000,
	}
}

func hostProviderSettingsPath() string {
	return filepath.Join(getAppConfigDir(), hostProviderSettingsFileName)
}

func loadPersistedHostProviderSettings() llm.ProviderSettings {
	settings := defaultHostProviderSettings()
	raw, err := os.ReadFile(hostProviderSettingsPath())
	if err != nil {
		return settings
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return defaultHostProviderSettings()
	}
	if strings.TrimSpace(settings.Mode) == "" {
		settings.Mode = "lmstudio"
	}
	if strings.TrimSpace(settings.Endpoint) == "" {
		settings.Endpoint = "http://127.0.0.1:1234"
	}
	if settings.SmartChunkSize <= 0 {
		settings.SmartChunkSize = 1000
	}
	return settings
}

func savePersistedHostProviderSettings(settings llm.ProviderSettings) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(hostProviderSettingsPath(), data, 0o600)
}

func sanitizeProviderSettingsForWeb(settings llm.ProviderSettings) llm.ProviderSettings {
	settings.APIKey = ""
	settings.Endpoint = ""
	settings.DebugTranslationPromptTemplate = ""
	settings.DebugPostEditPromptTemplate = ""
	return settings
}
