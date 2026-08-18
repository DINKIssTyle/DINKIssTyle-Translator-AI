# LiteRT-LM runtime staging

Run scripts/build-litertlm-runtime.sh on macOS/Linux or
scripts/build-litertlm-runtime.ps1 on Windows. The script packages the official
litert-lm 0.14.0 wheel into a self-contained executable under the matching
darwin, linux, or windows architecture directory.

Build each target on that target OS. Native LiteRT libraries cannot be
cross-packaged by copying a wheel from another operating system.

Large generated binaries and models are intentionally ignored by Git. Set
LITERTLM_MODEL_PATH to an absolute .litertlm path while packaging to stage
models/gemma-2b-it.litertlm.
