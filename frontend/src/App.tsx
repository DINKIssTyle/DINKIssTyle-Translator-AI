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

type TranslationChunkPayload = {
    chunk_index?: number;
    phase?: string;
    text?: string;
    final_closed?: boolean;
};

type ReviewOverlayState = {
    visible: boolean;
    hiding: boolean;
    text: string;
};

type OverallProgressEstimate = {
    activeStep: number;
    totalSteps: number;
    stepStartedAt: number;
    lastCompletedStepDurationMs: number;
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
    lastTranslationPromptPreview: string;
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

type RenderedChunkState = {
    displayedText: string;
    draftText: string;
    finalText: string;
    phase: "draft" | "final";
    showDraftSkeleton?: boolean;
    skeletonHiding?: boolean;
};

type WebTranslateResponse = {
    text?: string;
    stats?: TranslationStatsPayload;
};

type PromptSelectionModalState =
    | { type: "preset" }
    | { type: "reasoning" }
    | null;

type GlossaryTab = "user" | "extracted";

type GlossaryRow = {
    id: string;
    source: string;
    target: string;
    frequency?: number;
};

const STORAGE_KEY = "dkst-translator-ai-settings";
const SOURCE_LANGUAGES = ["auto", "English", "Korean", "Japanese", "Chinese", "French", "German"];
const TARGET_LANGUAGES = ["Korean", "English", "Japanese", "Chinese", "French", "German"];
const DEFAULT_REASONING_OPTIONS = ["off", "low", "medium", "high", "on"];
const DEFAULT_EDITOR_FONT_SIZE = 18;
const DRAFT_SKELETON_FADE_MS = 3000;
const MIN_EDITOR_FONT_SIZE = 14;
const MAX_EDITOR_FONT_SIZE = 26;
const DEFAULT_REASONING = "";
const DEFAULT_TEMPERATURE = 0;
const MIN_TEMPERATURE = 0;
const GLOSSARY_STOPWORDS = new Set([
    "the", "and", "that", "with", "from", "this", "have", "were", "their", "there", "into", "they",
    "them", "then", "than", "when", "where", "what", "which", "will", "would", "could", "should",
    "about", "after", "before", "under", "over", "between", "through", "while", "because", "being",
    "been", "just", "very", "more", "most", "some", "such", "only", "also", "onto", "upon", "your",
    "ours", "ourselves", "hers", "him", "his", "her", "its", "our", "for", "are", "was", "is", "to",
    "of", "in", "on", "at", "by", "an", "or", "as", "it", "be", "if", "we", "you", "he", "she", "i",
    "support", "series", "version", "versions", "system", "systems", "computer", "computers", "memory",
    "machine", "machines", "update", "updates", "user", "users", "application", "applications",
    "consumer", "consumers", "design", "constraint", "constraints", "average",
]);
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

function isSelectionInsideNode(selection: Selection | null, node: Node | null): boolean {
    if (!selection || !node) {
        return false;
    }
    const anchorNode = selection.anchorNode;
    const focusNode = selection.focusNode;
    return Boolean(
        (anchorNode && node.contains(anchorNode)) ||
        (focusNode && node.contains(focusNode))
    );
}

async function copyTextWithDomFallback(text: string): Promise<boolean> {
    if (typeof document === "undefined") {
        return false;
    }

    const textArea = document.createElement("textarea");
    textArea.value = text;
    textArea.setAttribute("readonly", "true");
    textArea.setAttribute("aria-hidden", "true");
    textArea.style.position = "fixed";
    textArea.style.top = "0";
    textArea.style.left = "-9999px";
    textArea.style.opacity = "0";
    textArea.style.pointerEvents = "none";
    document.body.appendChild(textArea);
    textArea.focus();
    textArea.select();
    textArea.setSelectionRange(0, text.length);

    try {
        return document.execCommand("copy");
    } catch {
        return false;
    } finally {
        document.body.removeChild(textArea);
    }
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

function createInitialOverallProgressEstimate(): OverallProgressEstimate {
    return {
        activeStep: 0,
        totalSteps: 0,
        stepStartedAt: 0,
        lastCompletedStepDurationMs: 0,
    };
}

function estimateTimedOverallProgress(progressState: Required<ProgressPayload>, estimate: OverallProgressEstimate, now: number): number {
    const baseOverall = Math.max(0, Math.min(1, progressState.overall_progress || 0));
    if (progressState.stage === "done") {
        return 1;
    }
    if (progressState.total_steps <= 0 || progressState.current_step <= 0) {
        return baseOverall;
    }

    const stepSpan = 1 / progressState.total_steps;
    const stepStartOverall = Math.max(0, progressState.current_step - 1) * stepSpan;
    const elapsedMs = estimate.stepStartedAt > 0 ? Math.max(0, now - estimate.stepStartedAt) : 0;
    const expectedDurationMs = estimate.lastCompletedStepDurationMs > 0
        ? estimate.lastCompletedStepDurationMs
        : 4000;
    const timedStepProgress = Math.max(0, Math.min(1, elapsedMs / expectedDurationMs));
    let nextOverall = Math.max(baseOverall, stepStartOverall + timedStepProgress * stepSpan);

    if (progressState.current_step >= progressState.total_steps) {
        nextOverall = Math.min(nextOverall, 0.99);
    }

    return Math.max(baseOverall, Math.min(1, nextOverall));
}

function deriveStageRingProgress(stage: string, progress: number): number {
    const safeProgress = Math.max(0, Math.min(1, progress || 0));
    if (stage === "model_load") {
        return Math.max(0, Math.min(1, safeProgress / 0.35));
    }
    if (stage === "prompt_processing") {
        return Math.max(0, Math.min(1, (safeProgress - 0.4) / 0.4));
    }
    return safeProgress;
}

function sanitizeTranslation(raw: string): string {
    return raw
        .replace(/\\r\\n/g, "\n")
        .replace(/\\n/g, "\n")
        .replace(/\\r/g, "\n")
        .replace(/\\t/g, "\t")
        .replace(/^\s*<<<\s*output\s*>>>\s*/i, "")
        .replace(/<<<\s*\/?output\s*>>>/gi, "")
        .replace(/<<<[^>\n]+>>>/g, "")
        .trim();
}

function formatReviewOverlayText(text: string): string {
    return text.replace(/->/g, "→");
}

function joinRenderedChunks(chunks: RenderedChunkState[]): string {
    let combined = "";
    for (const chunk of chunks) {
        if (!chunk || !chunk.displayedText) {
            continue;
        }
        if (combined && !combined.endsWith("\n") && !chunk.displayedText.startsWith("\n")) {
            combined += "\n\n";
        }
        combined += chunk.displayedText;
    }
    return combined;
}

function createGlossaryRow(source = "", target = "", frequency?: number): GlossaryRow {
    return {
        id: `glossary-${Math.random().toString(36).slice(2, 10)}`,
        source,
        target,
        frequency,
    };
}

function compareGlossarySource(a: GlossaryRow, b: GlossaryRow): number {
    const sourceCompare = a.source.trim().localeCompare(b.source.trim(), undefined, {
        sensitivity: "base",
        numeric: true,
    });
    if (sourceCompare !== 0) {
        return sourceCompare;
    }
    return a.target.trim().localeCompare(b.target.trim(), undefined, {
        sensitivity: "base",
        numeric: true,
    });
}

function normalizeUserGlossaryRows(rows: GlossaryRow[]): GlossaryRow[] {
    const filledRows = rows
        .filter(row => row.source.trim() || row.target.trim())
        .sort(compareGlossarySource);
    return [...filledRows, createGlossaryRow()];
}

function parseGlossaryText(text: string): Array<{ source: string; target: string }> {
    return text
        .split(/\r?\n/)
        .map(line => line.trim())
        .filter(Boolean)
        .map(line => {
            const separatorIndex = line.indexOf("=");
            if (separatorIndex < 0) {
                return null;
            }
            const source = line.slice(0, separatorIndex).trim();
            const target = line.slice(separatorIndex + 1).trim();
            if (!source) {
                return null;
            }
            return { source, target };
        })
        .filter((entry): entry is { source: string; target: string } => Boolean(entry));
}

function serializeGlossaryRows(rows: GlossaryRow[]): string {
    return [...rows]
        .sort(compareGlossarySource)
        .map(row => `${row.source.trim()} = ${row.target.trim()}`)
        .filter(line => line !== "=" && !line.startsWith(" =") && !line.endsWith("= "))
        .join("\n");
}

function buildCombinedGlossary(userRows: GlossaryRow[], extractedRows: GlossaryRow[]): string {
    const combined = new Map<string, GlossaryRow>();

    [...userRows, ...extractedRows].forEach(row => {
        const source = row.source.trim();
        const target = row.target.trim();
        if (!source || !target) {
            return;
        }
        const key = source.toLocaleLowerCase();
        if (!combined.has(key)) {
            combined.set(key, { ...row, source, target });
        }
    });

    return serializeGlossaryRows(Array.from(combined.values()));
}

function shouldCollapseGlossaryVariant(candidate: string, selected: string): boolean {
    const normalizedCandidate = candidate.trim();
    const normalizedSelected = selected.trim();
    if (!normalizedCandidate || !normalizedSelected || normalizedCandidate === normalizedSelected) {
        return false;
    }

    const lowerCandidate = normalizedCandidate.toLocaleLowerCase();
    const lowerSelected = normalizedSelected.toLocaleLowerCase();

    const isChildOfSelected = lowerCandidate.startsWith(`${lowerSelected} `) || lowerCandidate.startsWith(`${lowerSelected}-`);
    const isParentOfSelected = lowerSelected.startsWith(`${lowerCandidate} `) || lowerSelected.startsWith(`${lowerCandidate}-`);

    return isChildOfSelected || isParentOfSelected;
}

function extractFrequentGlossaryCandidates(text: string): GlossaryRow[] {
    const normalized = text.replace(/\r\n/g, "\n");
    const tokenPattern = /[\p{L}\p{N}][\p{L}\p{N}'’-]*/gu;
    const originalByKey = new Map<string, string>();
    const counts = new Map<string, number>();
    const boosts = new Map<string, number>();
    const tokenKeys: string[] = [];
    const tokenOriginals: string[] = [];
    let match: RegExpExecArray | null;

    while ((match = tokenPattern.exec(normalized)) !== null) {
        const original = match[0];
        const key = original.toLocaleLowerCase();
        tokenOriginals.push(original);
        tokenKeys.push(key);
        if (!originalByKey.has(key)) {
            originalByKey.set(key, original);
        }
    }

    const addCandidate = (phrase: string, boost = 0) => {
        const cleaned = phrase.replace(/\s+/g, " ").trim();
        if (!cleaned || cleaned.length < 3) {
            return;
        }
        const key = cleaned.toLocaleLowerCase();
        counts.set(key, (counts.get(key) || 0) + 1);
        if (!originalByKey.has(key)) {
            originalByKey.set(key, cleaned);
        }
        boosts.set(key, (boosts.get(key) || 0) + boost);
    };

    tokenKeys.forEach((key, index) => {
        if (GLOSSARY_STOPWORDS.has(key) || key.length < 3) {
            return;
        }
        addCandidate(tokenOriginals[index], /^[A-Z0-9]/.test(tokenOriginals[index]) ? 2 : 0);
    });

    for (let i = 0; i < tokenKeys.length; i += 1) {
        for (let size = 2; size <= 4; size += 1) {
            const keySlice = tokenKeys.slice(i, i + size);
            const originalSlice = tokenOriginals.slice(i, i + size);
            if (keySlice.length < size) {
                continue;
            }
            if (GLOSSARY_STOPWORDS.has(keySlice[0]) || GLOSSARY_STOPWORDS.has(keySlice[keySlice.length - 1])) {
                continue;
            }
            const joined = originalSlice.join(" ");
            const titleBoost = originalSlice.some(token => /^[A-Z0-9]/.test(token)) ? 3 : 0;
            addCandidate(joined, titleBoost + size);
        }
    }

    const scored = Array.from(counts.entries())
        .map(([key, count]) => ({
            key,
            count,
            boost: boosts.get(key) || 0,
            original: originalByKey.get(key) || key,
        }))
        .filter(item => item.count >= 2)
        .filter(item => item.original.length >= 3)
        .sort((a, b) => {
            const scoreA = a.count * 10 + a.boost + a.original.split(" ").length * 3;
            const scoreB = b.count * 10 + b.boost + b.original.split(" ").length * 3;
            return scoreB - scoreA;
        });

    const rows: GlossaryRow[] = [];
    const seen = new Set<string>();
    scored.forEach(item => {
        if (rows.length >= 24) {
            return;
        }
        const normalizedKey = item.original.toLocaleLowerCase();
        if (seen.has(normalizedKey)) {
            return;
        }
        if (rows.some(row => shouldCollapseGlossaryVariant(item.original, row.source))) {
            return;
        }
        seen.add(normalizedKey);
        rows.push(createGlossaryRow(item.original, "", item.count));
    });
    return rows;
}

function buildGlossaryDraftState(glossaryText: string, sourceText: string): { userRows: GlossaryRow[]; extractedRows: GlossaryRow[] } {
    const parsedEntries = parseGlossaryText(glossaryText);
    const savedMap = new Map(parsedEntries.map(entry => [entry.source.trim().toLocaleLowerCase(), entry.target.trim()]));
    const extractedRows = extractFrequentGlossaryCandidates(sourceText).map(row => ({
        ...row,
        target: savedMap.get(row.source.trim().toLocaleLowerCase()) || "",
    }));
    const extractedKeys = new Set(extractedRows.map(row => row.source.trim().toLocaleLowerCase()));
    const userRows = parsedEntries
        .filter(entry => !extractedKeys.has(entry.source.trim().toLocaleLowerCase()))
        .map(entry => createGlossaryRow(entry.source, entry.target));

    return {
        userRows: normalizeUserGlossaryRows(userRows),
        extractedRows,
    };
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
    lastTranslationPromptPreview: string;
    lastTopicAwareHintsPreview: string;
    setDebugTranslationPromptTemplate: React.Dispatch<React.SetStateAction<string>>;
    setDebugRequest: React.Dispatch<React.SetStateAction<string>>;
    setDebugResponse: React.Dispatch<React.SetStateAction<string>>;
}) {
    const {
        showDebugPanel,
        debugRequest,
        debugResponse,
        debugTranslationPromptTemplate,
        lastTranslationPromptPreview,
        lastTopicAwareHintsPreview,
        setDebugTranslationPromptTemplate,
        setDebugRequest,
        setDebugResponse,
    } = props;
    const [translationPromptDraft, setTranslationPromptDraft] = useState(debugTranslationPromptTemplate);
    const [applyToast, setApplyToast] = useState("");

    useEffect(() => {
        setTranslationPromptDraft(debugTranslationPromptTemplate);
    }, [debugTranslationPromptTemplate]);

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
        forceShowReasoning: storedSettings?.forceShowReasoning ?? true,
        temperature: clampTemperature(storedSettings?.temperature ?? DEFAULT_TEMPERATURE),
        forceShowTemperature: storedSettings?.forceShowTemperature ?? true,
        enablePostEdit: storedSettings?.enablePostEdit ?? true,
        enableTopicAwarePostEdit: storedSettings?.enableTopicAwarePostEdit ?? true,
        enableEnhancedContextTranslation: storedSettings?.enableEnhancedContextTranslation ?? false,
        enhancedContextGlossary: storedSettings?.enhancedContextGlossary || "",
        enableSmartChunking: storedSettings?.enableSmartChunking ?? true,
        smartChunkSize: storedSettings?.smartChunkSize || 1000,
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
    const [enhancedContextUserRows, setEnhancedContextUserRows] = useState<GlossaryRow[]>([createGlossaryRow()]);
    const [enhancedContextExtractedRows, setEnhancedContextExtractedRows] = useState<GlossaryRow[]>([]);
    const [enhancedContextActiveTab, setEnhancedContextActiveTab] = useState<GlossaryTab>("user");
    const [settingsStatus, setSettingsStatus] = useState("");
    const [webServerSettings, setWebServerSettings] = useState<WebServerSettings>(DEFAULT_WEB_SERVER_SETTINGS);
    const [webServerPasswordDraft, setWebServerPasswordDraft] = useState("");
    const [webServerStatus, setWebServerStatus] = useState("");
    const [isSavingWebServerSettings, setIsSavingWebServerSettings] = useState(false);
    const [showSavedToast, setShowSavedToast] = useState(false);
    const [savedToastMessage, setSavedToastMessage] = useState("Settings saved");
    const [editorFontSize, setEditorFontSize] = useState<number>(clampFontSize(storedSettings?.editorFontSize || DEFAULT_EDITOR_FONT_SIZE));
    const [smartChunkSizeDraft, setSmartChunkSizeDraft] = useState(String(storedSettings?.smartChunkSize || 1000));
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
    const [estimatedOverallProgress, setEstimatedOverallProgress] = useState(0);
    const [animatedProgressValue, setAnimatedProgressValue] = useState(0);
    const [debugTranslationPromptTemplate, setDebugTranslationPromptTemplate] = useState("");
    const [lastTranslationPromptPreview, setLastTranslationPromptPreview] = useState("");
    const [lastTopicAwareHintsPreview, setLastTopicAwareHintsPreview] = useState("");
    const [reviewOverlay, setReviewOverlay] = useState<ReviewOverlayState>({ visible: false, hiding: false, text: "" });
    const [chunkPresentationVersion, setChunkPresentationVersion] = useState(0);

    const outputRef = useRef<HTMLDivElement>(null);
    const translationViewerRef = useRef<HTMLDivElement>(null);
    const translationSearchMatchesRef = useRef<HTMLElement[]>([]);
    const promptInputRef = useRef<HTMLTextAreaElement>(null);
    const didHydrateSettingsRef = useRef(false);
    const progressHideTimerRef = useRef<number | null>(null);
    const progressStateRef = useRef<Required<ProgressPayload>>({
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
    const overallProgressEstimateRef = useRef<OverallProgressEstimate>(createInitialOverallProgressEstimate());
    const translateActionRef = useRef<() => void>(() => { });
    const openFileActionRef = useRef<() => void>(() => { });
    const latestStatsRef = useRef<TranslationStatsPayload | null>(null);
    const browserTranslateAbortRef = useRef<AbortController | null>(null);
    const translationRunIdRef = useRef(0);
    const reviewOverlayTimerRef = useRef<number | null>(null);
    const reviewOverlayBodyRef = useRef<HTMLDivElement>(null);
    const reviewOverlayScrollFrameRef = useRef<number | null>(null);
    const renderedChunksRef = useRef<RenderedChunkState[]>([]);
    const chunkAnimationTimersRef = useRef<Record<number, number[]>>({});
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
    const enhancedContextDraftGlossary = buildCombinedGlossary(enhancedContextUserRows, enhancedContextExtractedRows);
    const sourceStats = getTextStats(sourceText);
    const translationStats = getTextStats(cleanedTranslation);

    const clearChunkTimers = (chunkIndex?: number) => {
        if (typeof chunkIndex === "number") {
            const timers = chunkAnimationTimersRef.current[chunkIndex] || [];
            timers.forEach(timer => window.clearTimeout(timer));
            delete chunkAnimationTimersRef.current[chunkIndex];
            return;
        }
        Object.values(chunkAnimationTimersRef.current).forEach(timers => {
            timers.forEach(timer => window.clearTimeout(timer));
        });
        chunkAnimationTimersRef.current = {};
    };

    const syncTranslationFromChunks = () => {
        setTranslation(joinRenderedChunks(renderedChunksRef.current));
    };

    const refreshChunkPresentation = () => {
        setChunkPresentationVersion(prev => prev + 1);
    };

    const resetTranslationPresentation = () => {
        clearChunkTimers();
        renderedChunksRef.current = [];
        setTranslation("");
        resetOverallProgressEstimate();
        refreshChunkPresentation();
        if (reviewOverlayTimerRef.current !== null) {
            window.clearTimeout(reviewOverlayTimerRef.current);
            reviewOverlayTimerRef.current = null;
        }
        setReviewOverlay({ visible: false, hiding: false, text: "" });
    };

    const showReviewOverlay = (text: string) => {
        if (!text.trim()) {
            return;
        }
        setReviewOverlay({ visible: true, hiding: false, text });
        if (reviewOverlayTimerRef.current !== null) {
            window.clearTimeout(reviewOverlayTimerRef.current);
        }
        reviewOverlayTimerRef.current = window.setTimeout(() => {
            setReviewOverlay(prev => ({ ...prev, hiding: true }));
            reviewOverlayTimerRef.current = window.setTimeout(() => {
                setReviewOverlay({ visible: false, hiding: false, text: "" });
                reviewOverlayTimerRef.current = null;
            }, 180);
        }, 2200);
    };

    const setRenderedChunk = (chunkIndex: number, nextChunk: RenderedChunkState) => {
        const nextChunks = [...renderedChunksRef.current];
        while (nextChunks.length <= chunkIndex) {
            nextChunks.push({
                displayedText: "",
                draftText: "",
                finalText: "",
                phase: "draft",
            });
        }
        nextChunks[chunkIndex] = nextChunk;
        renderedChunksRef.current = nextChunks;
        syncTranslationFromChunks();
        refreshChunkPresentation();
    };

    const streamChunkToFinal = (chunkIndex: number, finalText: string) => {
        const current = renderedChunksRef.current[chunkIndex] || {
            displayedText: "",
            draftText: "",
            finalText: "",
            phase: "draft" as const,
            showDraftSkeleton: false,
            skeletonHiding: false,
        };
        clearChunkTimers(chunkIndex);
        setRenderedChunk(chunkIndex, {
            ...current,
            displayedText: finalText,
            finalText,
            phase: "final",
            showDraftSkeleton: Boolean(current.draftText),
            skeletonHiding: false,
        });
    };

    const fadeOutDraftSkeleton = (chunkIndex: number) => {
        const current = renderedChunksRef.current[chunkIndex];
        if (!current?.showDraftSkeleton || current.skeletonHiding) {
            return;
        }

        clearChunkTimers(chunkIndex);
        const nextChunks = [...renderedChunksRef.current];
        nextChunks[chunkIndex] = {
            ...current,
            skeletonHiding: true,
        };
        renderedChunksRef.current = nextChunks;
        refreshChunkPresentation();

        const timer = window.setTimeout(() => {
            const latest = renderedChunksRef.current[chunkIndex];
            if (!latest) {
                return;
            }
            const updatedChunks = [...renderedChunksRef.current];
            updatedChunks[chunkIndex] = {
                ...latest,
                showDraftSkeleton: false,
                skeletonHiding: false,
            };
            renderedChunksRef.current = updatedChunks;
            refreshChunkPresentation();
            clearChunkTimers(chunkIndex);
        }, DRAFT_SKELETON_FADE_MS);
        chunkAnimationTimersRef.current[chunkIndex] = [timer];
    };
    const temperatureLabel = formatTemperatureLabel(providerSettings.temperature);
    const selectedInstructionPreset = findMatchingInstructionPreset(instruction);
    const instructionPresetValue = selectedInstructionPreset?.id || "custom";
    const reasoningLabel = providerSettings.reasoning || "Auto";
    const usesStageRing = progressState.stage === "model_load" || progressState.stage === "prompt_processing";
    const stageRingProgress = deriveStageRingProgress(progressState.stage, progressState.progress);
    const displayedProgressValue = usesStageRing ? stageRingProgress : estimatedOverallProgress;
    const clampedAnimatedProgressValue = Math.max(0, Math.min(1, animatedProgressValue || 0));
    const progressPercent = Math.max(0, Math.min(100, Math.round(clampedAnimatedProgressValue * 100)));
    const progressRingRadius = 28;
    const progressRingCircumference = 2 * Math.PI * progressRingRadius;
    const progressRingOffset = progressRingCircumference * (1 - clampedAnimatedProgressValue);
    const progressRingColor = usesStageRing ? "var(--progress-stage)" : "var(--accent)";
    const progressRingCaption = usesStageRing
        ? (progressState.stage === "model_load" ? "Model" : "Prompt")
        : "Overall";
    const renderedChunks = renderedChunksRef.current;
    const shouldRenderLayeredChunks = renderedChunks.some(chunk => chunk && (chunk.showDraftSkeleton || chunk.phase === "final"));

    const resetOverallProgressEstimate = () => {
        overallProgressEstimateRef.current = createInitialOverallProgressEstimate();
        setEstimatedOverallProgress(0);
    };

    const applyProgressPayload = (payload: ProgressPayload, visibilityFallback: boolean) => {
        const previous = progressStateRef.current;
        const nextCurrentStep = typeof payload.current_step === "number" ? payload.current_step : previous.current_step;
        const nextTotalSteps = typeof payload.total_steps === "number" ? payload.total_steps : previous.total_steps;
        const nextStage = payload.stage ?? previous.stage;
        const nextOverallProgress = deriveOverallProgress(nextCurrentStep, nextTotalSteps, nextStage === "done");
        const usesStageProgressRing = nextStage === "model_load" || nextStage === "prompt_processing";

        if (!usesStageProgressRing) {
            const now = performance.now();
            const estimate = overallProgressEstimateRef.current;
            const shouldResetEstimate = nextTotalSteps <= 0 || nextCurrentStep <= 0 || nextCurrentStep < estimate.activeStep || nextTotalSteps !== estimate.totalSteps;
            if (shouldResetEstimate) {
                overallProgressEstimateRef.current = createInitialOverallProgressEstimate();
            }

            const currentEstimate = overallProgressEstimateRef.current;
            if (nextCurrentStep > 0 && nextTotalSteps > 0) {
                if (currentEstimate.activeStep > 0 && nextCurrentStep > currentEstimate.activeStep && currentEstimate.stepStartedAt > 0) {
                    currentEstimate.lastCompletedStepDurationMs = Math.max(1, now - currentEstimate.stepStartedAt);
                }
                if (nextCurrentStep !== currentEstimate.activeStep || currentEstimate.stepStartedAt <= 0) {
                    currentEstimate.activeStep = nextCurrentStep;
                    currentEstimate.totalSteps = nextTotalSteps;
                    currentEstimate.stepStartedAt = now;
                }
            }
        }

        const nextState: Required<ProgressPayload> = {
            stage: nextStage,
            label: payload.label ?? previous.label,
            detail: payload.detail ?? previous.detail,
            progress: typeof payload.progress === "number" ? payload.progress : previous.progress,
            overall_progress: usesStageProgressRing
                ? previous.overall_progress
                : (nextOverallProgress ?? previous.overall_progress),
            current_chunk: typeof payload.current_chunk === "number" ? payload.current_chunk : previous.current_chunk,
            completed_chunks: typeof payload.completed_chunks === "number" ? payload.completed_chunks : previous.completed_chunks,
            total_chunks: typeof payload.total_chunks === "number" ? payload.total_chunks : previous.total_chunks,
            current_step: nextCurrentStep,
            total_steps: nextTotalSteps,
            visible: payload.visible ?? visibilityFallback,
            indeterminate: payload.indeterminate ?? previous.indeterminate,
        };
        progressStateRef.current = nextState;
        setProgressState(nextState);
        if (!usesStageProgressRing) {
            setEstimatedOverallProgress(estimateTimedOverallProgress(nextState, overallProgressEstimateRef.current, performance.now()));
        }
    };

    const showSavedToastMessage = (message: string) => {
        setShowSavedToast(false);
        window.setTimeout(() => {
            setSavedToastMessage(message);
            setShowSavedToast(true);
        }, 0);
    };

    const commitSmartChunkSizeDraft = () => {
        const digitsOnly = smartChunkSizeDraft.replace(/[^\d]/g, "");
        const nextValue = digitsOnly === "" ? 2000 : Math.max(200, Number.parseInt(digitsOnly, 10) || 2000);
        setProviderSettings(prev => ({ ...prev, smartChunkSize: nextValue }));
        setSmartChunkSizeDraft(String(nextValue));
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

    const selectAllInTranslationContainer = () => {
        const container = getActiveTranslationContainer();
        if (!container) {
            return false;
        }
        const selection = window.getSelection();
        if (!selection) {
            return false;
        }
        const range = document.createRange();
        range.selectNodeContents(container);
        selection.removeAllRanges();
        selection.addRange(range);
        return true;
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
        progressStateRef.current = progressState;
    }, [progressState]);

    useEffect(() => {
        if (usesStageRing || !progressState.visible) {
            setEstimatedOverallProgress(progressState.stage === "done" ? 1 : progressState.overall_progress);
            return;
        }

        let frameId = 0;
        const tick = () => {
            setEstimatedOverallProgress(prev => {
                const next = estimateTimedOverallProgress(progressStateRef.current, overallProgressEstimateRef.current, performance.now());
                return Math.abs(next - prev) < 0.001 ? prev : next;
            });
            frameId = window.requestAnimationFrame(tick);
        };

        tick();
        return () => window.cancelAnimationFrame(frameId);
    }, [usesStageRing, progressState.visible, progressState.stage, progressState.overall_progress, progressState.current_step, progressState.total_steps]);

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
        if (!reviewOverlay.visible) {
            return;
        }
        const panel = reviewOverlayBodyRef.current;
        if (!panel) {
            return;
        }
        const animateScrollToBottom = () => {
            if (panel.scrollHeight <= panel.clientHeight + 4) {
                return;
            }
            const startTop = panel.scrollTop;
            const targetTop = Math.max(0, panel.scrollHeight - panel.clientHeight);
            if (targetTop <= startTop + 1) {
                return;
            }
            if (reviewOverlayScrollFrameRef.current !== null) {
                window.cancelAnimationFrame(reviewOverlayScrollFrameRef.current);
                reviewOverlayScrollFrameRef.current = null;
            }

            const duration = 260;
            const startTime = performance.now();
            const easeOutCubic = (t: number) => 1 - Math.pow(1 - t, 3);

            const tick = (now: number) => {
                const elapsed = now - startTime;
                const progress = Math.min(1, elapsed / duration);
                const eased = easeOutCubic(progress);
                panel.scrollTop = startTop + (targetTop - startTop) * eased;
                if (progress < 1) {
                    reviewOverlayScrollFrameRef.current = window.requestAnimationFrame(tick);
                } else {
                    reviewOverlayScrollFrameRef.current = null;
                }
            };

            reviewOverlayScrollFrameRef.current = window.requestAnimationFrame(tick);
        };
        const frame = window.requestAnimationFrame(animateScrollToBottom);
        return () => {
            window.cancelAnimationFrame(frame);
            if (reviewOverlayScrollFrameRef.current !== null) {
                window.cancelAnimationFrame(reviewOverlayScrollFrameRef.current);
                reviewOverlayScrollFrameRef.current = null;
            }
        };
    }, [reviewOverlay.visible, reviewOverlay.text]);

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
            clearChunkTimers();
            setTranslation(prev => prev + token);
            if (outputRef.current) {
                outputRef.current.scrollTop = outputRef.current.scrollHeight;
            }
        });

        EventsOn("translation:chunk", (payload: TranslationChunkPayload) => {
            const chunkIndex = typeof payload.chunk_index === "number" ? payload.chunk_index : 0;
            const nextText = payload.text || "";
            const current = renderedChunksRef.current[chunkIndex] || {
                displayedText: "",
                draftText: "",
                finalText: "",
                phase: "draft" as const,
                showDraftSkeleton: false,
                skeletonHiding: false,
            };
            if ((payload.phase || "").toLowerCase() === "review") {
                showReviewOverlay(nextText);
            } else if ((payload.phase || "").toLowerCase() === "final") {
                streamChunkToFinal(chunkIndex, nextText);
                if (payload.final_closed) {
                    fadeOutDraftSkeleton(chunkIndex);
                }
            } else {
                setRenderedChunk(chunkIndex, {
                    ...current,
                    displayedText: nextText,
                    draftText: nextText,
                    phase: "draft",
                });
            }
            if (outputRef.current) {
                outputRef.current.scrollTop = outputRef.current.scrollHeight;
            }
        });

        EventsOn("translation:clear", () => {
            resetTranslationPresentation();
        });

        EventsOn("translation:complete", (payload: TranslationCompletePayload) => {
            const renderedChunkText = joinRenderedChunks(renderedChunksRef.current);
            setTranslation(renderedChunkText || payload.text || "");
            if (outputRef.current) {
                outputRef.current.scrollTop = outputRef.current.scrollHeight;
            }
            setStatusMessage(formatCompletionStats(latestStatsRef.current));
            if (progressHideTimerRef.current !== null) {
                window.clearTimeout(progressHideTimerRef.current);
            }
            const nextState = {
                ...progressStateRef.current,
                stage: "done",
                label: "Done",
                detail: "Translation complete",
                progress: 1,
                overall_progress: 1,
                current_step: progressStateRef.current.total_steps || progressStateRef.current.current_step,
                completed_chunks: progressStateRef.current.total_chunks || progressStateRef.current.completed_chunks,
                visible: true,
                indeterminate: false,
            };
            progressStateRef.current = nextState;
            setProgressState(nextState);
            setEstimatedOverallProgress(1);
            progressHideTimerRef.current = window.setTimeout(() => {
                setProgressState(prev => {
                    const next = { ...prev, visible: false };
                    progressStateRef.current = next;
                    return next;
                });
                progressHideTimerRef.current = null;
            }, 500);
        });

        EventsOn("translation:debug", (payload: DebugPayload) => {
            if (payload.direction === "note" && payload.endpoint === "prompt:translation") {
                setLastTranslationPromptPreview(payload.payload || "");
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
            applyProgressPayload(payload, progressStateRef.current.visible);
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

            if (event.key.toLowerCase() === "a") {
                const selection = window.getSelection();
                const activeElement = document.activeElement as HTMLElement | null;
                const activeTranslationContainer = getActiveTranslationContainer();
                const isTypingField = activeElement instanceof HTMLInputElement
                    || activeElement instanceof HTMLTextAreaElement
                    || Boolean(activeElement?.isContentEditable);
                const shouldSelectTranslationOnly = !isTypingField && (
                    isSelectionInsideNode(selection, outputRef.current)
                    || isSelectionInsideNode(selection, translationViewerRef.current)
                    || (activeTranslationContainer ? activeTranslationContainer.contains(activeElement) : false)
                );

                if (shouldSelectTranslationOnly && selectAllInTranslationContainer()) {
                    event.preventDefault();
                    return;
                }
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
            if (reviewOverlayTimerRef.current !== null) {
                window.clearTimeout(reviewOverlayTimerRef.current);
            }
            window.removeEventListener("keydown", handleKeyDown);
            EventsOff("translation:token");
            EventsOff("translation:chunk");
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
            clearChunkTimers();
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
            setLastTranslationPromptPreview(snapshot.lastTranslationPromptPreview || "");
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
            lastTranslationPromptPreview,
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
        lastTranslationPromptPreview,
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
        const draftState = buildGlossaryDraftState(providerSettings.enhancedContextGlossary, sourceText);
        setEnhancedContextUserRows(draftState.userRows);
        setEnhancedContextExtractedRows(draftState.extractedRows);
        setEnhancedContextActiveTab("user");
    }, [showEnhancedContextModal, providerSettings.enableEnhancedContextTranslation, providerSettings.enhancedContextGlossary, sourceText]);

    useEffect(() => {
        if (!showSavedToast) {
            return;
        }
        const timer = window.setTimeout(() => setShowSavedToast(false), 1400);
        return () => window.clearTimeout(timer);
    }, [showSavedToast, savedToastMessage]);

    useEffect(() => {
        setSmartChunkSizeDraft(String(providerSettings.smartChunkSize));
    }, [providerSettings.smartChunkSize]);

    useEffect(() => {
        if (isDebugStudioWindow) {
            return;
        }
        if (!isBrowserMode) {
            void SaveHostProviderSettings(llm.ProviderSettings.createFrom({
                ...providerSettings,
                debugTranslationPromptTemplate: "",
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
        resetTranslationPresentation();
        setLastTopicAwareHintsPreview("");

        const runTranslation = async (settings: ProviderSettings) => {
            const payload = llm.TranslationRequest.createFrom({
                settings: llm.ProviderSettings.createFrom({
                    ...settings,
                    debugTranslationPromptTemplate: debugTranslationPromptTemplate.trim(),
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
                                clearChunkTimers();
                                setTranslation(prev => prev + (data?.token || ""));
                                return;
                            }
                            if (event === "chunk") {
                                const chunkIndex = typeof data?.chunk_index === "number" ? data.chunk_index : 0;
                                const nextText = data?.text || "";
                                const current = renderedChunksRef.current[chunkIndex] || {
                                    displayedText: "",
                                    draftText: "",
                                    finalText: "",
                                    phase: "draft" as const,
                                    showDraftSkeleton: false,
                                    skeletonHiding: false,
                                };
                                if ((data?.phase || "").toLowerCase() === "review") {
                                    showReviewOverlay(nextText);
                                } else if ((data?.phase || "").toLowerCase() === "final") {
                                    streamChunkToFinal(chunkIndex, nextText);
                                    if (data?.final_closed) {
                                        fadeOutDraftSkeleton(chunkIndex);
                                    }
                                } else {
                                    setRenderedChunk(chunkIndex, {
                                        ...current,
                                        displayedText: nextText,
                                        draftText: nextText,
                                        phase: "draft",
                                    });
                                }
                                return;
                            }
                            if (event === "progress") {
                                applyProgressPayload(data || {}, true);
                                return;
                            }
                            if (event === "stats") {
                                latestStatsRef.current = data || null;
                                return;
                            }
                            if (event === "complete") {
                                const renderedChunkText = joinRenderedChunks(renderedChunksRef.current);
                                setTranslation(renderedChunkText || data?.text || "");
                                if (progressHideTimerRef.current !== null) {
                                    window.clearTimeout(progressHideTimerRef.current);
                                }
                                const nextState = {
                                    ...progressStateRef.current,
                                    stage: "done",
                                    label: "Done",
                                    detail: "Translation complete",
                                    progress: 1,
                                    overall_progress: 1,
                                    current_step: progressStateRef.current.total_steps || progressStateRef.current.current_step,
                                    completed_chunks: progressStateRef.current.total_chunks || progressStateRef.current.completed_chunks,
                                    visible: true,
                                    indeterminate: false,
                                };
                                progressStateRef.current = nextState;
                                setProgressState(nextState);
                                setEstimatedOverallProgress(1);
                                progressHideTimerRef.current = window.setTimeout(() => {
                                    if (!isActiveRun()) {
                                        return;
                                    }
                                    setProgressState(prev => {
                                        const next = { ...prev, visible: false };
                                        progressStateRef.current = next;
                                        return next;
                                    });
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
            setProgressState(prev => {
                const next = {
                    ...prev,
                    stage: "retrying",
                    label: "Retrying translation",
                    detail: "Reasoning unsupported. Switched to Auto.",
                    visible: true,
                    indeterminate: true,
                };
                progressStateRef.current = next;
                return next;
            });
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
            setProgressState(prev => {
                const next = {
                    ...prev,
                    stage: "retrying",
                    label: "Retrying translation",
                    detail: "Reasoning option unsupported. Retrying without reasoning parameter.",
                    visible: true,
                    indeterminate: true,
                };
                progressStateRef.current = next;
                return next;
            });
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
        resetOverallProgressEstimate();
        const nextInitialProgressState = {
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
        };
        progressStateRef.current = nextInitialProgressState;
        setProgressState(nextInitialProgressState);
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
                    setProgressState(prev => {
                        const next = { ...prev, visible: false };
                        progressStateRef.current = next;
                        return next;
                    });
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
                    setProgressState(prev => {
                        const next = { ...prev, visible: false };
                        progressStateRef.current = next;
                        return next;
                    });
                }
            } else {
                setStatusMessage(`Translation failed: ${message}`);
                alert("Translation failed: " + err);
                setProgressState(prev => {
                    const next = { ...prev, visible: false };
                    progressStateRef.current = next;
                    return next;
                });
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
        setProgressState(prev => {
            const next = { ...prev, visible: false };
            progressStateRef.current = next;
            return next;
        });
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
            if (navigator.clipboard?.writeText) {
                try {
                    await navigator.clipboard.writeText(cleanedTranslation);
                    announceAction("Copied translation to clipboard.");
                    return;
                } catch (clipboardErr) {
                    console.warn("Navigator clipboard write failed, falling back.", clipboardErr);
                }
            }

            const domCopyWorked = await copyTextWithDomFallback(cleanedTranslation);
            if (domCopyWorked) {
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

    const handleTranslationCopyEvent = (event: React.ClipboardEvent<HTMLDivElement>) => {
        if (!cleanedTranslation) {
            return;
        }
        event.preventDefault();
        event.clipboardData.setData("text/plain", cleanedTranslation);
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

    const updateGlossaryRows = (tab: GlossaryTab, updater: (rows: GlossaryRow[]) => GlossaryRow[]) => {
        if (tab === "user") {
            setEnhancedContextUserRows(prev => normalizeUserGlossaryRows(updater(prev)));
            return;
        }
        setEnhancedContextExtractedRows(prev => updater(prev));
    };

    const handleGlossaryRowChange = (tab: GlossaryTab, rowId: string, field: "source" | "target", value: string) => {
        updateGlossaryRows(tab, rows => rows.map(row => row.id === rowId ? { ...row, [field]: value } : row));
    };

    const handleAddUserGlossaryRow = () => {
        setEnhancedContextUserRows(prev => normalizeUserGlossaryRows([...prev, createGlossaryRow()]));
    };

    const handleDeleteGlossaryRow = (tab: GlossaryTab, rowId: string) => {
        updateGlossaryRows(tab, rows => {
            const next = rows.filter(row => row.id !== rowId);
            return next;
        });
    };

    const handleOpenEnhancedContextGlossary = async (tab: GlossaryTab) => {
        try {
            const content = await OpenFile();
            if (content !== undefined && content !== null && content !== "") {
                const parsedRows = parseGlossaryText(content).map(entry => createGlossaryRow(entry.source, entry.target));
                if (tab === "user") {
                    setEnhancedContextUserRows(normalizeUserGlossaryRows(parsedRows));
                    announceAction("Loaded user glossary file.");
                } else {
                    setEnhancedContextExtractedRows(prev => {
                        const parsedMap = new Map(parsedRows.map(row => [row.source.trim().toLocaleLowerCase(), row.target]));
                        return prev.map(row => ({
                            ...row,
                            target: parsedMap.get(row.source.trim().toLocaleLowerCase()) || "",
                        }));
                    });
                    announceAction("Loaded extracted-candidate glossary file.");
                }
            }
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not open glossary file: ${String(err)}`);
        }
    };

    const handleSaveEnhancedContextGlossary = async (tab: GlossaryTab) => {
        try {
            const content = tab === "user"
                ? serializeGlossaryRows(enhancedContextUserRows.filter(row => row.source.trim() && row.target.trim()))
                : serializeGlossaryRows(enhancedContextExtractedRows.filter(row => row.source.trim() && row.target.trim()));
            const savedPath = await SaveFile(content);
            if (savedPath) {
                announceAction(`Saved glossary to: ${savedPath}`);
            }
        } catch (err: any) {
            console.error(err);
            setStatusMessage(`Could not save glossary file: ${String(err)}`);
        }
    };

    const handleClearEnhancedContextGlossary = (tab: GlossaryTab) => {
        if (tab === "user") {
            setEnhancedContextUserRows(normalizeUserGlossaryRows([]));
            announceAction("Cleared user glossary rows.");
            return;
        }
        setEnhancedContextExtractedRows(prev => prev.map(row => ({ ...row, target: "" })));
        announceAction("Cleared extracted glossary mappings.");
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
                lastTranslationPromptPreview={lastTranslationPromptPreview}
                lastTopicAwareHintsPreview={lastTopicAwareHintsPreview}
                setDebugTranslationPromptTemplate={setDebugTranslationPromptTemplate}
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
                            <div className="translation-output markdown-output" ref={outputRef} onCopy={handleTranslationCopyEvent} style={{ fontSize: `${editorFontSize}px` }}>
                                {shouldRenderLayeredChunks ? (
                                    <div className="translation-stream-layered" data-version={chunkPresentationVersion}>
                                        {renderedChunks.map((chunk, index) => {
                                            if (!chunk || (!chunk.displayedText && !chunk.draftText)) {
                                                return null;
                                            }
                                            const showSkeleton = Boolean(chunk.showDraftSkeleton && chunk.draftText);
                                            return (
                                                <div
                                                    key={`chunk-${index}`}
                                                    className={`translation-stream-chunk ${showSkeleton ? "has-skeleton" : ""}`}
                                                >
                                                    {showSkeleton && (
                                                        <div
                                                            className={`translation-stream-draft ${chunk.skeletonHiding ? "is-hiding" : ""}`}
                                                            aria-hidden="true"
                                                        >
                                                            {chunk.draftText}
                                                        </div>
                                                    )}
                                                    <div className={`translation-stream-final ${showSkeleton ? "is-overlaying" : ""}`}>
                                                        {chunk.displayedText}
                                                    </div>
                                                </div>
                                            );
                                        })}
                                        {isTranslating && <span className="cursor">|</span>}
                                    </div>
                                ) : (
                                    <>
                                        {renderMarkdown(translation)}
                                        {isTranslating && <span className="cursor">|</span>}
                                    </>
                                )}
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
                                    onCopy={handleTranslationCopyEvent}
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
                                                value={smartChunkSizeDraft}
                                                disabled={!providerSettings.enableSmartChunking}
                                                onChange={e => {
                                                    setSmartChunkSizeDraft(e.target.value.replace(/[^\d]/g, ""));
                                                }}
                                                onBlur={commitSmartChunkSizeDraft}
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
                                                        value={smartChunkSizeDraft}
                                                        disabled={!providerSettings.enableSmartChunking}
                                                        onChange={e => {
                                                            setSmartChunkSizeDraft(e.target.value.replace(/[^\d]/g, ""));
                                                        }}
                                                        onBlur={commitSmartChunkSizeDraft}
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
                    <div className="modal-overlay modal-overlay-fullscreen" onClick={handleCloseEnhancedContextModal}>
                        <div className="modal-card modal-card-fullscreen" onClick={e => e.stopPropagation()}>
                            <div className="modal-header">
                                <div>
                                    <div className="modal-title">Enhanced Context Translation</div>
                                    <div className="modal-subtitle">Improve long-form terminology and name consistency with stronger context rules, a user glossary, and document-derived candidate terms.</div>
                                </div>
                                <button className="btn btn-secondary btn-small" onClick={handleCloseEnhancedContextModal} title="Save changes and close">
                                    OK
                                </button>
                            </div>
                            <div className="modal-body modal-body-fullscreen">
                                <div className="enhanced-context-layout">
                                    <div className="enhanced-context-sidebar">
                                        <label className="settings-checkbox">
                                            <input
                                                type="checkbox"
                                                checked={enhancedContextDraftEnabled}
                                                onChange={e => setEnhancedContextDraftEnabled(e.target.checked)}
                                            />
                                            <span>Use Enhanced Context Translation</span>
                                        </label>
                                        <div className="settings-note">
                                            Rows are only used when the right-side user term is filled in.
                                        </div>
                                        <div className="settings-note">
                                            Final glossary entries: {parseGlossaryText(enhancedContextDraftGlossary).length}
                                        </div>
                                        <div className="settings-note">
                                            Document-derived candidates: {enhancedContextExtractedRows.length}
                                        </div>
                                    </div>
                                    <div className="enhanced-context-workspace">
                                        <div className="enhanced-context-tabs" role="tablist" aria-label="Enhanced context tabs">
                                            <button
                                                type="button"
                                                className={`btn btn-secondary enhanced-context-tab${enhancedContextActiveTab === "user" ? " is-active" : ""}`}
                                                onClick={() => setEnhancedContextActiveTab("user")}
                                            >
                                                User Glossary
                                            </button>
                                            <button
                                                type="button"
                                                className={`btn btn-secondary enhanced-context-tab${enhancedContextActiveTab === "extracted" ? " is-active" : ""}`}
                                                onClick={() => setEnhancedContextActiveTab("extracted")}
                                            >
                                                Frequent Terms From Document
                                            </button>
                                        </div>
                                        {enhancedContextActiveTab === "user" ? (
                                            <div className="enhanced-context-panel">
                                                <div className="enhanced-context-toolbar">
                                                    <div className="enhanced-context-toolbar-title">User-defined glossary rows</div>
                                                    <div className="glossary-toolbar">
                                                        <button type="button" className="icon-btn" onClick={() => void handleOpenEnhancedContextGlossary("user")} title="Open User Glossary File">
                                                            <span className="material-symbols-outlined">folder_open</span>
                                                        </button>
                                                        <button type="button" className="icon-btn" onClick={() => void handleSaveEnhancedContextGlossary("user")} title="Save User Glossary File">
                                                            <span className="material-symbols-outlined">save</span>
                                                        </button>
                                                        <button type="button" className="icon-btn" onClick={() => handleClearEnhancedContextGlossary("user")} title="Clear User Glossary">
                                                            <span className="material-symbols-outlined">delete</span>
                                                        </button>
                                                    </div>
                                                </div>
                                                <div className="enhanced-context-table-wrap">
                                                    <table className="enhanced-context-table">
                                                        <thead>
                                                            <tr>
                                                                <th>Source Term</th>
                                                                <th>User Term</th>
                                                                <th className="enhanced-context-col-actions">Actions</th>
                                                            </tr>
                                                        </thead>
                                                        <tbody>
                                                            {enhancedContextUserRows.map(row => (
                                                                <tr key={row.id}>
                                                                    <td data-label="Source Term">
                                                                        <input
                                                                            type="text"
                                                                            value={row.source}
                                                                            onChange={e => handleGlossaryRowChange("user", row.id, "source", e.target.value)}
                                                                            placeholder="Source term"
                                                                        />
                                                                    </td>
                                                                    <td data-label="User Term">
                                                                        <input
                                                                            type="text"
                                                                            value={row.target}
                                                                            onChange={e => handleGlossaryRowChange("user", row.id, "target", e.target.value)}
                                                                            placeholder="Preferred target term"
                                                                        />
                                                                    </td>
                                                                    <td className="enhanced-context-row-actions" data-label="Actions">
                                                                        <button type="button" className="icon-btn" onClick={() => handleDeleteGlossaryRow("user", row.id)} title="Delete Row">
                                                                            <span className="material-symbols-outlined">delete</span>
                                                                        </button>
                                                                    </td>
                                                                </tr>
                                                            ))}
                                                        </tbody>
                                                    </table>
                                                </div>
                                                <div className="enhanced-context-table-footer">
                                                    <button type="button" className="btn btn-secondary btn-small" onClick={handleAddUserGlossaryRow}>
                                                        <span className="material-symbols-outlined">add</span>
                                                        Add Row
                                                    </button>
                                                </div>
                                            </div>
                                        ) : (
                                            <div className="enhanced-context-panel">
                                                <div className="enhanced-context-toolbar">
                                                    <div className="enhanced-context-toolbar-title">Frequent terms or phrases extracted without LLM</div>
                                                    <div className="glossary-toolbar">
                                                        <button type="button" className="icon-btn" onClick={() => void handleOpenEnhancedContextGlossary("extracted")} title="Open Extracted Glossary File">
                                                            <span className="material-symbols-outlined">folder_open</span>
                                                        </button>
                                                        <button type="button" className="icon-btn" onClick={() => void handleSaveEnhancedContextGlossary("extracted")} title="Save Extracted Glossary File">
                                                            <span className="material-symbols-outlined">save</span>
                                                        </button>
                                                        <button type="button" className="icon-btn" onClick={() => handleClearEnhancedContextGlossary("extracted")} title="Clear Extracted Glossary Mappings">
                                                            <span className="material-symbols-outlined">delete</span>
                                                        </button>
                                                    </div>
                                                </div>
                                                <div className="enhanced-context-table-wrap">
                                                    <table className="enhanced-context-table">
                                                        <thead>
                                                            <tr>
                                                                <th>Source Term</th>
                                                                <th>User Term</th>
                                                                <th className="enhanced-context-col-frequency">Hits</th>
                                                            </tr>
                                                        </thead>
                                                        <tbody>
                                                            {enhancedContextExtractedRows.map(row => (
                                                                <tr key={row.id}>
                                                                    <td data-label="Source Term">
                                                                        <div className="enhanced-context-source-cell">{row.source}</div>
                                                                    </td>
                                                                    <td data-label="User Term">
                                                                        <input
                                                                            type="text"
                                                                            value={row.target}
                                                                            onChange={e => handleGlossaryRowChange("extracted", row.id, "target", e.target.value)}
                                                                            placeholder="Fill to activate this row"
                                                                        />
                                                                    </td>
                                                                    <td className="enhanced-context-frequency-cell" data-label="Hits">{row.frequency || "-"}</td>
                                                                </tr>
                                                            ))}
                                                        </tbody>
                                                    </table>
                                                </div>
                                            </div>
                                        )}
                                        <div className="settings-note enhanced-context-note">
                                            The final USER GLOSSARY injected into prompts is built from both tabs together. Rows without a user term are ignored.
                                        </div>
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
                    {reviewOverlay.visible && (
                        <div className={`progress-review-panel ${reviewOverlay.hiding ? "is-hiding" : ""}`} role="status" aria-live="polite">
                            <div className="progress-review-title">Review Notes</div>
                            <div className="progress-review-body markdown-output" ref={reviewOverlayBodyRef}>{renderMarkdown(formatReviewOverlayText(reviewOverlay.text))}</div>
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}

export default App;
