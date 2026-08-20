import Foundation
import CLiteRTLM
import Network

private actor DKSTLiteRTEngine {
  private var engine: OpaquePointer?
  private var configuredModelURL: URL?

  deinit {
    if let engine { litert_lm_engine_delete(engine) }
  }

  func configure(modelPath: String) throws {
    guard modelPath.lowercased().hasSuffix(".litertlm") else {
      throw NSError(domain: "DKSTLiteRTLM", code: 2, userInfo: [NSLocalizedDescriptionKey: "modelPath must reference a .litertlm file"])
    }
    var targetURL = URL(fileURLWithPath: modelPath)
    if !FileManager.default.fileExists(atPath: targetURL.path) {
      if let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first {
        let candidate = docs.appendingPathComponent("models").appendingPathComponent(targetURL.lastPathComponent)
        if FileManager.default.fileExists(atPath: candidate.path) {
          targetURL = candidate
        }
      }
      if !FileManager.default.fileExists(atPath: targetURL.path),
         let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first {
        let candidate = appSupport.appendingPathComponent("DKST Translator AI").appendingPathComponent("models").appendingPathComponent(targetURL.lastPathComponent)
        if FileManager.default.fileExists(atPath: candidate.path) {
          targetURL = candidate
        }
      }
    }
    configuredModelURL = targetURL
    if let engine { litert_lm_engine_delete(engine) }
    engine = nil
  }

  func findEffectiveModelURL() -> URL? {
    if let configured = configuredModelURL, FileManager.default.fileExists(atPath: configured.path) {
      return configured
    }
    var searchDirs: [URL] = []
    if let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first {
      searchDirs.append(appSupport.appendingPathComponent("DKST Translator AI").appendingPathComponent("models"))
      searchDirs.append(appSupport.appendingPathComponent("models"))
    }
    if let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first {
      searchDirs.append(docs.appendingPathComponent("models"))
    }
    for modelsDir in searchDirs {
      if let files = try? FileManager.default.contentsOfDirectory(at: modelsDir, includingPropertiesForKeys: nil) {
        let litertFiles = files.filter { $0.pathExtension.lowercased() == "litertlm" }
        if let gpuModel = litertFiles.first(where: { $0.lastPathComponent.contains("-gpu") }) {
          return gpuModel
        }
        if let first = litertFiles.first {
          return first
        }
      }
    }
    // Check Bundle
    return Bundle.main.url(forResource: "gemma-2b-it", withExtension: "litertlm", subdirectory: "models")
  }

  func modelExists() -> Bool {
    findEffectiveModelURL() != nil
  }

  func generate(_ prompt: String) throws -> String {
    guard let modelURL = findEffectiveModelURL() else {
      throw NSError(
        domain: "DKSTLiteRTLM",
        code: 1,
        userInfo: [NSLocalizedDescriptionKey: "No .litertlm model found in Documents/models or App Bundle"]
      )
    }

    let cache = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first?.path

    // Attempt 1: Try with existing or newly created GPU engine
    if engine == nil {
      engine = Self.createEngine(modelPath: modelURL.path, backend: "gpu", cacheDir: cache)
        ?? Self.createEngine(modelPath: modelURL.path, backend: "cpu", cacheDir: cache)
    }

    guard let activeEngine = engine else {
      throw NSError(domain: "DKSTLiteRTLM", code: 3, userInfo: [NSLocalizedDescriptionKey: "LiteRT-LM engine initialization failed"])
    }

    if let result = Self.runInference(engine: activeEngine, prompt: prompt) {
      return result
    }

    // Attempt 2: If GPU generation failed, recreate engine with CPU backend and retry
    NSLog("LiteRT-LM GPU inference failed; falling back to CPU backend...")
    if let oldEngine = engine { litert_lm_engine_delete(oldEngine) }
    engine = nil

    guard let cpuEngine = Self.createEngine(modelPath: modelURL.path, backend: "cpu", cacheDir: cache) else {
      throw NSError(domain: "DKSTLiteRTLM", code: 5, userInfo: [NSLocalizedDescriptionKey: "LiteRT-LM CPU engine fallback failed"])
    }
    engine = cpuEngine

    if let result = Self.runInference(engine: cpuEngine, prompt: prompt) {
      return result
    }

    throw NSError(domain: "DKSTLiteRTLM", code: 5, userInfo: [NSLocalizedDescriptionKey: "LiteRT-LM generation failed on both GPU and CPU"])
  }

  private static func runInference(engine: OpaquePointer, prompt: String) -> String? {
    guard let conversation = litert_lm_conversation_create(engine, nil) else {
      return nil
    }
    defer { litert_lm_conversation_delete(conversation) }

    // Try format 1: Structured content array
    let formats: [[String: Any]] = [
      ["role": "user", "content": [["type": "text", "text": prompt]]],
      ["role": "user", "content": prompt],
    ]

    for msgObj in formats {
      if let messageData = try? JSONSerialization.data(withJSONObject: msgObj),
         let messageJSON = String(data: messageData, encoding: .utf8),
         let response = litert_lm_conversation_send_message(conversation, messageJSON, nil, nil) {
        defer { litert_lm_json_response_delete(response) }
        if let responseChars = litert_lm_json_response_get_string(response) {
          let responseString = String(cString: responseChars)
          if let text = parseResponseText(responseString) {
            return text
          }
          if !responseString.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty {
            return responseString
          }
        }
      }
    }

    return nil
  }

  private static func parseResponseText(_ responseJSON: String) -> String? {
    guard let data = responseJSON.data(using: .utf8),
          let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
    else { return nil }

    if let content = object["content"] as? [[String: Any]] {
      let text = content.compactMap { $0["text"] as? String }.joined(separator: " ")
      if !text.isEmpty { return text }
    }
    if let text = object["text"] as? String, !text.isEmpty {
      return text
    }
    if let choices = object["choices"] as? [[String: Any]],
       let first = choices.first {
      if let delta = first["delta"] as? [String: Any], let c = delta["content"] as? String { return c }
      if let msg = first["message"] as? [String: Any], let c = msg["content"] as? String { return c }
    }
    return nil
  }

  private static func createEngine(modelPath: String, backend: String, cacheDir: String?) -> OpaquePointer? {
    guard let settings = litert_lm_engine_settings_create(modelPath, backend, nil, nil) else {
      return nil
    }
    defer { litert_lm_engine_settings_delete(settings) }
    if let cacheDir { litert_lm_engine_settings_set_cache_dir(settings, cacheDir) }
    return litert_lm_engine_create(settings)
  }
}

private final class DKSTLiteRTLMServer {
  static let shared = DKSTLiteRTLMServer()

  private let queue = DispatchQueue(label: "com.dinkisstyle.translatorai.litertlm")
  private let runtime = DKSTLiteRTEngine()
  private var listener: NWListener?

  func start() {
    guard listener == nil else { return }
    do {
      let parameters = NWParameters.tcp
      parameters.requiredLocalEndpoint = .hostPort(host: "127.0.0.1", port: 9379)
      let listener = try NWListener(using: parameters)
      listener.newConnectionHandler = { [weak self] connection in
        self?.receive(connection, buffer: Data())
      }
      listener.start(queue: queue)
      self.listener = listener
    } catch {
      NSLog("LiteRT-LM adapter failed to start: %@", error.localizedDescription)
    }
  }

  private func receive(_ connection: NWConnection, buffer: Data) {
    connection.start(queue: queue)
    connection.receive(minimumIncompleteLength: 1, maximumLength: 1_048_576) {
      [weak self] data, _, isComplete, error in
      guard let self else { return }
      var next = buffer
      if let data { next.append(data) }
      if self.requestIsComplete(next) {
        self.handle(connection, request: next)
      } else if !isComplete && error == nil {
        self.receive(connection, buffer: next)
      } else {
        connection.cancel()
      }
    }
  }

  private func requestIsComplete(_ data: Data) -> Bool {
    guard
      let text = String(data: data, encoding: .utf8),
      let boundary = text.range(of: "\r\n\r\n")
    else { return false }
    let headers = String(text[..<boundary.lowerBound])
    let length = headers.split(separator: "\r\n").first { $0.lowercased().hasPrefix("content-length:") }
      .flatMap { Int($0.split(separator: ":", maxSplits: 1).last?.trimmingCharacters(in: .whitespaces) ?? "") } ?? 0
    return text.distance(from: boundary.upperBound, to: text.endIndex) >= length
  }

  private func handle(_ connection: NWConnection, request data: Data) {
    guard let text = String(data: data, encoding: .utf8) else {
      send(connection, status: "400 Bad Request", type: "text/plain", body: Data("bad request".utf8))
      return
    }
    let requestLine = text.split(separator: "\r\n", maxSplits: 1).first.map(String.init) ?? ""
    if requestLine.hasPrefix("GET /v1/models ") {
      Task {
        var models: [[String: Any]] = []
        if let docs = FileManager.default.urls(for: .documentDirectory, in: .userDomainMask).first {
          let modelsDir = docs.appendingPathComponent("models")
          if let files = try? FileManager.default.contentsOfDirectory(at: modelsDir, includingPropertiesForKeys: nil) {
            for file in files where file.pathExtension.lowercased() == "litertlm" {
              let modelID = file.deletingPathExtension().lastPathComponent
              models.append(["id": modelID, "object": "model"])
            }
          }
        }
        if let effective = await runtime.findEffectiveModelURL() {
          let effectiveID = effective.deletingPathExtension().lastPathComponent
          if !models.contains(where: { ($0["id"] as? String) == effectiveID }) {
            models.insert(["id": effectiveID, "object": "model"], at: 0)
          }
        }
        sendJSON(connection, ["object": "list", "data": models])
      }
      return
    }
    if requestLine.hasPrefix("POST /configure") {
      let bodyData: Data
      if let boundary = data.range(of: Data("\r\n\r\n".utf8)) {
        bodyData = data.subdata(in: boundary.upperBound..<data.count)
      } else {
        bodyData = Data()
      }
      let payload = (try? JSONSerialization.jsonObject(with: bodyData)) as? [String: Any]
      let modelPath = (payload?["modelPath"] as? String) ?? ""
      Task {
        do {
          if !modelPath.isEmpty {
            try await runtime.configure(modelPath: modelPath)
          }
          send(connection, status: "204 No Content", type: "text/plain", body: Data())
        } catch {
          sendJSON(connection, ["error": ["message": error.localizedDescription]], status: "400 Bad Request")
        }
      }
      return
    }
    guard requestLine.hasPrefix("POST /v1/chat/completions") else {
      send(connection, status: "404 Not Found", type: "text/plain", body: Data("not found".utf8))
      return
    }
    let bodyData: Data
    if let boundary = data.range(of: Data("\r\n\r\n".utf8)) {
      bodyData = data.subdata(in: boundary.upperBound..<data.count)
    } else {
      bodyData = Data()
    }
    let payload = (try? JSONSerialization.jsonObject(with: bodyData)) as? [String: Any]
    let messages = payload?["messages"] as? [[String: Any]]
    let prompt = messages?.last?["content"] as? String ?? ""

    Task {
      do {
        let output = try await runtime.generate(prompt)
        let chunk: [String: Any] = [
          "id": "chatcmpl-litertlm",
          "object": "chat.completion.chunk",
          "model": "gemma-2b-it",
          "choices": [["index": 0, "delta": ["content": output]]],
        ]
        let json = try JSONSerialization.data(withJSONObject: chunk)
        let sse = "data: " + String(decoding: json, as: UTF8.self) + "\n\ndata: [DONE]\n\n"
        send(connection, status: "200 OK", type: "text/event-stream", body: Data(sse.utf8))
      } catch {
        sendJSON(connection, ["error": ["message": error.localizedDescription]], status: "500 Internal Server Error")
      }
    }
  }

  private func sendJSON(
    _ connection: NWConnection,
    _ object: Any,
    status: String = "200 OK"
  ) {
    let body = (try? JSONSerialization.data(withJSONObject: object)) ?? Data("{}".utf8)
    send(connection, status: status, type: "application/json", body: body)
  }

  private func send(_ connection: NWConnection, status: String, type: String, body: Data) {
    let header = "HTTP/1.1 \(status)\r\nContent-Type: \(type)\r\nContent-Length: \(body.count)\r\nConnection: close\r\nCache-Control: no-cache\r\n\r\n"
    var response = Data(header.utf8)
    response.append(body)
    connection.send(content: response, completion: .contentProcessed { _ in connection.cancel() })
  }
}

@_cdecl("dkst_litertlm_server_start")
public func dkst_litertlm_server_start() {
  DKSTLiteRTLMServer.shared.start()
}
