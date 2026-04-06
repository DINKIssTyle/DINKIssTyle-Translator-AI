포스트 에디팅은 “초벌 번역 생성 -> 필요하면 포스트 에디트 프롬프트 조립 -> 모델에 재투입” 순서로 동작합니다. 핵심 진입점은 [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L384) 부근이고, `EnablePostEdit`가 켜져 있으면 각 chunk 번역 직후 `postEditTranslation(...)`이 호출됩니다. 실제 포스트 에디트 프롬프트 조립 함수는 [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1260) 의 `buildPostEditPrompt(...)`입니다.

프롬프트 조립 단계만 순서대로 보면 이렇습니다.

1. 기본 재료 수집
`buildPostEditPrompt(...)`가 `sourceLang`, `targetLang`, `sourceText`, `draftTranslation`, `instruction`, `runtimeOptions`를 받습니다. 여기서 언어명 정규화와 보호 용어 추출을 먼저 합니다.
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1260)

2. 디버그 템플릿 override가 있으면 그걸 우선 사용
`settings.DebugPostEditPromptTemplate`가 비어있지 않으면, 기본 빌더를 쓰지 않고 템플릿에 변수들을 치환해서 그대로 사용합니다.
치환되는 값은 `SOURCE_TEXT`, `DRAFT_TRANSLATION`, `INSTRUCTION`, `PROTECTED_TERMS`, `GLOSSARY`, `TOPIC_AWARE_HINTS`, `CHUNK_LABEL`, `CONTEXT_SUMMARY`, `OVERLAP_CONTEXT`, `OPENING_SOURCE_PARAGRAPH`, `OPENING_TRANSLATED_PARAGRAPH` 입니다.
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1264), [App.tsx](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/frontend/src/App.tsx#L246)

3. 기본 역할 문장 추가
override가 없으면 맨 앞에
“당신은 source -> target 번역 포스트 에디터다. draft를 source와 대조해서 최종 번역을 만들어라”
성격의 역할 문장이 들어갑니다.
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1285)

4. 사용자 스타일 지시문 추가
사용자가 입력한 `instruction`이 있으면 `Style instruction:` 섹션으로 그대로 붙습니다.
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1290)

5. 보호 용어 추가
원문에서 추출한 고유명사/기술용어 목록이 있으면 `Protected names and terms:` 아래 bullet로 붙습니다.
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1296)

6. chunk 관련 문맥 추가
긴 문장은 chunk 단위로 처리되는데, 그때 `runtimeOptions`로 문맥이 들어옵니다.
들어갈 수 있는 값은:
- `ChunkLabel`: 현재 몇 번째 chunk인지
- `ContextSummary`: 직전 chunk의 원문 꼬리 + 번역 꼬리 요약
- `OpeningSourceParagraph`: 전체 문서 첫 문단
- `OpeningTranslatedParagraph`: 지금까지 나온 번역의 첫 문단
- `OverlapContext`: 직전 chunk 끝부분 일부
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1307), [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L384), [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1949)

7. 수정 규칙 고정 문구 추가
그 다음 `Rules:` 블록이 붙습니다. 여기서 핵심은:
- 최소 수정만 할 것
- 의미를 바꾸지 말 것
- 기관명/법적 의미/관계/연대기 왜곡 금지
- 명백한 오역, 깨진 음차, 혼합 언어, 남은 미번역만 고칠 것
- 이미 괜찮은 문장은 draft에 최대한 가깝게 둘 것
- 최종 번역만 출력할 것
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1338)

8. topic-aware 힌트 선택적 추가
`EnableTopicAwarePostEdit`가 켜져 있으면 장르/주제/톤을 추정해서 약한 힌트를 붙입니다.
이건 강한 규칙이 아니라 “register와 terminology consistency 개선용 참고”입니다.
예: technical documentation, legal or policy text, marketing copy 등
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1350), [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1370)

9. enhanced context consistency 규칙 선택적 추가
`EnableEnhancedContextTranslation`가 켜져 있으면 일관성 유지 규칙과 사용자 glossary가 붙습니다.
즉, 포스트 에디트 단계에서도 용어 통일을 다시 강제합니다.
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1358)

10. 마지막에 원문과 초벌 번역을 붙여서 완성
마지막 두 섹션이 항상 붙습니다.
- `Source Text:`
- `Translated Draft:`
즉 포스트 에디터는 “규칙 + 문맥 + 원문 + 초벌 번역”을 함께 보고 최종본만 내놓습니다.
참고: [client.go](/Users/dinki/Documents/GitHub/DINKIssTyle-Translator-AI/internal/llm/client.go#L1367)

정리하면, 현재 포스트 에디팅 프롬프트는 단순히 “초벌 번역을 다듬어라”가 아니라, `스타일 지시문 + 보호 용어 + chunk 문맥 + 약한 topic 힌트 + glossary 일관성 규칙 + 원문 + draft`를 순서대로 조립해서 만드는 구조입니다.  