# DKST Translator AI

Professional local LLM translation workspace designed for consistent style and context preservation.

[English](#english) | [한국어](#한국어)

---

## English

### Overview
**DKST Translator AI** is a powerful translation tool designed for translating long-form content while maintaining consistent style and context. By leveraging local Large Language Models (LLMs) via OpenAI-compatible APIs or LM Studio, it provides a professional, secure, and highly customizable translation environment.

### Key Features
- **Customizable Translation Prompts**: Use professional presets or define your own instructions to guide the translation style specifically for your needs.
- **LLM Temperature Control**: Fine-tune the balance between accuracy and creativity for each translation task.
- **Proofread After Translation**: Implements a two-step process—initial translation at a low temperature for precision, followed by a post-editing pass for natural phrasing.
- **Enhanced Context with User Glossary**: Maintain consistency for names and specific terminology (e.g., *Dorothy* → *도로시*, *Alice* → *앨리스*) using a persistent glossary.
- **Context-Aware Smart Post-Editing**: Intelligently preserves context across different segments of long documents to prevent drift in tone or meaning.
- **Integrated Web Server**: Access the translation workspace via a browser. Supports custom port configuration, password protection, and SSL/TLS certificates for secure remote access.

---

## 한국어

### 소개
**DKST Translator AI**는 스타일과 문맥을 유지하며 장문의 글을 효과적으로 번역할 수 있도록 설계된 도구입니다. OpenAI 호환 API 또는 LM Studio를 통한 로컬 LLM(대규모 언어 모델)을 활용하여 보안이 보장된 전문적인 맞춤형 번역 환경을 제공합니다.

### 주요 기능
- **프리셋 및 사용자 정의 프롬프트**: 내장된 전문 프리셋을 사용하거나 직접 정의한 지침을 통해 상황에 맞는 최적의 번역 스타일을 적용할 수 있습니다.
- **LLM Temperature 조절**: AI 출력의 정확도와 창의성 사이의 균형을 정교하게 조절할 수 있습니다.
- **초벌 번역 후 교정 (Proofread After Translation)**: 낮은 Temperature로 정확한 초벌 번역을 수행한 후, 사용자가 지정한 설정으로 자연스러운 문장 교정(Post-editing) 과정을 거칩니다.
- **사용자 사전을 통한 문맥 강화**: 사용자 사전을 활용하여 고유 명사나 특정 전문 용어(예: *Dorothy* → *도로시*, *Alice* → *도리스*)가 전체 문서에서 일관되게 번역되도록 관리합니다.
- **문맥 인지 스마트 포스트 에디팅**: 장문의 문서 전체에서 문맥을 지능적으로 파악하여 일관된 톤과 의미를 유지하도록 돕습니다.
- **내장 웹 서버 지원**: 웹 브라우저를 통해 원격으로 접속하여 사용할 수 있습니다. 포트 설정, 접근 비밀번호 및 SSL/TLS 인증서 기능을 지원하여 안전한 접속 환경을 제공합니다.

---

## Development & Build

### Prerequisites
- [Go](https://go.dev/) (1.21+)
- [Node.js](https://nodejs.org/) & NPM
- [Wails CLI](https://wails.io/docs/gettingstarted/installation) (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`)

### Live Development
To run in live development mode with hot-reloading:
```bash
wails dev
```
In this mode, the frontend runs on Vite, and calls to Go methods are bridged automatically.

### Production Build
We provide custom build scripts for each platform to handle advanced signing, naming, and resource injection. **Use these scripts instead of raw `wails build` for production releases.**

#### macOS
```bash
chmod +x build-mac.sh
./build-mac.sh
```
*Generates a signed `DKST Translator AI.app` in `build/bin/`.*

#### Windows
```batch
build-win.bat
```
*Generates `DKST Translator AI.exe` with embedded version info and icon.*

#### Linux
```bash
chmod +x build-linux.sh
./build-linux.sh
```
*Handles dependencies and WebKit2GTK versioning automatically.*

---

### Copyright
Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.
