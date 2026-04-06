// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

import React, { useState, useEffect, useRef } from 'react';
import './App.css';
import brandLogo from './assets/images/dskt-logo.png';
import {
    CancelTranslation,
    ConfirmClearSource,
    GetHostProviderSettings,
    GetWebServerSettings,
    GetWindowMode,
    GetModels,
    OpenCertificateFolder,
    OpenDebugStudioWindow,
    Translate,
    SaveWebServerSettings,
    OpenFile,
    ReadDebugStudioState,
    SaveFile,
    SaveHostProviderSettings,
    WriteDebugStudioState
} from "../wailsjs/go/app/App";
import { llm } from "../wailsjs/go/models";
import { ClipboardGetText, ClipboardSetText, Environment, EventsOn, EventsOff } from "../wailsjs/runtime/runtime";

type DebugPayload = {
    direction?: string;
    endpoint?: string;
    payload?: string;
};

type TranslationCompletePayload = {
    text?: string;
};

type ProgressPayload = {
    stage?: string;
    label?: string;
    detail?: string;
    progress?: number;
    overall_progress?: number;
    current_chunk?: number;
    completed_chunks?: number;
    total_chunks?: number;
    current_step?: number;
    total_steps?: number;
    visible?: boolean;
    indeterminate?: boolean;
};

type TranslationStatsPayload = {
    input_tokens?: number;
    reasoning_output_tokens?: number;
    time_to_first_token_seconds?: number;
    tokens_per_second?: number;
    total_output_tokens?: number;
};

type ProviderMode = "lmstudio" | "openai";

type ProviderSettings = {
    mode: ProviderMode;
    endpoint: string;
    apiKey: string;
    model: string;
    reasoning: string;
    forceShowReasoning: boolean;
    temperature: number;
    forceShowTemperature: boolean;
    enablePostEdit: boolean;
    enableTopicAwarePostEdit: boolean;
    enableEnhancedContextTranslation: boolean;
    enhancedContextGlossary: string;
    enableSmartChunking: boolean;
    smartChunkSize: number;
    debugTranslationPromptTemplate?: string;
    debugPostEditPromptTemplate?: string;
};

type ModelInfo = {
    id: string;
    displayName?: string;
    supportsReasoning?: boolean;
    reasoningOptions?: string[];
};

type DebugStudioSnapshot = {
    showDebugPanel: boolean;
    debugRequest: string;
    debugResponse: string;
    debugTranslationPromptTemplate: string;
    debugPostEditPromptTemplate: string;
    lastTranslationPromptPreview: string;
    lastPostEditPromptPreview: string;
    lastTopicAwareHintsPreview: string;
};

type WebServerSettings = {
    enabled: boolean;
    port: string;
    useTls: boolean;
    certDomain: string;
    certPath?: string;
    keyPath?: string;
    certificateDirectory: string;
    configDirectory: string;
    hasPassword: boolean;
    url?: string;
};

type WebServerSettingsInput = {
    enabled: boolean;
    port: string;
    password?: string;
    useTls: boolean;
    certDomain: string;
    certPath?: string;
    keyPath?: string;
};

type WebTranslateResponse = {
    text?: string;
    stats?: TranslationStatsPayload;
};

type PromptSelectionModalState =
    | { type: "preset" }
    | { type: "reasoning" }
    | null;

const STORAGE_KEY = "dkst-translator-ai-settings";
const SOURCE_LANGUAGES = ["auto", "English", "Korean", "Japanese", "Chinese", "French", "German"];
const TARGET_LANGUAGES = ["Korean", "English", "Japanese", "Chinese", "French", "German"];
const DEFAULT_REASONING_OPTIONS = ["off", "low", "medium", "high", "on"];
const DEFAULT_EDITOR_FONT_SIZE = 18;
const MIN_EDITOR_FONT_SIZE = 14;
const MAX_EDITOR_FONT_SIZE = 26;
const DEFAULT_REASONING = "off";
const DEFAULT_TEMPERATURE = 0.4;
const MIN_TEMPERATURE = 0;
const MAX_TEMPERATURE = 1;
const TEMPERATURE_STEP = 0.1;
const DEFAULT_WEB_SERVER_SETTINGS: WebServerSettings = {
    enabled: false,
    port: "8080",
    useTls: false,
    certDomain: "localhost",
    certPath: "",
    keyPath: "",
    certificateDirectory: "",
    configDirectory: "",
    hasPassword: false,
    url: "",
};
const isBrowserMode = typeof window !== "undefined" && !(window as any)?.go?.app?.App;

// 프리셋 지침
const INSTRUCTION_PRESETS = [
    {
        id: "natural",
        label: "Natural",
        instruction: "Prioritize natural phrasing and idiomatic accuracy in the target language. Do not translate word-for-word; instead, rephrase the sentences to ensure they sound like they were originally written by a native speaker of the target language while preserving the original intent.",
    },
    {
        id: "precision",
        label: "Precision & Consistency",
        instruction: "Focus on technical accuracy and stylistic consistency. Maintain the formal tone of the source text and ensure that specialized terms are translated consistently throughout. If a term is a globally recognized industry standard, prioritize the most commonly accepted technical equivalent in the target language.",
    },
    {
        id: "concise",
        label: "Concise & Clear",
        instruction: "Deliver the core message with maximum clarity and conciseness. Remove redundant fillers or repetitive structures inherent in the source language that may clutter the target language output. Focus on a clean, direct, and easy-to-read translation.",
    },
    {
        id: "news",
        label: "News & Journalism",
        instruction: "Adopt a journalistic tone suitable for news reporting. Focus on objectivity, clarity, and the inverted pyramid structure. Use standard media terminology and ensure a professional, authoritative voice.",
    },
    {
        id: "it",
        label: "IT & Technology",
        instruction: "Translate with a focus on information technology and software development. Use industry-standard technical terms (e.g., UI/UX, API, cloud computing) accurately. Maintain a modern, tech-oriented style that is clear for both experts and users.",
    },
    {
        id: "medical",
        label: "Medical & Healthcare",
        instruction: "Prioritize high precision in medical and clinical terminology. Ensure accuracy in anatomical terms, drug names, and diagnostic descriptions. Maintain a professional, formal tone appropriate for healthcare professionals.",
    },
    {
        id: "novel",
        label: "Literary & Novel",
        instruction: "Focus on creative and descriptive language suitable for literature. Capture the mood, tone, and artistic nuances. Prioritize evocative phrasing and stylistic richness while maintaining narrative flow and character voice.",
    },
    {
        id: "fairytale",
        label: "Fairy Tale",
        instruction: "Use whimsical and storytelling language appropriate for fairy tales. Employ classic narrative conventions and maintain a tone that is enchanting and accessible for a broad audience.",
    },
    {
        id: "children",
        label: "Children's Content",
        instruction: "Use simple, clear, and age-appropriate language. Avoid complex structures or overly sophisticated vocabulary. Maintain a friendly, engaging, and easy-to-understand tone for children.",
    },
    {
        id: "formal",
        label: "Formal & Official",
        instruction: "Maintain a highly formal, respectful, and official tone. Suitable for legal documents, official correspondence, or academic papers. Adhere to standard professional writing conventions and use appropriate honorifics.",
    },
    {
        id: "classical",
        label: "Classical Literature",
        instruction: "Adopt an archaic or classical literary style. Use sophisticated vocabulary and formal sentence structures to capture the historical and dignified essence of classical texts while remaining legible.",
    },
] as const;
const DEBUG_TRANSLATION_PROMPT_TEMPLATE = `You are a professional {{SOURCE_LANG}} to {{TARGET_LANG}} translator.

Style instruction:
{{INSTRUCTION}}

Protected names and terms:
{{PROTECTED_TERMS}}

User glossary:
{{GLOSSARY}}

Chunk label:
{{CHUNK_LABEL}}

Previous context:
{{CONTEXT_SUMMARY}}

Opening source paragraph:
{{OPENING_SOURCE_PARAGRAPH}}

Opening translated paragraph:
{{OPENING_TRANSLATED_PARAGRAPH}}

Recent overlap:
{{OVERLAP_CONTEXT}}

Produce only the {{TARGET_LANG}} translation.

Source text:
{{SOURCE_TEXT}}`;

const DEBUG_POST_EDIT_PROMPT_TEMPLATE = `You are a professional {{SOURCE_LANG}} to {{TARGET_LANG}} translation post-editor.

Style instruction:
{{INSTRUCTION}}

Protected names and terms:
{{PROTECTED_TERMS}}

User glossary:
{{GLOSSARY}}

Chunk label:
{{CHUNK_LABEL}}

Previous context:
{{CONTEXT_SUMMARY}}

Opening source paragraph:
{{OPENING_SOURCE_PARAGRAPH}}

Opening translated paragraph:
{{OPENING_TRANSLATED_PARAGRAPH}}

Recent overlap:
{{OVERLAP_CONTEXT}}

Topic-aware smart post-editing hint:
{{TOPIC_AWARE_HINTS}}

Rules:
- Fix awkward wording, broken transliterations, malformed loanwords, and obvious mistranslations.
- Remove mixed-language fragments, stray foreign-script insertions, and leftover untranslated words when they break the sentence.
- Preserve intentional bilingual notation only when clearly marked.
- Output only the final corrected translation.

Source text:
{{SOURCE_TEXT}}

Translated draft:
{{DRAFT_TRANSLATION}}`;

// 프리셋 지침 끝-

function normalizeInstructionPresetValue(value: string): string {
    return value.trim().replace(/\r\n/g, "\n");
}

function findMatchingInstructionPreset(instruction: string) {
    const normalized = normalizeInstructionPresetValue(instruction);
    return INSTRUCTION_PRESETS.find(preset => normalizeInstructionPresetValue(preset.instruction) === normalized) || null;
}

function loadStoredSettings() {
    try {
        const raw = window.localStorage.getItem(STORAGE_KEY);
        if (!raw) {
            return null;
        }
        return JSON.parse(raw);
    } catch {
        return null;
    }
}

async function callBrowserJSON<T>(path: string, init?: RequestInit): Promise<T> {
    const response = await fetch(path, {
        ...init,
        headers: {
            "Content-Type": "application/json",
            ...(init?.headers || {}),
        },
    });
    if (!response.ok) {
        throw new Error((await response.text()) || `Request failed (${response.status})`);
    }
    if (response.status === 204) {
        return undefined as T;
    }
    return await response.json() as T;
}

async function streamBrowserSSE(
    path: string,
    init: RequestInit,
    handlers: {
        onEvent: (event: string, payload: any) => void,
    }
) {
    const response = await fetch(path, init);
    if (!response.ok) {
        throw new Error((await response.text()) || `Request failed (${response.status})`);
    }
    if (!response.body) {
        throw new Error("Streaming response body is unavailable.");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    let currentEvent = "";
    let currentData: string[] = [];

    const flushEvent = () => {
        if (!currentEvent && currentData.length === 0) {
            return;
        }
        const raw = currentData.join("\n").trim();
        const eventName = currentEvent || "message";
        currentEvent = "";
        currentData = [];
        if (!raw) {
            return;
        }
        let payload: any = raw;
        try {
            payload = JSON.parse(raw);
        } catch {
            payload = raw;
        }
        handlers.onEvent(eventName, payload);
    };

    while (true) {
        const { value, done } = await reader.read();
        buffer += decoder.decode(value || new Uint8Array(), { stream: !done });

        let boundary = buffer.indexOf("\n");
        while (boundary !== -1) {
            const line = buffer.slice(0, boundary).replace(/\r$/, "");
            buffer = buffer.slice(boundary + 1);

            if (line.startsWith("event:")) {
                currentEvent = line.slice(6).trim();
            } else if (line.startsWith("data:")) {
                currentData.push(line.slice(5).trim());
            } else if (line.trim() === "") {
                flushEvent();
            }

            boundary = buffer.indexOf("\n");
        }

        if (done) {
            flushEvent();
            break;
        }
    }
}

function formatElapsed(seconds: number): string {
    const minutes = Math.floor(seconds / 60);
    const remainder = seconds % 60;
    return `${minutes}:${String(remainder).padStart(2, "0")}`;
}

function clampFontSize(size: number): number {
    return Math.max(MIN_EDITOR_FONT_SIZE, Math.min(MAX_EDITOR_FONT_SIZE, size));
}

function clampTemperature(value: number): number {
    if (!Number.isFinite(value)) {
        return 0;
    }
    return Math.max(MIN_TEMPERATURE, Math.min(MAX_TEMPERATURE, Math.round(value * 10) / 10));
}

function escapeRegExp(value: string): string {
    return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function formatTemperatureLabel(value: number): string {
    if (value <= 0) {
        return "Auto";
    }
    return value.toFixed(1);
}

function getTextStats(text: string): string {
    const lines = text
        ? text.split(/\r?\n/).filter(line => line.trim() !== "").length
        : 0;
    const chars = text.length;
    return `${lines} lines · ${chars} chars`;
}

function formatCompletionStats(stats?: TranslationStatsPayload | null): string {
    if (!stats || !stats.input_tokens || !stats.total_output_tokens) {
        return "Translation completed.";
    }

    const ttft = typeof stats.time_to_first_token_seconds === "number"
        ? stats.time_to_first_token_seconds.toFixed(2)
        : "0.00";
    const tps = typeof stats.tokens_per_second === "number"
        ? stats.tokens_per_second.toFixed(2)
        : "0.00";

    return `Translation completed. Input: ${stats.input_tokens}     TTFT: ${ttft}s     ${tps} tok/sec     Output: ${stats.total_output_tokens}`;
}

function deriveOverallProgress(step?: number, total?: number, isDone?: boolean): number | null {
    if (typeof step !== "number" || typeof total !== "number" || total <= 0) {
        return null;
    }
    const completedStep = isDone ? step : Math.max(0, step - 1);
    return Math.max(0, Math.min(1, completedStep / total));
}

function sanitizeTranslation(raw: string): string {
    return raw
        .replace(/^\s*<<<\s*output\s*>>>\s*/i, "")
        .replace(/<<<\s*\/?output\s*>>>/gi, "")
        .replace(/<<<[^>\n]+>>>/g, "")
        .trim();
}

function renderInlineMarkdown(text: string): React.ReactNode[] {
    const nodes: React.ReactNode[] = [];
    const pattern = /(\*\*[^*]+\*\*|`[^`]+`|\*[^*]+\*)/g;
    let lastIndex = 0;
    let match: RegExpExecArray | null;

    while ((match = pattern.exec(text)) !== null) {
        if (match.index > lastIndex) {
            nodes.push(text.slice(lastIndex, match.index));
        }

        const token = match[0];
        if (token.startsWith("**") && token.endsWith("**")) {
            nodes.push(<strong key={`${match.index}-strong`}>{token.slice(2, -2)}</strong>);
        } else if (token.startsWith("*") && token.endsWith("*")) {
            nodes.push(<em key={`${match.index}-em`}>{token.slice(1, -1)}</em>);
        } else if (token.startsWith("`") && token.endsWith("`")) {
            nodes.push(<code key={`${match.index}-code`}>{token.slice(1, -1)}</code>);
        } else {
            nodes.push(token);
        }

        lastIndex = match.index + token.length;
    }

    if (lastIndex < text.length) {
        nodes.push(text.slice(lastIndex));
    }

    return nodes;
}

function renderInlineMarkdownLines(lines: string[]): React.ReactNode[] {
    const nodes: React.ReactNode[] = [];

    lines.forEach((line, index) => {
        if (index > 0) {
            nodes.push(<br key={`br-${index}`} />);
        }
        nodes.push(...renderInlineMarkdown(line));
    });

    return nodes;
}

function renderMarkdown(text: string): React.ReactNode {
    const normalized = sanitizeTranslation(text).replace(/\r\n/g, "\n");
    if (!normalized) {
        return null;
    }

    const lines = normalized.split("\n");
    const elements: React.ReactNode[] = [];
    let i = 0;

    while (i < lines.length) {
        const line = lines[i];
        const trimmed = line.trim();

        if (!trimmed) {
            i += 1;
            continue;
        }

        if (trimmed.startsWith("```")) {
            const codeLines: string[] = [];
            i += 1;
            while (i < lines.length && !lines[i].trim().startsWith("```")) {
                codeLines.push(lines[i]);
                i += 1;
            }
            if (i < lines.length) {
                i += 1;
            }
            elements.push(
                <pre key={`code-${i}`} className="translation-code">
                    <code>{codeLines.join("\n")}</code>
                </pre>
            );
            continue;
        }

        if (/^#{1,6}\s/.test(trimmed)) {
            const level = Math.min(trimmed.match(/^#+/)?.[0].length || 1, 6);
            const content = trimmed.replace(/^#{1,6}\s+/, "");
            const Tag = `h${level}` as keyof JSX.IntrinsicElements;
            elements.push(<Tag key={`heading-${i}`}>{renderInlineMarkdown(content)}</Tag>);
            i += 1;
            continue;
        }

        if (/^[-*]\s+/.test(trimmed)) {
            const items: React.ReactNode[] = [];
            while (i < lines.length && /^[-*]\s+/.test(lines[i].trim())) {
                items.push(<li key={`ul-${i}`}>{renderInlineMarkdown(lines[i].trim().replace(/^[-*]\s+/, ""))}</li>);
                i += 1;
            }
            elements.push(<ul key={`list-${i}`}>{items}</ul>);
            continue;
        }

        if (/^\d+\.\s+/.test(trimmed)) {
            const items: React.ReactNode[] = [];
            while (i < lines.length && /^\d+\.\s+/.test(lines[i].trim())) {
                items.push(<li key={`ol-${i}`}>{renderInlineMarkdown(lines[i].trim().replace(/^\d+\.\s+/, ""))}</li>);
                i += 1;
            }
            elements.push(<ol key={`olist-${i}`}>{items}</ol>);
            continue;
        }

        if (/^>\s+/.test(trimmed)) {
            const quoteLines: string[] = [];
            while (i < lines.length && /^>\s+/.test(lines[i].trim())) {
                quoteLines.push(lines[i].trim().replace(/^>\s+/, ""));
                i += 1;
            }
            elements.push(<blockquote key={`quote-${i}`}>{renderInlineMarkdownLines(quoteLines)}</blockquote>);
            continue;
        }

        const paragraphLines: string[] = [trimmed];
        i += 1;
        while (i < lines.length) {
            const next = lines[i].trim();
            if (!next || /^(```|#{1,6}\s|[-*]\s+|\d+\.\s+|>\s+)/.test(next)) {
                break;
            }
            paragraphLines.push(next);
            i += 1;
        }
        elements.push(<p key={`p-${i}`}>{renderInlineMarkdownLines(paragraphLines)}</p>);
    }

    return elements;
}

function persistSettings(
    selectedModel: string,
    providerSettings: ProviderSettings,
    editorFontSize: number,
    sourceLang: string,
    targetLang: string,
    showDebugPanel: boolean,
    instruction?: string
) {
    const nextSettings: Record<string, unknown> = {
        selectedModel,
        providerMode: providerSettings.mode,
        endpoint: providerSettings.endpoint,
        apiKey: providerSettings.apiKey,
        reasoning: providerSettings.reasoning,
        forceShowReasoning: providerSettings.forceShowReasoning,
        temperature: providerSettings.temperature,
        forceShowTemperature: providerSettings.forceShowTemperature,
        enablePostEdit: providerSettings.enablePostEdit,
        enableTopicAwarePostEdit: providerSettings.enableTopicAwarePostEdit,
        enableEnhancedContextTranslation: providerSettings.enableEnhancedContextTranslation,
        enhancedContextGlossary: providerSettings.enhancedContextGlossary,
        enableSmartChunking: providerSettings.enableSmartChunking,
        smartChunkSize: providerSettings.smartChunkSize,
        editorFontSize,
        sourceLang,
        targetLang,
        showDebugPanel,
    };

    if (typeof instruction === "string") {
        nextSettings.instruction = instruction;
    }

    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(nextSettings));
}

function DebugStudioWindow(props: {
    showDebugPanel: boolean;
    debugRequest: string;
    debugResponse: string;
    debugTranslationPromptTemplate: string;
    debugPostEditPromptTemplate: string;
    lastTranslationPromptPreview: string;
    lastPostEditPromptPreview: string;
    lastTopicAwareHintsPreview: string;
    setDebugTranslationPromptTemplate: React.Dispatch<React.SetStateAction<string>>;
    setDebugPostEditPromptTemplate: React.Dispatch<React.SetStateAction<string>>;
    setDebugRequest: React.Dispatch<React.SetStateAction<string>>;
    setDebugResponse: React.Dispatch<React.SetStateAction<string>>;
}) {
    const {
        showDebugPanel,
        debugRequest,
        debugResponse,
        debugTranslationPromptTemplate,
        debugPostEditPromptTemplate,
        lastTranslationPromptPreview,
        lastPostEditPromptPreview,
        lastTopicAwareHintsPreview,
        setDebugTranslationPromptTemplate,
        setDebugPostEditPromptTemplate,
        setDebugRequest,
        setDebugResponse,
    } = props;
    const [translationPromptDraft, setTranslationPromptDraft] = useState(debugTranslationPromptTemplate);
    const [postEditPromptDraft, setPostEditPromptDraft] = useState(debugPostEditPromptTemplate);
    const [applyToast, setApplyToast] = useState("");

    useEffect(() => {
        setTranslationPromptDraft(debugTranslationPromptTemplate);
    }, [debugTranslationPromptTemplate]);

    useEffect(() => {
        setPostEditPromptDraft(debugPostEditPromptTemplate);
    }, [debugPostEditPromptTemplate]);

    useEffect(() => {
        if (!applyToast) {
            return;
        }
        const timer = window.setTimeout(() => setApplyToast(""), 1200);
        return () => window.clearTimeout(timer);
    }, [applyToast]);

    return (
        <div className="debug-window-shell">
            <div className="debug-window-header">
                <div>
                    <div className="debug-window-title">Debug Studio</div>
                    <div className="debug-window-subtitle">Tune the fully rendered prompts directly and review raw logs in a separate window.</div>
                </div>
                <div className="debug-studio-actions">
                    <button className="btn btn-secondary btn-small" onClick={() => {
                        setDebugRequest("");
                        setDebugResponse("");
                    }}>
                        Clear Logs
                    </button>
                    <button className="btn btn-secondary btn-small" onClick={() => {
                        setDebugTranslationPromptTemplate(DEBUG_TRANSLATION_PROMPT_TEMPLATE);
                        setDebugPostEditPromptTemplate(DEBUG_POST_EDIT_PROMPT_TEMPLATE);
                    }}>
                        Reset Overrides
                    </button>
                </div>
            </div>
            {!showDebugPanel && (
                <div className="debug-window-note">
                    The main window debug panel is hidden, but prompt captures and request logs are still collected here.
                </div>
            )}
            <div className="debug-studio-grid debug-studio-grid-window">
                <div className="debug-card debug-card-translation-prompt">
                    <div className="debug-title">Last Translation Prompt</div>
                    <pre className="debug-pre-xxl debug-preview-pre">{lastTranslationPromptPreview || "No translation prompt rendered yet."}</pre>
                </div>
                <div className="debug-card debug-card-postedit-prompt">
                    <div className="debug-title">Last Post-Edit Prompt</div>
                    <pre className="debug-pre-xxl debug-preview-pre">{lastPostEditPromptPreview || "No post-edit prompt rendered yet."}</pre>
                </div>
                <div className="debug-card debug-card-full debug-card-topic-hints">
                    <div className="debug-title">Last Topic-Aware Hints</div>
                    <pre className="debug-pre-large">{lastTopicAwareHintsPreview || "Topic-aware smart post-editing is off or no hints were generated yet."}</pre>
                </div>
                <div className="debug-card debug-card-translation-override">
                    <div className="debug-card-header">
                        <div className="debug-title">Translation Prompt Override</div>
                        <div className="debug-card-actions">
                            <button
                                className="btn btn-secondary btn-small"
                                type="button"
                                onClick={() => setTranslationPromptDraft(lastTranslationPromptPreview || DEBUG_TRANSLATION_PROMPT_TEMPLATE)}
                            >
                                Load Current Prompt
                            </button>
                            <button
                                className="icon-btn"
                                type="button"
                                onClick={() => {
                                    setDebugTranslationPromptTemplate(translationPromptDraft);
                                    setApplyToast("Translation prompt override applied");
                                }}
                                title="Apply Translation Prompt Override"
                            >
                                <span className="material-symbols-outlined">save</span>
                            </button>
                        </div>
                    </div>
                    <textarea
                        className="settings-textarea debug-prompt-textarea debug-prompt-textarea-wide"
                        value={translationPromptDraft}
                        onChange={e => setTranslationPromptDraft(e.target.value)}
                        placeholder="Load the current rendered prompt, tweak it, and the edited text will be used as-is for debug translations."
                    />
                </div>
                <div className="debug-card debug-card-postedit-override">
                    <div className="debug-card-header">
                        <div className="debug-title">Post-Edit Prompt Override</div>
                        <div className="debug-card-actions">
                            <button
                                className="btn btn-secondary btn-small"
                                type="button"
                                onClick={() => setPostEditPromptDraft(lastPostEditPromptPreview || DEBUG_POST_EDIT_PROMPT_TEMPLATE)}
                            >
                                Load Current Prompt
                            </button>
                            <button
                                className="icon-btn"
                                type="button"
                                onClick={() => {
                                    setDebugPostEditPromptTemplate(postEditPromptDraft);
                                    setApplyToast("Post-edit prompt override applied");
                                }}
                                title="Apply Post-Edit Prompt Override"
                            >
                                <span className="material-symbols-outlined">save</span>
                            </button>
                        </div>
                    </div>
                    <textarea
                        className="settings-textarea debug-prompt-textarea debug-prompt-textarea-wide"
                        value={postEditPromptDraft}
                        onChange={e => setPostEditPromptDraft(e.target.value)}
                        placeholder="Load the current rendered post-edit prompt, fine-tune its wording, and apply it only while debugging."
                    />
                </div>
                <div className="debug-card debug-card-request-log">
                    <div className="debug-title">Request Log</div>
                    <pre className="debug-pre-xl">{debugRequest || "No request captured yet."}</pre>
                </div>
                <div className="debug-card debug-card-response-log">
                    <div className="debug-title">Response Log</div>
                    <pre className="debug-pre-xl">{debugResponse || "No response captured yet."}</pre>
                </div>
            </div>
            {applyToast && <div className="debug-window-toast">{applyToast}</div>}
        </div>
    );
}

function App() {
    const storedSettings = loadStoredSettings();
    const [windowMode, setWindowMode] = useState<"loading" | "main" | "debug-studio">(isBrowserMode ? "main" : "loading");
    const isDebugStudioWindow = windowMode === "debug-studio";
    const [sourceText, setSourceText] = useState("");
    const [translation, setTranslation] = useState("");
    const [isTranslating, setIsTranslating] = useState(false);
    const [isLoadingModels, setIsLoadingModels] = useState(false);
    const [models, setModels] = useState<ModelInfo[]>([]);
    const [providerSettings, setProviderSettings] = useState<ProviderSettings>({
        mode: storedSettings?.providerMode || "lmstudio",
        endpoint: storedSettings?.endpoint || "http://127.0.0.1:1234",
        apiKey: storedSettings?.apiKey || "",
        model: storedSettings?.selectedModel || "",
        reasoning: storedSettings?.reasoning ?? DEFAULT_REASONING,
        forceShowReasoning: storedSettings?.forceShowReasoning ?? false,
        temperature: clampTemperature(storedSettings?.temperature ?? DEFAULT_TEMPERATURE),
        forceShowTemperature: storedSettings?.forceShowTemperature ?? true,
        enablePostEdit: storedSettings?.enablePostEdit ?? true,
        enableTopicAwarePostEdit: storedSettings?.enableTopicAwarePostEdit ?? true,
        enableEnhancedContextTranslation: storedSettings?.enableEnhancedContextTranslation ?? false,
        enhancedContextGlossary: storedSettings?.enhancedContextGlossary || "",
        enableSmartChunking: storedSettings?.enableSmartChunking ?? true,
        smartChunkSize: storedSettings?.smartChunkSize || 2000,
    });
    const [sourceLang, setSourceLang] = useState(storedSettings?.sourceLang || "auto");
    const [targetLang, setTargetLang] = useState(storedSettings?.targetLang || "Korean");
    const [instruction, setInstruction] = useState(storedSettings?.instruction ?? "");
    const [statusMessage, setStatusMessage] = useState("Loading models...");
    const [debugRequest, setDebugRequest] = useState("");
    const [debugResponse, setDebugResponse] = useState("");
    const [showDebugPanel, setShowDebugPanel] = useState(storedSettings?.showDebugPanel ?? true);
    const [elapsedSeconds, setElapsedSeconds] = useState(0);
    const [showModelModal, setShowModelModal] = useState(false);
    const [showModelPopover, setShowModelPopover] = useState(false);
    const [showEnhancedContextModal, setShowEnhancedContextModal] = useState(false);
    const [enhancedContextDraftEnabled, setEnhancedContextDraftEnabled] = useState(false);
    const [enhancedContextDraftGlossary, setEnhancedContextDraftGlossary] = useState("");
    const [settingsStatus, setSettingsStatus] = useState("");
    const [webServerSettings, setWebServerSettings] = useState<WebServerSettings>(DEFAULT_WEB_SERVER_SETTINGS);
    const [webServerPasswordDraft, setWebServerPasswordDraft] = useState("");
    const [webServerStatus, setWebServerStatus] = useState("");
    const [isSavingWebServerSettings, setIsSavingWebServerSettings] = useState(false);
    const [showSavedToast, setShowSavedToast] = useState(false);
    const [savedToastMessage, setSavedToastMessage] = useState("Settings saved");
    const [editorFontSize, setEditorFontSize] = useState<number>(clampFontSize(storedSettings?.editorFontSize || DEFAULT_EDITOR_FONT_SIZE));
    const [showTemperatureSlider, setShowTemperatureSlider] = useState(false);
    const [showStatusSummary, setShowStatusSummary] = useState(false);
    const [suppressPromptTooltips, setSuppressPromptTooltips] = useState(false);
    const [desktopPlatform, setDesktopPlatform] = useState("");
    const [isCompactPromptLayout, setIsCompactPromptLayout] = useState(false);
    const [isPromptOptionsExpanded, setIsPromptOptionsExpanded] = useState(false);
    const [promptSelectionModal, setPromptSelectionModal] = useState<PromptSelectionModalState>(null);
    const [showSourceEditorModal, setShowSourceEditorModal] = useState(false);
    const [sourceEditorDraft, setSourceEditorDraft] = useState("");
    const [showTranslationViewerModal, setShowTranslationViewerModal] = useState(false);
    const [showTranslationSearchModal, setShowTranslationSearchModal] = useState(false);
    const [translationSearchDraft, setTranslationSearchDraft] = useState("");
    const [translationSearchQuery, setTranslationSearchQuery] = useState("");
    const [translationSearchMatchCount, setTranslationSearchMatchCount] = useState(0);
    const [translationSearchActiveIndex, setTranslationSearchActiveIndex] = useState(-1);
    const [progressState, setProgressState] = useState<Required<ProgressPayload>>({
        stage: "",
        label: "",
        detail: "",
        progress: 0,
        overall_progress: 0,
        current_chunk: 0,
        completed_chunks: 0,
        total_chunks: 0,
        current_step: 0,
        total_steps: 0,
        visible: false,
        indeterminate: true,
    });
    const [animatedProgressValue, setAnimatedProgressValue] = useState(0);
    const [debugTranslationPromptTemplate, setDebugTranslationPromptTemplate] = useState("");
    const [debugPostEditPromptTemplate, setDebugPostEditPromptTemplate] = useState("");
    const [lastTranslationPromptPreview, setLastTranslationPromptPreview] = useState("");
    const [lastPostEditPromptPreview, setLastPostEditPromptPreview] = useState("");
    const [lastTopicAwareHintsPreview, setLastTopicAwareHintsPreview] = useState("");

    const outputRef = useRef<HTMLDivElement>(null);
    const translationViewerRef = useRef<HTMLDivElement>(null);
    const translationSearchMatchesRef = useRef<HTMLElement[]>([]);
    const promptInputRef = useRef<HTMLTextAreaElement>(null);
    const didHydrateSettingsRef = useRef(false);
    const progressHideTimerRef = useRef<number | null>(null);
    const translateActionRef = useRef<() => void>(() => { });
    const openFileActionRef = useRef<() => void>(() => { });
    const latestStatsRef = useRef<TranslationStatsPayload | null>(null);
    const browserTranslateAbortRef = useRef<AbortController | null>(null);
    const translationRunIdRef = useRef(0);
    const temperatureControlRef = useRef<HTMLDivElement>(null);
    const modelPopoverRef = useRef<HTMLDivElement>(null);
    const debugSnapshotRef = useRef("");
    const selectedModel = providerSettings.model;
    const selectedModelInfo = models.find(model => model.id === selectedModel) || null;
    const reasoningOptions = selectedModelInfo?.reasoningOptions?.length ? selectedModelInfo.reasoningOptions : DEFAULT_REASONING_OPTIONS;
    const supportsReasoning = Boolean(selectedModelInfo?.supportsReasoning);
    const showReasoningControl = supportsReasoning || providerSettings.forceShowReasoning;
    const showTemperatureControl = providerSettings.forceShowTemperature;
    const cleanedTranslation = sanitizeTranslation(translation);
    const sourceStats = getTextStats(sourceText);
    const translationStats = getTextStats(cleanedTranslation);
    const temperatureLabel = formatTemperatureLabel(providerSettings.temperature);
    const selectedInstructionPreset = findMatchingInstructionPreset(instruction);
    const instructionPresetValue = selectedInstructionPreset?.id || "custom";
    const reasoningLabel = providerSettings.reasoning || "Auto";
    const usesStageRing = progressState.stage === "model_load" || progressState.stage === "prompt_processing";
    const completedOverallStep = progressState.total_steps > 0
        ? (progressState.stage === "done" ? progressState.total_steps : Math.max(0, progressState.current_step - 1))
        : 0;
    const displayedProgressValue = usesStageRing ? progressState.progress : progressState.overall_progress;
    const clampedAnimatedProgressValue = Math.max(0, Math.min(1, animatedProgressValue || 0));
    const progressPercent = Math.max(0, Math.min(100, Math.round(clampedAnimatedProgressValue * 100)));
    const progressRingRadius = 28;
    const progressRingCircumference = 2 * Math.PI * progressRingRadius;
    const progressRingOffset = progressRingCircumference * (1 - clampedAnimatedProgressValue);
    const progressRingColor = usesStageRing ? "var(--progress-stage)" : "var(--accent)";
    const progressRingCaption = usesStageRing
        ? (progressState.stage === "model_load" ? "Model" : "Prompt")
        : "Overall";

    const showSavedToastMessage = (message: string) => {
        setShowSavedToast(false);
        window.setTimeout(() => {
            setSavedToastMessage(message);
            setShowSavedToast(true);
        }, 0);
    };

    const announceAction = (message: string) => {
        setStatusMessage(message);
        showSavedToastMessage(message);
    };

    const getActiveTranslationContainer = () => {
        if (showTranslationViewerModal && translationViewerRef.current) {
            return translationViewerRef.current;
        }
        return outputRef.current;
    };

    const clearTranslationSearchHighlights = () => {
        [outputRef.current, translationViewerRef.current].forEach(container => {
            if (!container) {
                return;
            }
            const highlights = Array.from(container.querySelectorAll("mark.translation-search-highlight"));
            highlights.forEach(highlight => {
                const textNode = document.createTextNode(highlight.textContent || "");
                highlight.replaceWith(textNode);
            });
            container.normalize();
        });
        translationSearchMatchesRef.current = [];
    };

    const renderTranslationSearchHighlights = (
        query: string,
        requestedIndex: number,
        options?: { scroll?: boolean }
    ) => {
        const container = getActiveTranslationContainer();
        clearTranslationSearchHighlights();

        if (!container) {
            setTranslationSearchMatchCount(0);
            setTranslationSearchActiveIndex(-1);
            return { count: 0, activeIndex: -1 };
        }

        const normalizedQuery = query.trim();
        if (!normalizedQuery) {
            setTranslationSearchMatchCount(0);
            setTranslationSearchActiveIndex(-1);
            return { count: 0, activeIndex: -1 };
        }

        const matches: HTMLElement[] = [];
        const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT);
        const textNodes: Text[] = [];
        let currentNode = walker.nextNode();
        while (currentNode) {
            if (currentNode.nodeType === Node.TEXT_NODE) {
                textNodes.push(currentNode as Text);
            }
            currentNode = walker.nextNode();
        }

        const pattern = new RegExp(escapeRegExp(normalizedQuery), "gi");
        textNodes.forEach(node => {
            const text = node.textContent || "";
            if (!text.trim()) {
                return;
            }

            pattern.lastIndex = 0;
            const matched = Array.from(text.matchAll(pattern));
            if (matched.length === 0) {
                return;
            }

            const fragment = document.createDocumentFragment();
            let cursor = 0;
            matched.forEach(match => {
                const start = match.index ?? 0;
                const value = match[0] || "";
                if (start > cursor) {
                    fragment.appendChild(document.createTextNode(text.slice(cursor, start)));
                }

                const mark = document.createElement("mark");
                mark.className = "translation-search-highlight";
                mark.textContent = value;
                fragment.appendChild(mark);
                matches.push(mark);
                cursor = start + value.length;
            });

            if (cursor < text.length) {
                fragment.appendChild(document.createTextNode(text.slice(cursor)));
            }
            node.parentNode?.replaceChild(fragment, node);
        });

        translationSearchMatchesRef.current = matches;
        const count = matches.length;
        const activeIndex = count > 0
            ? Math.max(0, Math.min(requestedIndex, count - 1))
            : -1;

        matches.forEach((match, index) => {
            match.classList.toggle("is-active", index === activeIndex);
        });

        if (count > 0 && options?.scroll !== false) {
            matches[activeIndex].scrollIntoView({ behavior: "smooth", block: "center", inline: "nearest" });
        }

        setTranslationSearchMatchCount(count);
        setTranslationSearchActiveIndex(activeIndex);
        return { count, activeIndex };
    };

    const closeTranslationSearch = () => {
        setShowTranslationSearchModal(false);
        setTranslationSearchDraft("");
        setTranslationSearchQuery("");
        setTranslationSearchMatchCount(0);
        setTranslationSearchActiveIndex(-1);
        clearTranslationSearchHighlights();
    };

    const handleOpenTranslationSearchModal = () => {
        setTranslationSearchDraft(translationSearchQuery);
        setShowTranslationSearchModal(true);
    };

    const handleSubmitTranslationSearch = () => {
        const query = translationSearchDraft.trim();
        if (!query) {
            closeTranslationSearch();
            setStatusMessage("Enter a word to search in the translation.");
            return;
        }

        setTranslationSearchQuery(query);
        setShowTranslationSearchModal(false);
        const result = renderTranslationSearchHighlights(query, 0);
        if (result.count === 0) {
            setStatusMessage(`No matches found for "${query}".`);
            return;
        }
        setStatusMessage(`Found ${result.count} match${result.count > 1 ? "es" : ""} for "${query}".`);
    };

    const handleMoveTranslationSearch = (direction: "prev" | "next") => {
        const query = translationSearchQuery.trim();
        if (!query) {
            return;
        }

        const count = translationSearchMatchesRef.current.length || translationSearchMatchCount;
        if (count <= 0) {
            const result = renderTranslationSearchHighlights(query, 0);
            if (result.count === 0) {
                setStatusMessage(`No matches found for "${query}".`);
            }
            return;
        }

        let nextIndex = translationSearchActiveIndex;
        let wrapped = false;
        if (direction === "next") {
            nextIndex += 1;
            if (nextIndex >= count) {
                nextIndex = 0;
                wrapped = true;
            }
        } else {
            nextIndex -= 1;
            if (nextIndex < 0) {
                nextIndex = count - 1;
            }
        }

        renderTranslationSearchHighlights(query, nextIndex);
        if (wrapped) {
            setStatusMessage(`Reached the first "${query}" result again.`);
        }
    };

    const handlePasteIntoSourceEditorDraft = async () => {
        try {
            if (isBrowserMode && navigator.clipboard) {
                const content = await navigator.clipboard.readText();
                if (!content) {
                    announceAction("Clipboard is empty.");
                    return;
                }
                setSourceEditorDraft(content);
                announceAction("Pasted clipboard text into the source editor.");
                return;
            }
            const content = await ClipboardGetText();
            if (!content) {
                announceAction("Clipboard is empty.");
                return;
            }
            setSourceEditorDraft(content);
            announceAction("Pasted clipboard text into the source editor.");
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not paste from clipboard: ${String(err)}`);
        }
    };

    const handleOpenFileIntoSourceEditorDraft = async () => {
        if (isBrowserMode) {
            announceAction("Opening local files is only available in the desktop app.");
            return;
        }
        try {
            const content = await OpenFile();
            if (content) {
                setSourceEditorDraft(content);
                announceAction("Loaded file content into the source editor.");
            }
        } catch (err: any) {
            console.error(err);
        }
    };

    const handleClearSourceEditorDraft = async () => {
        if (!sourceEditorDraft) {
            announceAction("There is no source text to clear.");
            return;
        }
        try {
            const shouldUseNativeDialog = !isBrowserMode && desktopPlatform === "darwin";
            const shouldClear = shouldUseNativeDialog
                ? await ConfirmClearSource()
                : window.confirm("Clear the source text?");
            if (!shouldClear) {
                announceAction("Kept the source text.");
                return;
            }
            setSourceEditorDraft("");
            announceAction("Cleared the source editor.");
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not clear the source text: ${String(err)}`);
        }
    };

    useEffect(() => {
        const textarea = promptInputRef.current;
        if (!textarea) {
            return;
        }
        textarea.style.height = "auto";
        const computedStyle = window.getComputedStyle(textarea);
        const lineHeight = Number.parseFloat(computedStyle.lineHeight) || editorFontSize * 1.55;
        const verticalPadding =
            (Number.parseFloat(computedStyle.paddingTop) || 0) +
            (Number.parseFloat(computedStyle.paddingBottom) || 0);
        const maxHeight = lineHeight * 3 + verticalPadding;
        textarea.style.height = `${Math.min(textarea.scrollHeight, maxHeight)}px`;
        textarea.style.overflowY = textarea.scrollHeight > maxHeight ? "auto" : "hidden";
    }, [instruction, editorFontSize]);

    useEffect(() => {
        const nextValue = Math.max(0, Math.min(1, displayedProgressValue || 0));
        if (!progressState.visible || nextValue === 0) {
            setAnimatedProgressValue(nextValue);
            return;
        }
        if (nextValue <= animatedProgressValue) {
            setAnimatedProgressValue(nextValue);
            return;
        }

        let frameId = 0;
        let startTime = 0;
        const startValue = animatedProgressValue;
        const duration = 220;

        const tick = (timestamp: number) => {
            if (!startTime) {
                startTime = timestamp;
            }
            const elapsed = timestamp - startTime;
            const ratio = Math.min(1, elapsed / duration);
            const eased = 1 - Math.pow(1 - ratio, 3);
            setAnimatedProgressValue(startValue + (nextValue - startValue) * eased);
            if (ratio < 1) {
                frameId = window.requestAnimationFrame(tick);
            }
        };

        frameId = window.requestAnimationFrame(tick);
        return () => window.cancelAnimationFrame(frameId);
    }, [displayedProgressValue, progressState.visible]);

    useEffect(() => {
        if (isBrowserMode) {
            setWindowMode("main");
            callBrowserJSON<ProviderSettings>("/api/client-config")
                .then((settings: any) => {
                    setProviderSettings(prev => ({
                        ...prev,
                        ...settings,
                        apiKey: "",
                        endpoint: "",
                    }));
                })
                .catch((err: any) => {
                    console.error(err);
                    setStatusMessage(`Could not load host settings: ${String(err)}`);
                });
            return;
        }
        let active = true;
        GetWindowMode()
            .then((mode: string) => {
                if (!active) {
                    return;
                }
                setWindowMode(mode === "debug-studio" ? "debug-studio" : "main");
            })
            .catch(() => {
                if (!active) {
                    return;
                }
                setWindowMode("main");
            });
        return () => {
            active = false;
        };
    }, []);

    useEffect(() => {
        if (isBrowserMode) {
            return;
        }
        let active = true;
        Environment()
            .then((info) => {
                if (!active) {
                    return;
                }
                setDesktopPlatform(info.platform || "");
            })
            .catch((err: any) => {
                console.error("Could not read environment info:", err);
            });
        return () => {
            active = false;
        };
    }, []);

    useEffect(() => {
        const mediaQuery = window.matchMedia("(max-width: 860px)");
        const syncCompactPromptLayout = (matches: boolean) => {
            setIsCompactPromptLayout(matches);
            if (!matches) {
                setIsPromptOptionsExpanded(false);
            }
        };

        syncCompactPromptLayout(mediaQuery.matches);

        const handleChange = (event: MediaQueryListEvent) => {
            syncCompactPromptLayout(event.matches);
        };

        if (typeof mediaQuery.addEventListener === "function") {
            mediaQuery.addEventListener("change", handleChange);
            return () => mediaQuery.removeEventListener("change", handleChange);
        }

        mediaQuery.addListener(handleChange);
        return () => mediaQuery.removeListener(handleChange);
    }, []);

    useEffect(() => {
        if (isBrowserMode) {
            return;
        }
        GetWebServerSettings()
            .then((settings: any) => {
                setWebServerSettings(settings as WebServerSettings);
            })
            .catch((err: any) => {
                console.error(err);
                setWebServerStatus(`Could not load web server settings: ${String(err)}`);
            });
    }, []);

    useEffect(() => {
        const shouldLockBackgroundScroll = Boolean(
            promptSelectionModal ||
            (isCompactPromptLayout && showTemperatureSlider) ||
            showSourceEditorModal ||
            showTranslationViewerModal ||
            showModelModal ||
            showEnhancedContextModal
        );

        document.documentElement.classList.toggle("modal-open", shouldLockBackgroundScroll);
        document.body.classList.toggle("modal-open", shouldLockBackgroundScroll);

        return () => {
            document.documentElement.classList.remove("modal-open");
            document.body.classList.remove("modal-open");
        };
    }, [
        promptSelectionModal,
        isCompactPromptLayout,
        showTemperatureSlider,
        showSourceEditorModal,
        showTranslationViewerModal,
        showModelModal,
        showEnhancedContextModal,
    ]);

    useEffect(() => {
        if (windowMode !== "debug-studio") {
            document.body.classList.remove("debug-studio-mode");
            document.documentElement.classList.remove("debug-studio-mode");
            document.getElementById("root")?.classList.remove("debug-studio-root");
            return;
        }

        document.body.classList.add("debug-studio-mode");
        document.documentElement.classList.add("debug-studio-mode");
        document.getElementById("root")?.classList.add("debug-studio-root");

        return () => {
            document.body.classList.remove("debug-studio-mode");
            document.documentElement.classList.remove("debug-studio-mode");
            document.getElementById("root")?.classList.remove("debug-studio-root");
        };
    }, [windowMode]);

    useEffect(() => {
        if (isDebugStudioWindow) {
            return;
        }
        fetchModels();

        if (isBrowserMode) {
            return;
        }

        // Listen for translation tokens
        EventsOn("translation:token", (token: string) => {
            setTranslation(prev => prev + token);
            if (outputRef.current) {
                outputRef.current.scrollTop = outputRef.current.scrollHeight;
            }
        });

        EventsOn("translation:clear", () => {
            setTranslation("");
        });

        EventsOn("translation:complete", (payload: TranslationCompletePayload) => {
            setTranslation(payload.text || "");
            if (outputRef.current) {
                outputRef.current.scrollTop = outputRef.current.scrollHeight;
            }
            setStatusMessage(formatCompletionStats(latestStatsRef.current));
            if (progressHideTimerRef.current !== null) {
                window.clearTimeout(progressHideTimerRef.current);
            }
            setProgressState(prev => ({
                ...prev,
                stage: "done",
                label: "Done",
                detail: "Translation complete",
                progress: 1,
                visible: true,
                indeterminate: false,
            }));
            progressHideTimerRef.current = window.setTimeout(() => {
                setProgressState(prev => ({ ...prev, visible: false }));
                progressHideTimerRef.current = null;
            }, 500);
        });

        EventsOn("translation:debug", (payload: DebugPayload) => {
            if (payload.direction === "note" && payload.endpoint === "prompt:translation") {
                setLastTranslationPromptPreview(payload.payload || "");
                return;
            }
            if (payload.direction === "note" && payload.endpoint === "prompt:postedit") {
                setLastPostEditPromptPreview(payload.payload || "");
                return;
            }
            if (payload.direction === "note" && payload.endpoint === "prompt:topic-aware-hints") {
                setLastTopicAwareHintsPreview(payload.payload || "");
                return;
            }
            const section = `${payload.endpoint || ""}\n${payload.payload || ""}`.trim();
            if (payload.direction === "request") {
                setDebugRequest(section);
            } else if (payload.direction === "response" || payload.direction === "note") {
                setDebugResponse(section);
            }
        });

        EventsOn("translation:progress", (payload: ProgressPayload) => {
            if (progressHideTimerRef.current !== null) {
                window.clearTimeout(progressHideTimerRef.current);
                progressHideTimerRef.current = null;
            }
            const nextOverallProgress = deriveOverallProgress(payload.current_step, payload.total_steps, payload.stage === "done");
            setProgressState(prev => ({
                stage: payload.stage ?? prev.stage,
                label: payload.label ?? prev.label,
                detail: payload.detail ?? prev.detail,
                progress: typeof payload.progress === "number" ? payload.progress : prev.progress,
                overall_progress: (payload.stage === "model_load" || payload.stage === "prompt_processing")
                    ? prev.overall_progress
                    : (nextOverallProgress ?? prev.overall_progress),
                current_chunk: typeof payload.current_chunk === "number" ? payload.current_chunk : prev.current_chunk,
                completed_chunks: typeof payload.completed_chunks === "number" ? payload.completed_chunks : prev.completed_chunks,
                total_chunks: typeof payload.total_chunks === "number" ? payload.total_chunks : prev.total_chunks,
                current_step: typeof payload.current_step === "number" ? payload.current_step : prev.current_step,
                total_steps: typeof payload.total_steps === "number" ? payload.total_steps : prev.total_steps,
                visible: payload.visible ?? prev.visible,
                indeterminate: payload.indeterminate ?? prev.indeterminate,
            }));
        });

        EventsOn("translation:stats", (payload: TranslationStatsPayload) => {
            latestStatsRef.current = payload;
        });

        // Listen for menu events
        EventsOn("menu:open-file", () => openFileActionRef.current());
        EventsOn("menu:translate", () => translateActionRef.current());
        EventsOn("menu:font-increase", () => setEditorFontSize(prev => clampFontSize(prev + 1)));
        EventsOn("menu:font-decrease", () => setEditorFontSize(prev => clampFontSize(prev - 1)));
        EventsOn("menu:font-reset", () => setEditorFontSize(DEFAULT_EDITOR_FONT_SIZE));

        const handleKeyDown = (event: KeyboardEvent) => {
            const isMod = event.metaKey || event.ctrlKey;
            if (!isMod) {
                return;
            }

            if (event.key.toLowerCase() === "t") {
                event.preventDefault();
                translateActionRef.current();
                return;
            }

            if (event.key === "0") {
                event.preventDefault();
                setEditorFontSize(DEFAULT_EDITOR_FONT_SIZE);
                return;
            }

            if (event.key === "=" || event.key === "+") {
                event.preventDefault();
                setEditorFontSize(prev => clampFontSize(prev + 1));
                return;
            }

            if (event.key === "-") {
                event.preventDefault();
                setEditorFontSize(prev => clampFontSize(prev - 1));
            }
        };
        window.addEventListener("keydown", handleKeyDown);

        return () => {
            if (progressHideTimerRef.current !== null) {
                window.clearTimeout(progressHideTimerRef.current);
            }
            window.removeEventListener("keydown", handleKeyDown);
            EventsOff("translation:token");
            EventsOff("translation:clear");
            EventsOff("translation:complete");
            EventsOff("translation:debug");
            EventsOff("translation:progress");
            EventsOff("translation:stats");
            EventsOff("menu:open-file");
            EventsOff("menu:translate");
            EventsOff("menu:font-increase");
            EventsOff("menu:font-decrease");
            EventsOff("menu:font-reset");
        };
    }, [isDebugStudioWindow]);

    useEffect(() => {
        if (isBrowserMode) {
            return;
        }
        const applySnapshot = (snapshot: DebugStudioSnapshot | null) => {
            if (!snapshot) {
                return;
            }
            setShowDebugPanel(snapshot.showDebugPanel);
            setDebugRequest(snapshot.debugRequest || "");
            setDebugResponse(snapshot.debugResponse || "");
            setDebugTranslationPromptTemplate(snapshot.debugTranslationPromptTemplate || "");
            setDebugPostEditPromptTemplate(snapshot.debugPostEditPromptTemplate || "");
            setLastTranslationPromptPreview(snapshot.lastTranslationPromptPreview || "");
            setLastPostEditPromptPreview(snapshot.lastPostEditPromptPreview || "");
            setLastTopicAwareHintsPreview(snapshot.lastTopicAwareHintsPreview || "");
        };

        let active = true;
        const syncFromBackend = async () => {
            try {
                const raw = await ReadDebugStudioState();
                if (!active || !raw || raw === debugSnapshotRef.current) {
                    return;
                }
                const snapshot = JSON.parse(raw) as DebugStudioSnapshot;
                debugSnapshotRef.current = raw;
                applySnapshot(snapshot);
            } catch {
                // Ignore sync failures so the main UI stays responsive.
            }
        };

        syncFromBackend();
        const timer = window.setInterval(syncFromBackend, 350);
        return () => {
            active = false;
            window.clearInterval(timer);
        };
    }, []);

    useEffect(() => {
        if (isBrowserMode) {
            return;
        }
        const snapshot: DebugStudioSnapshot = {
            showDebugPanel,
            debugRequest,
            debugResponse,
            debugTranslationPromptTemplate,
            debugPostEditPromptTemplate,
            lastTranslationPromptPreview,
            lastPostEditPromptPreview,
            lastTopicAwareHintsPreview,
        };
        const raw = JSON.stringify(snapshot);
        if (raw === debugSnapshotRef.current) {
            return;
        }
        debugSnapshotRef.current = raw;
        void WriteDebugStudioState(raw);
    }, [
        showDebugPanel,
        debugRequest,
        debugResponse,
        debugTranslationPromptTemplate,
        debugPostEditPromptTemplate,
        lastTranslationPromptPreview,
        lastPostEditPromptPreview,
        lastTopicAwareHintsPreview,
    ]);

    useEffect(() => {
        const handlePointerDown = (event: MouseEvent) => {
            if (!isCompactPromptLayout && temperatureControlRef.current && !temperatureControlRef.current.contains(event.target as Node)) {
                setShowTemperatureSlider(false);
                setSuppressPromptTooltips(false);
            }
            if (modelPopoverRef.current && !modelPopoverRef.current.contains(event.target as Node)) {
                setShowModelPopover(false);
            }
        };

        document.addEventListener("mousedown", handlePointerDown);
        return () => document.removeEventListener("mousedown", handlePointerDown);
    }, [isCompactPromptLayout]);

    useEffect(() => {
        if (!showEnhancedContextModal) {
            return;
        }
        setEnhancedContextDraftEnabled(providerSettings.enableEnhancedContextTranslation);
        setEnhancedContextDraftGlossary(providerSettings.enhancedContextGlossary);
    }, [showEnhancedContextModal, providerSettings.enableEnhancedContextTranslation, providerSettings.enhancedContextGlossary]);

    useEffect(() => {
        if (!showSavedToast) {
            return;
        }
        const timer = window.setTimeout(() => setShowSavedToast(false), 1400);
        return () => window.clearTimeout(timer);
    }, [showSavedToast, savedToastMessage]);

    useEffect(() => {
        if (isDebugStudioWindow) {
            return;
        }
        if (!isBrowserMode) {
            void SaveHostProviderSettings(llm.ProviderSettings.createFrom({
                ...providerSettings,
                debugTranslationPromptTemplate: "",
                debugPostEditPromptTemplate: "",
            })).catch((err: any) => {
                console.error("Could not persist host provider settings:", err);
            });
        }
        persistSettings(
            selectedModel,
            providerSettings,
            editorFontSize,
            sourceLang,
            targetLang,
            showDebugPanel
        );

        if (!didHydrateSettingsRef.current) {
            didHydrateSettingsRef.current = true;
            return;
        }

        showSavedToastMessage("Settings saved");
    }, [providerSettings, editorFontSize, sourceLang, targetLang, showDebugPanel, selectedModel, isDebugStudioWindow]);

    useEffect(() => {
        if (!translationSearchQuery.trim()) {
            clearTranslationSearchHighlights();
            return;
        }
        renderTranslationSearchHighlights(
            translationSearchQuery,
            translationSearchActiveIndex < 0 ? 0 : translationSearchActiveIndex,
            { scroll: false }
        );
    }, [cleanedTranslation, showTranslationViewerModal]);

    useEffect(() => {
        if (!isTranslating) {
            setElapsedSeconds(0);
            return;
        }

        const timer = window.setInterval(() => {
            setElapsedSeconds(prev => prev + 1);
        }, 1000);

        return () => window.clearInterval(timer);
    }, [isTranslating]);

    useEffect(() => {
        if (!showReasoningControl && providerSettings.reasoning) {
            setProviderSettings(prev => ({ ...prev, reasoning: "" }));
            return;
        }
        if (showReasoningControl && providerSettings.reasoning && !reasoningOptions.includes(providerSettings.reasoning)) {
            setProviderSettings(prev => ({ ...prev, reasoning: "" }));
        }
    }, [showReasoningControl, reasoningOptions, providerSettings.reasoning]);

    useEffect(() => {
        translateActionRef.current = handleTranslate;
        openFileActionRef.current = handleOpenFile;
    });

    const fetchModels = async () => {
        setIsLoadingModels(true);
        setSettingsStatus("");
        try {
            const list = isBrowserMode
                ? await callBrowserJSON<ModelInfo[]>("/api/models", {
                    method: "GET",
                })
                : await GetModels(providerSettings) as ModelInfo[];
            setModels(list || []);
            if (list && list.length > 0) {
                setProviderSettings(prev => {
                    let nextModel = prev.model;
                    const ids = list.map(item => item.id);
                    if (!nextModel || !ids.includes(nextModel)) {
                        if (storedSettings?.selectedModel && ids.includes(storedSettings.selectedModel)) {
                            nextModel = storedSettings.selectedModel;
                        } else {
                            nextModel = list[0].id;
                        }
                    }
                    const nextModelInfo = list.find(item => item.id === nextModel);
                    return {
                        ...prev,
                        model: nextModel,
                        reasoning: nextModelInfo?.supportsReasoning ? prev.reasoning : "",
                    };
                });
                setStatusMessage(`Found ${list.length} available models.`);
                setSettingsStatus(`Loaded ${list.length} models.`);
            } else {
                setProviderSettings(prev => ({ ...prev, model: "" }));
                setStatusMessage("Connected, but the model list is empty.");
                setSettingsStatus("Connected, but no models are available.");
            }
        } catch (err: any) {
            console.error("Failed to fetch models:", err);
            setModels([]);
            setProviderSettings(prev => ({ ...prev, model: "" }));
            const message = `Could not load models. Check the endpoint and API key. (${String(err)})`;
            setStatusMessage(message);
            setSettingsStatus(message);
        } finally {
            setIsLoadingModels(false);
        }
    };

    const handleTranslate = async () => {
        if (!sourceText.trim() || !selectedModel || isTranslating) return;

        const runID = translationRunIdRef.current + 1;
        translationRunIdRef.current = runID;
        const isActiveRun = () => translationRunIdRef.current === runID;

        const runTranslation = async (settings: ProviderSettings) => {
            const payload = llm.TranslationRequest.createFrom({
                settings: llm.ProviderSettings.createFrom({
                    ...settings,
                    debugTranslationPromptTemplate: debugTranslationPromptTemplate.trim(),
                    debugPostEditPromptTemplate: debugPostEditPromptTemplate.trim(),
                }),
                sourceText,
                sourceLang,
                targetLang,
                instruction,
            });
            if (isBrowserMode) {
                browserTranslateAbortRef.current?.abort();
                const controller = new AbortController();
                browserTranslateAbortRef.current = controller;
                try {
                    await streamBrowserSSE("/api/translate/stream", {
                        method: "POST",
                        headers: {
                            "Content-Type": "application/json",
                        },
                        body: JSON.stringify(payload),
                        signal: controller.signal,
                    }, {
                        onEvent: (event, data) => {
                            if (!isActiveRun()) {
                                return;
                            }
                            if (event === "clear") {
                                setTranslation("");
                                return;
                            }
                            if (event === "token") {
                                setTranslation(prev => prev + (data?.token || ""));
                                return;
                            }
                            if (event === "progress") {
                                const nextOverallProgress = deriveOverallProgress(data?.current_step, data?.total_steps, data?.stage === "done");
                                setProgressState(prev => ({
                                    stage: data?.stage ?? prev.stage,
                                    label: data?.label ?? prev.label,
                                    detail: data?.detail ?? prev.detail,
                                    progress: typeof data?.progress === "number" ? data.progress : prev.progress,
                                    overall_progress: (data?.stage === "model_load" || data?.stage === "prompt_processing")
                                        ? prev.overall_progress
                                        : (nextOverallProgress ?? prev.overall_progress),
                                    current_chunk: typeof data?.current_chunk === "number" ? data.current_chunk : prev.current_chunk,
                                    completed_chunks: typeof data?.completed_chunks === "number" ? data.completed_chunks : prev.completed_chunks,
                                    total_chunks: typeof data?.total_chunks === "number" ? data.total_chunks : prev.total_chunks,
                                    current_step: typeof data?.current_step === "number" ? data.current_step : prev.current_step,
                                    total_steps: typeof data?.total_steps === "number" ? data.total_steps : prev.total_steps,
                                    visible: data?.visible ?? true,
                                    indeterminate: data?.indeterminate ?? prev.indeterminate,
                                }));
                                return;
                            }
                            if (event === "stats") {
                                latestStatsRef.current = data || null;
                                return;
                            }
                            if (event === "complete") {
                                setTranslation(data?.text || "");
                                if (progressHideTimerRef.current !== null) {
                                    window.clearTimeout(progressHideTimerRef.current);
                                }
                                setProgressState({
                                    stage: "done",
                                    label: "Done",
                                    detail: "Translation complete",
                                    progress: 1,
                                    overall_progress: 1,
                                    current_chunk: progressState.current_chunk,
                                    completed_chunks: progressState.total_chunks || progressState.completed_chunks,
                                    total_chunks: progressState.total_chunks,
                                    current_step: progressState.total_steps || progressState.current_step,
                                    total_steps: progressState.total_steps,
                                    visible: true,
                                    indeterminate: false,
                                });
                                progressHideTimerRef.current = window.setTimeout(() => {
                                    if (!isActiveRun()) {
                                        return;
                                    }
                                    setProgressState(prev => ({ ...prev, visible: false }));
                                    progressHideTimerRef.current = null;
                                }, 500);
                                return;
                            }
                            if (event === "error") {
                                throw new Error(data?.message || "Streaming translation failed.");
                            }
                        },
                    });
                } finally {
                    if (browserTranslateAbortRef.current === controller) {
                        browserTranslateAbortRef.current = null;
                    }
                }
                return;
            }
            await Translate(payload);
        };

        const isReasoningRequestError = (message: string) =>
            message.toLowerCase().includes("translation request failed (400)");
        const isCancellationError = (message: string) => {
            const lowered = message.toLowerCase();
            return lowered.includes("cancelled") || lowered.includes("aborted") || lowered.includes("aborterror");
        };

        const autoReasoningProbeSettings = {
            ...providerSettings,
            reasoning: "off",
        };
        const shouldProbeReasoningOffFirst = !providerSettings.reasoning;

        const retryWithAutoReasoning = async () => {
            if (!isActiveRun()) {
                throw new Error("Translation cancelled.");
            }
            const fallbackSettings = {
                ...providerSettings,
                reasoning: "",
            };
            setProgressState(prev => ({
                ...prev,
                stage: "retrying",
                label: "Retrying translation",
                detail: "Reasoning unsupported. Switched to Auto.",
                visible: true,
                indeterminate: true,
            }));
            setProviderSettings(fallbackSettings);
            persistSettings(
                selectedModel,
                fallbackSettings,
                editorFontSize,
                sourceLang,
                targetLang,
                showDebugPanel,
                instruction
            );
            setStatusMessage("Reasoning unsupported. Retrying with Auto.");
            await runTranslation(fallbackSettings);
        };

        const retryWithoutReasoningOption = async () => {
            if (!isActiveRun()) {
                throw new Error("Translation cancelled.");
            }
            setProgressState(prev => ({
                ...prev,
                stage: "retrying",
                label: "Retrying translation",
                detail: "Reasoning option unsupported. Retrying without reasoning parameter.",
                visible: true,
                indeterminate: true,
            }));
            setStatusMessage("Retrying without reasoning parameter.");
            await runTranslation({
                ...providerSettings,
                reasoning: "",
            });
        };

        persistSettings(
            selectedModel,
            providerSettings,
            editorFontSize,
            sourceLang,
            targetLang,
            showDebugPanel,
            instruction
        );
        setTranslation("");
        latestStatsRef.current = null;
        setElapsedSeconds(0);
        setProgressState({
            stage: "preparing",
            label: "Preparing request",
            detail: providerSettings.mode === "lmstudio" ? "Connecting to LM Studio" : "Connecting to LLM endpoint",
            progress: 0,
            overall_progress: 0,
            current_chunk: 0,
            completed_chunks: 0,
            total_chunks: 0,
            current_step: 0,
            total_steps: 0,
            visible: true,
            indeterminate: true,
        });
        setIsTranslating(true);
        setStatusMessage(`Translating ${sourceLang} -> ${targetLang} with "${selectedModel}".`);
        try {
            await runTranslation(shouldProbeReasoningOffFirst ? autoReasoningProbeSettings : providerSettings);
            if (isActiveRun() && !latestStatsRef.current) {
                setStatusMessage("Translation completed.");
            }
        } catch (err: any) {
            console.error("Translation failed:", err);
            const message = String(err);
            if (isCancellationError(message)) {
                setStatusMessage("Translation cancelled.");
            } else if (shouldProbeReasoningOffFirst && isReasoningRequestError(message)) {
                try {
                    await retryWithoutReasoningOption();
                    if (isActiveRun() && !latestStatsRef.current) {
                        setStatusMessage("Translation completed.");
                    }
                } catch (retryErr) {
                    console.error("Retry without reasoning parameter failed:", retryErr);
                    const retryMessage = String(retryErr);
                    if (isCancellationError(retryMessage)) {
                        setStatusMessage("Translation cancelled.");
                    } else {
                        setStatusMessage(`Translation failed: ${retryMessage}`);
                        alert("Translation failed: " + retryErr);
                    }
                    setProgressState(prev => ({ ...prev, visible: false }));
                }
            } else if (
                providerSettings.reasoning &&
                isReasoningRequestError(message)
            ) {
                try {
                    await retryWithAutoReasoning();
                    if (isActiveRun() && !latestStatsRef.current) {
                        setStatusMessage("Translation completed.");
                    }
                } catch (retryErr) {
                    console.error("Retry with Reasoning Auto failed:", retryErr);
                    const retryMessage = String(retryErr);
                    if (isCancellationError(retryMessage)) {
                        setStatusMessage("Translation cancelled.");
                    } else {
                        setStatusMessage(`Translation failed: ${retryMessage}`);
                        alert("Translation failed: " + retryErr);
                    }
                    setProgressState(prev => ({ ...prev, visible: false }));
                }
            } else {
                setStatusMessage(`Translation failed: ${message}`);
                alert("Translation failed: " + err);
                setProgressState(prev => ({ ...prev, visible: false }));
            }
        } finally {
            if (isActiveRun()) {
                setIsTranslating(false);
            }
        }
    };

    const handleCancel = async () => {
        translationRunIdRef.current += 1;
        browserTranslateAbortRef.current?.abort();
        browserTranslateAbortRef.current = null;
        try {
            if (isBrowserMode) {
                await callBrowserJSON<void>("/api/cancel", {
                    method: "POST",
                    body: JSON.stringify({}),
                });
            } else {
                await CancelTranslation();
            }
            setStatusMessage("Sent translation cancel request.");
        } catch (err: any) {
            console.error("Cancel failed:", err);
            setStatusMessage(`Cancel failed: ${String(err)}`);
        }
        setIsTranslating(false);
        setProgressState(prev => ({ ...prev, visible: false }));
    };

    const handleOpenFile = async () => {
        if (isBrowserMode) {
            announceAction("Opening local files is only available in the desktop app.");
            return;
        }
        try {
            const content = await OpenFile();
            if (content) {
                setSourceText(content);
                announceAction("Loaded file content into the source editor.");
            }
        } catch (err: any) {
            console.error(err);
        }
    };

    const handlePasteSource = async () => {
        try {
            if (isBrowserMode && navigator.clipboard) {
                const content = await navigator.clipboard.readText();
                if (!content) {
                    announceAction("Clipboard is empty.");
                    return;
                }
                setSourceText(content);
                announceAction("Pasted clipboard text into the source editor.");
                return;
            }
            const content = await ClipboardGetText();
            if (!content) {
                announceAction("Clipboard is empty.");
                return;
            }
            setSourceText(content);
            announceAction("Pasted clipboard text into the source editor.");
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not paste from clipboard: ${String(err)}`);
        }
    };

    const handleClearSource = async () => {
        if (!sourceText) {
            announceAction("There is no source text to clear.");
            return;
        }
        try {
            const shouldUseNativeDialog = !isBrowserMode && desktopPlatform === "darwin";
            const shouldClear = shouldUseNativeDialog
                ? await ConfirmClearSource()
                : window.confirm("Clear the source text?");
            if (!shouldClear) {
                announceAction("Kept the source text.");
                return;
            }
            setSourceText("");
            announceAction("Cleared the source editor.");
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not clear the source text: ${String(err)}`);
        }
    };

    const handleOpenSourceEditorModal = () => {
        setSourceEditorDraft(sourceText);
        setShowSourceEditorModal(true);
    };

    const handleCloseSourceEditorModal = () => {
        setSourceText(sourceEditorDraft);
        setShowSourceEditorModal(false);
        announceAction("Updated the source editor.");
    };

    const handleOpenTranslationViewerModal = () => {
        setShowTranslationViewerModal(true);
    };

    const handleCloseTranslationViewerModal = () => {
        setShowTranslationViewerModal(false);
        closeTranslationSearch();
    };

    const handleSaveFile = async () => {
        if (isBrowserMode) {
            try {
                const blob = new Blob([cleanedTranslation], { type: "text/plain;charset=utf-8" });
                const url = window.URL.createObjectURL(blob);
                const link = document.createElement("a");
                link.href = url;
                link.download = "dkst-translation.txt";
                link.click();
                window.URL.revokeObjectURL(url);
                announceAction("Downloaded translation file.");
            } catch (err: any) {
                console.error(err);
                setStatusMessage(`Could not download translation: ${String(err)}`);
            }
            return;
        }
        try {
            const savedPath = await SaveFile(cleanedTranslation);
            if (savedPath) {
                announceAction(`Saved translation to: ${savedPath}`);
            }
        } catch (err: any) {
            console.error(err);
        }
    };

    const handleCopyTranslation = async () => {
        try {
            if (!cleanedTranslation) {
                announceAction("There is no translated text to copy.");
                return;
            }
            if (isBrowserMode && navigator.clipboard) {
                await navigator.clipboard.writeText(cleanedTranslation);
                announceAction("Copied translation to clipboard.");
                return;
            }
            const ok = await ClipboardSetText(cleanedTranslation);
            if (ok) {
                announceAction("Copied translation to clipboard.");
            } else {
                announceAction("Could not copy translation to clipboard.");
            }
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not copy translation: ${String(err)}`);
        }
    };

    const handleOpenDebugStudioWindow = () => {
        if (isBrowserMode) {
            setStatusMessage("Debug Studio is only available in the desktop app.");
            return;
        }
        void OpenDebugStudioWindow().catch((err: any) => {
            console.error(err);
            setStatusMessage(`Could not open a separate Debug Studio window: ${String(err)}`);
        });
    };

    const handleSaveWebServerSettings = async () => {
        if (isBrowserMode) {
            setWebServerStatus("Web server settings can only be changed in the desktop app.");
            return;
        }
        setIsSavingWebServerSettings(true);
        setWebServerStatus("");
        try {
            const saved = await SaveWebServerSettings({
                enabled: webServerSettings.enabled,
                port: webServerSettings.port,
                password: webServerPasswordDraft,
                useTls: webServerSettings.useTls,
                certDomain: webServerSettings.certDomain,
                certPath: webServerSettings.certPath || "",
                keyPath: webServerSettings.keyPath || "",
            } as any) as WebServerSettings;
            setWebServerSettings(saved);
            setWebServerPasswordDraft("");
            setWebServerStatus(saved.enabled
                ? `Web server ready at ${saved.url || `${saved.useTls ? "https" : "http"}://localhost:${saved.port}`}`
                : "Web server disabled.");
            showSavedToastMessage("Saved");
        } catch (err: any) {
            console.error(err);
            setWebServerStatus(`Could not save web server settings: ${String(err)}`);
        } finally {
            setIsSavingWebServerSettings(false);
        }
    };

    const handleOpenCertificateFolder = () => {
        if (isBrowserMode) {
            setWebServerStatus("Certificate folder access is only available in the desktop app.");
            return;
        }
        void OpenCertificateFolder().catch((err: any) => {
            console.error(err);
            setWebServerStatus(`Could not open certificate folder: ${String(err)}`);
        });
    };

    const handleSwapLanguages = () => {
        setSourceLang((currentSource: string) => {
            if (currentSource === "auto") {
                return targetLang;
            }
            return targetLang;
        });
        setTargetLang((currentTarget: string) => {
            if (sourceLang === "auto") {
                return currentTarget;
            }
            return sourceLang;
        });
        setStatusMessage("Swapped source and target languages.");
    };

    const handleSelectModel = (modelId: string) => {
        const nextModel = models.find(model => model.id === modelId);
        setProviderSettings(prev => ({
            ...prev,
            model: modelId,
            reasoning: nextModel?.supportsReasoning ? prev.reasoning : "",
        }));
        setTranslation("");
        setShowModelPopover(false);
        setSuppressPromptTooltips(false);
    };

    const handleCloseEnhancedContextModal = () => {
        setProviderSettings(prev => ({
            ...prev,
            enableEnhancedContextTranslation: enhancedContextDraftEnabled,
            enhancedContextGlossary: enhancedContextDraftGlossary,
        }));
        setShowEnhancedContextModal(false);
        showSavedToastMessage("Saved");
    };

    const handleOpenEnhancedContextGlossary = async () => {
        try {
            const content = await OpenFile();
            if (content !== undefined && content !== null && content !== "") {
                setEnhancedContextDraftGlossary(content);
                setStatusMessage("Loaded glossary content.");
            }
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not open glossary file: ${String(err)}`);
        }
    };

    const handleSaveEnhancedContextGlossary = async () => {
        try {
            const savedPath = await SaveFile(enhancedContextDraftGlossary);
            if (savedPath) {
                setStatusMessage(`Saved glossary to: ${savedPath}`);
            }
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not save glossary file: ${String(err)}`);
        }
    };

    const handleInstructionPresetChange = (presetId: string) => {
        const preset = INSTRUCTION_PRESETS.find(item => item.id === presetId);
        if (!preset) {
            return;
        }
        setInstruction(preset.instruction);
        setStatusMessage(`Applied prompt preset: ${preset.label}`);
    };

    const handleSelectPromptPreset = (presetId: string) => {
        handleInstructionPresetChange(presetId);
        setPromptSelectionModal(null);
    };

    const handleSelectReasoningOption = (value: string) => {
        setProviderSettings(prev => ({ ...prev, reasoning: value }));
        setPromptSelectionModal(null);
        setStatusMessage(`Reasoning set to ${value || "Auto"}.`);
    };

    const canTranslate = Boolean(sourceText.trim() && selectedModel && !isTranslating);

    const primaryActionLabel = "Translate";
    const primaryActionIcon = "translate";
    const primaryActionClassName = "btn btn-primary btn-shimmer-hover hero-translate-btn";

    const activityLabel = isTranslating
        ? `Translating ${formatElapsed(elapsedSeconds)}`
        : selectedModel
            ? `${selectedModel} ready`
            : "No model selected";

    if (windowMode === "loading" && !isBrowserMode) {
        return <div className="debug-window-shell">Loading window...</div>;
    }

    if (isDebugStudioWindow) {
        return (
            <DebugStudioWindow
                showDebugPanel={showDebugPanel}
                debugRequest={debugRequest}
                debugResponse={debugResponse}
                debugTranslationPromptTemplate={debugTranslationPromptTemplate}
                debugPostEditPromptTemplate={debugPostEditPromptTemplate}
                lastTranslationPromptPreview={lastTranslationPromptPreview}
                lastPostEditPromptPreview={lastPostEditPromptPreview}
                lastTopicAwareHintsPreview={lastTopicAwareHintsPreview}
                setDebugTranslationPromptTemplate={setDebugTranslationPromptTemplate}
                setDebugPostEditPromptTemplate={setDebugPostEditPromptTemplate}
                setDebugRequest={setDebugRequest}
                setDebugResponse={setDebugResponse}
            />
        );
    }

    return (
        <div className="app-container">
            <div className="app-shell">
                <header className="topbar no-select">
                    <div className="brand-group">
                        <img className="brand-mark" src={brandLogo} alt="DKST logo" />
                        <div>
                            <div className="brand-title">DKST Translator AI</div>
                            <div className="brand-subtitle">Professional local LLM translation workspace for focused AI-assisted translation</div>
                        </div>
                    </div>
                    <div className="model-popover-wrap" ref={modelPopoverRef}>
                        <button
                            type="button"
                            className={`activity-pill ${isTranslating ? "is-busy" : ""}`}
                            onClick={() => {
                                setShowModelPopover(prev => {
                                    const next = !prev;
                                    if (next) {
                                        setSuppressPromptTooltips(false);
                                    }
                                    return next;
                                });
                            }}
                            title="Select Model"
                        >
                            {activityLabel}
                        </button>
                        {showModelPopover && (
                            <div className="model-popover">
                                {models.length > 0 ? (
                                    models.map(model => (
                                        <button
                                            key={model.id}
                                            type="button"
                                            className={`model-popover-item ${providerSettings.model === model.id ? "is-selected" : ""}`}
                                            onClick={() => handleSelectModel(model.id)}
                                        >
                                            <span className="model-popover-name">{model.displayName || model.id}</span>
                                            {providerSettings.model === model.id && (
                                                <span className="material-symbols-outlined model-popover-check">check</span>
                                            )}
                                        </button>
                                    ))
                                ) : (
                                    <div className="model-popover-empty">No models loaded</div>
                                )}
                            </div>
                        )}
                    </div>
                    <div className="toolbar-group">
                        <button className="icon-btn topbar-btn" onClick={() => setEditorFontSize(prev => clampFontSize(prev - 1))} title="Decrease Font Size">
                            <span className="material-symbols-outlined">remove</span>
                        </button>
                        <button className="icon-btn topbar-btn" onClick={() => setEditorFontSize(prev => clampFontSize(prev + 1))} title="Increase Font Size">
                            <span className="material-symbols-outlined">add</span>
                        </button>
                    </div>
                    <button className="icon-btn topbar-btn" onClick={() => setShowModelModal(true)} title="Model Settings">
                        <span className="material-symbols-outlined">settings</span>
                    </button>
                </header>

                <div className="status-bar">
                    <span className="status-message">{statusMessage}</span>
                    <div className={`status-summary-shell ${showStatusSummary ? "is-open" : "is-closed"}`}>
                        <button
                            className="status-summary-toggle"
                            onClick={() => setShowStatusSummary(prev => !prev)}
                            title={showStatusSummary ? "Hide Summary" : "Show Summary"}
                        >
                            <span className={`material-symbols-outlined status-summary-icon ${showStatusSummary ? "is-open" : ""}`}>expand_circle_right</span>
                        </button>
                        <div className={`status-summary ${showStatusSummary ? "is-open" : ""}`}>
                            <span className="status-summary-text">
                                Source: {sourceLang} | Target: {targetLang} | Reasoning: {providerSettings.reasoning || "auto"} | Temperature: {temperatureLabel} | Proofread: {providerSettings.enablePostEdit ? "on" : "off"} | Smart Chunking: {providerSettings.enableSmartChunking ? `on ${providerSettings.smartChunkSize}` : "off"} | Prompt: {instruction ? "set" : "empty"} |
                            </span>
                            {!isBrowserMode && <div className="status-summary-actions">
                                <button className={`debug-toggle ${showDebugPanel ? "is-on" : "is-off"}`} onClick={() => setShowDebugPanel((prev: boolean) => !prev)}>
                                    Debug: {showDebugPanel ? "on" : "off"}
                                </button>
                                <button className="debug-open-btn" onClick={handleOpenDebugStudioWindow}>
                                    Open Debug Studio
                                </button>
                            </div>}
                        </div>
                    </div>
                </div>

                <section className="prompt-hero" style={{ ["--editor-font-size" as string]: `${editorFontSize}px` }}>
                    <div className="prompt-card">
                        <textarea
                            ref={promptInputRef}
                            style={{ fontSize: `${editorFontSize}px` }}
                            className="prompt-input"
                            placeholder="Enter translation style, protected terms, or Markdown preservation rules."
                            value={instruction}
                            onChange={e => setInstruction(e.target.value)}
                        />
                        {isCompactPromptLayout && (
                            <>
                                <div className="prompt-controls prompt-controls-main is-compact">
                                    <div className="prompt-select-chip prompt-select-chip-language">
                                        <select
                                            className="prompt-chip-select language-select"
                                            value={sourceLang}
                                            onChange={e => setSourceLang(e.target.value)}
                                        >
                                            {SOURCE_LANGUAGES.map(language => (
                                                <option key={language} value={language}>{language === "auto" ? "Auto Detect" : language}</option>
                                            ))}
                                        </select>
                                    </div>
                                    <div className="prompt-select-chip prompt-select-chip-language">
                                        <select
                                            className="prompt-chip-select language-select"
                                            value={targetLang}
                                            onChange={e => setTargetLang(e.target.value)}
                                        >
                                            {TARGET_LANGUAGES.map(language => (
                                                <option key={language} value={language}>{language}</option>
                                            ))}
                                        </select>
                                    </div>
                                </div>
                                <div className="prompt-controls prompt-controls-mobile-actions">
                                    <button
                                        type="button"
                                        className={`prompt-options-toggle ${isPromptOptionsExpanded ? "is-open" : ""}`}
                                        onClick={() => setIsPromptOptionsExpanded(prev => !prev)}
                                        title={isPromptOptionsExpanded ? "Hide Options" : "Show Options"}
                                    >
                                        <span className="material-symbols-outlined">keyboard_double_arrow_up</span>
                                    </button>
                                    <button className="swap-btn" onClick={handleSwapLanguages} title="Swap Languages">
                                        <span className="material-symbols-outlined">swap_horiz</span>
                                    </button>
                                    <button
                                        className={primaryActionClassName}
                                        onClick={handleTranslate}
                                        disabled={!canTranslate}
                                    >
                                        <span className="material-symbols-outlined">{primaryActionIcon}</span>
                                        <span className="hero-translate-label">{primaryActionLabel}</span>
                                    </button>
                                </div>
                            </>
                        )}
                        <div className={`prompt-controls prompt-controls-secondary ${isCompactPromptLayout ? "is-compact" : ""}`}>
                            <div className={`prompt-secondary-panel ${!isCompactPromptLayout || isPromptOptionsExpanded ? "is-open" : ""}`}>
                                <div className="prompt-secondary-group prompt-secondary-group-selects">
                                    <button
                                        type="button"
                                        className="prompt-option-button prompt-inline-action prompt-tooltip"
                                        onClick={() => {
                                            setShowTemperatureSlider(false);
                                            setPromptSelectionModal({ type: "preset" });
                                        }}
                                        data-tooltip="Prompt Preset"
                                    >
                                        <span className="material-symbols-outlined prompt-inline-feature-icon">list_alt_add</span>
                                    </button>
                                    {showReasoningControl && (
                                        <button
                                            type="button"
                                            className="prompt-option-button prompt-inline-action prompt-tooltip"
                                            onClick={() => {
                                                setShowTemperatureSlider(false);
                                                setPromptSelectionModal({ type: "reasoning" });
                                            }}
                                            data-tooltip="Reasoning"
                                        >
                                            <span className="material-symbols-outlined prompt-inline-feature-icon">lightbulb</span>
                                            <span className="prompt-option-value">{reasoningLabel}</span>
                                        </button>
                                    )}
                                    {showTemperatureControl && (
                                        <div
                                            className={`temperature-control prompt-tooltip ${showTemperatureSlider || suppressPromptTooltips ? "is-disabled" : ""}`}
                                            ref={temperatureControlRef}
                                            data-tooltip="Temperature"
                                        >
                                            <button
                                                type="button"
                                                className={`temperature-trigger temperature-trigger-inline ${showTemperatureSlider ? "is-open" : ""}`}
                                                onClick={() => {
                                                    if (isCompactPromptLayout) {
                                                        setPromptSelectionModal(null);
                                                        setShowTemperatureSlider(true);
                                                        setSuppressPromptTooltips(false);
                                                        return;
                                                    }
                                                    setShowTemperatureSlider(prev => {
                                                        const next = !prev;
                                                        setSuppressPromptTooltips(next);
                                                        return next;
                                                    });
                                                }}
                                            >
                                                <span className="material-symbols-outlined temperature-icon">device_thermostat</span>
                                                <strong className="temperature-value">{temperatureLabel}</strong>
                                                {!isCompactPromptLayout && (
                                                    <span className="material-symbols-outlined prompt-inline-chevron">expand_more</span>
                                                )}
                                            </button>
                                            {showTemperatureSlider && !isCompactPromptLayout && (
                                                <div className="temperature-popover">
                                                    <div className="temperature-popover-header">
                                                        <span>Temperature</span>
                                                        <strong>{temperatureLabel}</strong>
                                                    </div>
                                                    <input
                                                        className="temperature-slider"
                                                        type="range"
                                                        min={MIN_TEMPERATURE}
                                                        max={MAX_TEMPERATURE}
                                                        step={TEMPERATURE_STEP}
                                                        value={providerSettings.temperature}
                                                        onChange={e => setProviderSettings(prev => ({
                                                            ...prev,
                                                            temperature: clampTemperature(Number.parseFloat(e.target.value)),
                                                        }))}
                                                    />
                                                    <div className="temperature-scale">
                                                        <span>Auto</span>
                                                        <span>1.0</span>
                                                    </div>
                                                </div>
                                            )}
                                        </div>
                                    )}
                                </div>
                                {!isCompactPromptLayout && (
                                    <div className="prompt-secondary-group prompt-secondary-group-main">
                                        <div className="prompt-select-chip prompt-select-chip-language">
                                            <select
                                                className="prompt-chip-select language-select"
                                                value={sourceLang}
                                                onChange={e => setSourceLang(e.target.value)}
                                            >
                                                {SOURCE_LANGUAGES.map(language => (
                                                    <option key={language} value={language}>{language === "auto" ? "Auto Detect" : language}</option>
                                                ))}
                                            </select>
                                        </div>
                                        <button className="swap-btn" onClick={handleSwapLanguages} title="Swap Languages">
                                            <span className="material-symbols-outlined">swap_horiz</span>
                                        </button>
                                        <div className="prompt-select-chip prompt-select-chip-language">
                                            <select
                                                className="prompt-chip-select language-select"
                                                value={targetLang}
                                                onChange={e => setTargetLang(e.target.value)}
                                            >
                                                {TARGET_LANGUAGES.map(language => (
                                                    <option key={language} value={language}>{language}</option>
                                                ))}
                                            </select>
                                        </div>
                                        <button
                                            className={primaryActionClassName}
                                            onClick={handleTranslate}
                                            disabled={!canTranslate}
                                        >
                                            <span className="material-symbols-outlined">{primaryActionIcon}</span>
                                            <span className="hero-translate-label">{primaryActionLabel}</span>
                                        </button>
                                    </div>
                                )}
                                <div className="prompt-secondary-group prompt-secondary-group-toggles">
                                    <button
                                        type="button"
                                        className={`prompt-inline-action prompt-inline-action-proofread prompt-tooltip ${providerSettings.enablePostEdit ? "is-on" : ""}`}
                                        onClick={() => setProviderSettings(prev => ({ ...prev, enablePostEdit: !prev.enablePostEdit }))}
                                        data-tooltip="Proofread After Translation"
                                    >
                                        <span className="material-symbols-outlined prompt-inline-feature-icon">grading</span>
                                        <span className="material-symbols-outlined prompt-inline-toggle-icon">
                                            {providerSettings.enablePostEdit ? "toggle_on" : "toggle_off"}
                                        </span>
                                    </button>
                                    <button
                                        type="button"
                                        className={`prompt-inline-action prompt-inline-action-enhanced prompt-tooltip ${providerSettings.enableEnhancedContextTranslation ? "is-on" : ""}`}
                                        onClick={() => setShowEnhancedContextModal(true)}
                                        data-tooltip="Enhanced Context Translation"
                                    >
                                        <span className="material-symbols-outlined prompt-inline-feature-icon">contextual_token_add</span>
                                        <span className="material-symbols-outlined prompt-inline-toggle-icon">
                                            {providerSettings.enableEnhancedContextTranslation ? "toggle_on" : "toggle_off"}
                                        </span>
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>
                </section>

                <main>
                    <div className="pane pane-left">
                        <div className="pane-label">
                            Source
                            <div className="pane-actions">
                                <button onClick={handleClearSource} title="Clear Source" disabled={!sourceText}>
                                    <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>delete_sweep</span>
                                </button>
                                <button onClick={handlePasteSource} title="Paste Source">
                                    <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>content_paste_go</span>
                                </button>
                                <button onClick={handleOpenSourceEditorModal} title="Open Source Editor">
                                    <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>edit_note</span>
                                </button>
                                <button onClick={handleOpenFile} title="Open File">
                                    <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>folder_open</span>
                                </button>
                            </div>
                        </div>
                        <div className="pane-body">
                            <textarea
                                style={{ fontSize: `${editorFontSize}px` }}
                                placeholder="Paste or open source text..."
                                value={sourceText}
                                onChange={e => setSourceText(e.target.value)}
                            />
                            <div className="pane-stats">{sourceStats}</div>
                        </div>
                    </div>

                    <div className="pane pane-right">
                        <div className="pane-label">
                            Translation
                            <div className="pane-actions">
                                <button onClick={handleCopyTranslation} title="Copy Translation">
                                    <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>copy_all</span>
                                </button>
                                <button onClick={handleOpenTranslationSearchModal} title="Search Translation">
                                    <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>search</span>
                                </button>
                                <button onClick={handleOpenTranslationViewerModal} title="Open Translation Viewer">
                                    <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>pageview</span>
                                </button>
                                <button onClick={handleSaveFile} title="Save to File">
                                    <span className="material-symbols-outlined" style={{ fontSize: '18px' }}>save</span>
                                </button>
                            </div>
                        </div>
                        <div className="pane-body">
                            <div className="translation-output markdown-output" ref={outputRef} style={{ fontSize: `${editorFontSize}px` }}>
                                {renderMarkdown(translation)}
                                {isTranslating && <span className="cursor">|</span>}
                            </div>
                            {translationSearchQuery && translationSearchMatchCount > 0 && (
                                <div className="translation-search-nav" role="status" aria-live="polite">
                                    <button type="button" onClick={() => handleMoveTranslationSearch("prev")} title="Previous match">
                                        <span className="material-symbols-outlined">arrow_upward</span>
                                    </button>
                                    <div className="translation-search-nav-count">
                                        {translationSearchActiveIndex + 1}/{translationSearchMatchCount}
                                    </div>
                                    <button type="button" onClick={() => handleMoveTranslationSearch("next")} title="Next match">
                                        <span className="material-symbols-outlined">arrow_downward</span>
                                    </button>
                                    <button type="button" onClick={closeTranslationSearch} title="Close search">
                                        <span className="material-symbols-outlined">close</span>
                                    </button>
                                </div>
                            )}
                            <div className="pane-stats">{translationStats}</div>
                        </div>
                    </div>
                </main>

                {showTranslationSearchModal && (
                    <div className="modal-overlay modal-overlay-centered modal-overlay-top" onClick={() => setShowTranslationSearchModal(false)}>
                        <div className="modal-card translation-search-modal" onClick={e => e.stopPropagation()}>
                            <div className="modal-header">
                                <div>
                                    <div className="modal-title">Search Translation</div>
                                    <div className="modal-subtitle">Find a word or phrase in the translated result.</div>
                                </div>
                                <button className="icon-btn" onClick={() => setShowTranslationSearchModal(false)} title="Close">
                                    <span className="material-symbols-outlined">close</span>
                                </button>
                            </div>
                            <div className="modal-body translation-search-modal-body">
                                <input
                                    type="text"
                                    className="translation-search-input"
                                    value={translationSearchDraft}
                                    onChange={e => setTranslationSearchDraft(e.target.value)}
                                    onKeyDown={e => {
                                        if (e.key === "Enter") {
                                            e.preventDefault();
                                            handleSubmitTranslationSearch();
                                        }
                                    }}
                                    placeholder="Enter search text"
                                    autoFocus
                                />
                                <div className="translation-search-actions">
                                    <button className="btn btn-secondary btn-small" type="button" onClick={() => setShowTranslationSearchModal(false)}>
                                        Cancel
                                    </button>
                                    <button className="btn btn-primary btn-small" type="button" onClick={handleSubmitTranslationSearch}>
                                        Search
                                    </button>
                                </div>
                            </div>
                        </div>
                    </div>
                )}

                {promptSelectionModal && (
                    <div className="modal-overlay" onClick={() => setPromptSelectionModal(null)}>
                        <div className="modal-card prompt-picker-modal" onClick={e => e.stopPropagation()}>
                            <div className="modal-header">
                                <div>
                                    <div className="modal-title">
                                        {promptSelectionModal.type === "preset" ? "Prompt Presets" : "Reasoning"}
                                    </div>
                                    <div className="modal-subtitle">
                                        {promptSelectionModal.type === "preset"
                                            ? "Choose a preset to fill the prompt field."
                                            : "Choose how much reasoning the model should use."}
                                    </div>
                                </div>
                                <button className="icon-btn" onClick={() => setPromptSelectionModal(null)} title="Close">
                                    <span className="material-symbols-outlined">close</span>
                                </button>
                            </div>
                            <div className="modal-body prompt-picker-body">
                                {promptSelectionModal.type === "preset" ? (
                                    <div className="prompt-picker-list">
                                        {INSTRUCTION_PRESETS.map(preset => {
                                            const isSelected = preset.id === instructionPresetValue;
                                            return (
                                                <button
                                                    key={preset.id}
                                                    type="button"
                                                    className={`prompt-picker-item ${isSelected ? "is-selected" : ""}`}
                                                    onClick={() => handleSelectPromptPreset(preset.id)}
                                                >
                                                    <span className="prompt-picker-item-label">{preset.label}</span>
                                                    <span className="prompt-picker-item-text">{preset.instruction}</span>
                                                </button>
                                            );
                                        })}
                                    </div>
                                ) : (
                                    <div className="prompt-picker-list">
                                        <button
                                            type="button"
                                            className={`prompt-picker-item ${!providerSettings.reasoning ? "is-selected" : ""}`}
                                            onClick={() => handleSelectReasoningOption("")}
                                        >
                                            <span className="prompt-picker-item-label">Auto</span>
                                            <span className="prompt-picker-item-text">Let the model choose the best reasoning level.</span>
                                        </button>
                                        {reasoningOptions.map(option => (
                                            <button
                                                key={option}
                                                type="button"
                                                className={`prompt-picker-item ${providerSettings.reasoning === option ? "is-selected" : ""}`}
                                                onClick={() => handleSelectReasoningOption(option)}
                                            >
                                                <span className="prompt-picker-item-label">{option}</span>
                                            </button>
                                        ))}
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                )}

                {isCompactPromptLayout && showTemperatureSlider && (
                    <div className="modal-overlay" onClick={() => setShowTemperatureSlider(false)}>
                        <div className="modal-card prompt-picker-modal temperature-modal-card" onClick={e => e.stopPropagation()}>
                            <div className="modal-header">
                                <div>
                                    <div className="modal-title">Temperature</div>
                                    <div className="modal-subtitle">Adjust creativity for translation output.</div>
                                </div>
                                <button className="icon-btn" onClick={() => setShowTemperatureSlider(false)} title="Close">
                                    <span className="material-symbols-outlined">close</span>
                                </button>
                            </div>
                            <div className="modal-body temperature-modal-body">
                                <div className="temperature-popover-header">
                                    <span>Temperature</span>
                                    <strong>{temperatureLabel}</strong>
                                </div>
                                <input
                                    className="temperature-slider"
                                    type="range"
                                    min={MIN_TEMPERATURE}
                                    max={MAX_TEMPERATURE}
                                    step={TEMPERATURE_STEP}
                                    value={providerSettings.temperature}
                                    onChange={e => setProviderSettings(prev => ({
                                        ...prev,
                                        temperature: clampTemperature(Number.parseFloat(e.target.value)),
                                    }))}
                                />
                                <div className="temperature-scale">
                                    <span>Auto</span>
                                    <span>1.0</span>
                                </div>
                            </div>
                        </div>
                    </div>
                )}

                {showSourceEditorModal && (
                    <div className="modal-overlay modal-overlay-centered modal-overlay-fullscreen" onClick={handleCloseSourceEditorModal}>
                        <div className="modal-card modal-card-fullscreen" onClick={e => e.stopPropagation()}>
                            <div className="modal-header">
                                <div className="modal-title">Edit Source Text</div>
                                <div className="modal-header-actions">
                                    <button className="icon-btn" onClick={() => setEditorFontSize(prev => clampFontSize(prev - 1))} title="Decrease Font Size">
                                        <span className="material-symbols-outlined">remove</span>
                                    </button>
                                    <button className="icon-btn" onClick={() => setEditorFontSize(prev => clampFontSize(prev + 1))} title="Increase Font Size">
                                        <span className="material-symbols-outlined">add</span>
                                    </button>
                                    <button className="icon-btn" onClick={handleClearSourceEditorDraft} title="Clear Source">
                                        <span className="material-symbols-outlined">delete_sweep</span>
                                    </button>
                                    <button className="icon-btn" onClick={handlePasteIntoSourceEditorDraft} title="Paste Source">
                                        <span className="material-symbols-outlined">content_paste_go</span>
                                    </button>
                                    <button className="icon-btn" onClick={handleOpenFileIntoSourceEditorDraft} title="Open File">
                                        <span className="material-symbols-outlined">folder_open</span>
                                    </button>
                                    <button className="btn btn-secondary btn-small modal-ok-btn" onClick={handleCloseSourceEditorModal} title="Save changes and close">
                                        OK
                                    </button>
                                </div>
                            </div>
                            <div className="modal-body modal-body-fullscreen">
                                <textarea
                                    className="fullscreen-textarea"
                                    style={{ fontSize: `${editorFontSize}px` }}
                                    value={sourceEditorDraft}
                                    onChange={e => setSourceEditorDraft(e.target.value)}
                                    placeholder="Write or paste source text..."
                                />
                            </div>
                        </div>
                    </div>
                )}

                {showTranslationViewerModal && (
                    <div className="modal-overlay modal-overlay-centered modal-overlay-fullscreen" onClick={handleCloseTranslationViewerModal}>
                        <div className="modal-card modal-card-fullscreen" onClick={e => e.stopPropagation()}>
                            <div className="modal-header">
                                <div className="modal-title">Translation Preview</div>
                                <div className="modal-header-actions">
                                    <button className="icon-btn" onClick={() => setEditorFontSize(prev => clampFontSize(prev - 1))} title="Decrease Font Size">
                                        <span className="material-symbols-outlined">remove</span>
                                    </button>
                                    <button className="icon-btn" onClick={() => setEditorFontSize(prev => clampFontSize(prev + 1))} title="Increase Font Size">
                                        <span className="material-symbols-outlined">add</span>
                                    </button>
                                    <button className="icon-btn" onClick={handleCopyTranslation} title="Copy Translation">
                                        <span className="material-symbols-outlined">copy_all</span>
                                    </button>
                                    <button className="icon-btn" onClick={handleOpenTranslationSearchModal} title="Search Translation">
                                        <span className="material-symbols-outlined">search</span>
                                    </button>
                                    <button className="icon-btn" onClick={handleSaveFile} title="Save to File">
                                        <span className="material-symbols-outlined">save</span>
                                    </button>
                                    <button className="icon-btn" onClick={handleCloseTranslationViewerModal} title="Close">
                                        <span className="material-symbols-outlined">close</span>
                                    </button>
                                </div>
                            </div>
                            <div className="modal-body modal-body-fullscreen translation-viewer-body">
                                <div
                                    ref={translationViewerRef}
                                    className="translation-output markdown-output fullscreen-viewer"
                                    style={{ fontSize: `${editorFontSize}px` }}
                                >
                                    {renderMarkdown(cleanedTranslation)}
                                </div>
                                {translationSearchQuery && translationSearchMatchCount > 0 && (
                                    <div className="translation-search-nav" role="status" aria-live="polite">
                                        <button type="button" onClick={() => handleMoveTranslationSearch("prev")} title="Previous match">
                                            <span className="material-symbols-outlined">arrow_upward</span>
                                        </button>
                                        <div className="translation-search-nav-count">
                                            {translationSearchActiveIndex + 1}/{translationSearchMatchCount}
                                        </div>
                                        <button type="button" onClick={() => handleMoveTranslationSearch("next")} title="Next match">
                                            <span className="material-symbols-outlined">arrow_downward</span>
                                        </button>
                                        <button type="button" onClick={closeTranslationSearch} title="Close search">
                                            <span className="material-symbols-outlined">close</span>
                                        </button>
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                )}

                {showModelModal && (
                    <div className="modal-overlay modal-overlay-centered" onClick={() => setShowModelModal(false)}>
                        <div className={`modal-card ${isBrowserMode ? "" : "modal-card-settings"}`} onClick={e => e.stopPropagation()}>
                            <div className="modal-header">
                                <div>
                                    <div className="modal-title">LLM Settings</div>
                                    <div className="modal-subtitle">
                                        {isBrowserMode
                                            ? "Use the host application's LLM connection and adjust only browser-appropriate translation options."
                                            : "Configure translation and web access side by side."}
                                    </div>
                                </div>
                                <button className="btn btn-secondary btn-small" onClick={() => {
                                    setShowModelModal(false);
                                    showSavedToastMessage("Saved");
                                }} title="OK">
                                    OK
                                </button>
                            </div>
                            <div className="modal-body">
                                {isBrowserMode ? (
                                    <div className="settings-grid">
                                        <div className="settings-field">
                                            <span>Model</span>
                                            <div className="toolbar-group model-group">
                                                <select value={providerSettings.model} onChange={e => {
                                                    handleSelectModel(e.target.value);
                                                }} disabled={isLoadingModels}>
                                                    <option value="">Select a Model</option>
                                                    {models.map(model => (
                                                        <option key={model.id} value={model.id}>
                                                            {model.displayName || model.id}
                                                        </option>
                                                    ))}
                                                </select>
                                                <button className="icon-btn" onClick={fetchModels} disabled={isLoadingModels} title="Refresh Models">
                                                    <span className="material-symbols-outlined">refresh</span>
                                                </button>
                                            </div>
                                        </div>
                                        <label className="settings-field">
                                            <span>Smart Chunking</span>
                                            <label className="settings-checkbox">
                                                <input
                                                    type="checkbox"
                                                    checked={providerSettings.enableSmartChunking}
                                                    onChange={e => setProviderSettings(prev => ({
                                                        ...prev,
                                                        enableSmartChunking: e.target.checked,
                                                    }))}
                                                />
                                                <span>Enable smart chunking</span>
                                            </label>
                                        </label>
                                        <label className="settings-field">
                                            <span>Smart Chunk Size</span>
                                            <input
                                                type="text"
                                                inputMode="numeric"
                                                pattern="[0-9]*"
                                                value={String(providerSettings.smartChunkSize)}
                                                disabled={!providerSettings.enableSmartChunking}
                                                onChange={e => {
                                                    const digitsOnly = e.target.value.replace(/[^\d]/g, "");
                                                    setProviderSettings(prev => ({
                                                        ...prev,
                                                        smartChunkSize: digitsOnly === "" ? 2000 : Math.max(200, Number.parseInt(digitsOnly, 10) || 2000),
                                                    }));
                                                }}
                                                placeholder="2000"
                                            />
                                        </label>
                                        <label className="settings-checkbox">
                                            <input
                                                type="checkbox"
                                                checked={providerSettings.forceShowReasoning}
                                                onChange={e => setProviderSettings(prev => ({
                                                    ...prev,
                                                    forceShowReasoning: e.target.checked,
                                                }))}
                                            />
                                            <span>Force show reasoning control</span>
                                        </label>
                                        <label className="settings-checkbox">
                                            <input
                                                type="checkbox"
                                                checked={providerSettings.enableTopicAwarePostEdit}
                                                onChange={e => setProviderSettings(prev => ({
                                                    ...prev,
                                                    enableTopicAwarePostEdit: e.target.checked,
                                                }))}
                                            />
                                            <span>Context-Aware Smart Post-Editing</span>
                                        </label>
                                        <label className="settings-checkbox">
                                            <input
                                                type="checkbox"
                                                checked={providerSettings.forceShowTemperature}
                                                onChange={e => {
                                                    const isVisible = e.target.checked;
                                                    setProviderSettings(prev => ({
                                                        ...prev,
                                                        forceShowTemperature: isVisible,
                                                    }));
                                                    if (!isVisible) {
                                                        setShowTemperatureSlider(false);
                                                    }
                                                }}
                                            />
                                            <span>Show temperature control</span>
                                        </label>
                                        {settingsStatus && (
                                            <div className={`settings-status ${settingsStatus.toLowerCase().includes("could not load") ? "is-error" : "is-ok"}`}>
                                                {settingsStatus}
                                            </div>
                                        )}
                                    </div>
                                ) : (
                                    <div className="settings-columns">
                                        <section className="settings-panel">
                                            <div className="settings-panel-header">
                                                <div className="settings-panel-title">LLM</div>
                                                <div className="settings-panel-subtitle">Provider, model, and translation behavior</div>
                                            </div>
                                            <div className="settings-grid">
                                                <label className="settings-field">
                                                    <span>Mode</span>
                                                    <select value={providerSettings.mode} onChange={e => setProviderSettings(prev => ({
                                                        ...prev,
                                                        mode: e.target.value as ProviderMode,
                                                        model: "",
                                                    }))}>
                                                        <option value="lmstudio">LM Studio</option>
                                                        <option value="openai">OpenAI</option>
                                                    </select>
                                                </label>
                                                <label className="settings-field">
                                                    <span>Endpoint</span>
                                                    <input
                                                        type="text"
                                                        value={providerSettings.endpoint}
                                                        onChange={e => setProviderSettings(prev => ({ ...prev, endpoint: e.target.value }))}
                                                        placeholder="http://127.0.0.1:1234"
                                                    />
                                                </label>
                                                <label className="settings-field">
                                                    <span>API Key</span>
                                                    <input
                                                        type="password"
                                                        value={providerSettings.apiKey}
                                                        onChange={e => setProviderSettings(prev => ({ ...prev, apiKey: e.target.value }))}
                                                        placeholder={providerSettings.mode === "lmstudio" ? "Required if your local gateway enforces auth" : "Required for OpenAI"}
                                                    />
                                                </label>
                                                <div className="settings-field">
                                                    <span>Model</span>
                                                    <div className="toolbar-group model-group">
                                                        <select value={providerSettings.model} onChange={e => {
                                                            handleSelectModel(e.target.value);
                                                        }} disabled={isLoadingModels}>
                                                            <option value="">Select a Model</option>
                                                            {models.map(model => (
                                                                <option key={model.id} value={model.id}>
                                                                    {model.displayName || model.id}
                                                                </option>
                                                            ))}
                                                        </select>
                                                        <button className="icon-btn" onClick={fetchModels} disabled={isLoadingModels} title="Refresh Models">
                                                            <span className="material-symbols-outlined">refresh</span>
                                                        </button>
                                                    </div>
                                                </div>
                                                <label className="settings-field">
                                                    <span>Smart Chunking</span>
                                                    <label className="settings-checkbox">
                                                        <input
                                                            type="checkbox"
                                                            checked={providerSettings.enableSmartChunking}
                                                            onChange={e => setProviderSettings(prev => ({
                                                                ...prev,
                                                                enableSmartChunking: e.target.checked,
                                                            }))}
                                                        />
                                                        <span>Enable smart chunking</span>
                                                    </label>
                                                </label>
                                                <label className="settings-field">
                                                    <span>Smart Chunk Size</span>
                                                    <input
                                                        type="text"
                                                        inputMode="numeric"
                                                        pattern="[0-9]*"
                                                        value={String(providerSettings.smartChunkSize)}
                                                        disabled={!providerSettings.enableSmartChunking}
                                                        onChange={e => {
                                                            const digitsOnly = e.target.value.replace(/[^\d]/g, "");
                                                            setProviderSettings(prev => ({
                                                                ...prev,
                                                                smartChunkSize: digitsOnly === "" ? 2000 : Math.max(200, Number.parseInt(digitsOnly, 10) || 2000),
                                                            }));
                                                        }}
                                                        placeholder="2000"
                                                    />
                                                </label>
                                                <label className="settings-checkbox">
                                                    <input
                                                        type="checkbox"
                                                        checked={providerSettings.forceShowReasoning}
                                                        onChange={e => setProviderSettings(prev => ({
                                                            ...prev,
                                                            forceShowReasoning: e.target.checked,
                                                        }))}
                                                    />
                                                    <span>Force show reasoning control</span>
                                                </label>
                                                <label className="settings-checkbox">
                                                    <input
                                                        type="checkbox"
                                                        checked={providerSettings.enableTopicAwarePostEdit}
                                                        onChange={e => setProviderSettings(prev => ({
                                                            ...prev,
                                                            enableTopicAwarePostEdit: e.target.checked,
                                                        }))}
                                                    />
                                                    <span>Context-Aware Smart Post-Editing</span>
                                                </label>
                                                <label className="settings-checkbox">
                                                    <input
                                                        type="checkbox"
                                                        checked={providerSettings.forceShowTemperature}
                                                        onChange={e => {
                                                            const isVisible = e.target.checked;
                                                            setProviderSettings(prev => ({
                                                                ...prev,
                                                                forceShowTemperature: isVisible,
                                                            }));
                                                            if (!isVisible) {
                                                                setShowTemperatureSlider(false);
                                                            }
                                                        }}
                                                    />
                                                    <span>Show temperature control</span>
                                                </label>
                                                {settingsStatus && (
                                                    <div className={`settings-status ${settingsStatus.toLowerCase().includes("could not load") ? "is-error" : "is-ok"}`}>
                                                        {settingsStatus}
                                                    </div>
                                                )}
                                            </div>
                                        </section>
                                        <section className="settings-panel">
                                            <div className="settings-panel-header">
                                                <div className="settings-panel-title">Server</div>
                                                <div className="settings-panel-subtitle">Browser access, auth, and TLS certificates</div>
                                            </div>
                                            <div className="settings-grid">
                                                <div className="settings-field">
                                                    <span>Web Server</span>
                                                    <label className="settings-checkbox">
                                                        <input
                                                            type="checkbox"
                                                            checked={webServerSettings.enabled}
                                                            onChange={e => setWebServerSettings(prev => ({ ...prev, enabled: e.target.checked }))}
                                                            disabled={isBrowserMode}
                                                        />
                                                        <span>Enable web server access</span>
                                                    </label>
                                                </div>
                                                <label className="settings-field">
                                                    <span>Web Server Port</span>
                                                    <input
                                                        type="text"
                                                        inputMode="numeric"
                                                        pattern="[0-9]*"
                                                        value={webServerSettings.port}
                                                        onChange={e => setWebServerSettings(prev => ({
                                                            ...prev,
                                                            port: e.target.value.replace(/[^\d]/g, "") || "8080",
                                                        }))}
                                                        disabled={isBrowserMode}
                                                        placeholder="8080"
                                                    />
                                                </label>
                                                <div className="settings-field">
                                                    <span>Web Server Password</span>
                                                    <div className="settings-inline-action">
                                                        <input
                                                            type="password"
                                                            value={webServerPasswordDraft}
                                                            onChange={e => setWebServerPasswordDraft(e.target.value)}
                                                            disabled={isBrowserMode}
                                                            placeholder={webServerSettings.hasPassword ? "Leave blank to keep current password" : "Required when enabling the web server"}
                                                        />
                                                        <button
                                                            className="btn btn-secondary btn-small"
                                                            type="button"
                                                            onClick={handleSaveWebServerSettings}
                                                            disabled={isBrowserMode || isSavingWebServerSettings || webServerPasswordDraft.trim() === ""}
                                                            title="Save Password"
                                                        >
                                                            {isSavingWebServerSettings ? "Saving..." : "Save"}
                                                        </button>
                                                    </div>
                                                </div>
                                                <div className="settings-field">
                                                    <span>TLS / HTTPS</span>
                                                    <label className="settings-checkbox">
                                                        <input
                                                            type="checkbox"
                                                            checked={webServerSettings.useTls}
                                                            onChange={e => setWebServerSettings(prev => ({ ...prev, useTls: e.target.checked }))}
                                                            disabled={isBrowserMode}
                                                        />
                                                        <span>Use certificate files from DKST Translator AI/certs</span>
                                                    </label>
                                                </div>
                                                <label className="settings-field">
                                                    <span>Certificate Domain</span>
                                                    <input
                                                        type="text"
                                                        value={webServerSettings.certDomain}
                                                        onChange={e => setWebServerSettings(prev => ({ ...prev, certDomain: e.target.value }))}
                                                        disabled={isBrowserMode || !webServerSettings.useTls}
                                                        placeholder="localhost"
                                                    />
                                                </label>
                                                <div className="settings-field">
                                                    <span>Certificate Folder</span>
                                                    <div className="settings-note">{webServerSettings.certificateDirectory || "Loaded when the desktop app starts."}</div>
                                                    <div className="toolbar-group model-group">
                                                        <button className="btn btn-secondary btn-small" type="button" onClick={handleOpenCertificateFolder} disabled={isBrowserMode}>
                                                            Open Folder
                                                        </button>
                                                        <button className="btn btn-secondary btn-small" type="button" onClick={handleSaveWebServerSettings} disabled={isBrowserMode || isSavingWebServerSettings}>
                                                            {isSavingWebServerSettings ? "Saving..." : "Apply Web Server"}
                                                        </button>
                                                    </div>
                                                    <div className="settings-note">
                                                        Copy certificate files into <code>DKST Translator AI/certs</code>. Default names: <code>cert.pem</code> and <code>key.pem</code>. Domain-specific names like <code>{'{domain}.crt'}</code> + <code>{'{domain}.key'}</code> are also supported.
                                                    </div>
                                                    <div className="settings-note">
                                                        Browser trust warnings can still appear unless the certificate is trusted by the operating system and matches the host you use to connect.
                                                    </div>
                                                    {webServerStatus && (
                                                        <div className={`settings-status ${webServerStatus.toLowerCase().includes("could not") ? "is-error" : "is-ok"}`}>
                                                            {webServerStatus}
                                                        </div>
                                                    )}
                                                </div>
                                            </div>
                                        </section>
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                )}
                {showEnhancedContextModal && (
                    <div className="modal-overlay" onClick={handleCloseEnhancedContextModal}>
                        <div className="modal-card modal-card-wide" onClick={e => e.stopPropagation()}>
                            <div className="modal-header">
                                <div>
                                    <div className="modal-title">Enhanced Context Translation</div>
                                    <div className="modal-subtitle">Improve long-form terminology and name consistency with stronger context rules and a user glossary.</div>
                                </div>
                                <button className="btn btn-secondary btn-small" onClick={handleCloseEnhancedContextModal} title="Save changes and close">
                                    OK
                                </button>
                            </div>
                            <div className="modal-body">
                                <div className="settings-grid">
                                    <label className="settings-checkbox">
                                        <input
                                            type="checkbox"
                                            checked={enhancedContextDraftEnabled}
                                            onChange={e => setEnhancedContextDraftEnabled(e.target.checked)}
                                        />
                                        <span>Use Enhanced Context Translation</span>
                                    </label>
                                    <label className="settings-field">
                                        <div className="settings-field-header">
                                            <span>User Glossary</span>
                                            <div className="glossary-toolbar">
                                                <button type="button" className="icon-btn" onClick={handleOpenEnhancedContextGlossary} title="Open Glossary File">
                                                    <span className="material-symbols-outlined">folder_open</span>
                                                </button>
                                                <button type="button" className="icon-btn" onClick={handleSaveEnhancedContextGlossary} title="Save Glossary File">
                                                    <span className="material-symbols-outlined">save</span>
                                                </button>
                                                <button type="button" className="icon-btn" onClick={() => setEnhancedContextDraftGlossary("")} title="Clear Glossary">
                                                    <span className="material-symbols-outlined">delete</span>
                                                </button>
                                            </div>
                                        </div>
                                        <textarea
                                            className="settings-textarea glossary-textarea"
                                            value={enhancedContextDraftGlossary}
                                            onChange={e => setEnhancedContextDraftGlossary(e.target.value)}
                                            placeholder={`One rule per line\nAlice = Alice\nGoogle Search = Google Search\nTypeScript = TypeScript`}
                                        />
                                    </label>
                                    <div className="settings-note">
                                        When enabled, the translator will prioritize consistent names and terminology across long texts, and apply your glossary before generating translations.
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                )}
                {showSavedToast && <div className="settings-toast">{savedToastMessage}</div>}
                <div className={`progress-widget ${progressState.visible ? "show" : ""}`}>
                    <div className="progress-layout">
                        <div className="progress-ring">
                            <svg className="progress-ring-svg" viewBox="0 0 72 72" aria-hidden="true">
                                <circle className="progress-ring-track" cx="36" cy="36" r={progressRingRadius} />
                                <circle
                                    className="progress-ring-progress"
                                    cx="36"
                                    cy="36"
                                    r={progressRingRadius}
                                    style={{
                                        stroke: progressRingColor,
                                        strokeDasharray: progressRingCircumference,
                                        strokeDashoffset: progressRingOffset,
                                    }}
                                />
                            </svg>
                            <div className="progress-ring-inner">
                                <div className="progress-ring-value">{progressPercent}%</div>
                                {progressRingCaption && <div className="progress-ring-caption">{progressRingCaption}</div>}
                            </div>
                        </div>
                        <div className="progress-content">
                            <div className="progress-widget-header">
                                <div className="progress-header-main">
                                    <span className={`progress-title ${progressState.indeterminate ? "shimmering" : ""}`}>
                                        {progressState.label || "Working..."}
                                    </span>
                                </div>
                                {isTranslating && (
                                    <button className="progress-cancel-btn" onClick={handleCancel} title="Stop Translation">
                                        <span className="material-symbols-outlined">close</span>
                                    </button>
                                )}
                            </div>
                            <div className="progress-detail">{progressState.detail || "Waiting for progress events"}</div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}

export default App;
