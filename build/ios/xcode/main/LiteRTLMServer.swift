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
    guard modelPath.lowercased().hasSuffix(".litertlm"),
      FileManager.default.fileExists(atPath: modelPath)
    else {
      throw NSError(domain: "DKSTLiteRTLM", code: 2, userInfo: [NSLocalizedDescriptionKey: "modelPath must reference an existing .litertlm file"])
    }
    configuredModelURL = URL(fileURLWithPath: modelPath)
    if let engine { litert_lm_engine_delete(engine) }
    engine = nil
  }

  func modelExists() -> Bool {
    configuredModelURL ?? Bundle.main.url(
      forResource: "gemma-2b-it",
      withExtension: "litertlm",
      subdirectory: "models"
    ) != nil
  }

  func generate(_ prompt: String) throws -> String {
    guard
      let modelURL = configuredModelURL ?? Bundle.main.url(
        forResource: "gemma-2b-it",
        withExtension: "litertlm",
        subdirectory: "models"
      )
    else {
      throw NSError(
        domain: "DKSTLiteRTLM",
        code: 1,
        userInfo: [NSLocalizedDescriptionKey: "models/gemma-2b-it.litertlm is not bundled"]
      )
    }

    let activeEngine: OpaquePointer
    if let engine {
      activeEngine = engine
    } else {
      let cache = FileManager.default.urls(for: .cachesDirectory, in: .userDomainMask).first?.path
      guard let created = Self.createEngine(modelPath: modelURL.path, backend: "gpu", cacheDir: cache)
        ?? Self.createEngine(modelPath: modelURL.path, backend: "cpu", cacheDir: cache)
      else {
        throw NSError(domain: "DKSTLiteRTLM", code: 3, userInfo: [NSLocalizedDescriptionKey: "LiteRT-LM engine initialization failed"])
      }
      activeEngine = created
      engine = activeEngine
    }

    guard let conversation = litert_lm_conversation_create(activeEngine, nil) else {
      throw NSError(domain: "DKSTLiteRTLM", code: 4, userInfo: [NSLocalizedDescriptionKey: "LiteRT-LM conversation creation failed"])
    }
    defer { litert_lm_conversation_delete(conversation) }

    let message: [String: Any] = [
      "role": "user",
      "content": [["type": "text", "text": prompt]],
    ]
    let messageData = try JSONSerialization.data(withJSONObject: message)
    let messageJSON = String(decoding: messageData, as: UTF8.self)
    guard let response = litert_lm_conversation_send_message(conversation, messageJSON, nil, nil) else {
      throw NSError(domain: "DKSTLiteRTLM", code: 5, userInfo: [NSLocalizedDescriptionKey: "LiteRT-LM generation failed"])
    }
    defer { litert_lm_json_response_delete(response) }
    guard let responseChars = litert_lm_json_response_get_string(response) else {
      throw NSError(domain: "DKSTLiteRTLM", code: 6, userInfo: [NSLocalizedDescriptionKey: "LiteRT-LM returned an empty response"])
    }
    let responseJSON = String(cString: responseChars)
    guard let data = responseJSON.data(using: .utf8),
      let object = try JSONSerialization.jsonObject(with: data) as? [String: Any],
      let content = object["content"] as? [[String: Any]]
    else {
      throw NSError(domain: "DKSTLiteRTLM", code: 7, userInfo: [NSLocalizedDescriptionKey: "LiteRT-LM returned invalid JSON"])
    }
    return content.compactMap { $0["text"] as? String }.joined(separator: " ")
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
        let exists = await runtime.modelExists()
        let models: [[String: Any]] = exists ? [["id": "gemma-2b-it", "object": "model"]] : []
        sendJSON(connection, ["object": "list", "data": models])
      }
      return
    }
    if requestLine.hasPrefix("POST /configure "),
      let bodyStart = text.range(of: "\r\n\r\n")?.upperBound,
      let bodyData = String(text[bodyStart...]).data(using: .utf8),
      let payload = try? JSONSerialization.jsonObject(with: bodyData) as? [String: Any],
      let modelPath = payload["modelPath"] as? String
    {
      Task {
        do {
          try await runtime.configure(modelPath: modelPath)
          send(connection, status: "204 No Content", type: "text/plain", body: Data())
        } catch {
          sendJSON(connection, ["error": ["message": error.localizedDescription]], status: "400 Bad Request")
        }
      }
      return
    }
    guard requestLine.hasPrefix("POST /v1/chat/completions "),
      let bodyStart = text.range(of: "\r\n\r\n")?.upperBound,
      let bodyData = String(text[bodyStart...]).data(using: .utf8),
      let payload = try? JSONSerialization.jsonObject(with: bodyData) as? [String: Any],
      let messages = payload["messages"] as? [[String: Any]],
      let prompt = messages.last?["content"] as? String
    else {
      send(connection, status: "404 Not Found", type: "text/plain", body: Data("not found".utf8))
      return
    }

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
