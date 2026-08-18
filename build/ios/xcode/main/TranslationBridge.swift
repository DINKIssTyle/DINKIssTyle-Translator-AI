import Foundation
import SwiftUI
import NaturalLanguage
import Translation
#if os(iOS) && canImport(MLKitTranslate)
import MLKitTranslate
#endif

public typealias DKSTTranslationCompletion = @convention(c) (UInt64, UnsafePointer<CChar>?) -> Void

private struct TranslationPayload: Decodable {
    let texts: [String]
    let sourceLanguage: String
    let targetLanguage: String
}

private struct TranslationResult: Encodable {
    let texts: [String]
    let error: String
}

private struct TranslationBridgeError: LocalizedError {
    let errorDescription: String?
}

private func detailedTranslationError(_ error: Error) -> String {
    let nsError = error as NSError
    var details = [String]()
    if let description = (error as? LocalizedError)?.errorDescription,
       !description.isEmpty {
        details.append(description)
    } else if !nsError.localizedDescription.isEmpty {
        details.append(nsError.localizedDescription)
    }
    if let reason = (error as? LocalizedError)?.failureReason,
       !reason.isEmpty,
       !details.contains(reason) {
        details.append(reason)
    }
    details.append("\(nsError.domain) (\(nsError.code))")
    return details.joined(separator: " — ")
}

private func isTranslationServiceInterruption(_ error: Error) -> Bool {
    let nsError = error as NSError
    return (nsError.domain == "TranslationErrorDomain" && nsError.code == 13)
        || (nsError.domain == NSCocoaErrorDomain && nsError.code == 4097)
}

@available(iOS 18.0, macOS 15.0, *)
@MainActor
private final class TranslationCoordinator: ObservableObject {
    struct Job {
        let id: UInt64
        let payload: TranslationPayload
        let completion: DKSTTranslationCompletion
    }

    @Published var configuration: TranslationSession.Configuration?
    private var job: Job?
    private var configuredSource = ""
    private var configuredTarget = ""
    private weak var activeSession: TranslationSession?
    private var hostReady = false
    private var serviceRetryCount = 0
    private(set) var requiresFreshSession = false

    var hasActiveJob: Bool { job != nil }

    private var shouldTranslateSerially: Bool {
        serviceRetryCount > 0
    }

    private var nativeBatchSize: Int {
#if os(macOS)
        return 12
#else
        return 8
#endif
    }

    func submit(_ job: Job) {
        guard self.job == nil else {
            complete(job, texts: [], error: "Apple Translation still has an active document")
            return
        }
        self.job = job
        serviceRetryCount = 0
        print("[DKSTTranslation/Swift] Job #\(job.id) submitted (texts: \(job.payload.texts.count), \(job.payload.sourceLanguage) -> \(job.payload.targetLanguage))")

        guard hostReady else { return }
        configure(for: job)
    }

    func hostDidAppear() {
        guard !hostReady else { return }
        hostReady = true
        if let job { configure(for: job) }
    }

    func reject(_ job: Job, error: String) {
        self.job = job
        finish(job, texts: [], error: error)
    }

    private func normalizeLanguageCode(_ raw: String) -> String? {
        let lower = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if lower.isEmpty || lower == "und" || lower == "auto" || lower == "automatic" || lower == "detect" {
            return nil
        }
        switch lower {
        case "en", "eng", "english": return "en"
        case "ko", "kor", "korean": return "ko"
        case "ja", "jpn", "japanese": return "ja"
        case "zh", "zho", "chi", "chinese": return "zh"
        case "de", "deu", "ger", "german": return "de"
        case "fr", "fra", "fre", "french": return "fr"
        case "es", "spa", "spanish": return "es"
        case "it", "ita", "italian": return "it"
        case "ru", "rus", "russian": return "ru"
        case "pt", "por", "portuguese": return "pt"
        case "ar", "ara", "arabic": return "ar"
        case "hi", "hin", "hindi": return "hi"
        case "vi", "vie", "vietnamese": return "vi"
        case "th", "tha", "thai": return "th"
        case "id", "ind", "indonesian": return "id"
        case "nl", "nld", "dut", "dutch": return "nl"
        case "pl", "pol", "polish": return "pl"
        case "tr", "tur", "turkish": return "tr"
        case "uk", "ukr", "ukrainian": return "uk"
        default:
            let parts = lower.split(separator: "-")
            if let first = parts.first, let base = normalizeLanguageCode(String(first)) {
                if parts.count > 1 { return "\(base)-\(parts[1])" }
                return base
            }
            return lower
        }
    }

    private func detectLanguage(from texts: [String]) -> String? {
        let sample = texts.prefix(3).joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !sample.isEmpty else { return nil }
        let recognizer = NLLanguageRecognizer()
        recognizer.processString(sample)
        if let dominant = recognizer.dominantLanguage?.rawValue {
            return normalizeLanguageCode(dominant)
        }
        return nil
    }

    private func configure(for job: Job) {
        guard self.job?.id == job.id else { return }

        var sourceCode = normalizeLanguageCode(job.payload.sourceLanguage)
        if sourceCode == nil {
            sourceCode = detectLanguage(from: job.payload.texts)
        }
        let targetCode = normalizeLanguageCode(job.payload.targetLanguage) ?? "ko"

        let source = sourceCode == nil ? nil : Locale.Language(identifier: sourceCode!)
        let target = Locale.Language(identifier: targetCode)

        if configuration != nil,
           configuredSource == (sourceCode ?? ""),
           configuredTarget == targetCode {
            configuration?.invalidate()
        } else {
            configuredSource = sourceCode ?? ""
            configuredTarget = targetCode
            configuration = .init(source: source, target: target)
        }
    }

    func translate(using session: TranslationSession) async {
        guard let activeJob = job else { return }
        let jobStartTime = CFAbsoluteTimeGetCurrent()
        activeSession = session
        do {
            if let source = session.sourceLanguage,
               let target = session.targetLanguage,
               await LanguageAvailability().status(from: source, to: target) == .unsupported {
                throw TranslationBridgeError(
                    errorDescription: "Apple Translation does not support \(source.minimalIdentifier) → \(target.minimalIdentifier)"
                )
            }
            let prepStart = CFAbsoluteTimeGetCurrent()
            try await session.prepareTranslation()
            let prepElapsed = (CFAbsoluteTimeGetCurrent() - prepStart) * 1000
            print(String(format: "[DKSTTranslation/Swift] Job #%llu prepareTranslation completed in %.1fms", activeJob.id, prepElapsed))

            var translated = Array(repeating: "", count: activeJob.payload.texts.count)

            if shouldTranslateSerially {
                for index in activeJob.payload.texts.indices {
                    guard job?.id == activeJob.id else { return }
                    let response = try await session.translate(activeJob.payload.texts[index])
                    translated[index] = response.targetText
                }
            } else {
                for offset in stride(from: 0, to: activeJob.payload.texts.count, by: nativeBatchSize) {
                    let end = min(offset + nativeBatchSize, activeJob.payload.texts.count)
                    let requests = (offset..<end).map { index in
                        TranslationSession.Request(
                            sourceText: activeJob.payload.texts[index],
                            clientIdentifier: String(index)
                        )
                    }
                    let batchStart = CFAbsoluteTimeGetCurrent()
                    let responses = try await session.translations(from: requests)
                    let batchElapsed = (CFAbsoluteTimeGetCurrent() - batchStart) * 1000
                    print(String(format: "[DKSTTranslation/Swift] Job #%llu batch %d..%d (%d texts) inference in %.1fms", activeJob.id, offset, end, requests.count, batchElapsed))

                    for response in responses {
                        guard let identifier = response.clientIdentifier,
                              let index = Int(identifier),
                              translated.indices.contains(index) else { continue }
                        translated[index] = response.targetText
                    }
                }
            }
            let totalElapsed = (CFAbsoluteTimeGetCurrent() - jobStartTime) * 1000
            print(String(format: "[DKSTTranslation/Swift] Job #%llu finished in %.1fms", activeJob.id, totalElapsed))
            if translated.contains(where: \.isEmpty) && !activeJob.payload.texts.contains("") {
                finish(activeJob, texts: [], error: "Apple Translation returned an incomplete batch")
            } else {
                finish(activeJob, texts: translated, error: "")
            }
        } catch {
            let totalElapsed = (CFAbsoluteTimeGetCurrent() - jobStartTime) * 1000
            print(String(format: "[DKSTTranslation/Swift] Job #%llu error after %.1fms: %@", activeJob.id, totalElapsed, error.localizedDescription))
            if isTranslationServiceInterruption(error), serviceRetryCount < 2,
               job?.id == activeJob.id {
                serviceRetryCount += 1
                let restartDelay = UInt64(serviceRetryCount) * 1_000_000_000
                try? await Task.sleep(nanoseconds: restartDelay)
                if job?.id == activeJob.id { configuration?.invalidate() }
                return
            }
            finish(activeJob, texts: [], error: detailedTranslationError(error))
        }
    }

    @discardableResult
    func cancel(_ id: UInt64) -> Bool {
        guard let activeJob = job, activeJob.id == id else { return false }
        cancelActiveSession()
        finish(activeJob, texts: [], error: "Translation cancelled")
        return true
    }

    @discardableResult
    func supersedeActiveJob() -> Bool {
        guard let activeJob = job else { return false }
        cancelActiveSession()
        finish(activeJob, texts: [], error: "Translation was superseded by a newer document")
        return true
    }

    private func cancelActiveSession() {
#if os(iOS)
        if #available(iOS 26.0, *) { activeSession?.cancel() }
#elseif os(macOS)
        if #available(macOS 26.0, *) { activeSession?.cancel() }
#endif
        configuration = nil
    }

    private func finish(_ activeJob: Job, texts: [String], error: String) {
        guard job?.id == activeJob.id else { return }
        job = nil
        activeSession = nil
        serviceRetryCount = 0
        if !error.isEmpty {
            requiresFreshSession = true
            configuration = nil
            configuredSource = ""
            configuredTarget = ""
        }
        complete(activeJob, texts: texts, error: error)
    }

    private func complete(_ activeJob: Job, texts: [String], error: String) {
        let result = TranslationResult(texts: texts, error: error)
        let encoded = (try? JSONEncoder().encode(result))
            ?? Data(#"{"texts":[],"error":"Apple Translation failed"}"#.utf8)
        let json = String(decoding: encoded, as: UTF8.self)
        json.withCString { activeJob.completion(activeJob.id, $0) }
    }
}

@available(iOS 18.0, macOS 15.0, *)
private struct TranslationHost: View {
    @ObservedObject var coordinator: TranslationCoordinator

    var body: some View {
        Color.clear
            .frame(width: 1, height: 1)
            .accessibilityHidden(true)
            .onAppear { coordinator.hostDidAppear() }
            .translationTask(coordinator.configuration) { session in
                await coordinator.translate(using: session)
            }
    }
}

@available(iOS 18.0, macOS 15.0, *)
@MainActor
private final class TranslationBridge {
    static let shared = TranslationBridge()

    private var coordinator = TranslationCoordinator()
    private var host: AnyObject?
    private var cancelledBeforeSubmit = Set<UInt64>()

    func submit(_ job: TranslationCoordinator.Job) {
        if cancelledBeforeSubmit.remove(job.id) != nil {
            coordinator.reject(job, error: "Translation cancelled")
            resetSessionHost()
            return
        }
        if coordinator.hasActiveJob {
            coordinator.supersedeActiveJob()
            resetSessionHost()
        } else if coordinator.requiresFreshSession {
            resetSessionHost()
        }
        guard installHostIfNeeded() else {
            coordinator.reject(job, error: "Apple Translation could not attach its session to the active window")
            return
        }
        coordinator.submit(job)
    }

    func cancel(_ id: UInt64) {
        if coordinator.cancel(id) {
            resetSessionHost()
        } else {
            cancelledBeforeSubmit.insert(id)
            Task { @MainActor [weak self] in
                try? await Task.sleep(nanoseconds: 60_000_000_000)
                self?.cancelledBeforeSubmit.remove(id)
            }
        }
    }

    private func resetSessionHost() {
#if os(iOS)
        if let controller = host as? UIViewController {
            controller.willMove(toParent: nil)
            controller.view.removeFromSuperview()
            controller.removeFromParent()
        }
#elseif os(macOS)
        (host as? NSView)?.removeFromSuperview()
#endif
        host = nil
        coordinator = TranslationCoordinator()
    }

    private func installHostIfNeeded() -> Bool {
        guard host == nil else { return true }
#if os(iOS)
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        let windows = scenes.filter { $0.activationState == .foregroundActive }.flatMap(\.windows)
            + scenes.filter { $0.activationState != .foregroundActive }.flatMap(\.windows)
        guard let root = windows.first(where: \.isKeyWindow)?.rootViewController
                ?? windows.first(where: { !$0.isHidden })?.rootViewController else { return false }
        let controller = UIHostingController(rootView: TranslationHost(coordinator: coordinator))
        controller.view.backgroundColor = .clear
        controller.view.isUserInteractionEnabled = false
        controller.view.translatesAutoresizingMaskIntoConstraints = false
        root.addChild(controller)
        root.view.addSubview(controller.view)
        NSLayoutConstraint.activate([
            controller.view.leadingAnchor.constraint(equalTo: root.view.leadingAnchor),
            controller.view.topAnchor.constraint(equalTo: root.view.topAnchor),
            controller.view.widthAnchor.constraint(equalToConstant: 1),
            controller.view.heightAnchor.constraint(equalToConstant: 1),
        ])
        controller.didMove(toParent: root)
        host = controller
#elseif os(macOS)
        guard let container = NSApp.keyWindow?.contentView
                ?? NSApp.mainWindow?.contentView
                ?? NSApp.windows.first(where: { $0.isVisible })?.contentView else { return false }
        let view = NSHostingView(rootView: TranslationHost(coordinator: coordinator))
        view.translatesAutoresizingMaskIntoConstraints = false
        container.addSubview(view)
        NSLayoutConstraint.activate([
            view.leadingAnchor.constraint(equalTo: container.leadingAnchor),
            view.topAnchor.constraint(equalTo: container.topAnchor),
            view.widthAnchor.constraint(equalToConstant: 1),
            view.heightAnchor.constraint(equalToConstant: 1),
        ])
        host = view
#endif
        return host != nil
    }
}

@_cdecl("DKSTAppleTranslationLanguages")
public func DKSTAppleTranslationLanguages(
    _ requestID: UInt64,
    _ completion: DKSTTranslationCompletion?
) {
    guard let completion else { return }
    Task { @MainActor in
#if os(iOS)
        guard #available(iOS 18.0, *) else { return }
#elseif os(macOS)
        guard #available(macOS 15.0, *) else { return }
#endif
        let languages = await LanguageAvailability().supportedLanguages
            .map(\.minimalIdentifier)
            .sorted()
        let result = TranslationResult(texts: languages, error: "")
        let encoded = (try? JSONEncoder().encode(result))
            ?? Data(#"{"texts":[],"error":"Unable to list Apple Translation languages"}"#.utf8)
        String(decoding: encoded, as: UTF8.self).withCString { completion(requestID, $0) }
    }
}

@_cdecl("DKSTAppleTranslationCancel")
public func DKSTAppleTranslationCancel(_ requestID: UInt64) {
    Task { @MainActor in
#if os(iOS)
        guard #available(iOS 18.0, *) else { return }
#elseif os(macOS)
        guard #available(macOS 15.0, *) else { return }
#endif
        TranslationBridge.shared.cancel(requestID)
    }
}

@_cdecl("DKSTAppleTranslationAvailable")
public func DKSTAppleTranslationAvailable() -> Int32 {
#if os(iOS)
    if #available(iOS 18.0, *) { return 1 }
#elseif os(macOS)
    if #available(macOS 15.0, *) { return 1 }
#endif
    return 0
}

@_cdecl("DKSTAppleTranslationSubmit")
public func DKSTAppleTranslationSubmit(
    _ requestJSON: UnsafePointer<CChar>?,
    _ requestID: UInt64,
    _ completion: DKSTTranslationCompletion?
) {
    guard let requestJSON, let completion else { return }
    let json = String(cString: requestJSON)
    guard let data = json.data(using: .utf8),
          let payload = try? JSONDecoder().decode(TranslationPayload.self, from: data) else {
        let error = #"{"texts":[],"error":"Invalid Apple Translation request"}"#
        error.withCString { completion(requestID, $0) }
        return
    }

    Task { @MainActor in
#if os(iOS)
        guard #available(iOS 18.0, *) else {
            let error = #"{"texts":[],"error":"Apple Translation requires iOS 18 or later"}"#
            error.withCString { completion(requestID, $0) }
            return
        }
#elseif os(macOS)
        guard #available(macOS 15.0, *) else {
            let error = #"{"texts":[],"error":"Apple Translation requires macOS 15 or later"}"#
            error.withCString { completion(requestID, $0) }
            return
        }
#endif
        TranslationBridge.shared.submit(.init(id: requestID, payload: payload, completion: completion))
    }
}

#if os(iOS) && canImport(MLKitTranslate)
@MainActor
private final class GoogleTranslationJob {
    let id: UInt64
    let payload: TranslationPayload
    let translator: Translator
    let completion: DKSTTranslationCompletion
    var translated: [String]

    init(id: UInt64, payload: TranslationPayload, translator: Translator, completion: @escaping DKSTTranslationCompletion) {
        self.id = id
        self.payload = payload
        self.translator = translator
        self.completion = completion
        self.translated = []
    }
}

@MainActor
private final class GoogleTranslationBridge {
    static let shared = GoogleTranslationBridge()
    private var jobs: [UInt64: GoogleTranslationJob] = [:]

    private func normalizeLanguageCode(_ raw: String) -> String? {
        let lower = raw.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if lower.isEmpty || lower == "und" || lower == "auto" || lower == "automatic" || lower == "detect" || lower == "auto detect" {
            return nil
        }
        switch lower {
        case "en", "eng", "english": return "en"
        case "ko", "kor", "korean": return "ko"
        case "ja", "jpn", "japanese": return "ja"
        case "zh", "zho", "chi", "chinese": return "zh"
        case "de", "deu", "ger", "german": return "de"
        case "fr", "fra", "fre", "french": return "fr"
        case "es", "spa", "spanish": return "es"
        case "it", "ita", "italian": return "it"
        case "ru", "rus", "russian": return "ru"
        case "pt", "por", "portuguese": return "pt"
        case "ar", "ara", "arabic": return "ar"
        case "hi", "hin", "hindi": return "hi"
        case "vi", "vie", "vietnamese": return "vi"
        case "th", "tha", "thai": return "th"
        case "id", "ind", "indonesian": return "id"
        case "nl", "nld", "dut", "dutch": return "nl"
        case "pl", "pol", "polish": return "pl"
        case "tr", "tur", "turkish": return "tr"
        case "uk", "ukr", "ukrainian": return "uk"
        default:
            let parts = lower.split(separator: "-")
            if let first = parts.first, let base = normalizeLanguageCode(String(first)) {
                return base
            }
            return lower
        }
    }

    private func language(_ identifier: String) -> TranslateLanguage {
        let base = identifier.replacingOccurrences(of: "_", with: "-")
            .split(separator: "-", maxSplits: 1)
            .first
            .map(String.init) ?? identifier
        return TranslateLanguage(rawValue: base.lowercased())
    }

    private func detectLanguage(from texts: [String]) -> String? {
        let sample = texts.prefix(3).joined(separator: "\n").trimmingCharacters(in: .whitespacesAndNewlines)
        guard !sample.isEmpty else { return nil }
        let recognizer = NLLanguageRecognizer()
        recognizer.processString(sample)
        if let dominant = recognizer.dominantLanguage?.rawValue {
            return normalizeLanguageCode(dominant)
        }
        return nil
    }

    func submit(id: UInt64, payload: TranslationPayload, completion: @escaping DKSTTranslationCompletion) {
        var sourceCode = normalizeLanguageCode(payload.sourceLanguage)
        if sourceCode == nil {
            sourceCode = detectLanguage(from: payload.texts) ?? "en"
        }
        let targetCode = normalizeLanguageCode(payload.targetLanguage) ?? "ko"

        let source = language(sourceCode!)
        let target = language(targetCode)
        let supported = TranslateLanguage.allLanguages()
        guard supported.contains(source), supported.contains(target) else {
            finish(id: id, completion: completion, texts: [], error: "This language pair (\(sourceCode ?? "auto") -> \(targetCode)) is not supported by Google ML Kit")
            return
        }
        if let previous = jobs[id] {
            finish(previous, texts: [], error: "Google ML Kit translation was superseded")
        }
        let options = TranslatorOptions(sourceLanguage: source, targetLanguage: target)
        let job = GoogleTranslationJob(
            id: id,
            payload: payload,
            translator: Translator.translator(options: options),
            completion: completion
        )
        jobs[id] = job
        let conditions = ModelDownloadConditions(allowsCellularAccess: true, allowsBackgroundDownloading: true)
        job.translator.downloadModelIfNeeded(with: conditions) { [weak self, weak job] (error: Error?) in
            Task { @MainActor in
                guard let self, let job, self.jobs[id] === job else { return }
                if let error {
                    self.finish(job, texts: [], error: detailedTranslationError(error))
                } else {
                    self.translateNext(job, index: 0)
                }
            }
        }
    }

    func cancel(_ id: UInt64) {
        if let job = jobs.removeValue(forKey: id) {
            finish(job, texts: [], error: "Translation cancelled")
        }
    }

    private func translateNext(_ job: GoogleTranslationJob, index: Int) {
        guard jobs[job.id] === job else { return }
        guard index < job.payload.texts.count else {
            finish(job, texts: job.translated, error: "")
            return
        }
        let currentText = job.payload.texts[index]
        job.translator.translate(currentText) { [weak self, weak job] (result: String?, error: Error?) in
            Task { @MainActor in
                guard let self, let job, self.jobs[job.id] === job else { return }
                if let error {
                    self.finish(job, texts: [], error: detailedTranslationError(error))
                    return
                }
                job.translated.append(result ?? "")
                self.translateNext(job, index: index + 1)
            }
        }
    }

    private func finish(_ job: GoogleTranslationJob, texts: [String], error: String) {
        jobs.removeValue(forKey: job.id)
        finish(id: job.id, completion: job.completion, texts: texts, error: error)
    }

    private func finish(id: UInt64, completion: DKSTTranslationCompletion, texts: [String], error: String) {
        let result = TranslationResult(texts: texts, error: error)
        let encoded = (try? JSONEncoder().encode(result))
            ?? Data(#"{"texts":[],"error":"Google ML Kit translation failed"}"#.utf8)
        String(decoding: encoded, as: UTF8.self).withCString { completion(id, $0) }
    }
}
#endif

@_cdecl("DKSTGoogleTranslationAvailable")
public func DKSTGoogleTranslationAvailable() -> Int32 {
#if os(iOS) && canImport(MLKitTranslate)
    return 1
#else
    return 0
#endif
}

@_cdecl("DKSTGoogleTranslationCancel")
public func DKSTGoogleTranslationCancel(_ requestID: UInt64) {
#if os(iOS) && canImport(MLKitTranslate)
    Task { @MainActor in GoogleTranslationBridge.shared.cancel(requestID) }
#endif
}

@_cdecl("DKSTGoogleTranslationSubmit")
public func DKSTGoogleTranslationSubmit(
    _ requestJSON: UnsafePointer<CChar>?,
    _ requestID: UInt64,
    _ completion: DKSTTranslationCompletion?
) {
    guard let requestJSON, let completion else { return }
#if os(iOS) && canImport(MLKitTranslate)
    let json = String(cString: requestJSON)
    guard let data = json.data(using: .utf8),
          let payload = try? JSONDecoder().decode(TranslationPayload.self, from: data) else {
        let error = #"{"texts":[],"error":"Invalid Google ML Kit translation request"}"#
        error.withCString { completion(requestID, $0) }
        return
    }
    Task { @MainActor in
        GoogleTranslationBridge.shared.submit(id: requestID, payload: payload, completion: completion)
    }
#else
    let error = #"{"texts":[],"error":"Google ML Kit is unavailable in this build"}"#
    error.withCString { completion(requestID, $0) }
#endif
}
