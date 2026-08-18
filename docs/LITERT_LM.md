# LiteRT-LM integration

This application integrates LiteRT-LM 0.14.0 as an OpenAI-compatible local
provider on desktop and through the native Google SDKs on mobile.

## Model format

LiteRT-LM 0.14.0 loads a `.litertlm` model package. The older MediaPipe/Gemma
`.bin` format is not accepted by the current runtime or mobile SDKs. The model
picker keeps `.bin` visible only so the app can report that incompatibility
clearly; inference will not start with it.

Use a LiteRT-LM export of Gemma 2B IT named `gemma-2b-it.litertlm`. If the only
available artifact is a legacy `.bin`, re-export it from the original model
weights with the matching LiteRT-LM model-authoring workflow. Renaming the file
extension does not convert it.

Models are intentionally not committed to the repository. Set an absolute
path while packaging:

```bash
export LITERTLM_MODEL_PATH=/absolute/path/gemma-2b-it.litertlm
```

## Desktop: Windows, macOS, and Linux

Build the self-contained runtime on each target operating system:

```bash
wails3 task litertlm:bundle
```

The task installs the official `litert-lm==0.14.0` Python package into an
isolated build environment and produces a single executable under
`third_party/litertlm/<os>-<arch>/`. Platform packagers copy that directory
into the application. Native runtime files must be built on their destination
OS; they are not portable between Windows, macOS, and Linux.

At runtime, choose **LiteRT-LM (On-device)** in Settings and select the
`.litertlm` file. The app imports the model once, starts `litert-lm serve`, and
uses its OpenAI-compatible endpoint on `127.0.0.1:9379`. A source path, size,
and modification-time marker prevents multi-gigabyte re-imports on every app
launch. `DKST_LITERTLM_RUNTIME` or the Runtime Binary field can override the
bundled executable for development.

## Android

The restored Gradle scaffold uses
`com.google.ai.edge.litertlm:litertlm-android:0.14.0` and a loopback adapter at
`127.0.0.1:9379`. The adapter initializes GPU first when requested and falls
back to the text-only CPU backend.

```bash
LITERTLM_MODEL_PATH=/absolute/path/gemma-2b-it.litertlm wails3 task android:package
```

The model is staged as `assets/models/gemma-2b-it.litertlm`. Without a staged
model, a file selected through the app can still be configured at runtime.
Android requires API 26 or newer. The scaffold includes arm64-v8a for devices
and x86_64 for emulators.

## iOS

The restored Xcode scaffold references a small local Swift Package that pins
Google's official `CLiteRTLM.xcframework` release at 0.14.0. The app-facing
Swift adapter uses the public C API, starts a loopback endpoint on port 9379,
tries GPU first, and falls back to CPU. Generate and open the project with:

```bash
wails3 task ios:xcode
```

`litertlm:bundle` is the desktop runtime task and is not used for iOS. The iOS
task resolves Google's XCFramework through the generated local Swift Package.

The generated project supports iOS 15 and newer. Select a `.litertlm` package
inside the app, or add `gemma-2b-it.litertlm` to the Xcode target's Copy Bundle
Resources under a `models` folder for a preloaded build.

## Server modes

**On-device only** binds the LiteRT-LM adapter to loopback. **On-device +
OpenAI server** exposes the desktop `litert-lm serve` endpoint on all network
interfaces. The raw Google CLI endpoint has no authentication, so do not expose
that mode directly to an untrusted network.

For password-protected remote translation, keep LiteRT-LM in on-device mode and
enable the app's Web Server setting. That server retains the existing password
and TLS controls while using LiteRT-LM as its translation provider.

## Runtime layout

| Platform | Runtime integration | Model location |
| --- | --- | --- |
| Windows | bundled `litert-lm.exe` | selected file |
| macOS | bundled `litert-lm` | selected file or app Resources `models/` |
| Linux | bundled `litert-lm` | selected file |
| Android | Google Kotlin SDK 0.14.0 | app assets or selected file |
| iOS | Google `CLiteRTLM.xcframework` 0.14.0 | bundle resource or selected file |
