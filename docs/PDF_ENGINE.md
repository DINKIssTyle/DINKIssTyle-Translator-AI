# DKST Clean-Room PDF Engine

The PDF engine is deliberately separated from translation providers. It accepts
source PDF geometry plus already translated block text; it does not own model
selection, prompts, glossary behavior, or translation style.

## Pipeline

1. Extract positioned glyph fragments from the source PDF.
2. Group fragments into lines and semantic layout blocks. Fragments sharing a
   baseline are separated when a large horizontal gap indicates a new column.
   Zero-size rotated extraction noise is excluded from horizontal layout.
3. Send each block through the existing DKST translation pipeline. Long blocks
   may be chunked internally, but block identity is restored by application code.
4. Parse page content streams and remove complete `BT ... ET` text objects.
5. Preserve page geometry, images, vector graphics and annotations.
6. Preserve each semantic block's ordered source-line regions, including changes
   in line width around portraits, pull quotes and circular images.
7. Recompose translated text through those regions using Unicode font metrics,
   role-aware font sizing and line spacing.
8. Enforce a hard vertical line budget for every block. Overfull blocks are
   clipped with an ellipsis and reported as a composition warning instead of
   painting across photographs, headings or neighboring columns.
9. Fall back to opaque local patches only when a source content stream cannot be
   rewritten safely.

## Clean-room boundary

No PDFMathTranslate or BabelDOC source code is included or adapted. The content
stream scanner, block identity protocol and composition policy are implemented
in this repository from the public PDF content-stream grammar.

General-purpose libraries are limited to infrastructure:

- `pdfcpu` (Apache-2.0): PDF object parsing and standards-compliant serialization.
- `ledongthuc/pdf` (BSD-style): positioned text extraction.
- `gopdf` (MIT): page import, font embedding and translated text drawing.

These components do not provide the translation workflow or the clean-room
layout policy. Model weights are not currently part of the engine; any future
layout model must pass a separate redistribution-license review.

## Current limits

- Text drawn inside nested Form XObjects may survive the first cleanup pass.
- Complex clipping paths, rotated text and vertical writing need dedicated block
  transforms.
- Font family, weight, source text color and baseline reconstruction remain
  incremental work.
- Overfull text is currently clipped rather than continued into a newly detected
  free region; automatic continuation-region discovery is planned.
- Image-only PDFs require a separately licensed OCR stage.
