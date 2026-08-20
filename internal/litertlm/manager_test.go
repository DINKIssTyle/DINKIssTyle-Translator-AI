package litertlm

import "testing"

func TestValidateModelPath(t *testing.T) {
	if err := validateModelPath("gemma-2b-it.litertlm"); err != nil {
		t.Fatalf("valid LiteRT-LM model rejected: %v", err)
	}
	if err := validateModelPath("gemma-2b-it.bin"); err == nil {
		t.Fatal("legacy .bin model should be rejected with a conversion hint")
	}
	if err := validateModelPath("model.gguf"); err == nil {
		t.Fatal("non-LiteRT-LM model should be rejected")
	}
}

func TestNormalizeConfig(t *testing.T) {
	config := normalizeConfig(Config{RuntimeMode: "SERVER", Port: 70000})
	if config.RuntimeMode != "ondevice" {
		t.Fatalf("runtime mode = %q, want ondevice", config.RuntimeMode)
	}
	if config.Port != 9379 {
		t.Fatalf("port = %d, want 9379", config.Port)
	}
	if got := modelIDForPath("/models/gemma-2b-it.litertlm"); got != "gemma-2b-it" {
		t.Fatalf("model ID = %q", got)
	}
}
