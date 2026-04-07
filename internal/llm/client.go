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
	inlineProofreadPattern   = regexp.MustCompile(`(?is)\{draft:\s*(.*?)\s*\}\s*\{review:\s*(.*?)\s*\}\s*\{final:\s*(.*?)\s*\}\s*$`)
	fencedCodeBlockPattern   = regexp.MustCompile("(?s)^```[a-zA-Z0-9_-]*\\s*(.*?)\\s*```$")
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
	Chunk(TranslationChunkPayload)
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

type TranslationChunkPayload struct {
	ChunkIndex int    `json:"chunk_index"`
	Phase      string `json:"phase"`
	Text       string `json:"text"`
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

type inlineProofreadResult struct {
	Draft  string
	Review string
	Final  string
}

type inlineProofreadSections struct {
	Draft       string
	Review      string
	Final       string
	HasDraft    bool
	HasReview   bool
	HasFinal    bool
	FinalClosed bool
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
	OnToken             func(string)
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

type translationHarnessMode struct {
	emitLifecycle bool
	clearAtStart  bool
}

type plannedTranslation struct {
	chunkSize              int
	chunks                 []smartChunk
	openingSourceParagraph string
}

// chunkPlanner는 입력 텍스트를 실행 가능한 번역 청크 계획으로 바꾼다.
// 전처리된 원문을 받아 스마트 청킹 여부, 청크 크기, 오프닝 앵커를 한 번에 계산한다.
type chunkPlanner struct{}

func (p chunkPlanner) plan(reqData TranslationRequest) plannedTranslation {
	chunkSize := reqData.Settings.SmartChunkSize
	if chunkSize <= 0 {
		chunkSize = 2000
	}

	chunks := []smartChunk{{Text: strings.TrimSpace(reqData.SourceText)}}
	if reqData.Settings.EnableSmartChunking {
		chunks = buildSmartChunks(reqData.SourceText, chunkSize)
	}
	_, _ = chunkCharacterStats(chunks)

	return plannedTranslation{
		chunkSize:              chunkSize,
		chunks:                 chunks,
		openingSourceParagraph: leadingParagraph(reqData.SourceText, 420),
	}
}

// draftTranslator는 초벌 번역 패스를 담당한다.
// 실제 provider 분기와 draft 프롬프트 실행은 이 컴포넌트 안에서만 이뤄진다.
type draftTranslator struct {
	client *Client
	reqCtx context.Context
}

func (t draftTranslator) translate(reqData TranslationRequest, options translationRuntimeOptions) (string, TranslationStatsPayload, error) {
	if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
		return t.client.translateWithLMStudio(t.reqCtx, reqData, options)
	}
	return t.client.translateWithCompatibleAPI(t.reqCtx, reqData, options)
}

// postEditPass는 선택적 proofread/post-edit 단계를 담당한다.
// 포스트 에디트 활성화 여부, 진행률 이벤트, 실제 post-edit 호출을 한 곳에 모은다.
type postEditPass struct {
	client        *Client
	reqCtx        context.Context
	emitLifecycle bool
}

func (p postEditPass) apply(reqData TranslationRequest, draft string, options translationRuntimeOptions, currentChunk int, totalChunks int) (string, error) {
	if !reqData.Settings.EnablePostEdit {
		return draft, nil
	}

	if p.emitLifecycle {
		label := "Post-editing translation"
		detail := "Checking for mixed-language or garbled fragments"
		if totalChunks > 1 {
			label = fmt.Sprintf("Post-editing %s", options.ChunkLabel)
			detail = "Checking this translated section for mixed-language or garbled fragments"
		}
		p.client.emitProgressWithMetrics(
			"post_edit",
			label,
			detail,
			overallPassProgress(totalChunks, currentChunk, reqData.Settings.EnablePostEdit, "post_edit"),
			false,
			buildProgressMetrics(totalChunks, currentChunk, reqData.Settings.EnablePostEdit, "post_edit"),
		)
	}

	postEditOptions := options
	postEditOptions.OverallProgressBase = derefFloat(overallPassProgress(totalChunks, currentChunk, reqData.Settings.EnablePostEdit, "post_edit"))
	return p.client.postEditTranslation(p.reqCtx, reqData, draft, postEditOptions)
}

type inlineProofreadPass struct {
	client        *Client
	reqCtx        context.Context
	emitLifecycle bool
}

type inlineProofreadStreamState struct {
	raw          strings.Builder
	lastDraft    string
	lastReview   string
	lastFinal    string
	enteredFinal bool
	finalClosed  bool
}

func (s *inlineProofreadStreamState) consume(piece string) (string, string, string) {
	if piece == "" {
		return "", "", ""
	}
	s.raw.WriteString(piece)
	parsed := extractInlineProofreadSections(s.raw.String())

	var nextDraft string
	if !s.enteredFinal && parsed.Draft != s.lastDraft {
		s.lastDraft = parsed.Draft
		nextDraft = parsed.Draft
	}

	var nextReview string
	if parsed.HasReview && parsed.Review != s.lastReview {
		s.lastReview = parsed.Review
		nextReview = parsed.Review
	}

	var nextFinal string
	if parsed.HasFinal {
		if strings.TrimSpace(parsed.Final) != "" {
			s.enteredFinal = true
		}
		if parsed.Final != s.lastFinal {
			s.lastFinal = parsed.Final
			nextFinal = parsed.Final
		}
	}
	s.finalClosed = parsed.FinalClosed

	return nextDraft, nextReview, nextFinal
}

func (p inlineProofreadPass) apply(reqData TranslationRequest, options translationRuntimeOptions, currentChunk int, totalChunks int) (inlineProofreadResult, TranslationStatsPayload, error) {
	prompt := buildInlineProofreadPrompt(reqData.Settings, reqData.SourceLang, reqData.TargetLang, reqData.SourceText, reqData.Instruction, options)
	resolvedTemperature := resolveInlineProofreadTemperature(reqData.Settings)
	streamState := inlineProofreadStreamState{}
	emitChunkUpdate := func(phase string, text string) {
		if strings.TrimSpace(text) == "" {
			return
		}
		p.client.emitChunk(TranslationChunkPayload{
			ChunkIndex: currentChunk - 1,
			Phase:      phase,
			Text:       cleanupTranslatedText(text),
		})
	}
	handlePiece := func(piece string) {
		draft, review, final := streamState.consume(piece)
		if draft != "" && !streamState.enteredFinal {
			emitChunkUpdate("draft", draft)
		}
		if review != "" {
			emitChunkUpdate("review", review)
		}
		if final != "" {
			emitChunkUpdate("final", final)
		}
	}

	if p.emitLifecycle {
		label := "Translating and proofreading"
		detail := "Creating a draft and refining it in one pass"
		if totalChunks > 1 {
			label = fmt.Sprintf("Translating and proofreading %s", options.ChunkLabel)
			detail = "Generating draft and final text for this section"
		}
		p.client.emitProgressWithMetrics(
			"chunking",
			label,
			detail,
			overallPassProgress(totalChunks, currentChunk, false, "translate"),
			false,
			buildProgressMetrics(totalChunks, currentChunk, false, "translate"),
		)
	}

	p.client.emitDebug("note", "prompt:translation", prompt)
	p.client.emitDebug("note", "temperature:translation", formatTemperatureDebugNote("inline-proofread", resolvedTemperature))

	var (
		raw   string
		stats TranslationStatsPayload
		err   error
	)
	if strings.EqualFold(reqData.Settings.Mode, "lmstudio") {
		raw, stats, err = p.client.runLMStudioPrompt(p.reqCtx, reqData.Settings, promptPassOptions{
			Prompt:              prompt,
			PreparingLabel:      "Preparing request",
			PreparingDetail:     "Connecting to LM Studio",
			LoadingLabel:        "Loading model",
			LoadingDetail:       "Preparing model weights",
			PromptLabel:         "Processing prompt",
			PromptDetail:        "Tokenizing multi-step translation prompt",
			GenerateLabel:       "Translating and proofreading",
			GenerateDetail:      "Waiting for draft and final output",
			StreamTokens:        false,
			OnToken:             handlePiece,
			EmitStats:           true,
			Temperature:         resolvedTemperature,
			OverallProgressBase: options.OverallProgressBase,
			OverallProgressSpan: options.OverallProgressSpan,
			ProgressMetrics:     options.ProgressMetrics,
		})
	} else {
		raw, stats, err = p.client.runCompatiblePrompt(p.reqCtx, reqData.Settings, promptPassOptions{
			Prompt:              prompt,
			PreparingLabel:      "Preparing request",
			PreparingDetail:     "Connecting to completion endpoint",
			GenerateLabel:       "Translating and proofreading",
			GenerateDetail:      "Waiting for draft and final output",
			StreamTokens:        false,
			OnToken:             handlePiece,
			EmitStats:           false,
			Temperature:         resolvedTemperature,
			OverallProgressBase: options.OverallProgressBase,
			OverallProgressSpan: options.OverallProgressSpan,
			ProgressMetrics:     options.ProgressMetrics,
		})
	}
	if err != nil {
		return inlineProofreadResult{}, TranslationStatsPayload{}, err
	}

	parsed, err := parseInlineProofreadResponse(raw)
	if err != nil {
		p.client.emitDebug("note", "inline-proofread:parse-fallback", map[string]any{
			"message": "inline proofread output did not match expected structure; falling back to legacy two-pass flow",
			"raw":     raw,
		})
		return inlineProofreadResult{}, TranslationStatsPayload{}, err
	}

	parsed.Draft = cleanupTranslatedText(parsed.Draft)
	parsed.Final = cleanupTranslatedText(parsed.Final)
	return parsed, stats, nil
}

// contextMemory는 청크 간 이어져야 하는 문맥 기억을 관리한다.
// 이전 요약, opening anchor, overlap 관련 runtime 옵션 생성과 갱신을 맡는다.
type contextMemory struct {
	settings                   ProviderSettings
	instruction                string
	separatePostEditPass       bool
	previousSummary            string
	openingSourceParagraph     string
	openingTranslatedParagraph string
}

func newContextMemory(reqData TranslationRequest, openingSourceParagraph string, separatePostEditPass bool) *contextMemory {
	return &contextMemory{
		settings:               reqData.Settings,
		instruction:            reqData.Instruction,
		separatePostEditPass:   separatePostEditPass,
		openingSourceParagraph: openingSourceParagraph,
	}
}

func (m *contextMemory) runtimeOptions(chunk smartChunk, currentChunk int, totalChunks int) translationRuntimeOptions {
	return translationRuntimeOptions{
		ContextSummary:             m.previousSummary,
		ChunkLabel:                 fmt.Sprintf("Chunk %d/%d", currentChunk, totalChunks),
		OverlapContext:             chunk.OverlapContext,
		OpeningSourceParagraph:     m.openingSourceParagraph,
		OpeningTranslatedParagraph: m.openingTranslatedParagraph,
		ProgressMetrics:            buildProgressMetrics(totalChunks, currentChunk, m.separatePostEditPass, "translate"),
		OverallProgressBase:        derefFloat(overallPassProgress(totalChunks, currentChunk, m.separatePostEditPass, "translate")),
	}
}

func (m *contextMemory) update(sourceChunk string, translatedChunk string, combinedTranslation string) {
	m.previousSummary = buildContextSummary(m.settings, m.instruction, sourceChunk, translatedChunk)
	if m.openingTranslatedParagraph == "" {
		m.openingTranslatedParagraph = leadingParagraph(combinedTranslation, 420)
	}
}

// translationHarness는 번역 요청 하나를 실행하는 하네스다.
// 입력 전처리, 청킹, 초벌 번역, 선택적 포스트 에디트, 문맥 계승, 결과 결합을
// 한 구조 안에서 순차적으로 관리해 엔트리 포인트별 중복을 줄인다.
type translationHarness struct {
	client               *Client
	reqCtx               context.Context
	reqData              TranslationRequest
	mode                 translationHarnessMode
	plan                 plannedTranslation
	draftTranslator      draftTranslator
	inlineProofread      inlineProofreadPass
	postEditor           postEditPass
	contextMemory        *contextMemory
	separatePostEditPass bool

	aggregated TranslationStatsPayload
	finalText  strings.Builder
}

func (c *Client) Translate(reqData TranslationRequest) error {
	reqCtx, cancel, requestID := c.beginRequest()
	defer cancel()
	defer c.finishRequest(requestID)

	harness := newTranslationHarness(c, reqCtx, reqData, translationHarnessMode{emitLifecycle: true})
	text, _, err := harness.run()
	if err != nil {
		return err
	}
	c.emitComplete(TranslationCompletePayload{Text: text})
	return nil
}

func (c *Client) TranslateText(reqData TranslationRequest) (string, TranslationStatsPayload, error) {
	reqCtx, cancel, requestID := c.beginRequest()
	defer cancel()
	defer c.finishRequest(requestID)

	harness := newTranslationHarness(c, reqCtx, reqData, translationHarnessMode{})
	return harness.run()
}

func (c *Client) TranslateTextStream(reqData TranslationRequest, sink eventSink) (string, TranslationStatsPayload, error) {
	reqCtx, cancel, requestID := c.beginRequestWithSink(sink)
	defer cancel()
	defer c.finishRequest(requestID)

	harness := newTranslationHarness(c, reqCtx, reqData, translationHarnessMode{
		emitLifecycle: true,
		clearAtStart:  true,
	})
	text, stats, err := harness.run()
	if err != nil {
		return "", TranslationStatsPayload{}, err
	}
	c.emitComplete(TranslationCompletePayload{Text: text})
	return text, stats, nil
}

func newTranslationHarness(c *Client, reqCtx context.Context, reqData TranslationRequest, mode translationHarnessMode) *translationHarness {
	reqData.SourceText = preprocessSourceText(reqData.SourceText)
	plan := (chunkPlanner{}).plan(reqData)
	separatePostEditPass := reqData.Settings.EnablePostEdit && !useInlineProofread(reqData.Settings)

	return &translationHarness{
		client:               c,
		reqCtx:               reqCtx,
		reqData:              reqData,
		mode:                 mode,
		plan:                 plan,
		draftTranslator:      draftTranslator{client: c, reqCtx: reqCtx},
		inlineProofread:      inlineProofreadPass{client: c, reqCtx: reqCtx, emitLifecycle: mode.emitLifecycle},
		postEditor:           postEditPass{client: c, reqCtx: reqCtx, emitLifecycle: mode.emitLifecycle},
		contextMemory:        newContextMemory(reqData, plan.openingSourceParagraph, separatePostEditPass),
		separatePostEditPass: separatePostEditPass,
	}
}

func (h *translationHarness) run() (string, TranslationStatsPayload, error) {
	if h.mode.emitLifecycle && h.mode.clearAtStart {
		h.client.emitClear()
	}

	if len(h.plan.chunks) <= 1 {
		return h.runSingleChunk()
	}
	return h.runChunked()
}

func (h *translationHarness) runSingleChunk() (string, TranslationStatsPayload, error) {
	if h.mode.emitLifecycle {
		h.client.emitProgressWithMetrics(
			"chunking",
			"Generating translation",
			"Translating the full text",
			overallPassProgress(1, 1, h.separatePostEditPass, "translate"),
			false,
			buildProgressMetrics(1, 1, h.separatePostEditPass, "translate"),
		)
	}

	text, stats, err := h.runChunk(0, h.plan.chunks[0])
	if err != nil {
		return "", TranslationStatsPayload{}, err
	}

	h.finalizeLifecycle(text, stats)
	return text, stats, nil
}

func (h *translationHarness) runChunked() (string, TranslationStatsPayload, error) {
	if h.mode.emitLifecycle && !h.mode.clearAtStart {
		h.client.emitClear()
	}

	for index, chunk := range h.plan.chunks {
		if err := h.reqCtx.Err(); err != nil {
			return "", TranslationStatsPayload{}, fmt.Errorf("translation cancelled")
		}

		translated, stats, err := h.runChunk(index, chunk)
		if err != nil {
			return "", TranslationStatsPayload{}, err
		}

		if index > 0 && h.finalText.Len() > 0 && !strings.HasSuffix(h.finalText.String(), "\n") && !strings.HasPrefix(translated, "\n") {
			h.finalText.WriteString("\n\n")
			if h.mode.emitLifecycle {
				h.client.emitToken("\n\n")
			}
		}
		h.finalText.WriteString(translated)
		h.addStats(stats)
		h.contextMemory.update(chunk.Text, translated, h.finalText.String())
	}

	h.averageAggregatedStats()
	final := h.finalText.String()
	h.finalizeLifecycle(final, h.aggregated)
	return final, h.aggregated, nil
}

func (h *translationHarness) runChunk(index int, chunk smartChunk) (string, TranslationStatsPayload, error) {
	chunkReq := h.reqData
	chunkReq.SourceText = chunk.Text

	options := h.contextMemory.runtimeOptions(chunk, index+1, len(h.plan.chunks))

	if h.mode.emitLifecycle {
		h.client.emitProgressWithMetrics(
			"chunking",
			fmt.Sprintf("Translating %s", options.ChunkLabel),
			fmt.Sprintf("Smart chunking active for long text (%d chars)", h.plan.chunkSize),
			overallPassProgress(len(h.plan.chunks), index+1, h.separatePostEditPass, "translate"),
			false,
			options.ProgressMetrics,
		)
	}

	if useInlineProofread(chunkReq.Settings) {
		result, stats, err := h.inlineProofread.apply(chunkReq, options, index+1, len(h.plan.chunks))
		if err == nil {
			return result.Final, stats, nil
		}
		return "", TranslationStatsPayload{}, err
	}

	translated, stats, err := h.draftTranslator.translate(chunkReq, options)
	if err != nil {
		return "", TranslationStatsPayload{}, err
	}
	translated, err = h.postEditor.apply(chunkReq, translated, options, index+1, len(h.plan.chunks))
	if err != nil {
		return "", TranslationStatsPayload{}, err
	}
	return cleanupTranslatedText(translated), stats, nil
}

func (h *translationHarness) addStats(stats TranslationStatsPayload) {
	h.aggregated.InputTokens += stats.InputTokens
	h.aggregated.ReasoningOutputTokens += stats.ReasoningOutputTokens
	h.aggregated.TotalOutputTokens += stats.TotalOutputTokens
	h.aggregated.TimeToFirstTokenSeconds += stats.TimeToFirstTokenSeconds
	h.aggregated.TokensPerSecond += stats.TokensPerSecond
}

func (h *translationHarness) averageAggregatedStats() {
	if len(h.plan.chunks) == 0 {
		return
	}
	h.aggregated.TimeToFirstTokenSeconds = h.aggregated.TimeToFirstTokenSeconds / float64(len(h.plan.chunks))
	h.aggregated.TokensPerSecond = h.aggregated.TokensPerSecond / float64(len(h.plan.chunks))
}

func (h *translationHarness) finalizeLifecycle(text string, stats TranslationStatsPayload) {
	if !h.mode.emitLifecycle {
		return
	}
	if stats.InputTokens > 0 || stats.TotalOutputTokens > 0 {
		h.client.emitStats(map[string]any{
			"input_tokens":                stats.InputTokens,
			"reasoning_output_tokens":     stats.ReasoningOutputTokens,
			"time_to_first_token_seconds": stats.TimeToFirstTokenSeconds,
			"tokens_per_second":           stats.TokensPerSecond,
			"total_output_tokens":         stats.TotalOutputTokens,
		})
	}
	h.client.emitProgress("done", "Done", "Translation complete", floatPtr(1), false)
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
						if options.OnToken != nil {
							options.OnToken(piece)
						}
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
				if options.OnToken != nil {
					options.OnToken(piece)
				}
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
	glossary := effectiveGlossary(settings, sourceText)
	protectedTerms := filterProtectedTermsByGlossary(extractProtectedTerms(sourceText), glossary)

	var builder strings.Builder
	if strings.TrimSpace(settings.DebugTranslationPromptTemplate) != "" {
		prompt := applyPromptTemplate(normalizeDebugPromptTemplateInput(settings.DebugTranslationPromptTemplate), map[string]string{
			"SOURCE_LANG":                  sourceLabel,
			"TARGET_LANG":                  targetLabel,
			"INSTRUCTION":                  strings.TrimSpace(instruction),
			"SOURCE_TEXT":                  sourceText,
			"PROTECTED_TERMS":              strings.Join(protectedTerms, "\n"),
			"GLOSSARY":                     glossary,
			"CHUNK_LABEL":                  strings.TrimSpace(runtimeOptions.ChunkLabel),
			"CONTEXT_SUMMARY":              strings.TrimSpace(runtimeOptions.ContextSummary),
			"OVERLAP_CONTEXT":              strings.TrimSpace(runtimeOptions.OverlapContext),
			"OPENING_SOURCE_PARAGRAPH":     strings.TrimSpace(runtimeOptions.OpeningSourceParagraph),
			"OPENING_TRANSLATED_PARAGRAPH": strings.TrimSpace(runtimeOptions.OpeningTranslatedParagraph),
		})
		return sanitizeDebugPromptOverride(prompt, settings.EnableEnhancedContextTranslation, false, true)
	}
	builder.WriteString("[Role And Core Persona]\n")
	builder.WriteString(fmt.Sprintf(
		"You are a professional %s to %s translator. Your goal is to accurately convey the meaning and nuances of the original %s text while adhering to %s grammar, vocabulary, and cultural sensitivities.\n",
		sourceLabel,
		targetLabel,
		sourceLabel,
		targetLabel,
	))

	builder.WriteString("\n---\n[Instruction Priority Rules]\n")
	builder.WriteString("- The user style instruction is mandatory for every chunk, including later chunks.\n")
	builder.WriteString("- If previous chunk wording, overlap context, or opening style anchor conflicts with the user instruction, follow the user instruction.\n")
	builder.WriteString("- Use continuity context only to preserve meaning, terminology, names, and narrative flow. Do not let it override the requested style.\n")

	builder.WriteString("\n---\n[User Style And Instructions]\n")
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

	if settings.EnableEnhancedContextTranslation {
		builder.WriteString("\n---\n[Enhanced Context Translation Rules]\n")
		builder.WriteString(fmt.Sprintf(
			"Maintain consistent %s translations for names, places, organizations, products, commands, and technical terms across the entire text. Reuse the same established translation whenever the same source term appears again. However, do not flatten meaningful surface-form differences such as titles, kinship labels, vocatives, or direct forms of address into one uniform rendering. If the user instruction or glossary specifies a transliteration or %s-script rendering, follow that specification consistently. Explicit user glossary mappings override generic protected-term handling.\n",
			targetLabel,
			targetLabel,
		))

		if glossary != "" {
			builder.WriteString("\nUse the following User Glossary for consistent translation. If these terms appear in the source text, they MUST be translated as specified:\n")
			builder.WriteString("<GLOSSARY>\n")
			builder.WriteString(glossary)
			builder.WriteString("\n</GLOSSARY>\n")
		}
	}

	if len(protectedTerms) > 0 {
		builder.WriteString("\n---\n[Protected Names And Terms]\n")
		builder.WriteString(fmt.Sprintf(
			"Treat these as protected names or technical terms only when the user instruction and user glossary do not already define a rendering. These entries must not override any explicit glossary mapping. If no explicit rendering rule is given, keep them in their established %s form and handle them consistently:\n",
			targetLabel,
		))
		for _, term := range protectedTerms {
			builder.WriteString("- ")
			builder.WriteString(term)
			builder.WriteString("\n")
		}
	}

	if chunkLabel := strings.TrimSpace(runtimeOptions.ChunkLabel); chunkLabel != "" {
		builder.WriteString("\n---\n[Current Section]\n")
		builder.WriteString(fmt.Sprintf("%s.\n", chunkLabel))
	}

	if contextSummary := strings.TrimSpace(runtimeOptions.ContextSummary); contextSummary != "" {
		builder.WriteString("\n---\n[Previous Context]\n")
		builder.WriteString("Context from the previous translated section:\n")
		builder.WriteString(contextSummary)
		builder.WriteString("\nUse this only to preserve continuity and terminology. If it conflicts with the user instruction, ignore the conflicting part.\n")
	}

	if openingSource := strings.TrimSpace(runtimeOptions.OpeningSourceParagraph); openingSource != "" {
		builder.WriteString("\n---\n[Opening Style Anchor]\n")
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
		builder.WriteString("\n---\n[Recent Source Overlap]\n")
		builder.WriteString("Recent source overlap for continuity:\n")
		builder.WriteString(overlapContext)
		builder.WriteString("\nUse this only as reference context. Do not translate or repeat this overlap again.\n")
	}

	builder.WriteString("\n---\n[Output Constraints]\n")
	builder.WriteString(fmt.Sprintf(
		"Produce only the %s translation, without any additional explanations or commentary. Please translate the following %s text into %s:\n\n",
		targetLabel,
		sourceLabel,
		targetLabel,
	))
	builder.WriteString("- Preserve the source paragraph structure and blank-line breaks whenever natural in the target language.\n")
	builder.WriteString("- Do not collapse multiple source paragraphs into one paragraph.\n")
	builder.WriteString("[End Of Instructions]\n")
	builder.WriteString("---\n[Source Text Begins]\n")
	builder.WriteString(sourceText)
	builder.WriteString("\n---\n[Source Text Ends]\n")

	return builder.String()
}

func buildInlineProofreadPrompt(settings ProviderSettings, sourceLang, targetLang, sourceText, instruction string, runtimeOptions translationRuntimeOptions) string {
	sourceLabel := normalizeLanguageLabel(sourceLang)
	targetLabel := normalizeLanguageLabel(targetLang)
	glossary := effectiveGlossary(settings, sourceText)
	protectedTerms := filterProtectedTermsByGlossary(extractProtectedTerms(sourceText), glossary)

	// Proofread After Translation 교정용 프롬프트 조립부
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(
		"You are a professional %s to %s translator.\n",
		sourceLabel,
		targetLabel,
	))
	builder.WriteString(fmt.Sprintf("Task: Translate the ENTIRE source text into %s. Do not omit any part.\n", targetLabel))
	builder.WriteString("Use this exact internal workflow: draft, brief review notes, final.\n")
	builder.WriteString("Output ONLY the exact format below. Do not add any other text or conversational filler.\n\n")

	// 구조적 명확성을 위해 중괄호 포맷 강조
	builder.WriteString("{draft: full draft translation}\n\n")
	builder.WriteString("{review: brief revision notes}\n\n")
	builder.WriteString("{final: final natural translation}\n\n")

	builder.WriteString("Hard requirements:\n")
	builder.WriteString("- The response must contain the COMPLETE translation of the input, not just the beginning.\n")
	builder.WriteString("- Do not summarize. Every paragraph in the source must have a corresponding paragraph in the output.\n")
	builder.WriteString("- The very first characters of your reply must be `{draft:`.\n")
	builder.WriteString("- The very last character of your reply must be `}`.\n")
	builder.WriteString("- Do not output introductions, explanations, labels like 'Step 1', or Markdown code fences (```).\n")
	builder.WriteString("- Keep the {draft:} faithful and literal.\n")
	// builder.WriteString("- Keep the {review:} concise and practical.\n")
	// 기존의 짧은 리뷰 지시를 아래 내용으로 교체
	builder.WriteString("- Formulate the {review:} strictly as a bulleted list. Do not write paragraphs.\n")
	builder.WriteString("- Each bullet must follow this exact format: `- Issue: [awkward/literal part from draft] -> Fix: [natural alternative for final]`.\n")
	builder.WriteString(fmt.Sprintf("- In {review:}, explicitly check whether the draft is fully and correctly rendered in %s and whether any leftover source-language or third-language text is unintended. Do not flag intentional bilingual notation, quoted originals, titles, or required retained forms.\n", targetLabel))
	builder.WriteString("- Focus ONLY on unnatural phrasing, structural adjustments, tone, and incorrect language carry-over. If no changes are needed, write `- Issue: None -> Fix: None`.\n")
	// 기존 프롬프트
	builder.WriteString(fmt.Sprintf("- Make the {final:} more natural in %s while preserving all meaning and paragraph breaks.\n", targetLabel))
	builder.WriteString("- The user style instruction is mandatory.\n")

	if trimmedInstruction := strings.TrimSpace(instruction); trimmedInstruction != "" {
		builder.WriteString("\n[User Style Instruction]\n")
		builder.WriteString(trimmedInstruction)
		builder.WriteString("\n")
	}

	if settings.EnableEnhancedContextTranslation {
		builder.WriteString("\n[Consistency Rules]\n")
		builder.WriteString(fmt.Sprintf(
			"Maintain consistent %s translations for names, places, organizations, products, commands, and technical terms. Reuse established renderings unless the source text, user instruction, or user glossary explicitly requires a different form.\n",
			targetLabel,
		))
		if glossary != "" {
			builder.WriteString("\nUser Glossary:\n<GLOSSARY>\n")
			builder.WriteString(glossary)
			builder.WriteString("\n</GLOSSARY>\n")
		}
	}

	if len(protectedTerms) > 0 {
		builder.WriteString("\n[Protected Names And Terms]\n")
		for _, term := range protectedTerms {
			builder.WriteString("- ")
			builder.WriteString(term)
			builder.WriteString("\n")
		}
	}

	if chunkLabel := strings.TrimSpace(runtimeOptions.ChunkLabel); chunkLabel != "" {
		builder.WriteString("\n[Current Section]\n")
		builder.WriteString(chunkLabel)
		builder.WriteString("\n")
	}

	if contextSummary := strings.TrimSpace(runtimeOptions.ContextSummary); contextSummary != "" {
		builder.WriteString("\n[Previous Context]\n")
		builder.WriteString(contextSummary)
		builder.WriteString("\nUse this only to preserve continuity and terminology. Do not repeat already translated content.\n")
	}

	if openingSource := strings.TrimSpace(runtimeOptions.OpeningSourceParagraph); openingSource != "" {
		builder.WriteString("\n[Opening Style Anchor]\n")
		builder.WriteString("Use the opening paragraph only as weak guidance for voice and register. Do not copy it.\n")
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
		builder.WriteString("\n[Recent Source Overlap]\n")
		builder.WriteString(overlapContext)
		builder.WriteString("\nUse this only as reference context. Do not repeat or retranslate this overlap.\n")
	}

	if settings.EnableTopicAwarePostEdit {
		if topicHints := buildTopicAwarePostEditHints(sourceText, "", instruction); topicHints != "" {
			builder.WriteString("\n[Likely Genre Or Topic Hint]\n")
			builder.WriteString(topicHints)
			builder.WriteString("Use these hints only to improve register and terminology consistency.\n")
		}
	}

	builder.WriteString("\n[Source Text]\n")
	builder.WriteString(sourceText)
	return builder.String()
}

// buildPostEditPrompt는 초벌 번역(draftTranslation)을 다듬는 포스트 에디팅용 프롬프트를 조립한다.
// 원문과 초벌 번역을 함께 넣고, 사용자 지침 및 문맥을 기준으로
// 의미 보존을 유지하면서 표현을 교정하도록 모델에 지시하는 최종 교정 프롬프트이다.
func buildPostEditPrompt(settings ProviderSettings, sourceLang, targetLang, sourceText, draftTranslation, instruction string, runtimeOptions translationRuntimeOptions) string {
	sourceLabel := normalizeLanguageLabel(sourceLang)
	targetLabel := normalizeLanguageLabel(targetLang)
	glossary := effectiveGlossary(settings, sourceText)
	protectedTerms := filterProtectedTermsByGlossary(extractProtectedTerms(sourceText), glossary)

	if strings.TrimSpace(settings.DebugPostEditPromptTemplate) != "" {
		prompt := applyPromptTemplate(normalizeDebugPromptTemplateInput(settings.DebugPostEditPromptTemplate), map[string]string{
			"SOURCE_LANG":                  sourceLabel,
			"TARGET_LANG":                  targetLabel,
			"INSTRUCTION":                  strings.TrimSpace(instruction),
			"SOURCE_TEXT":                  sourceText,
			"DRAFT_TRANSLATION":            draftTranslation,
			"PROTECTED_TERMS":              strings.Join(protectedTerms, "\n"),
			"GLOSSARY":                     glossary,
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
	builder.WriteString("[Role And Core Persona]\n")
	builder.WriteString(fmt.Sprintf(
		"You are a professional %s to %s translation post-editor. Review the draft against the source and produce a clean final %s translation.\n",
		sourceLabel,
		targetLabel,
		targetLabel,
	))

	builder.WriteString("\n---\n[Instruction Priority Rules]\n")
	builder.WriteString("- The user style instruction is mandatory for this chunk.\n")
	builder.WriteString("- If the draft conflicts with the user instruction, revise the draft to match the instruction.\n")
	builder.WriteString("- If previous context, overlap context, or opening style anchor conflicts with the user instruction, follow the user instruction.\n")

	if trimmedInstruction := strings.TrimSpace(instruction); trimmedInstruction != "" {
		builder.WriteString("\n---\n[Style Instruction]\n")
		builder.WriteString(trimmedInstruction)
		builder.WriteString("\n")
	}

	if chunkLabel := strings.TrimSpace(runtimeOptions.ChunkLabel); chunkLabel != "" {
		builder.WriteString("\n---\n[Current Section]\n")
		builder.WriteString(chunkLabel)
		builder.WriteString("\n")
	}

	if contextSummary := strings.TrimSpace(runtimeOptions.ContextSummary); contextSummary != "" {
		builder.WriteString("\n---\n[Previous Context]\n")
		builder.WriteString(contextSummary)
		builder.WriteString("\nUse this only to preserve continuity and terminology. Do not repeat already translated content.\n")
	}

	if openingSource := strings.TrimSpace(runtimeOptions.OpeningSourceParagraph); openingSource != "" {
		builder.WriteString("\n---\n[Opening Style Anchor]\n")
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
		builder.WriteString("\n---\n[Recent Overlap]\n")
		builder.WriteString(overlapContext)
		builder.WriteString("\nUse this only as reference context. Do not repeat or retranslate this overlap.\n")
	}

	builder.WriteString("\n---\n[Rules]\n")
	builder.WriteString("- Make the smallest edits that still fully resolve awkwardness, mistranslation, broken phrasing, and mixed-language artifacts.\n")
	builder.WriteString("- Preserve the source meaning strictly. Do not add, remove, generalize, or reinterpret facts, legal meanings, relationships, chronology, or emphasis.\n")
	builder.WriteString("- Do not replace a specific institution, qualification, admission, or legal action with a different meaning.\n")
	builder.WriteString(fmt.Sprintf(
		"- Do not revert already translated names, places, organizations, products, commands, or technical terms back into the source-language form unless the user instruction, user glossary, or source text explicitly requires the original %s spelling.\n",
		sourceLabel,
	))
	builder.WriteString(fmt.Sprintf(
		"- Fix clear errors aggressively when needed: malformed transliterations, mixed-language fragments, stray foreign-script insertions, leftover untranslated words, obvious mistranslations, or stiff literal phrasing that reads unnaturally in %s.\n",
		targetLabel,
	))
	builder.WriteString("- Preserve intentional bilingual notation only when it is clearly marked with parentheses, quotes, aliases, or original-title notation.\n")
	builder.WriteString("- User instruction compliance has priority over preserving the existing draft wording.\n")
	builder.WriteString("- Compare the source against the low-temperature draft and revise wherever a correction, fluency improvement, or more native phrasing is clearly justified.\n")
	builder.WriteString("- Preserve the source paragraph structure and blank-line breaks whenever natural in the target language.\n")
	builder.WriteString("- Do not collapse multiple source paragraphs into one paragraph.\n")
	builder.WriteString(fmt.Sprintf(
		"- If the draft is accurate but sounds translation-like, rewrite it into more natural %s phrasing while preserving the exact meaning.\n",
		targetLabel,
	))
	builder.WriteString("- If a sentence is already accurate and natural, keep it close to the draft; otherwise prefer a cleaner final sentence over minimal surface edits.\n")
	builder.WriteString("- Output only the final corrected translation.\n")

	if settings.EnableTopicAwarePostEdit {
		if topicHints := buildTopicAwarePostEditHints(sourceText, draftTranslation, instruction); topicHints != "" {
			builder.WriteString("\n---\n[Likely Genre Or Topic Hint]\n")
			builder.WriteString(topicHints)
			builder.WriteString("Use these hints only to improve register and terminology consistency. If they conflict with the source, ignore them.\n")
		}
	}

	if settings.EnableEnhancedContextTranslation {
		builder.WriteString(fmt.Sprintf(
			"\n---\n[Consistency Rule]\nMaintain consistent %s translations for names, places, organizations, products, commands, and technical terms. If a term is already correctly rendered in %s, do not switch it back to the original %s form unless the user instruction, glossary, or source text explicitly requires that original form. If the user instruction or glossary specifies a transliteration or %s-script rendering, follow that specification consistently. Explicit user glossary mappings override generic protected-term handling.\n",
			targetLabel,
			targetLabel,
			sourceLabel,
			targetLabel,
		))

		if glossary != "" {
			builder.WriteString("User Glossary:\n")
			builder.WriteString("<GLOSSARY>\n")
			builder.WriteString(glossary)
			builder.WriteString("\n</GLOSSARY>\n")
		}
	}

	if len(protectedTerms) > 0 {
		builder.WriteString("\n---\n[Protected Names And Terms]\n")
		builder.WriteString(fmt.Sprintf(
			"Follow the user instruction and user glossary first for how these should be rendered. These entries must not override any explicit glossary mapping. If no explicit rendering rule is given, keep them in their established %s form and handle them consistently:\n",
			targetLabel,
		))
		for _, term := range protectedTerms {
			builder.WriteString("- ")
			builder.WriteString(term)
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\n---\n[End Of Instructions]\n")
	builder.WriteString("---\n[Source Text Begins]\n")
	builder.WriteString(sourceText)
	builder.WriteString("\n---\n[Source Text Ends]\n")
	builder.WriteString("---\n[Translated Draft Begins]\n")
	builder.WriteString(draftTranslation)
	builder.WriteString("\n---\n[Translated Draft Ends]\n")

	return builder.String()
}

func parseInlineProofreadResponse(raw string) (inlineProofreadResult, error) {
	sections := extractInlineProofreadSections(raw)
	result := inlineProofreadResult{
		Draft:  strings.TrimSpace(sections.Draft),
		Review: strings.TrimSpace(sections.Review),
		Final:  strings.TrimSpace(sections.Final),
	}
	if result.Draft == "" || result.Final == "" {
		return inlineProofreadResult{}, fmt.Errorf("inline proofread response was missing draft or final text")
	}
	return result, nil
}

func extractInlineProofreadVisibleText(raw string) (string, string, bool) {
	sections := extractInlineProofreadSections(raw)
	return sections.Draft, sections.Final, sections.HasFinal
}

func extractInlineProofreadSections(raw string) inlineProofreadSections {
	trimmed := strings.TrimSpace(raw)
	if match := fencedCodeBlockPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		raw = match[1]
	}

	draftStart := strings.Index(raw, "{draft:")
	if draftStart < 0 {
		return inlineProofreadSections{}
	}
	draftBodyStart := draftStart + len("{draft:")
	reviewStart := findFirstIndex(raw, draftBodyStart, []string{"{review:"})
	finalMarkerStart := strings.Index(raw[draftBodyStart:], "{final:")
	if finalMarkerStart >= 0 {
		finalMarkerStart += draftBodyStart
	}
	draftEnd := len(raw)
	if reviewStart >= 0 && reviewStart < draftEnd {
		draftEnd = reviewStart
	}
	if finalMarkerStart >= 0 && finalMarkerStart < draftEnd {
		draftEnd = finalMarkerStart
	}

	sections := inlineProofreadSections{
		Draft:    trimInlineSectionBoundary(raw[draftBodyStart:draftEnd], false),
		HasDraft: true,
	}
	if reviewStart < 0 && finalMarkerStart < 0 {
		return sections
	}

	if reviewStart >= 0 {
		reviewBodyStart := reviewStart + len(detectedReviewNeedle(raw[reviewStart:]))
		reviewEnd := len(raw)
		if finalMarkerStart >= 0 && finalMarkerStart > reviewBodyStart {
			reviewEnd = finalMarkerStart
		}
		sections.Review = trimInlineSectionBoundary(raw[reviewBodyStart:reviewEnd], false)
		sections.HasReview = true
	}

	if finalMarkerStart < 0 {
		return sections
	}
	finalBodyStart := finalMarkerStart + len("{final:")
	finalEnd := len(raw)
	if strings.Contains(raw[finalBodyStart:], "}") {
		sections.FinalClosed = true
	}
	sections.Final = trimInlineSectionBoundary(raw[finalBodyStart:finalEnd], true)
	sections.HasFinal = true
	return sections
}

func trimInlineSectionBoundary(text string, allowTrailingText bool) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}

	trimmed = stripEarlyClosingBrace(trimmed)
	trimmed = strings.TrimSpace(trimmed)

	if strings.HasSuffix(trimmed, "}") {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "}"))
	}

	return strings.TrimSpace(trimmed)
}

func stripEarlyClosingBrace(text string) string {
	braceIndex := strings.Index(text, "}")
	if braceIndex < 0 {
		return text
	}

	newlineIndex := strings.Index(text, "\n")
	switch {
	case newlineIndex >= 0 && braceIndex < newlineIndex:
		return strings.TrimSpace(text[:braceIndex] + text[braceIndex+1:])
	case newlineIndex < 0 && braceIndex <= 80:
		return strings.TrimSpace(text[:braceIndex] + text[braceIndex+1:])
	default:
		return text
	}
}

func detectedReviewNeedle(raw string) string {
	for _, needle := range []string{"{review:"} {
		if strings.HasPrefix(raw, needle) {
			return needle
		}
	}
	return ""
}

func findFirstIndex(raw string, start int, needles []string) int {
	first := -1
	for _, needle := range needles {
		index := strings.Index(raw[start:], needle)
		if index < 0 {
			continue
		}
		index += start
		if first < 0 || index < first {
			first = index
		}
	}
	return first
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

type glossaryEntry struct {
	Source string
	Target string
}

func parseGlossaryEntries(glossary string) []glossaryEntry {
	lines := strings.Split(glossary, "\n")
	entries := make([]glossaryEntry, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		separator := strings.Index(trimmed, "=")
		if separator < 0 {
			continue
		}
		source := strings.TrimSpace(trimmed[:separator])
		target := strings.TrimSpace(trimmed[separator+1:])
		if source == "" || target == "" {
			continue
		}
		entries = append(entries, glossaryEntry{Source: source, Target: target})
	}
	return entries
}

func expandGlossaryEntries(entries []glossaryEntry, protectedTerms []string) []glossaryEntry {
	if len(entries) == 0 || len(protectedTerms) == 0 {
		return entries
	}

	seen := make(map[string]struct{}, len(entries))
	expanded := make([]glossaryEntry, 0, len(entries)+8)
	for _, entry := range entries {
		key := strings.ToLower(strings.TrimSpace(entry.Source))
		seen[key] = struct{}{}
		expanded = append(expanded, entry)
	}

	for _, term := range protectedTerms {
		trimmedTerm := strings.TrimSpace(term)
		lowerTerm := strings.ToLower(trimmedTerm)
		if trimmedTerm == "" {
			continue
		}
		if _, exists := seen[lowerTerm]; exists {
			continue
		}

		for _, entry := range entries {
			source := strings.TrimSpace(entry.Source)
			target := strings.TrimSpace(entry.Target)
			if source == "" || target == "" {
				continue
			}

			lowerSource := strings.ToLower(source)
			if !strings.HasPrefix(lowerTerm, lowerSource) || len(trimmedTerm) <= len(source) {
				continue
			}

			suffix := strings.TrimSpace(trimmedTerm[len(source):])
			if suffix == "" {
				continue
			}
			if !strings.HasPrefix(trimmedTerm[len(source):], " ") && !strings.HasPrefix(trimmedTerm[len(source):], "-") {
				continue
			}

			derived := glossaryEntry{
				Source: trimmedTerm,
				Target: target + trimmedTerm[len(source):],
			}
			expanded = append(expanded, derived)
			seen[lowerTerm] = struct{}{}
			break
		}
	}

	return expanded
}

func formatGlossaryEntries(entries []glossaryEntry) string {
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		source := strings.TrimSpace(entry.Source)
		target := strings.TrimSpace(entry.Target)
		if source == "" || target == "" {
			continue
		}
		lines = append(lines, source+" = "+target)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func effectiveGlossary(settings ProviderSettings, sourceText string) string {
	if !settings.EnableEnhancedContextTranslation {
		return ""
	}
	baseGlossary := strings.TrimSpace(settings.EnhancedContextGlossary)
	if baseGlossary == "" {
		return ""
	}
	entries := parseGlossaryEntries(baseGlossary)
	entries = expandGlossaryEntries(entries, extractProtectedTerms(sourceText))
	entries = filterGlossaryEntriesBySourceText(entries, sourceText)
	return formatGlossaryEntries(entries)
}

func filterGlossaryEntriesBySourceText(entries []glossaryEntry, sourceText string) []glossaryEntry {
	if len(entries) == 0 {
		return nil
	}
	normalizedSource := strings.ToLower(" " + normalizeGlossaryMatchText(sourceText) + " ")
	if normalizedSource == "" {
		return nil
	}

	filtered := make([]glossaryEntry, 0, len(entries))
	for _, entry := range entries {
		source := strings.TrimSpace(entry.Source)
		if source == "" {
			continue
		}
		needle := " " + normalizeGlossaryMatchText(source) + " "
		if needle != "  " && strings.Contains(normalizedSource, needle) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func normalizeGlossaryMatchText(text string) string {
	lowered := strings.ToLower(strings.TrimSpace(text))
	var builder strings.Builder
	lastSpace := false
	for _, r := range lowered {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if r == '\'' {
			builder.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			builder.WriteRune(' ')
			lastSpace = true
		}
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func glossarySourceTermSet(glossary string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, line := range strings.Split(glossary, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		separator := strings.Index(trimmed, "=")
		if separator < 0 {
			continue
		}
		source := strings.ToLower(strings.TrimSpace(trimmed[:separator]))
		if source == "" {
			continue
		}
		result[source] = struct{}{}
	}
	return result
}

func filterProtectedTermsByGlossary(protectedTerms []string, glossary string) []string {
	if len(protectedTerms) == 0 {
		return nil
	}
	glossaryTerms := glossarySourceTermSet(glossary)
	if len(glossaryTerms) == 0 {
		return protectedTerms
	}

	filtered := make([]string, 0, len(protectedTerms))
	for _, term := range protectedTerms {
		if _, exists := glossaryTerms[strings.ToLower(strings.TrimSpace(term))]; exists {
			continue
		}
		filtered = append(filtered, term)
	}
	return filtered
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
	normalized := normalizeEscapedLineBreaks(text)
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		lines[i] = cleanInlineNoise(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func normalizeEscapedLineBreaks(text string) string {
	replacer := strings.NewReplacer(
		`\r\n`, "\n",
		`\n`, "\n",
		`\r`, "\n",
		`\t`, "\t",
	)
	return replacer.Replace(text)
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

func trailingParagraphs(text string, maxChars int, maxParagraphs int) string {
	paragraphs := splitParagraphs(strings.TrimSpace(text))
	if len(paragraphs) == 0 {
		return trailingRunes(text, maxChars)
	}

	if maxParagraphs <= 0 {
		maxParagraphs = 1
	}

	collected := make([]string, 0, maxParagraphs)
	total := 0
	for i := len(paragraphs) - 1; i >= 0; i-- {
		paragraphLen := len([]rune(paragraphs[i]))
		separatorLen := 0
		if total > 0 {
			separatorLen = 2
		}
		if total > 0 && total+separatorLen+paragraphLen > maxChars {
			break
		}
		collected = append([]string{paragraphs[i]}, collected...)
		total += separatorLen + paragraphLen
		if len(collected) >= maxParagraphs {
			break
		}
	}
	return strings.Join(collected, "\n\n")
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
	sourceTail := trailingParagraphs(sourceChunk, 320, 2)
	translatedTail := trailingParagraphs(translatedChunk, 320, 2)
	parts := make([]string, 0, 5)
	if styleMemory := buildStyleMemorySummary(settings, instruction, sourceChunk, translatedChunk); styleMemory != "" {
		parts = append(parts, styleMemory)
	}
	if sourceTail != "" {
		parts = append(parts, "Source tail:\n"+sourceTail)
	}
	if translatedTail != "" {
		parts = append(parts, "Translated tail:\n"+translatedTail)
	}
	return strings.Join(parts, "\n")
}

func buildStyleMemorySummary(settings ProviderSettings, instruction, sourceChunk, translatedChunk string) string {
	lines := make([]string, 0, 6)
	lines = append(lines, "Carry-forward style memory:")
	lines = append(lines, "- User instruction remains mandatory for all remaining chunks.")
	if trimmedInstruction := strings.TrimSpace(instruction); trimmedInstruction != "" {
		lines = append(lines, "- Active user style: "+singleLinePreview(trimmedInstruction, 220))
	}
	if tone := detectPostEditTone(translatedChunk, instruction); tone != "" {
		lines = append(lines, "- Established tone/register so far: "+tone)
	}
	if settings.EnableEnhancedContextTranslation {
		if lockedTerms := glossaryMemoryLines(effectiveGlossary(settings, sourceChunk), 4); len(lockedTerms) > 0 {
			lines = append(lines, "- Locked terminology from glossary:")
			for _, term := range lockedTerms {
				lines = append(lines, "  "+term)
			}
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

func resolveInlineProofreadTemperature(settings ProviderSettings) *float64 {
	return resolvePostEditTemperature(settings)
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

func useInlineProofread(settings ProviderSettings) bool {
	return settings.EnablePostEdit
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

func (c *Client) emitChunk(payload TranslationChunkPayload) {
	if sink := c.currentSink(); sink != nil {
		sink.Chunk(payload)
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

func (s runtimeEventSink) Chunk(payload TranslationChunkPayload) {
	runtime.EventsEmit(s.ctx, "translation:chunk", payload)
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
