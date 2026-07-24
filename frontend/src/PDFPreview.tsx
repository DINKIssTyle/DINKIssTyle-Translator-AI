// Created by DINKIssTyle on 2026. Copyright (C) 2026 DINKI'ssTyle. All rights reserved.

import React, { useEffect, useMemo, useRef, useState } from "react";
import pdfWorkerURL from "pdfjs-dist/legacy/build/pdf.worker.min.mjs?url";

let pdfjsPromise: Promise<any> | null = null;

function loadPDFJS(): Promise<any> {
    if (!pdfjsPromise) {
        pdfjsPromise = import("pdfjs-dist/legacy/build/pdf.mjs").then(module => {
            (module as any).GlobalWorkerOptions.workerSrc = pdfWorkerURL;
            return module;
        });
    }
    return pdfjsPromise;
}

type PDFPreviewProps = {
    dataBase64: string;
    documentName: string;
    busy?: boolean;
    busyLabel?: string;
    activePage?: number;
    totalPageCount?: number;
};

type PDFDocumentProxy = any;

function decodeBase64(base64: string): Uint8Array {
    const binary = window.atob(base64);
    const bytes = new Uint8Array(binary.length);
    for (let index = 0; index < binary.length; index += 1) {
        bytes[index] = binary.charCodeAt(index);
    }
    return bytes;
}

function PDFPageCanvas({
    document,
    pageNumber,
    availableWidth,
    zoom,
}: {
    document: PDFDocumentProxy;
    pageNumber: number;
    availableWidth: number;
    zoom: number;
}) {
    const wrapperRef = useRef<HTMLDivElement>(null);
    const canvasRef = useRef<HTMLCanvasElement>(null);
    const [isVisible, setIsVisible] = useState(pageNumber <= 2);
    const [aspectRatio, setAspectRatio] = useState(1.414);
    const [renderError, setRenderError] = useState("");

    useEffect(() => {
        const node = wrapperRef.current;
        if (!node || isVisible) return;
        const observer = new IntersectionObserver(entries => {
            if (entries.some(entry => entry.isIntersecting)) {
                setIsVisible(true);
                observer.disconnect();
            }
        }, { rootMargin: "700px 0px" });
        observer.observe(node);
        return () => observer.disconnect();
    }, [isVisible]);

    useEffect(() => {
        if (!isVisible || !document || !canvasRef.current || availableWidth <= 0) return;
        let cancelled = false;
        let renderTask: any = null;
        const renderPage = async () => {
            try {
                const page = await document.getPage(pageNumber);
                if (cancelled) return;
                const naturalViewport = page.getViewport({ scale: 1 });
                setAspectRatio(naturalViewport.height / naturalViewport.width);
                const cssScale = Math.max(0.15, (availableWidth / naturalViewport.width) * zoom);
                const pixelRatio = Math.min(window.devicePixelRatio || 1, 2);
                const renderViewport = page.getViewport({ scale: cssScale * pixelRatio });
                // Render into a per-task offscreen canvas. Resize/scroll events can
                // cancel one PDF.js task and start the next before cancellation has
                // fully settled; sharing the visible canvas between those tasks
                // causes PDF.js to reject the newer render.
                const renderCanvas = window.document.createElement("canvas");
                renderCanvas.width = Math.floor(renderViewport.width);
                renderCanvas.height = Math.floor(renderViewport.height);
                const context = renderCanvas.getContext("2d", { alpha: false });
                if (!context) throw new Error("Canvas is unavailable.");
                renderTask = page.render({ canvasContext: context, viewport: renderViewport });
                await renderTask.promise;
                if (cancelled) return;
                const canvas = canvasRef.current;
                if (!canvas) return;
                canvas.width = renderCanvas.width;
                canvas.height = renderCanvas.height;
                canvas.style.width = `${Math.floor(naturalViewport.width * cssScale)}px`;
                canvas.style.height = `${Math.floor(naturalViewport.height * cssScale)}px`;
                const visibleContext = canvas.getContext("2d", { alpha: false });
                if (!visibleContext) throw new Error("Canvas is unavailable.");
                visibleContext.drawImage(renderCanvas, 0, 0);
                setRenderError("");
            } catch (error: any) {
                if (!cancelled && error?.name !== "RenderingCancelledException") {
                    console.error(`Could not render PDF page ${pageNumber}:`, error);
                    setRenderError(String(error));
                }
            }
        };
        void renderPage();
        return () => {
            cancelled = true;
            renderTask?.cancel?.();
        };
    }, [availableWidth, document, isVisible, pageNumber, zoom]);

    const placeholderWidth = Math.max(240, availableWidth * zoom);
    return (
        <div
            className="pdf-page-shell"
            ref={wrapperRef}
            data-page-number={pageNumber}
            style={{ minHeight: `${Math.min(1200, placeholderWidth * aspectRatio)}px` }}
        >
            <div className="pdf-page-number">{pageNumber}</div>
            {renderError ? (
                <div className="pdf-preview-error">Could not render page {pageNumber}.</div>
            ) : (
                <canvas ref={canvasRef} className={`pdf-page-canvas ${isVisible ? "is-visible" : ""}`} />
            )}
        </div>
    );
}

export default function PDFPreview({
    dataBase64,
    documentName,
    busy = false,
    busyLabel = "Building PDF preview...",
    activePage = 0,
    totalPageCount = 0,
}: PDFPreviewProps) {
    const viewportRef = useRef<HTMLDivElement>(null);
    const pagesRef = useRef<HTMLDivElement>(null);
    const [document, setDocument] = useState<PDFDocumentProxy | null>(null);
    const [pageCount, setPageCount] = useState(0);
    const [availableWidth, setAvailableWidth] = useState(0);
    const [zoom, setZoom] = useState(1);
    const [error, setError] = useState("");

    const bytes = useMemo(() => dataBase64 ? decodeBase64(dataBase64) : null, [dataBase64]);

    useEffect(() => {
        const node = viewportRef.current;
        if (!node) return;
        const updateWidth = () => setAvailableWidth(Math.max(240, node.clientWidth - 34));
        updateWidth();
        const observer = new ResizeObserver(updateWidth);
        observer.observe(node);
        return () => observer.disconnect();
    }, []);

    useEffect(() => {
        if (!bytes) {
            setDocument(null);
            setPageCount(0);
            return;
        }
        let active = true;
        let loadingTask: any = null;
        void loadPDFJS().then(pdfjsLib => {
            if (!active) return;
            loadingTask = pdfjsLib.getDocument({
                data: bytes,
                isEvalSupported: false,
                useWorkerFetch: false,
            });
            return loadingTask.promise;
        }).then((loaded: PDFDocumentProxy | undefined) => {
            if (!loaded) return;
            if (!active) {
                void loaded.destroy();
                return;
            }
            setDocument(loaded);
            setPageCount(loaded.numPages);
            setError("");
        }).catch((loadError: unknown) => {
            if (active) setError(String(loadError));
        });
        return () => {
            active = false;
            void loadingTask?.destroy?.();
        };
    }, [bytes]);

    const pageNumbers = useMemo(() => Array.from({ length: pageCount }, (_, index) => index + 1), [pageCount]);

    useEffect(() => {
        if (!document || activePage <= 0 || pageCount < activePage) return;
        const frame = window.requestAnimationFrame(() => {
            const page = pagesRef.current?.querySelector<HTMLElement>(`[data-page-number="${activePage}"]`);
            page?.scrollIntoView({ behavior: "smooth", block: "start" });
        });
        return () => window.cancelAnimationFrame(frame);
    }, [activePage, document, pageCount]);

    return (
        <div className="pdf-preview" ref={viewportRef}>
            <div className="pdf-preview-toolbar">
                <span className="material-symbols-outlined pdf-preview-icon">picture_as_pdf</span>
                <span className="pdf-preview-name" title={documentName}>{documentName}</span>
                <span className="pdf-preview-count">
                    {pageCount ? `${pageCount}${totalPageCount > pageCount ? ` / ${totalPageCount}` : ""} pages` : "Loading..."}
                </span>
                <button type="button" onClick={() => setZoom(value => Math.max(0.55, value - 0.1))} title="Zoom out" disabled={zoom <= 0.55}>
                    <span className="material-symbols-outlined">zoom_out</span>
                </button>
                <span className="pdf-preview-zoom">{Math.round(zoom * 100)}%</span>
                <button type="button" onClick={() => setZoom(value => Math.min(1.8, value + 0.1))} title="Zoom in" disabled={zoom >= 1.8}>
                    <span className="material-symbols-outlined">zoom_in</span>
                </button>
            </div>
            <div className="pdf-pages" ref={pagesRef}>
                {error && <div className="pdf-preview-error">Could not open this PDF preview.<br />{error}</div>}
                {!error && document && pageNumbers.map(pageNumber => (
                    <PDFPageCanvas
                        key={`${documentName}-${pageNumber}`}
                        document={document}
                        pageNumber={pageNumber}
                        availableWidth={availableWidth}
                        zoom={zoom}
                    />
                ))}
            </div>
            {busy && (
                <div className="pdf-preview-busy" role="status">
                    <span className="pdf-preview-spinner" />
                    <span>{busyLabel}</span>
                </div>
            )}
        </div>
    );
}
