package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	wikiCitationPattern      = regexp.MustCompile(`\[(\d+)\]`)
	multiSpacePattern        = regexp.MustCompile(`[ \t]{2,}`)
	spaceBeforePunctPattern  = regexp.MustCompile(`\s+([,.;:!?])`)
	protectedTermPattern     = regexp.MustCompile(`\b(?:[A-Z][a-z]+(?:[\s-]+[A-Z][a-z]+)+|[A-Z]{2,}(?:[-/][A-Z0-9]{2,})*|[A-Z][a-zA-Z0-9]+(?:[-/][A-Za-z0-9]+)+)\b`)
	glossaryBlockPattern     = regexp.MustCompile(`(?s)\n?(?:Use the following User Glossary.*?\n<GLOSSARY>\n.*?\n</GLOSSARY>\n?|User Glossary:\n<GLOSSARY>\n.*?\n</GLOSSARY>\n?)`)
	emptyDebugSectionPattern = regexp.MustCompile(`(?m)^(Protected names and terms|User glossary|Chunk label|Previous context|Opening source paragraph|Opening translated paragraph|Recent overlap):\n(?:[ \t]*\n)?`)
)

type Client struct {
	ctx         context.Context
	mu          sync.Mutex
	cancel      context.CancelFunc
	active      uint64
	requestSink eventSink
}

type eventSink interface {
	Token(string)
	Clear()
	Complete(TranslationCompletePayload)
	Progress(TranslationProgressPayload)
	Stats(TranslationStatsPayload)
	Debug(direction string, endpoint string, payload string)
}

func NewClient() *Client {
	return &Client{}
}

func (c *Client) SetContext(ctx context.Context) {
	c.ctx = ctx
}

func (c *Client) beginRequest() (context.Context, context.CancelFunc, uint64) {
	parent := c.ctx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)

	c.mu.Lock()
	if c.cancel != nil {
		c.cancel()
	}
	c.active++
	c.cancel = cancel
	requestID := c.active
	c.mu.Unlock()

	return ctx, cancel, requestID
}

func (c *Client) beginRequestWithSink(sink eventSink) (context.Context, context.CancelFunc, uint64) {
	ctx, cancel, requestID := c.beginRequest()
	c.mu.Lock()
	c.requestSink = sink
	c.mu.Unlock()
	return ctx, cancel, requestID
}

func (c *Client) finishRequest(requestID uint64) {
	c.mu.Lock()
	if c.active == requestID {
		c.cancel = nil
		c.requestSink = nil
	}
	c.mu.Unlock()
}

func (c *Client) CancelTranslation() {
	c.mu.Lock()
	cancel := c.cancel
	c.cancel = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (c *Client) httpClient() *http.Client {
	return &http.Client{Timeout: 15 * time.Second}
}

func (c *Client) streamingClient() *http.Client {
	return &http.Client{}
}

type ProviderSettings struct {
	Mode                             string  `json:"mode"`
	Endpoint                         string  `json:"endpoint"`
	APIKey                           string  `json:"apiKey"`
	Model                            string  `json:"model"`
	Reasoning                        string  `json:"reasoning,omitempty"`
	Temperature                      float64 `json:"temperature,omitempty"`
	ForceShowTemperature             bool    `json:"forceShowTemperature"`
	ForceShowReasoning               bool    `json:"forceShowReasoning"`
	EnablePostEdit                   bool    `json:"enablePostEdit"`
	EnableTopicAwarePostEdit         bool    `json:"enableTopicAwarePostEdit"`
	EnableEnhancedContextTranslation bool    `json:"enableEnhancedContextTranslation"`
	EnhancedContextGlossary          string  `json:"enhancedContextGlossary,omitempty"`
	EnableSmartChunking              bool    `json:"enableSmartChunking"`
	SmartChunkSize                   int     `json:"smartChunkSize,omitempty"`
	DebugTranslationPromptTemplate   string  `json:"debugTranslationPromptTemplate,omitempty"`
	DebugPostEditPromptTemplate      string  `json:"debugPostEditPromptTemplate,omitempty"`
}

type TranslationRequest struct {
	Settings    ProviderSettings `json:"settings"`
	SourceText  string           `json:"sourceText"`
	SourceLang  string           `json:"sourceLang"`
	TargetLang  string           `json:"targetLang"`
	Instruction string           `json:"instruction"`
}

type modelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

type lmStudioModelsResponse struct {
	Models []struct {
		Type        string `json:"type"`
		Key         string `json:"key"`
		DisplayName string `json:"display_name"`
		Reasoning   any    `json:"reasoning"`
	} `json:"models"`
}

type ModelInfo struct {
	ID                string   `json:"id"`
	DisplayName       string   `json:"displayName,omitempty"`
	SupportsReasoning bool     `json:"supportsReasoning"`
	ReasoningOptions  []string `json:"reasoningOptions,omitempty"`
}

type chatCompletionsRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Stream          bool          `json:"stream"`
	Store           *bool         `json:"store,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
	Temperature     *float64      `json:"temperature,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type TranslationCompletePayload struct {
	Text string `json:"text"`
}

type TranslationProgressPayload struct {
	Stage           string   `json:"stage"`
	Label           string   `json:"label"`
	Detail          string   `json:"detail,omitempty"`
	Progress        *float64 `json:"progress,omitempty"`
	CurrentChunk    int      `json:"current_chunk,omitempty"`
	CompletedChunks int      `json:"completed_chunks,omitempty"`
	TotalChunks     int      `json:"total_chunks,omitempty"`
	CurrentStep     int      `json:"current_step,omitempty"`
	TotalSteps      int      `json:"total_steps,omitempty"`
	Visible         bool     `json:"visible"`
	Indeterminate   bool     `json:"indeterminate"`
}

type progressMetrics struct {
	CurrentChunk    int
	CompletedChunks int
	TotalChunks     int
	CurrentStep     int
	TotalSteps      int
}

type TranslationStatsPayload struct {
	InputTokens             int     `json:"input_tokens"`
	ReasoningOutputTokens   int     `json:"reasoning_output_tokens,omitempty"`
	TimeToFirstTokenSeconds float64 `json:"time_to_first_token_seconds,omitempty"`
	TokensPerSecond         float64 `json:"tokens_per_second,omitempty"`
	TotalOutputTokens       int     `json:"total_output_tokens"`
}

type translationRuntimeOptions struct {
	ContextSummary             string
	ChunkLabel                 string
	OverlapContext             string
	OpeningSourceParagraph     string
	OpeningTranslatedParagraph string
	OverallProgressBase        float64
	OverallProgressSpan        float64
	ProgressMetrics            progressMetrics
}

type lmStudioChatRequest struct {
	Model       string   `json:"model"`
	Input       string   `json:"input"`
	Stream      bool     `json:"stream"`
	Store       *bool    `json:"store,omitempty"`
	Reasoning   string   `json:"reasoning,omitempty"`
	Temperature *float64 `json:"temperature,omitempty"`
}

type promptPassOptions struct {
	Prompt              string
	PreparingLabel      string
	PreparingDetail     string
	LoadingLabel        string
	LoadingDetail       string
	PromptLabel         string
	PromptDetail        string
	GenerateLabel       string
	GenerateDetail      string
	StreamTokens        bool
	EmitStats           bool
	Temperature         *float64
	OverallProgressBase float64
	OverallProgressSpan float64
	ProgressMetrics     progressMetrics
}

func (c *Client) ListModels(settings ProviderSettings) ([]ModelInfo, error) {
	baseURL := normalizeModelsBaseURL(settings.Mode, settings.Endpoint)
	req, err := http.NewRequest(http.MethodGet, baseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	applyAuth(req, settings)
	c.emitDebug("request", baseURL+"/models", map[string]any{
		"mode":     settings.Mode,
		"endpoint": baseURL,
		"model":    settings.Model,
	})

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to reach %s: %w", baseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.emitDebug("response", baseURL+"/models", map[string]any{
			"status": resp.StatusCode,
			"body":   string(body),
		})
		return nil, fmt.Errorf("failed to fetch models (%d): %s", resp.StatusCode, string(body))
	}

	models := make([]ModelInfo, 0)
	if strings.EqualFold(settings.Mode, "lmstudio") {
		var parsed lmStudioModelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return nil, err
		}
		for _, item := range parsed.Models {
			if item.Type != "" && item.Type != "llm" {
				continue
			}
			if item.Key != "" {
				supportsReasoning, reasoningOptions := parseReasoningCapability(item.Reasoning)
				models = append(models, ModelInfo{
					ID:                item.Key,
					DisplayName:       item.DisplayName,
					SupportsReasoning: supportsReasoning,
					ReasoningOptions:  reasoningOptions,
				})
			}
		}
	} else {
		var parsed modelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return nil, err
		}
		for _, item := range parsed.Data {
			models = append(models, ModelInfo{ID: item.ID})
		}
	}

	return models, nil
}

func (c *Client) Translate(reqData TranslationRequest) error {
	reqCtx, cancel, requestID := c.beginRequest()
	defer cancel()
	defer c.finishRequest(requestID)

	reqData.SourceText = preprocessSourceText(reqData.SourceText)

	chunkSize := reqData.Settings.SmartChunkSize
	if chunkSize <= 0 {
		chunkSize = 2000
	}

	chunks := []smartChunk{{Text: strings.TrimSpace(reqData.SourceText)}}
	if reqData.Settings.EnableSmartChunking {
		chunks = buildSmartChunks(reqData.SourceText, chunkSize)
	}
	_, _ = chunkCharacterStats(chunks)
	if len(chunks) <= 1 {
		c.emitProgressWithMetrics(
			"chunking",
			"Generating translation",
			"Translating the full text",
			overallPassProgress(1, 1, reqData.Settings.EnablePostEdit, "translate"),
			false,
			buildProgressMetrics(1, 1, reqData.Settings.EnablePostEdit, "translate"),
		)
		var (
			text  string
			stats TranslationStatsPayload
			err   error
		)
		if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
			text, stats, err = c.translateWithLMStudio(reqCtx, reqData, translationRuntimeOptions{})
		} else {
			text, stats, err = c.translateWithCompatibleAPI(reqCtx, reqData, translationRuntimeOptions{})
		}
		if err != nil {
			return err
		}
		if reqData.Settings.EnablePostEdit {
			c.emitProgressWithMetrics(
				"post_edit",
				"Post-editing translation",
				"Checking for mixed-language or garbled fragments",
				overallPassProgress(1, 1, reqData.Settings.EnablePostEdit, "post_edit"),
				false,
				buildProgressMetrics(1, 1, reqData.Settings.EnablePostEdit, "post_edit"),
			)
			postEdited, err := c.postEditTranslation(reqCtx, reqData, text, translationRuntimeOptions{})
			if err != nil {
				return err
			}
			text = postEdited
		}
		text = cleanupTranslatedText(text)
		if stats.InputTokens > 0 || stats.TotalOutputTokens > 0 {
			c.emitStats(map[string]any{
				"input_tokens":                stats.InputTokens,
				"reasoning_output_tokens":     stats.ReasoningOutputTokens,
				"time_to_first_token_seconds": stats.TimeToFirstTokenSeconds,
				"tokens_per_second":           stats.TokensPerSecond,
				"total_output_tokens":         stats.TotalOutputTokens,
			})
		}
		c.emitProgress("done", "Done", "Translation complete", floatPtr(1), false)
		c.emitComplete(TranslationCompletePayload{Text: text})
		return nil
	}

	c.emitClear()

	aggregated := TranslationStatsPayload{}
	var finalText strings.Builder
	previousSummary := ""
	openingSourceParagraph := leadingParagraph(reqData.SourceText, 420)
	openingTranslatedParagraph := ""
	for index, chunk := range chunks {
		if err := reqCtx.Err(); err != nil {
			return fmt.Errorf("translation cancelled")
		}

		chunkReq := reqData
		chunkReq.SourceText = chunk.Text

		options := translationRuntimeOptions{
			ContextSummary:             previousSummary,
			ChunkLabel:                 fmt.Sprintf("Chunk %d/%d", index+1, len(chunks)),
			OverlapContext:             chunk.OverlapContext,
			OpeningSourceParagraph:     openingSourceParagraph,
			OpeningTranslatedParagraph: openingTranslatedParagraph,
			ProgressMetrics:            buildProgressMetrics(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate"),
			OverallProgressBase:        derefFloat(overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate")),
		}
		c.emitProgressWithMetrics(
			"chunking",
			fmt.Sprintf("Translating %s", options.ChunkLabel),
			fmt.Sprintf("Smart chunking active for long text (%d chars)", chunkSize),
			overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate"),
			false,
			buildProgressMetrics(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate"),
		)

		var translated string
		var stats TranslationStatsPayload
		var err error
		if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
			translated, stats, err = c.translateWithLMStudio(reqCtx, chunkReq, options)
		} else {
			translated, stats, err = c.translateWithCompatibleAPI(reqCtx, chunkReq, options)
		}
		if err != nil {
			return err
		}
		if reqData.Settings.EnablePostEdit {
			c.emitProgressWithMetrics(
				"post_edit",
				fmt.Sprintf("Post-editing %s", options.ChunkLabel),
				"Checking this translated section for mixed-language or garbled fragments",
				overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "post_edit"),
				false,
				buildProgressMetrics(len(chunks), index+1, reqData.Settings.EnablePostEdit, "post_edit"),
			)
			postEditOptions := options
			postEditOptions.OverallProgressBase = derefFloat(overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "post_edit"))
			postEdited, err := c.postEditTranslation(reqCtx, chunkReq, translated, postEditOptions)
			if err != nil {
				return err
			}
			translated = postEdited
		}
		translated = cleanupTranslatedText(translated)

		if index > 0 && finalText.Len() > 0 && !strings.HasSuffix(finalText.String(), "\n") && !strings.HasPrefix(translated, "\n") {
			finalText.WriteString("\n\n")
			c.emitToken("\n\n")
		}
		finalText.WriteString(translated)
		aggregated.InputTokens += stats.InputTokens
		aggregated.ReasoningOutputTokens += stats.ReasoningOutputTokens
		aggregated.TotalOutputTokens += stats.TotalOutputTokens
		aggregated.TimeToFirstTokenSeconds += stats.TimeToFirstTokenSeconds
		aggregated.TokensPerSecond += stats.TokensPerSecond
		previousSummary = buildContextSummary(reqData.Settings, reqData.Instruction, chunk.Text, translated)
		if openingTranslatedParagraph == "" {
			openingTranslatedParagraph = leadingParagraph(finalText.String(), 420)
		}
	}

	if len(chunks) > 0 {
		aggregated.TimeToFirstTokenSeconds = aggregated.TimeToFirstTokenSeconds / float64(len(chunks))
		aggregated.TokensPerSecond = aggregated.TokensPerSecond / float64(len(chunks))
	}

	c.emitStats(map[string]any{
		"input_tokens":                aggregated.InputTokens,
		"reasoning_output_tokens":     aggregated.ReasoningOutputTokens,
		"time_to_first_token_seconds": aggregated.TimeToFirstTokenSeconds,
		"tokens_per_second":           aggregated.TokensPerSecond,
		"total_output_tokens":         aggregated.TotalOutputTokens,
	})
	c.emitProgress("done", "Done", "Translation complete", floatPtr(1), false)
	c.emitComplete(TranslationCompletePayload{Text: finalText.String()})
	return nil
}

func (c *Client) TranslateText(reqData TranslationRequest) (string, TranslationStatsPayload, error) {
	reqCtx, cancel, requestID := c.beginRequest()
	defer cancel()
	defer c.finishRequest(requestID)

	reqData.SourceText = preprocessSourceText(reqData.SourceText)

	chunkSize := reqData.Settings.SmartChunkSize
	if chunkSize <= 0 {
		chunkSize = 2000
	}

	chunks := []smartChunk{{Text: strings.TrimSpace(reqData.SourceText)}}
	if reqData.Settings.EnableSmartChunking {
		chunks = buildSmartChunks(reqData.SourceText, chunkSize)
	}
	_, _ = chunkCharacterStats(chunks)
	if len(chunks) <= 1 {
		var (
			text  string
			stats TranslationStatsPayload
			err   error
		)
		if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
			text, stats, err = c.translateWithLMStudio(reqCtx, reqData, translationRuntimeOptions{})
		} else {
			text, stats, err = c.translateWithCompatibleAPI(reqCtx, reqData, translationRuntimeOptions{})
		}
		if err != nil {
			return "", TranslationStatsPayload{}, err
		}
		if reqData.Settings.EnablePostEdit {
			postEdited, err := c.postEditTranslation(reqCtx, reqData, text, translationRuntimeOptions{})
			if err != nil {
				return "", TranslationStatsPayload{}, err
			}
			text = postEdited
		}
		return cleanupTranslatedText(text), stats, nil
	}

	aggregated := TranslationStatsPayload{}
	var finalText strings.Builder
	previousSummary := ""
	openingSourceParagraph := leadingParagraph(reqData.SourceText, 420)
	openingTranslatedParagraph := ""
	for index, chunk := range chunks {
		if err := reqCtx.Err(); err != nil {
			return "", TranslationStatsPayload{}, fmt.Errorf("translation cancelled")
		}

		chunkReq := reqData
		chunkReq.SourceText = chunk.Text

		options := translationRuntimeOptions{
			ContextSummary:             previousSummary,
			ChunkLabel:                 fmt.Sprintf("Chunk %d/%d", index+1, len(chunks)),
			OverlapContext:             chunk.OverlapContext,
			OpeningSourceParagraph:     openingSourceParagraph,
			OpeningTranslatedParagraph: openingTranslatedParagraph,
			ProgressMetrics:            progressMetrics{CurrentChunk: index + 1, CompletedChunks: index, TotalChunks: len(chunks)},
			OverallProgressBase:        derefFloat(overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate")),
		}

		var translated string
		var stats TranslationStatsPayload
		var err error
		if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
			translated, stats, err = c.translateWithLMStudio(reqCtx, chunkReq, options)
		} else {
			translated, stats, err = c.translateWithCompatibleAPI(reqCtx, chunkReq, options)
		}
		if err != nil {
			return "", TranslationStatsPayload{}, err
		}
		if reqData.Settings.EnablePostEdit {
			postEditOptions := options
			postEditOptions.OverallProgressBase = derefFloat(overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "post_edit"))
			postEdited, err := c.postEditTranslation(reqCtx, chunkReq, translated, postEditOptions)
			if err != nil {
				return "", TranslationStatsPayload{}, err
			}
			translated = postEdited
		}
		translated = cleanupTranslatedText(translated)

		if index > 0 && finalText.Len() > 0 && !strings.HasSuffix(finalText.String(), "\n") && !strings.HasPrefix(translated, "\n") {
			finalText.WriteString("\n\n")
		}
		finalText.WriteString(translated)
		aggregated.InputTokens += stats.InputTokens
		aggregated.ReasoningOutputTokens += stats.ReasoningOutputTokens
		aggregated.TotalOutputTokens += stats.TotalOutputTokens
		aggregated.TimeToFirstTokenSeconds += stats.TimeToFirstTokenSeconds
		aggregated.TokensPerSecond += stats.TokensPerSecond
		previousSummary = buildContextSummary(reqData.Settings, reqData.Instruction, chunk.Text, translated)
		if openingTranslatedParagraph == "" {
			openingTranslatedParagraph = leadingParagraph(finalText.String(), 420)
		}
	}

	if len(chunks) > 0 {
		aggregated.TimeToFirstTokenSeconds = aggregated.TimeToFirstTokenSeconds / float64(len(chunks))
		aggregated.TokensPerSecond = aggregated.TokensPerSecond / float64(len(chunks))
	}

	return finalText.String(), aggregated, nil
}

func (c *Client) TranslateTextStream(reqData TranslationRequest, sink eventSink) (string, TranslationStatsPayload, error) {
	reqCtx, cancel, requestID := c.beginRequestWithSink(sink)
	defer cancel()
	defer c.finishRequest(requestID)

	c.emitClear()

	reqData.SourceText = preprocessSourceText(reqData.SourceText)

	chunkSize := reqData.Settings.SmartChunkSize
	if chunkSize <= 0 {
		chunkSize = 2000
	}

	chunks := []smartChunk{{Text: strings.TrimSpace(reqData.SourceText)}}
	if reqData.Settings.EnableSmartChunking {
		chunks = buildSmartChunks(reqData.SourceText, chunkSize)
	}
	_, _ = chunkCharacterStats(chunks)
	if len(chunks) <= 1 {
		c.emitProgressWithMetrics(
			"chunking",
			"Generating translation",
			"Translating the full text",
			overallPassProgress(1, 1, reqData.Settings.EnablePostEdit, "translate"),
			false,
			buildProgressMetrics(1, 1, reqData.Settings.EnablePostEdit, "translate"),
		)
		var (
			text  string
			stats TranslationStatsPayload
			err   error
		)
		if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
			text, stats, err = c.translateWithLMStudio(reqCtx, reqData, translationRuntimeOptions{})
		} else {
			text, stats, err = c.translateWithCompatibleAPI(reqCtx, reqData, translationRuntimeOptions{})
		}
		if err != nil {
			return "", TranslationStatsPayload{}, err
		}
		if reqData.Settings.EnablePostEdit {
			c.emitProgressWithMetrics(
				"post_edit",
				"Post-editing translation",
				"Checking for mixed-language or garbled fragments",
				overallPassProgress(1, 1, reqData.Settings.EnablePostEdit, "post_edit"),
				false,
				buildProgressMetrics(1, 1, reqData.Settings.EnablePostEdit, "post_edit"),
			)
			postEdited, err := c.postEditTranslation(reqCtx, reqData, text, translationRuntimeOptions{})
			if err != nil {
				return "", TranslationStatsPayload{}, err
			}
			text = postEdited
		}
		text = cleanupTranslatedText(text)
		if stats.InputTokens > 0 || stats.TotalOutputTokens > 0 {
			c.emitStats(map[string]any{
				"input_tokens":                stats.InputTokens,
				"reasoning_output_tokens":     stats.ReasoningOutputTokens,
				"time_to_first_token_seconds": stats.TimeToFirstTokenSeconds,
				"tokens_per_second":           stats.TokensPerSecond,
				"total_output_tokens":         stats.TotalOutputTokens,
			})
		}
		c.emitProgress("done", "Done", "Translation complete", floatPtr(1), false)
		c.emitComplete(TranslationCompletePayload{Text: text})
		return text, stats, nil
	}

	aggregated := TranslationStatsPayload{}
	var finalText strings.Builder
	previousSummary := ""
	openingSourceParagraph := leadingParagraph(reqData.SourceText, 420)
	openingTranslatedParagraph := ""
	for index, chunk := range chunks {
		if err := reqCtx.Err(); err != nil {
			return "", TranslationStatsPayload{}, fmt.Errorf("translation cancelled")
		}

		chunkReq := reqData
		chunkReq.SourceText = chunk.Text

		options := translationRuntimeOptions{
			ContextSummary:             previousSummary,
			ChunkLabel:                 fmt.Sprintf("Chunk %d/%d", index+1, len(chunks)),
			OverlapContext:             chunk.OverlapContext,
			OpeningSourceParagraph:     openingSourceParagraph,
			OpeningTranslatedParagraph: openingTranslatedParagraph,
			OverallProgressBase:        derefFloat(overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate")),
			ProgressMetrics:            buildProgressMetrics(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate"),
		}
		c.emitProgressWithMetrics(
			"chunking",
			fmt.Sprintf("Translating %s", options.ChunkLabel),
			fmt.Sprintf("Smart chunking active for long text (%d chars)", chunkSize),
			overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate"),
			false,
			buildProgressMetrics(len(chunks), index+1, reqData.Settings.EnablePostEdit, "translate"),
		)

		var translated string
		var stats TranslationStatsPayload
		var err error
		if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
			translated, stats, err = c.translateWithLMStudio(reqCtx, chunkReq, options)
		} else {
			translated, stats, err = c.translateWithCompatibleAPI(reqCtx, chunkReq, options)
		}
		if err != nil {
			return "", TranslationStatsPayload{}, err
		}
		if reqData.Settings.EnablePostEdit {
			c.emitProgressWithMetrics(
				"post_edit",
				fmt.Sprintf("Post-editing %s", options.ChunkLabel),
				"Checking this translated section for mixed-language or garbled fragments",
				overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "post_edit"),
				false,
				buildProgressMetrics(len(chunks), index+1, reqData.Settings.EnablePostEdit, "post_edit"),
			)
			postEditOptions := options
			postEditOptions.OverallProgressBase = derefFloat(overallPassProgress(len(chunks), index+1, reqData.Settings.EnablePostEdit, "post_edit"))
			postEdited, err := c.postEditTranslation(reqCtx, chunkReq, translated, postEditOptions)
			if err != nil {
				return "", TranslationStatsPayload{}, err
			}
			translated = postEdited
		}
		translated = cleanupTranslatedText(translated)

		if index > 0 && finalText.Len() > 0 && !strings.HasSuffix(finalText.String(), "\n") && !strings.HasPrefix(translated, "\n") {
			finalText.WriteString("\n\n")
			c.emitToken("\n\n")
		}
		finalText.WriteString(translated)
		aggregated.InputTokens += stats.InputTokens
		aggregated.ReasoningOutputTokens += stats.ReasoningOutputTokens
		aggregated.TotalOutputTokens += stats.TotalOutputTokens
		aggregated.TimeToFirstTokenSeconds += stats.TimeToFirstTokenSeconds
		aggregated.TokensPerSecond += stats.TokensPerSecond
		previousSummary = buildContextSummary(reqData.Settings, reqData.Instruction, chunk.Text, translated)
		if openingTranslatedParagraph == "" {
			openingTranslatedParagraph = leadingParagraph(finalText.String(), 420)
		}
	}

	if len(chunks) > 0 {
		aggregated.TimeToFirstTokenSeconds = aggregated.TimeToFirstTokenSeconds / float64(len(chunks))
		aggregated.TokensPerSecond = aggregated.TokensPerSecond / float64(len(chunks))
	}

	c.emitStats(map[string]any{
		"input_tokens":                aggregated.InputTokens,
		"reasoning_output_tokens":     aggregated.ReasoningOutputTokens,
		"time_to_first_token_seconds": aggregated.TimeToFirstTokenSeconds,
		"tokens_per_second":           aggregated.TokensPerSecond,
		"total_output_tokens":         aggregated.TotalOutputTokens,
	})
	c.emitProgress("done", "Done", "Translation complete", floatPtr(1), false)
	final := finalText.String()
	c.emitComplete(TranslationCompletePayload{Text: final})
	return final, aggregated, nil
}

func (c *Client) translateWithCompatibleAPI(reqCtx context.Context, reqData TranslationRequest, runtimeOptions translationRuntimeOptions) (string, TranslationStatsPayload, error) {
	prompt := buildPrompt(reqData.Settings, reqData.SourceLang, reqData.TargetLang, reqData.SourceText, reqData.Instruction, runtimeOptions)
	resolvedTemperature := resolveDraftTemperature(reqData.Settings)
	c.emitDebug("note", "prompt:translation", prompt)
	c.emitDebug("note", "temperature:translation", formatTemperatureDebugNote("draft", resolvedTemperature))
	return c.runCompatiblePrompt(reqCtx, reqData.Settings, promptPassOptions{
		Prompt:              prompt,
		PreparingLabel:      "Preparing request",
		PreparingDetail:     "Connecting to completion endpoint",
		GenerateLabel:       "Generating translation",
		GenerateDetail:      "Waiting for model output",
		StreamTokens:        true,
		EmitStats:           false,
		Temperature:         resolvedTemperature,
		OverallProgressBase: runtimeOptions.OverallProgressBase,
		OverallProgressSpan: runtimeOptions.OverallProgressSpan,
		ProgressMetrics:     runtimeOptions.ProgressMetrics,
	})
}

func (c *Client) runCompatiblePrompt(reqCtx context.Context, settings ProviderSettings, options promptPassOptions) (string, TranslationStatsPayload, error) {
	baseURL := normalizeCompatibleBaseURL(settings.Mode, settings.Endpoint)
	payload := chatCompletionsRequest{
		Model: settings.Model,
		Messages: []chatMessage{
			{
				Role:    "user",
				Content: options.Prompt,
			},
		},
		Stream: true,
		Store:  boolPtr(false),
	}
	if reasoning := normalizeReasoningValue(settings.Reasoning); reasoning != "" {
		payload.ReasoningEffort = reasoning
	}
	if temperature := resolvePromptTemperature(settings, options.Temperature); temperature != nil {
		payload.Temperature = temperature
	}

	jsonData, _ := json.Marshal(payload)
	c.emitPromptProgress("preparing", options.PreparingLabel, options.PreparingDetail, floatPtr(0), true, options)
	c.emitDebug("request", baseURL+"/chat/completions", payload)

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", TranslationStatsPayload{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuth(httpReq, settings)

	resp, err := c.streamingClient().Do(httpReq)
	if err != nil {
		c.emitProgressHidden()
		if errors.Is(err, context.Canceled) || errors.Is(reqCtx.Err(), context.Canceled) {
			return "", TranslationStatsPayload{}, fmt.Errorf("translation cancelled")
		}
		return "", TranslationStatsPayload{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.emitDebug("response", baseURL+"/chat/completions", map[string]any{
			"status": resp.StatusCode,
			"body":   string(body),
		})
		c.emitProgressHidden()
		return "", TranslationStatsPayload{}, fmt.Errorf("translation request failed (%d): %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	chunks := make([]map[string]any, 0)
	var fullResponse strings.Builder
	tokenChunkCount := 0
	var eventData []string

	c.emitPromptProgress("generating", options.GenerateLabel, options.GenerateDetail, floatPtr(1), true, options)

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			if errors.Is(err, context.Canceled) || errors.Is(reqCtx.Err(), context.Canceled) {
				c.emitDebug("response", baseURL+"/chat/completions", chunks)
				c.emitProgressHidden()
				return "", TranslationStatsPayload{}, fmt.Errorf("translation cancelled")
			}
			c.emitProgressHidden()
			return "", TranslationStatsPayload{}, err
		}

		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "data:") {
			eventData = append(eventData, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		} else if strings.HasPrefix(trimmed, "{") {
			eventData = append(eventData, trimmed)
		}

		if trimmed == "" || errors.Is(err, io.EOF) {
			for _, data := range eventData {
				if data == "" {
					continue
				}
				if data == "[DONE]" {
					c.emitDebug("note", baseURL+"/chat/completions", map[string]any{
						"chunkCount": tokenChunkCount,
						"message":    "stream finished",
					})
					eventData = nil
					goto streamDone
				}

				var raw map[string]any
				if json.Unmarshal([]byte(data), &raw) == nil {
					chunks = append(chunks, map[string]any{"data": raw})
					pieces := extractContentPieces(raw)
					for _, piece := range pieces {
						if piece == "" {
							continue
						}
						tokenChunkCount++
						fullResponse.WriteString(piece)
						if options.StreamTokens {
							c.emitToken(piece)
						}
					}
				}
			}
			eventData = nil
		}

		if errors.Is(err, io.EOF) {
			break
		}
	}

streamDone:

	c.emitDebug("response", baseURL+"/chat/completions", chunks)

	if strings.TrimSpace(fullResponse.String()) == "" {
		c.emitProgressHidden()
		return "", TranslationStatsPayload{}, fmt.Errorf("the model returned an empty translation")
	}

	return fullResponse.String(), TranslationStatsPayload{}, nil
}

func (c *Client) translateWithLMStudio(reqCtx context.Context, reqData TranslationRequest, runtimeOptions translationRuntimeOptions) (string, TranslationStatsPayload, error) {
	prompt := buildPrompt(reqData.Settings, reqData.SourceLang, reqData.TargetLang, reqData.SourceText, reqData.Instruction, runtimeOptions)
	resolvedTemperature := resolveDraftTemperature(reqData.Settings)
	c.emitDebug("note", "prompt:translation", prompt)
	c.emitDebug("note", "temperature:translation", formatTemperatureDebugNote("draft", resolvedTemperature))
	return c.runLMStudioPrompt(reqCtx, reqData.Settings, promptPassOptions{
		Prompt:              prompt,
		PreparingLabel:      "Preparing request",
		PreparingDetail:     "Connecting to LM Studio",
		LoadingLabel:        "Loading model",
		LoadingDetail:       "Preparing model weights",
		PromptLabel:         "Processing prompt",
		PromptDetail:        "Tokenizing and evaluating prompt",
		GenerateLabel:       "Generating translation",
		GenerateDetail:      "Streaming translated text",
		StreamTokens:        true,
		EmitStats:           true,
		Temperature:         resolvedTemperature,
		OverallProgressBase: runtimeOptions.OverallProgressBase,
		OverallProgressSpan: runtimeOptions.OverallProgressSpan,
		ProgressMetrics:     runtimeOptions.ProgressMetrics,
	})
}

func (c *Client) runLMStudioPrompt(reqCtx context.Context, settings ProviderSettings, options promptPassOptions) (string, TranslationStatsPayload, error) {
	baseURL := normalizeLMStudioNativeBaseURL(settings.Endpoint)
	payload := lmStudioChatRequest{
		Model:  settings.Model,
		Input:  options.Prompt,
		Stream: true,
		Store:  boolPtr(false),
	}
	if reasoning := normalizeReasoningValue(settings.Reasoning); reasoning != "" {
		payload.Reasoning = reasoning
	}
	if temperature := resolvePromptTemperature(settings, options.Temperature); temperature != nil {
		payload.Temperature = temperature
	}

	jsonData, _ := json.Marshal(payload)
	c.emitPromptProgress("preparing", options.PreparingLabel, options.PreparingDetail, floatPtr(0), true, options)
	c.emitDebug("request", baseURL+"/chat", payload)

	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, baseURL+"/chat", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", TranslationStatsPayload{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	applyAuth(httpReq, settings)

	resp, err := c.streamingClient().Do(httpReq)
	if err != nil {
		c.emitProgressHidden()
		if errors.Is(err, context.Canceled) || errors.Is(reqCtx.Err(), context.Canceled) {
			return "", TranslationStatsPayload{}, fmt.Errorf("translation cancelled")
		}
		return "", TranslationStatsPayload{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.emitDebug("response", baseURL+"/chat", map[string]any{
			"status": resp.StatusCode,
			"body":   string(body),
		})
		c.emitProgressHidden()
		return "", TranslationStatsPayload{}, fmt.Errorf("translation request failed (%d): %s", resp.StatusCode, string(body))
	}

	reader := bufio.NewReader(resp.Body)
	chunks := make([]map[string]any, 0)
	var fullResponse strings.Builder
	chunkCount := 0
	statsEmitted := false
	collectedStats := TranslationStatsPayload{}
	eventName := ""
	var eventData []string

	flushEvent := func(forceDone bool) error {
		if len(eventData) == 0 && eventName == "" {
			return nil
		}

		rawEventName := eventName
		joined := strings.Join(eventData, "\n")
		eventName = ""
		eventData = nil

		if strings.TrimSpace(joined) == "" {
			return nil
		}

		var raw map[string]any
		if err := json.Unmarshal([]byte(joined), &raw); err != nil {
			chunks = append(chunks, map[string]any{
				"event": rawEventName,
				"data":  joined,
			})
			return nil
		}

		chunkCount++
		if rawEventName == "" {
			if inferred, _ := raw["type"].(string); inferred != "" {
				rawEventName = inferred
			}
		}
		chunks = append(chunks, map[string]any{
			"event": rawEventName,
			"data":  raw,
		})
		if statsMap, ok := raw["stats"].(map[string]any); ok {
			if options.EmitStats {
				c.emitStats(statsMap)
			}
			statsEmitted = true
			collectedStats = statsFromMap(statsMap)
		} else if resultMap, ok := raw["result"].(map[string]any); ok {
			if statsMap, ok := resultMap["stats"].(map[string]any); ok {
				if options.EmitStats {
					c.emitStats(statsMap)
				}
				statsEmitted = true
				collectedStats = statsFromMap(statsMap)
			}
		}

		switch rawEventName {
		case "model_load.start":
			c.emitPromptProgress("model_load", options.LoadingLabel, options.LoadingDetail, floatPtr(0.1), true, options)
		case "model_load.progress":
			progress := mapFloat(raw["progress"])
			c.emitPromptProgress("model_load", options.LoadingLabel, options.LoadingDetail, floatPtr(progress*0.35), false, options)
		case "model_load.end":
			c.emitPromptProgress("model_load", options.LoadingLabel, "Starting prompt processing", floatPtr(0.35), false, options)
		case "prompt_processing.start":
			c.emitPromptProgress("prompt_processing", options.PromptLabel, options.PromptDetail, floatPtr(0.4), true, options)
		case "prompt_processing.progress":
			progress := mapFloat(raw["progress"])
			c.emitPromptProgress("prompt_processing", options.PromptLabel, options.PromptDetail, floatPtr(0.4+progress*0.4), false, options)
		case "prompt_processing.end":
			c.emitPromptProgress("generating", options.GenerateLabel, options.GenerateDetail, floatPtr(0.8), true, options)
		case "message.start":
			c.emitPromptProgress("generating", options.GenerateLabel, options.GenerateDetail, floatPtr(0.82), true, options)
		case "message.delta":
			for _, piece := range extractLMStudioMessageContent(raw) {
				if piece == "" {
					continue
				}
				fullResponse.WriteString(piece)
				if options.StreamTokens {
					c.emitToken(piece)
				}
			}
		case "error":
			c.emitProgressHidden()
			return fmt.Errorf("lm studio error: %s", strings.TrimSpace(joined))
		case "chat.end":
			if fullResponse.Len() == 0 {
				fullResponse.WriteString(extractLMStudioFinalMessage(raw))
			}
			c.emitPromptProgress("generating", options.GenerateLabel, options.GenerateDetail, floatPtr(1), false, options)
		}

		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			if errors.Is(err, context.Canceled) || errors.Is(reqCtx.Err(), context.Canceled) {
				c.emitDebug("response", baseURL+"/chat", chunks)
				c.emitProgressHidden()
				return "", TranslationStatsPayload{}, fmt.Errorf("translation cancelled")
			}
			c.emitProgressHidden()
			return "", TranslationStatsPayload{}, err
		}

		trimmedRight := strings.TrimRight(line, "\r\n")
		trimmed := strings.TrimSpace(trimmedRight)

		switch {
		case strings.HasPrefix(trimmedRight, "event:"):
			eventName = strings.TrimSpace(strings.TrimPrefix(trimmedRight, "event:"))
		case strings.HasPrefix(trimmedRight, "data:"):
			eventData = append(eventData, strings.TrimSpace(strings.TrimPrefix(trimmedRight, "data:")))
		case trimmed == "":
			if err := flushEvent(false); err != nil {
				return "", TranslationStatsPayload{}, err
			}
		}

		if errors.Is(err, io.EOF) {
			if flushErr := flushEvent(true); flushErr != nil {
				return "", TranslationStatsPayload{}, flushErr
			}
			break
		}
	}

	c.emitDebug("response", baseURL+"/chat", map[string]any{
		"chunkCount":    chunkCount,
		"statsDetected": statsEmitted,
		"events":        chunks,
	})

	if strings.TrimSpace(fullResponse.String()) == "" {
		c.emitProgressHidden()
		return "", TranslationStatsPayload{}, fmt.Errorf("the model returned an empty translation")
	}

	return fullResponse.String(), collectedStats, nil
}

func normalizeModelsBaseURL(mode, endpoint string) string {
	if strings.EqualFold(mode, "lmstudio") {
		return normalizeLMStudioNativeBaseURL(endpoint)
	}
	return normalizeCompatibleBaseURL(mode, endpoint)
}

func normalizeCompatibleBaseURL(mode, endpoint string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if trimmed == "" {
		if strings.EqualFold(mode, "openai") {
			trimmed = "https://api.openai.com"
		} else {
			trimmed = "http://127.0.0.1:1234"
		}
	}

	if strings.HasSuffix(trimmed, "/v1") {
		return trimmed
	}
	return trimmed + "/v1"
}

func normalizeLMStudioNativeBaseURL(endpoint string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if trimmed == "" {
		trimmed = "http://127.0.0.1:1234"
	}

	trimmed = strings.TrimSuffix(trimmed, "/api/v1")
	trimmed = strings.TrimSuffix(trimmed, "/v1")
	return trimmed + "/api/v1"
}

func applyAuth(req *http.Request, settings ProviderSettings) {
	if strings.TrimSpace(settings.APIKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(settings.APIKey))
	}
}

// buildPrompt는 1차 번역용 프롬프트를 조립한다.
// 사용자의 지침, 보호 용어, 청크 문맥, 오프닝 앵커 등을 합쳐
// "현재 청크를 어떻게 번역해야 하는가"를 모델에게 전달하는 메인 번역 프롬프트이다.
func buildPrompt(settings ProviderSettings, sourceLang, targetLang, sourceText, instruction string, runtimeOptions translationRuntimeOptions) string {
	sourceLabel := normalizeLanguageLabel(sourceLang)
	targetLabel := normalizeLanguageLabel(targetLang)
	protectedTerms := extractProtectedTerms(sourceText)

	var builder strings.Builder
	if strings.TrimSpace(settings.DebugTranslationPromptTemplate) != "" {
		prompt := applyPromptTemplate(normalizeDebugPromptTemplateInput(settings.DebugTranslationPromptTemplate), map[string]string{
			"SOURCE_LANG":                  sourceLabel,
			"TARGET_LANG":                  targetLabel,
			"INSTRUCTION":                  strings.TrimSpace(instruction),
			"SOURCE_TEXT":                  sourceText,
			"PROTECTED_TERMS":              strings.Join(protectedTerms, "\n"),
			"GLOSSARY":                     effectiveGlossary(settings),
			"CHUNK_LABEL":                  strings.TrimSpace(runtimeOptions.ChunkLabel),
			"CONTEXT_SUMMARY":              strings.TrimSpace(runtimeOptions.ContextSummary),
			"OVERLAP_CONTEXT":              strings.TrimSpace(runtimeOptions.OverlapContext),
			"OPENING_SOURCE_PARAGRAPH":     strings.TrimSpace(runtimeOptions.OpeningSourceParagraph),
			"OPENING_TRANSLATED_PARAGRAPH": strings.TrimSpace(runtimeOptions.OpeningTranslatedParagraph),
		})
		return sanitizeDebugPromptOverride(prompt, settings.EnableEnhancedContextTranslation, false, true)
	}
	builder.WriteString("# 1. Role And Core Persona\n")
	builder.WriteString(fmt.Sprintf(
		"You are a professional %s to %s translator. Your goal is to accurately convey the meaning and nuances of the original %s text while adhering to %s grammar, vocabulary, and cultural sensitivities.\n",
		sourceLabel,
		targetLabel,
		sourceLabel,
		targetLabel,
	))

	builder.WriteString("\n# 2. Instruction Priority Rules\n")
	builder.WriteString("- The user style instruction is mandatory for every chunk, including later chunks.\n")
	builder.WriteString("- If previous chunk wording, overlap context, or opening style anchor conflicts with the user instruction, follow the user instruction.\n")
	builder.WriteString("- Use continuity context only to preserve meaning, terminology, names, and narrative flow. Do not let it override the requested style.\n")

	builder.WriteString("\n# 3. User Style And Instructions\n")
	if trimmedInstruction := strings.TrimSpace(instruction); trimmedInstruction != "" {
		builder.WriteString(trimmedInstruction)
		if !strings.HasSuffix(trimmedInstruction, ".") &&
			!strings.HasSuffix(trimmedInstruction, "!") &&
			!strings.HasSuffix(trimmedInstruction, "?") {
			builder.WriteString(".")
		}
		builder.WriteString("\n")
	} else {
		builder.WriteString("No additional style instruction provided.\n")
	}

	if len(protectedTerms) > 0 {
		builder.WriteString("\n# 3-0. Protected Names And Terms\n")
		builder.WriteString("Do not translate these proper nouns or technical names into literal Hanja/Chinese-character compounds. Preserve them in their established target-language form or keep the original script when appropriate:\n")
		for _, term := range protectedTerms {
			builder.WriteString("- ")
			builder.WriteString(term)
			builder.WriteString("\n")
		}
	}

	if settings.EnableEnhancedContextTranslation {
		builder.WriteString("\n# 3-1. Enhanced Context Translation Rules\n")
		builder.WriteString("Maintain consistent translations for names, places, organizations, products, commands, and technical terms across the entire text. Reuse the same established translation whenever the same source term appears again. If a term should remain in the original language, keep it unchanged consistently.\n")

		if glossary := strings.TrimSpace(settings.EnhancedContextGlossary); glossary != "" {
			builder.WriteString("\nUse the following User Glossary for consistent translation. If these terms appear in the source text, they MUST be translated as specified:\n")
			builder.WriteString("<GLOSSARY>\n")
			builder.WriteString(glossary)
			builder.WriteString("\n</GLOSSARY>\n")
		}
	}

	if chunkLabel := strings.TrimSpace(runtimeOptions.ChunkLabel); chunkLabel != "" {
		builder.WriteString("\n# 3-2. Current Section\n")
		builder.WriteString(fmt.Sprintf("%s.\n", chunkLabel))
	}

	if contextSummary := strings.TrimSpace(runtimeOptions.ContextSummary); contextSummary != "" {
		builder.WriteString("\n# 3-3. Previous Context\n")
		builder.WriteString("Context from the previous translated section:\n")
		builder.WriteString(contextSummary)
		builder.WriteString("\nUse this only to preserve continuity and terminology. If it conflicts with the user instruction, ignore the conflicting part.\n")
	}

	if openingSource := strings.TrimSpace(runtimeOptions.OpeningSourceParagraph); openingSource != "" {
		builder.WriteString("\n# 3-4. Opening Style Anchor\n")
		builder.WriteString("Use the opening paragraph only as weak guidance for narrative voice, register, and stylistic consistency. Do not copy it, do not force archaic diction, and do not override the current source text.\n")
		builder.WriteString("Opening source paragraph:\n")
		builder.WriteString(openingSource)
		builder.WriteString("\n")
		if openingTranslated := strings.TrimSpace(runtimeOptions.OpeningTranslatedParagraph); openingTranslated != "" {
			builder.WriteString("Opening translated paragraph:\n")
			builder.WriteString(openingTranslated)
			builder.WriteString("\n")
		}
	}

	if overlapContext := strings.TrimSpace(runtimeOptions.OverlapContext); overlapContext != "" {
		builder.WriteString("\n# 3-5. Recent Source Overlap\n")
		builder.WriteString("Recent source overlap for continuity:\n")
		builder.WriteString(overlapContext)
		builder.WriteString("\nUse this only as reference context. Do not translate or repeat this overlap again.\n")
	}

	builder.WriteString("\n# 4. Output Constraints And Source Text\n")
	builder.WriteString(fmt.Sprintf(
		"Produce only the %s translation, without any additional explanations or commentary. Please translate the following %s text into %s:\n\n",
		targetLabel,
		sourceLabel,
		targetLabel,
	))
	builder.WriteString(sourceText)

	return builder.String()
}

// buildPostEditPrompt는 초벌 번역(draftTranslation)을 다듬는 포스트 에디팅용 프롬프트를 조립한다.
// 원문과 초벌 번역을 함께 넣고, 사용자 지침 및 문맥을 기준으로
// 의미 보존을 유지하면서 표현을 교정하도록 모델에 지시하는 최종 교정 프롬프트이다.
func buildPostEditPrompt(settings ProviderSettings, sourceLang, targetLang, sourceText, draftTranslation, instruction string, runtimeOptions translationRuntimeOptions) string {
	sourceLabel := normalizeLanguageLabel(sourceLang)
	targetLabel := normalizeLanguageLabel(targetLang)
	protectedTerms := extractProtectedTerms(sourceText)

	if strings.TrimSpace(settings.DebugPostEditPromptTemplate) != "" {
		prompt := applyPromptTemplate(normalizeDebugPromptTemplateInput(settings.DebugPostEditPromptTemplate), map[string]string{
			"SOURCE_LANG":                  sourceLabel,
			"TARGET_LANG":                  targetLabel,
			"INSTRUCTION":                  strings.TrimSpace(instruction),
			"SOURCE_TEXT":                  sourceText,
			"DRAFT_TRANSLATION":            draftTranslation,
			"PROTECTED_TERMS":              strings.Join(protectedTerms, "\n"),
			"GLOSSARY":                     effectiveGlossary(settings),
			"TOPIC_AWARE_HINTS":            topicAwarePostEditTemplateValue(settings, sourceText, draftTranslation, instruction),
			"CHUNK_LABEL":                  strings.TrimSpace(runtimeOptions.ChunkLabel),
			"CONTEXT_SUMMARY":              strings.TrimSpace(runtimeOptions.ContextSummary),
			"OVERLAP_CONTEXT":              strings.TrimSpace(runtimeOptions.OverlapContext),
			"OPENING_SOURCE_PARAGRAPH":     strings.TrimSpace(runtimeOptions.OpeningSourceParagraph),
			"OPENING_TRANSLATED_PARAGRAPH": strings.TrimSpace(runtimeOptions.OpeningTranslatedParagraph),
		})
		return sanitizeDebugPromptOverride(prompt, settings.EnableEnhancedContextTranslation, settings.EnableTopicAwarePostEdit, false)
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(
		"You are a professional %s to %s translation post-editor. Review the draft against the source and produce a clean final %s translation.\n",
		sourceLabel,
		targetLabel,
		targetLabel,
	))

	builder.WriteString("\nInstruction priority rules:\n")
	builder.WriteString("- The user style instruction is mandatory for this chunk.\n")
	builder.WriteString("- If the draft conflicts with the user instruction, revise the draft to match the instruction.\n")
	builder.WriteString("- If previous context, overlap context, or opening style anchor conflicts with the user instruction, follow the user instruction.\n")

	if trimmedInstruction := strings.TrimSpace(instruction); trimmedInstruction != "" {
		builder.WriteString("\nStyle instruction:\n")
		builder.WriteString(trimmedInstruction)
		builder.WriteString("\n")
	}

	if len(protectedTerms) > 0 {
		builder.WriteString("\nProtected names and terms:\n")
		for _, term := range protectedTerms {
			builder.WriteString("- ")
			builder.WriteString(term)
			builder.WriteString("\n")
		}
	}

	if chunkLabel := strings.TrimSpace(runtimeOptions.ChunkLabel); chunkLabel != "" {
		builder.WriteString("\nCurrent section:\n")
		builder.WriteString(chunkLabel)
		builder.WriteString("\n")
	}

	if contextSummary := strings.TrimSpace(runtimeOptions.ContextSummary); contextSummary != "" {
		builder.WriteString("\nPrevious context:\n")
		builder.WriteString(contextSummary)
		builder.WriteString("\nUse this only to preserve continuity and terminology. Do not repeat already translated content.\n")
	}

	if openingSource := strings.TrimSpace(runtimeOptions.OpeningSourceParagraph); openingSource != "" {
		builder.WriteString("\nOpening style anchor:\n")
		builder.WriteString("Use the opening paragraph only as weak guidance for narrative voice, register, and stylistic consistency. Do not copy it, do not force archaic diction, and do not override the current source text.\n")
		builder.WriteString("Opening source paragraph:\n")
		builder.WriteString(openingSource)
		builder.WriteString("\n")
		if openingTranslated := strings.TrimSpace(runtimeOptions.OpeningTranslatedParagraph); openingTranslated != "" {
			builder.WriteString("Opening translated paragraph:\n")
			builder.WriteString(openingTranslated)
			builder.WriteString("\n")
		}
	}

	if overlapContext := strings.TrimSpace(runtimeOptions.OverlapContext); overlapContext != "" {
		builder.WriteString("\nRecent overlap:\n")
		builder.WriteString(overlapContext)
		builder.WriteString("\nUse this only as reference context. Do not repeat or retranslate this overlap.\n")
	}

	builder.WriteString("\nRules:\n")
	builder.WriteString("- Make the smallest possible edits needed to correct the draft.\n")
	builder.WriteString("- Preserve the source meaning strictly. Do not add, remove, generalize, or reinterpret facts, legal meanings, relationships, chronology, or emphasis.\n")
	builder.WriteString("- Do not replace a specific institution, qualification, admission, or legal action with a different meaning.\n")
	builder.WriteString("- Fix only clear errors: malformed transliterations, mixed-language fragments, stray foreign-script insertions, leftover untranslated words, or obvious mistranslations.\n")
	builder.WriteString("- Preserve intentional bilingual notation only when it is clearly marked with parentheses, quotes, aliases, or original-title notation.\n")
	builder.WriteString("- User instruction compliance has priority over preserving the existing draft wording.\n")
	builder.WriteString("- Compare the source against the low-temperature draft and revise only where a correction or clear naturalness improvement is justified.\n")
	builder.WriteString("- If a sentence is already acceptable, keep it as close to the draft as possible.\n")
	builder.WriteString("- Output only the final corrected translation.\n")

	if settings.EnableTopicAwarePostEdit {
		if topicHints := buildTopicAwarePostEditHints(sourceText, draftTranslation, instruction); topicHints != "" {
			builder.WriteString("\nLikely genre/topic hint (weak guidance):\n")
			builder.WriteString(topicHints)
			builder.WriteString("Use these hints only to improve register and terminology consistency. If they conflict with the source, ignore them.\n")
		}
	}

	if settings.EnableEnhancedContextTranslation {
		builder.WriteString("\nConsistency rule:\nMaintain consistent translations for names, places, organizations, products, commands, and technical terms.\n")

		if glossary := strings.TrimSpace(settings.EnhancedContextGlossary); glossary != "" {
			builder.WriteString("User Glossary:\n")
			builder.WriteString("<GLOSSARY>\n")
			builder.WriteString(glossary)
			builder.WriteString("\n</GLOSSARY>\n")
		}
	}

	builder.WriteString("\nSource Text:\n")
	builder.WriteString(sourceText)
	builder.WriteString("\n\nTranslated Draft:\n")
	builder.WriteString(draftTranslation)

	return builder.String()
}

// buildTopicAwarePostEditHints는 포스트 에디팅 단계에서 참고할 약한 장르/주제/톤 힌트를 만든다.
// 강제 규칙이 아니라, register와 terminology consistency를 조금 더 안정시키기 위한 보조 프롬프트 조각이다.
func buildTopicAwarePostEditHints(sourceText, draftTranslation, instruction string) string {
	genre := detectPostEditGenre(sourceText, draftTranslation, instruction)
	topic := detectPostEditTopic(sourceText, draftTranslation)
	tone := detectPostEditTone(draftTranslation, instruction)

	var builder strings.Builder
	if genre != "" {
		builder.WriteString("- Likely genre: ")
		builder.WriteString(genre)
		builder.WriteString("\n")
	}
	if topic != "" {
		builder.WriteString("- Likely topic: ")
		builder.WriteString(topic)
		builder.WriteString("\n")
	}
	if tone != "" {
		builder.WriteString("- Likely tone/register: ")
		builder.WriteString(tone)
		builder.WriteString("\n")
	}
	return builder.String()
}

// topicAwarePostEditTemplateValue는 디버그/템플릿 기반 포스트 에디트 프롬프트에
// 주제 인식 힌트를 문자열로 주입하기 위한 값 생성 함수이다.
// 기능이 꺼져 있으면 빈 문자열을 반환해 해당 섹션이 빠지도록 만든다.
func topicAwarePostEditTemplateValue(settings ProviderSettings, sourceText, draftTranslation, instruction string) string {
	if !settings.EnableTopicAwarePostEdit {
		return ""
	}
	result := strings.TrimSpace(buildTopicAwarePostEditHints(sourceText, draftTranslation, instruction))
	if result == "" {
		return ""
	}
	return result + "\nUse these hints only to improve register and terminology consistency. If they conflict with the source, ignore them."
}

func detectPostEditGenre(sourceText, draftTranslation, instruction string) string {
	combined := strings.ToLower(strings.Join([]string{sourceText, draftTranslation, instruction}, "\n"))

	switch {
	case containsAny(combined, []string{"api", "endpoint", "model", "prompt", "parameter", "json", "http", "config", "llm", "translation"}):
		return "technical documentation"
	case containsAny(combined, []string{"shall", "pursuant", "agreement", "liability", "article", "hereby", "contract", "regulation"}):
		return "legal or policy text"
	case containsAny(combined, []string{"step", "how to", "follow these", "instructions", "click", "select", "open", "enable"}):
		return "instructional guide"
	case containsAny(combined, []string{"discover", "boost", "powerful", "seamless", "experience", "best", "premium"}):
		return "marketing copy"
	case containsAny(combined, []string{"\"", "'", "dialogue", "conversation"}) || strings.Count(sourceText, "?") >= 2:
		return "dialogue or conversational prose"
	default:
		return "general prose"
	}
}

func detectPostEditTopic(sourceText, draftTranslation string) string {
	candidate := firstNonEmptyLine(sourceText)
	if candidate == "" {
		candidate = firstNonEmptyLine(draftTranslation)
	}
	if candidate == "" {
		return ""
	}
	words := strings.Fields(candidate)
	if len(words) > 12 {
		words = words[:12]
	}
	return strings.TrimSpace(strings.Join(words, " "))
}

func detectPostEditTone(draftTranslation, instruction string) string {
	combined := strings.ToLower(strings.Join([]string{instruction, draftTranslation}, "\n"))

	switch {
	case containsAny(combined, []string{"formal", "precise", "consistency"}):
		return "formal and precise"
	case containsAny(combined, []string{"natural", "native speaker", "idiomatic"}):
		return "natural and idiomatic"
	case containsAny(combined, []string{"concise", "clear", "direct"}):
		return "concise and clear"
	case containsAny(combined, []string{"persuasive", "engaging", "promotional"}):
		return "persuasive"
	case containsAny(combined, []string{"instruction", "step", "guide"}):
		return "instructional"
	default:
		return "neutral"
	}
}

func containsAny(text string, terms []string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if len([]rune(trimmed)) > 80 {
			return strings.TrimSpace(string([]rune(trimmed)[:80]))
		}
		return trimmed
	}
	return ""
}

// applyPromptTemplate는 디버그용 프롬프트 템플릿의 {{PLACEHOLDER}} 값을 실제 문자열로 치환한다.
// 사용자가 프롬프트 실험을 할 때 렌더링된 최종 프롬프트를 만드는 용도이다.
func applyPromptTemplate(template string, values map[string]string) string {
	result := template
	for key, value := range values {
		result = strings.ReplaceAll(result, "{{"+key+"}}", value)
	}
	return strings.TrimSpace(result)
}

// normalizeDebugPromptTemplateInput은 디버그 템플릿 입력에 들어온 이스케이프 문자열을
// 실제 줄바꿈/탭으로 되돌려, 템플릿 편집창에서 저장한 내용을 정상적인 프롬프트로 만들기 위한 전처리이다.
func normalizeDebugPromptTemplateInput(template string) string {
	replacer := strings.NewReplacer(
		`\r\n`, "\n",
		`\n`, "\n",
		`\t`, "\t",
	)
	return replacer.Replace(template)
}

func effectiveGlossary(settings ProviderSettings) string {
	if !settings.EnableEnhancedContextTranslation {
		return ""
	}
	return strings.TrimSpace(settings.EnhancedContextGlossary)
}

// sanitizeDebugPromptOverride는 디버그 프롬프트 오버라이드에 대해
// 현재 옵션에서 비활성화된 섹션(예: glossary, topic-aware hint)을 제거해
// 실제 설정 상태와 맞는 최종 프롬프트만 남기도록 정리한다.
func sanitizeDebugPromptOverride(prompt string, enhancedEnabled bool, topicAwareEnabled bool, isTranslationPrompt bool) string {
	result := prompt
	if !enhancedEnabled {
		result = glossaryBlockPattern.ReplaceAllString(result, "\n")
		if isTranslationPrompt {
			result = removeDebugSection(result, "# 2-1. Enhanced Context Translation Rules\n", []string{
				"\n# 2-2. Current Section\n",
				"\n# 2-3. Previous Context\n",
				"\n# 2-4. Recent Source Overlap\n",
				"\n# 3. Output Constraints And Source Text\n",
			})
		} else {
			result = removeDebugSection(result, "Consistency rule:\n", []string{
				"\nSource Text:\n",
				"\nTranslated Draft:\n",
			})
		}
	}

	if !topicAwareEnabled && !isTranslationPrompt {
		result = removeDebugSection(result, "Topic-aware smart post-editing hint:\n", []string{
			"\nRules:\n",
			"\nSource text:\n",
			"\nTranslated draft:\n",
		})
	}

	result = emptyDebugSectionPattern.ReplaceAllString(result, "")
	result = regexp.MustCompile(`\n{3,}`).ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}

func removeDebugSection(text, startMarker string, endMarkers []string) string {
	start := strings.Index(text, startMarker)
	if start == -1 {
		return text
	}

	end := len(text)
	searchFrom := start + len(startMarker)
	for _, marker := range endMarkers {
		if idx := strings.Index(text[searchFrom:], marker); idx != -1 {
			candidate := searchFrom + idx
			if candidate < end {
				end = candidate
			}
		}
	}

	return text[:start] + text[end:]
}

func preprocessSourceText(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		lines[i] = cleanInlineNoise(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func cleanupTranslatedText(text string) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		lines[i] = cleanInlineNoise(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func cleanInlineNoise(text string) string {
	text = wikiCitationPattern.ReplaceAllString(text, "")
	text = multiSpacePattern.ReplaceAllString(text, " ")
	text = spaceBeforePunctPattern.ReplaceAllString(text, `$1`)
	return strings.TrimSpace(text)
}

func extractProtectedTerms(sourceText string) []string {
	matches := protectedTermPattern.FindAllString(sourceText, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	terms := make([]string, 0, 12)
	for _, match := range matches {
		term := strings.TrimSpace(match)
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
		if len(terms) >= 12 {
			break
		}
	}
	return terms
}

func (c *Client) postEditTranslation(reqCtx context.Context, reqData TranslationRequest, draftTranslation string, runtimeOptions translationRuntimeOptions) (string, error) {
	prompt := buildPostEditPrompt(reqData.Settings, reqData.SourceLang, reqData.TargetLang, reqData.SourceText, draftTranslation, reqData.Instruction, runtimeOptions)
	topicAwareHints := topicAwarePostEditTemplateValue(reqData.Settings, reqData.SourceText, draftTranslation, reqData.Instruction)
	resolvedTemperature := resolvePostEditTemperature(reqData.Settings)
	c.emitDebug("note", "prompt:postedit", prompt)
	c.emitDebug("note", "prompt:topic-aware-hints", topicAwareHints)
	c.emitDebug("note", "temperature:postedit", formatTemperatureDebugNote("post-edit", resolvedTemperature))
	options := promptPassOptions{
		Prompt:          prompt,
		PreparingLabel:  "Preparing post-edit",
		PreparingDetail: "Sending draft for final review",
		LoadingLabel:    "Post-editing translation",
		LoadingDetail:   "Loading model for final review",
		PromptLabel:     "Post-editing translation",
		PromptDetail:    "Reviewing source text and translated draft",
		GenerateLabel:   "Post-editing translation",
		GenerateDetail:  "Fixing mixed-language or garbled fragments",
		StreamTokens:    false,
		EmitStats:       false,
		Temperature:     resolvedTemperature,
	}

	var (
		edited string
		err    error
	)
	if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
		edited, _, err = c.runLMStudioPrompt(reqCtx, reqData.Settings, options)
	} else {
		edited, _, err = c.runCompatiblePrompt(reqCtx, reqData.Settings, options)
	}
	if err != nil {
		return "", err
	}

	trimmed := strings.TrimSpace(edited)
	if trimmed == "" {
		return "", fmt.Errorf("post-edit returned an empty translation")
	}
	return trimmed, nil
}

type smartChunk struct {
	Text           string
	OverlapContext string
}

const draftStageProgressWeight = 0.75

func chunkCharacterStats(chunks []smartChunk) ([]int, int) {
	if len(chunks) == 0 {
		return nil, 0
	}

	lengths := make([]int, len(chunks))
	total := 0
	for i, chunk := range chunks {
		length := len([]rune(strings.TrimSpace(chunk.Text)))
		lengths[i] = length
		total += length
	}
	return lengths, total
}

func overallChunkProgress(totalChars, completedChars, currentChunkChars int, phaseWeight float64) *float64 {
	if totalChars <= 0 {
		return nil
	}
	if phaseWeight < 0 {
		phaseWeight = 0
	} else if phaseWeight > 1 {
		phaseWeight = 1
	}

	progress := (float64(completedChars) + float64(currentChunkChars)*phaseWeight) / float64(totalChars)
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}
	return floatPtr(progress)
}

func overallPassProgress(totalChunks, currentChunk int, postEditEnabled bool, phase string) *float64 {
	if totalChunks <= 0 || currentChunk <= 0 {
		return nil
	}

	totalUnits := totalChunks
	completedUnits := 0

	if postEditEnabled {
		totalUnits = totalChunks * 2
		switch phase {
		case "post_edit":
			completedUnits = currentChunk * 2
		default:
			completedUnits = currentChunk*2 - 1
		}
	} else {
		completedUnits = currentChunk
	}

	progress := float64(completedUnits) / float64(totalUnits)
	if progress < 0 {
		progress = 0
	} else if progress > 1 {
		progress = 1
	}
	return floatPtr(progress)
}

func buildProgressMetrics(totalChunks, currentChunk int, postEditEnabled bool, phase string) progressMetrics {
	completedChunks := currentChunk - 1
	if completedChunks < 0 {
		completedChunks = 0
	}
	metrics := progressMetrics{
		CurrentChunk:    currentChunk,
		CompletedChunks: completedChunks,
		TotalChunks:     totalChunks,
	}
	if totalChunks <= 0 || currentChunk <= 0 {
		return metrics
	}

	totalSteps := totalChunks
	currentStep := currentChunk
	if postEditEnabled {
		totalSteps = totalChunks * 2
		if phase == "post_edit" {
			currentStep = currentChunk * 2
		} else {
			currentStep = currentChunk*2 - 1
		}
	}
	metrics.CurrentStep = currentStep
	metrics.TotalSteps = totalSteps
	return metrics
}

func buildSmartChunks(text string, maxChars int) []smartChunk {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}
	if maxChars <= 0 || len([]rune(trimmed)) <= maxChars {
		return []smartChunk{{Text: trimmed}}
	}

	paragraphs := splitParagraphs(trimmed)
	chunks := make([]smartChunk, 0)
	current := make([]string, 0)
	currentLen := 0

	flushCurrent := func() {
		if len(current) == 0 {
			return
		}
		text := strings.Join(current, "\n\n")
		chunks = append(chunks, smartChunk{Text: text})
		current = nil
		currentLen = 0
	}

	for _, paragraph := range paragraphs {
		paragraphLen := len([]rune(paragraph))
		if paragraphLen > maxChars {
			flushCurrent()
			for _, sentenceChunk := range splitLongParagraph(paragraph, maxChars) {
				chunks = append(chunks, smartChunk{Text: sentenceChunk})
			}
			continue
		}

		if currentLen > 0 && currentLen+2+paragraphLen > maxChars {
			flushCurrent()
		}
		current = append(current, paragraph)
		currentLen += paragraphLen
	}
	flushCurrent()

	if len(chunks) <= 1 {
		return chunks
	}

	overlapSize := maxChars / 8
	if overlapSize < 120 {
		overlapSize = 120
	}
	for i := 1; i < len(chunks); i++ {
		overlap := trailingSentences(chunks[i-1].Text, overlapSize)
		if overlap == "" {
			continue
		}
		chunks[i].OverlapContext = overlap
	}
	return chunks
}

func splitParagraphs(text string) []string {
	raw := strings.Split(text, "\n\n")
	result := make([]string, 0, len(raw))
	for _, paragraph := range raw {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func leadingParagraph(text string, maxChars int) string {
	paragraphs := splitParagraphs(strings.TrimSpace(text))
	if len(paragraphs) == 0 {
		return ""
	}
	first := strings.TrimSpace(paragraphs[0])
	if maxChars <= 0 {
		return first
	}
	return leadingRunes(first, maxChars)
}

func splitLongParagraph(paragraph string, maxChars int) []string {
	sentences := splitSentences(paragraph)
	if len(sentences) <= 1 {
		return splitByRunes(paragraph, maxChars)
	}

	result := make([]string, 0)
	current := make([]string, 0)
	currentLen := 0
	for _, sentence := range sentences {
		sentenceLen := len([]rune(sentence))
		if sentenceLen > maxChars {
			if len(current) > 0 {
				result = append(result, strings.Join(current, " "))
				current = nil
				currentLen = 0
			}
			result = append(result, splitByRunes(sentence, maxChars)...)
			continue
		}
		if currentLen > 0 && currentLen+1+sentenceLen > maxChars {
			result = append(result, strings.Join(current, " "))
			current = nil
			currentLen = 0
		}
		current = append(current, sentence)
		currentLen += sentenceLen
	}
	if len(current) > 0 {
		result = append(result, strings.Join(current, " "))
	}
	return result
}

func splitSentences(text string) []string {
	normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\n", " ")
	parts := strings.Fields(normalized)
	if len(parts) == 0 {
		return nil
	}
	sentences := make([]string, 0)
	var current strings.Builder
	for _, part := range parts {
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(part)
		if strings.HasSuffix(part, ".") || strings.HasSuffix(part, "!") || strings.HasSuffix(part, "?") {
			sentences = append(sentences, current.String())
			current.Reset()
		}
	}
	if current.Len() > 0 {
		sentences = append(sentences, current.String())
	}
	return sentences
}

func splitByRunes(text string, maxChars int) []string {
	runes := []rune(text)
	if len(runes) <= maxChars {
		return []string{text}
	}
	result := make([]string, 0)
	for start := 0; start < len(runes); start += maxChars {
		end := start + maxChars
		if end > len(runes) {
			end = len(runes)
		}
		result = append(result, strings.TrimSpace(string(runes[start:end])))
	}
	return result
}

func trailingSentences(text string, maxChars int) string {
	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return trailingRunes(text, maxChars)
	}

	collected := make([]string, 0)
	total := 0
	for i := len(sentences) - 1; i >= 0; i-- {
		sentenceLen := len([]rune(sentences[i]))
		if total > 0 && total+1+sentenceLen > maxChars {
			break
		}
		collected = append([]string{sentences[i]}, collected...)
		total += sentenceLen
		if len(collected) >= 2 {
			break
		}
	}
	return strings.Join(collected, " ")
}

func trailingRunes(text string, maxChars int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxChars {
		return string(runes)
	}
	return string(runes[len(runes)-maxChars:])
}

func leadingRunes(text string, maxChars int) string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) <= maxChars {
		return string(runes)
	}
	return string(runes[:maxChars])
}

func buildContextSummary(settings ProviderSettings, instruction, sourceChunk, translatedChunk string) string {
	sourceTail := trailingSentences(sourceChunk, 240)
	translatedTail := trailingSentences(translatedChunk, 240)
	parts := make([]string, 0, 5)
	if styleMemory := buildStyleMemorySummary(settings, instruction, translatedChunk); styleMemory != "" {
		parts = append(parts, styleMemory)
	}
	if sourceTail != "" {
		parts = append(parts, "Source tail: "+sourceTail)
	}
	if translatedTail != "" {
		parts = append(parts, "Translated tail: "+translatedTail)
	}
	return strings.Join(parts, "\n")
}

func buildStyleMemorySummary(settings ProviderSettings, instruction, translatedChunk string) string {
	lines := make([]string, 0, 6)
	lines = append(lines, "Carry-forward style memory:")
	lines = append(lines, "- User instruction remains mandatory for all remaining chunks.")
	if trimmedInstruction := strings.TrimSpace(instruction); trimmedInstruction != "" {
		lines = append(lines, "- Active user style: "+singleLinePreview(trimmedInstruction, 220))
	}
	if tone := detectPostEditTone(translatedChunk, instruction); tone != "" {
		lines = append(lines, "- Established tone/register so far: "+tone)
	}
	if lockedTerms := glossaryMemoryLines(settings.EnhancedContextGlossary, 4); len(lockedTerms) > 0 {
		lines = append(lines, "- Locked terminology from glossary:")
		for _, term := range lockedTerms {
			lines = append(lines, "  "+term)
		}
	}
	return strings.Join(lines, "\n")
}

func glossaryMemoryLines(glossary string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	lines := make([]string, 0, limit)
	for _, raw := range strings.Split(strings.ReplaceAll(glossary, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			continue
		}
		lines = append(lines, "- "+singleLinePreview(trimmed, 120))
		if len(lines) >= limit {
			break
		}
	}
	return lines
}

func singleLinePreview(text string, maxChars int) string {
	normalized := strings.TrimSpace(strings.Join(strings.Fields(text), " "))
	if normalized == "" {
		return ""
	}
	if maxChars <= 0 {
		return normalized
	}
	return leadingRunes(normalized, maxChars)
}

func normalizeLanguageLabel(language string) string {
	switch strings.TrimSpace(strings.ToLower(language)) {
	case "auto":
		return "source language"
	case "english":
		return "English"
	case "korean":
		return "Korean"
	case "japanese":
		return "Japanese"
	case "chinese":
		return "Chinese"
	case "french":
		return "French"
	case "german":
		return "German"
	default:
		return language
	}
}

func extractContentPieces(raw map[string]any) []string {
	choices, ok := raw["choices"].([]any)
	if !ok {
		return nil
	}

	pieces := make([]string, 0)
	for _, choice := range choices {
		choiceMap, ok := choice.(map[string]any)
		if !ok {
			continue
		}

		if delta, ok := choiceMap["delta"].(map[string]any); ok {
			pieces = append(pieces, extractContentValue(delta["content"])...)
		}
		if message, ok := choiceMap["message"].(map[string]any); ok {
			pieces = append(pieces, extractContentValue(message["content"])...)
		}
		if text, ok := choiceMap["text"].(string); ok && text != "" {
			pieces = append(pieces, text)
		}
	}

	return pieces
}

func extractContentValue(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed == "" {
			return nil
		}
		return []string{typed}
	case []any:
		pieces := make([]string, 0)
		for _, item := range typed {
			itemMap, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := itemMap["text"].(string); ok && text != "" {
				pieces = append(pieces, text)
			}
		}
		return pieces
	default:
		return nil
	}
}

func extractLMStudioMessageContent(raw map[string]any) []string {
	if content, ok := raw["content"].(string); ok && content != "" {
		return []string{content}
	}
	return nil
}

func extractLMStudioFinalMessage(raw map[string]any) string {
	output, ok := raw["output"].([]any)
	if !ok {
		if resultMap, ok := raw["result"].(map[string]any); ok {
			output, ok = resultMap["output"].([]any)
		}
	}
	if !ok {
		return ""
	}

	var builder strings.Builder
	for _, item := range output {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if itemType, _ := itemMap["type"].(string); itemType != "message" {
			continue
		}
		if content, ok := itemMap["content"].(string); ok {
			builder.WriteString(content)
		}
	}

	return builder.String()
}

func parseReasoningCapability(value any) (bool, []string) {
	defaultOptions := []string{"off", "low", "medium", "high", "on"}
	switch typed := value.(type) {
	case nil:
		return false, nil
	case bool:
		if typed {
			return true, defaultOptions
		}
		return false, nil
	case string:
		trimmed := normalizeReasoningValue(typed)
		if trimmed == "" {
			return false, nil
		}
		return true, uniqueReasoningOptions([]string{trimmed}, defaultOptions)
	case []any:
		options := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok {
				options = append(options, str)
			}
		}
		if len(options) == 0 {
			return false, nil
		}
		return true, uniqueReasoningOptions(options, nil)
	case map[string]any:
		if supported, ok := typed["supported"].(bool); ok && !supported {
			return false, nil
		}
		if enabled, ok := typed["enabled"].(bool); ok && !enabled {
			return false, nil
		}
		for _, key := range []string{"options", "levels", "supported_levels", "supported_values"} {
			if rawOptions, ok := typed[key].([]any); ok {
				options := make([]string, 0, len(rawOptions))
				for _, item := range rawOptions {
					if str, ok := item.(string); ok {
						options = append(options, str)
					}
				}
				if len(options) > 0 {
					return true, uniqueReasoningOptions(options, nil)
				}
			}
		}
		return true, defaultOptions
	default:
		return false, nil
	}
}

func uniqueReasoningOptions(values []string, fallback []string) []string {
	order := []string{"off", "low", "medium", "high", "on"}
	set := make(map[string]struct{})
	for _, value := range values {
		if normalized := normalizeReasoningValue(value); normalized != "" {
			set[normalized] = struct{}{}
		}
	}
	if len(set) == 0 {
		for _, value := range fallback {
			if normalized := normalizeReasoningValue(value); normalized != "" {
				set[normalized] = struct{}{}
			}
		}
	}
	if len(set) == 0 {
		return nil
	}

	result := make([]string, 0, len(set))
	for _, candidate := range order {
		if _, ok := set[candidate]; ok {
			result = append(result, candidate)
		}
	}
	return result
}

func normalizeReasoningValue(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "off", "low", "medium", "high", "on":
		return strings.TrimSpace(strings.ToLower(value))
	default:
		return ""
	}
}

func normalizeTemperatureValue(value float64) *float64 {
	if value <= 0 {
		return nil
	}
	if value > 1 {
		value = 1
	}
	rounded := float64(int(value*10+0.5)) / 10
	if rounded <= 0 {
		return nil
	}
	return &rounded
}

func resolvePromptTemperature(settings ProviderSettings, override *float64) *float64 {
	if override != nil {
		return override
	}
	return normalizeTemperatureValue(settings.Temperature)
}

func resolveDraftTemperature(settings ProviderSettings) *float64 {
	if settings.EnablePostEdit {
		return normalizeTemperatureValue(0.1)
	}
	return normalizeTemperatureValue(settings.Temperature)
}

func resolvePostEditTemperature(settings ProviderSettings) *float64 {
	temperature := normalizeTemperatureValue(settings.Temperature)
	if temperature == nil {
		return nil
	}
	if *temperature == 0.1 {
		return floatPtr(0.2)
	}
	return temperature
}

func formatTemperatureDebugNote(stage string, temperature *float64) string {
	if temperature == nil {
		return fmt.Sprintf("%s temperature omitted (auto)", stage)
	}
	return fmt.Sprintf("%s temperature %.1f", stage, *temperature)
}

func mapFloat(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}

func floatPtr(value float64) *float64 {
	clamped := value
	if clamped < 0 {
		clamped = 0
	}
	if clamped > 1 {
		clamped = 1
	}
	return &clamped
}

func derefFloat(value *float64) float64 {
	if value == nil {
		return 0
	}
	return *value
}

func boolPtr(value bool) *bool {
	return &value
}

func (c *Client) currentSink() eventSink {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.requestSink != nil {
		return c.requestSink
	}
	if c.ctx == nil {
		return nil
	}
	return runtimeEventSink{ctx: c.ctx}
}

func (c *Client) emitToken(token string) {
	if sink := c.currentSink(); sink != nil {
		sink.Token(token)
	}
}

func (c *Client) emitClear() {
	if sink := c.currentSink(); sink != nil {
		sink.Clear()
	}
}

func (c *Client) emitComplete(payload TranslationCompletePayload) {
	if sink := c.currentSink(); sink != nil {
		sink.Complete(payload)
	}
}

func (c *Client) emitProgress(stage, label, detail string, progress *float64, indeterminate bool) {
	c.emitProgressWithMetrics(stage, label, detail, progress, indeterminate, progressMetrics{})
}

func (c *Client) emitProgressWithMetrics(stage, label, detail string, progress *float64, indeterminate bool, metrics progressMetrics) {
	sink := c.currentSink()
	if sink == nil {
		return
	}
	sink.Progress(TranslationProgressPayload{
		Stage:           stage,
		Label:           label,
		Detail:          detail,
		Progress:        progress,
		CurrentChunk:    metrics.CurrentChunk,
		CompletedChunks: metrics.CompletedChunks,
		TotalChunks:     metrics.TotalChunks,
		CurrentStep:     metrics.CurrentStep,
		TotalSteps:      metrics.TotalSteps,
		Visible:         true,
		Indeterminate:   indeterminate,
	})
}

func (c *Client) emitPromptProgress(stage, label, detail string, localProgress *float64, indeterminate bool, options promptPassOptions) {
	c.emitProgressWithMetrics(
		stage,
		label,
		detail,
		localProgress,
		indeterminate,
		options.ProgressMetrics,
	)
}

func (c *Client) emitProgressHidden() {
	sink := c.currentSink()
	if sink == nil {
		return
	}
	sink.Progress(TranslationProgressPayload{
		Visible: false,
	})
}

func (c *Client) emitStats(raw map[string]any) {
	sink := c.currentSink()
	if sink == nil {
		return
	}
	sink.Stats(statsFromMap(raw))
}

func statsFromMap(raw map[string]any) TranslationStatsPayload {
	return TranslationStatsPayload{
		InputTokens:             int(mapFloat(raw["input_tokens"])),
		ReasoningOutputTokens:   int(mapFloat(raw["reasoning_output_tokens"])),
		TimeToFirstTokenSeconds: mapFloat(raw["time_to_first_token_seconds"]),
		TokensPerSecond:         mapFloat(raw["tokens_per_second"]),
		TotalOutputTokens:       int(mapFloat(raw["total_output_tokens"])),
	}
}

func (c *Client) emitDebug(direction string, endpoint string, payload any) {
	sink := c.currentSink()
	if sink == nil {
		return
	}

	pretty, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		pretty = []byte(fmt.Sprintf("%v", payload))
	}
	sink.Debug(direction, endpoint, string(pretty))
}

type runtimeEventSink struct {
	ctx context.Context
}

func (s runtimeEventSink) Token(token string) {
	runtime.EventsEmit(s.ctx, "translation:token", token)
}

func (s runtimeEventSink) Clear() {
	runtime.EventsEmit(s.ctx, "translation:clear")
}

func (s runtimeEventSink) Complete(payload TranslationCompletePayload) {
	runtime.EventsEmit(s.ctx, "translation:complete", payload)
}

func (s runtimeEventSink) Progress(payload TranslationProgressPayload) {
	runtime.EventsEmit(s.ctx, "translation:progress", payload)
}

func (s runtimeEventSink) Stats(payload TranslationStatsPayload) {
	runtime.EventsEmit(s.ctx, "translation:stats", payload)
}

func (s runtimeEventSink) Debug(direction string, endpoint string, payload string) {
	runtime.EventsEmit(s.ctx, "translation:debug", map[string]string{
		"direction": direction,
		"endpoint":  endpoint,
		"payload":   payload,
	})
}
