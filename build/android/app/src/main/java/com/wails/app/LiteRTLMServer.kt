package com.wails.app

import android.content.Context
import android.util.Log
import com.google.ai.edge.litertlm.Backend
import com.google.ai.edge.litertlm.Engine
import com.google.ai.edge.litertlm.EngineConfig
import com.google.ai.edge.litertlm.Message
import com.google.ai.edge.litertlm.MessageCallback
import fi.iki.elonen.NanoHTTPD
import java.io.File
import java.io.FileOutputStream
import java.io.PipedInputStream
import java.io.PipedOutputStream
import java.nio.charset.StandardCharsets
import java.util.concurrent.ExecutorService
import java.util.concurrent.Executors
import org.json.JSONArray
import org.json.JSONObject

class LiteRTLMServer(
    private val context: Context,
    port: Int,
) : NanoHTTPD("127.0.0.1", port), AutoCloseable {
    private val executor: ExecutorService = Executors.newSingleThreadExecutor()
    private val lock = Any()
    private var engine: Engine? = null
    private var activeBackend = "cpu"
    private val modelID = "gemma-2b-it"
    private var configuredModelPath: String? = null

    override fun serve(session: IHTTPSession): Response {
        return try {
            when {
                session.method == Method.GET && session.uri == "/v1/models" -> modelsResponse()
                session.method == Method.POST && session.uri == "/configure" -> configure(session)
                session.method == Method.POST && session.uri == "/v1/chat/completions" -> chatResponse(session)
                else -> newFixedLengthResponse(Response.Status.NOT_FOUND, MIME_PLAINTEXT, "not found")
            }
        } catch (error: Throwable) {
            Log.e("LiteRTLMServer", "request failed", error)
            newFixedLengthResponse(
                Response.Status.INTERNAL_ERROR,
                "application/json",
                JSONObject().put("error", JSONObject().put("message", error.message ?: error.toString())).toString(),
            )
        }
    }

    private fun configure(session: IHTTPSession): Response {
        val bodyFiles = HashMap<String, String>()
        session.parseBody(bodyFiles)
        val nextPath = JSONObject(bodyFiles["postData"] ?: "{}").optString("modelPath")
        require(nextPath.endsWith(".litertlm", ignoreCase = true)) { "modelPath must be a .litertlm file" }
        require(File(nextPath).isFile) { "modelPath does not exist" }
        synchronized(lock) {
            engine?.close()
            engine = null
            configuredModelPath = nextPath
        }
        return newFixedLengthResponse(Response.Status.NO_CONTENT, MIME_PLAINTEXT, "")
    }

    private fun modelsResponse(): Response {
        val data = JSONArray()
        if (configuredModelFile() != null) {
            data.put(JSONObject().put("id", modelID).put("object", "model"))
        }
        return jsonResponse(JSONObject().put("object", "list").put("data", data))
    }

    private fun chatResponse(session: IHTTPSession): Response {
        val bodyFiles = HashMap<String, String>()
        session.parseBody(bodyFiles)
        val body = JSONObject(bodyFiles["postData"] ?: "{}")
        val prompt = extractPrompt(body.optJSONArray("messages"))
        if (prompt.isBlank()) {
            return newFixedLengthResponse(Response.Status.BAD_REQUEST, "application/json", "{\"error\":{\"message\":\"empty prompt\"}}")
        }

        val input = PipedInputStream(64 * 1024)
        val output = PipedOutputStream(input)
        executor.execute {
            try {
                val activeEngine = ensureEngine(parseBackend(body.optString("model", modelID)))
                val conversation = activeEngine.createConversation()
                conversation.sendMessageAsync(
                    prompt,
                    object : MessageCallback {
                        override fun onMessage(message: Message) {
                            writeSSE(output, message.toString())
                        }

                        override fun onDone() {
                            output.write("data: [DONE]\n\n".toByteArray(StandardCharsets.UTF_8))
                            output.close()
                            conversation.close()
                        }

                        override fun onError(throwable: Throwable) {
                            writeSSE(output, "", throwable.message ?: throwable.toString())
                            output.close()
                            conversation.close()
                        }
                    },
                )
            } catch (error: Throwable) {
                writeSSE(output, "", error.message ?: error.toString())
                output.close()
            }
        }

        return newChunkedResponse(Response.Status.OK, "text/event-stream", input).apply {
            addHeader("Cache-Control", "no-cache")
            addHeader("Connection", "keep-alive")
        }
    }

    private fun ensureEngine(requestedBackend: String): Engine {
        synchronized(lock) {
            engine?.let {
                if (activeBackend == requestedBackend) return it
                it.close()
                engine = null
            }

            val modelPath = configuredModelFile()?.absolutePath
                ?: error("models/gemma-2b-it.litertlm is not bundled; set LITERTLM_MODEL_PATH while building")
            val preferred = if (requestedBackend == "gpu") Backend.GPU() else Backend.CPU()
            var selectedBackend = requestedBackend
            val created = try {
                Engine(
                    EngineConfig(
                        modelPath = modelPath,
                        backend = preferred,
                        visionBackend = preferred,
                        audioBackend = Backend.CPU(),
                        cacheDir = context.cacheDir.absolutePath,
                    ),
                ).also { it.initialize() }
            } catch (first: Throwable) {
                Log.w("LiteRTLMServer", "Selected backend failed; retrying text-only CPU", first)
                selectedBackend = "cpu"
                Engine(
                    EngineConfig(
                        modelPath = modelPath,
                        backend = Backend.CPU(),
                        cacheDir = context.cacheDir.absolutePath,
                    ),
                ).also { it.initialize() }
            }
            engine = created
            activeBackend = selectedBackend
            return created
        }
    }

    private fun bundledModelFile(): File? {
        val destination = File(context.filesDir, "models/gemma-2b-it.litertlm")
        if (destination.isFile && destination.length() > 0) return destination
        return try {
            destination.parentFile?.mkdirs()
            context.assets.open("models/gemma-2b-it.litertlm").use { input ->
                FileOutputStream(destination).use { output -> input.copyTo(output) }
            }
            destination
        } catch (_: Throwable) {
            null
        }
    }

    private fun configuredModelFile(): File? =
        configuredModelPath?.let(::File)?.takeIf { it.isFile } ?: bundledModelFile()

    private fun extractPrompt(messages: JSONArray?): String {
        if (messages == null || messages.length() == 0) return ""
        val content = messages.optJSONObject(messages.length() - 1)?.opt("content")
        if (content is String) return content
        if (content is JSONArray) {
            val parts = mutableListOf<String>()
            for (index in 0 until content.length()) {
                content.optJSONObject(index)?.optString("text")?.takeIf { it.isNotBlank() }?.let(parts::add)
            }
            return parts.joinToString("\n")
        }
        return ""
    }

    private fun parseBackend(model: String): String =
        if (model.lowercase().contains("@gpu")) "gpu" else "cpu"

    private fun writeSSE(output: PipedOutputStream, text: String, error: String? = null) {
        val payload = if (error == null) {
            JSONObject()
                .put("id", "chatcmpl-litertlm")
                .put("object", "chat.completion.chunk")
                .put("model", modelID)
                .put("choices", JSONArray().put(JSONObject().put("index", 0).put("delta", JSONObject().put("content", text))))
        } else {
            JSONObject().put("error", JSONObject().put("message", error))
        }
        output.write(("data: $payload\n\n").toByteArray(StandardCharsets.UTF_8))
        output.flush()
    }

    private fun jsonResponse(value: JSONObject): Response =
        newFixedLengthResponse(Response.Status.OK, "application/json", value.toString())

    override fun close() {
        stop()
        executor.shutdownNow()
        synchronized(lock) {
            engine?.close()
            engine = null
        }
    }
}
