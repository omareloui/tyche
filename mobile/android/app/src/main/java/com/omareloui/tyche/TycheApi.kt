package com.omareloui.tyche

import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant

class UnauthorizedException : IOException("unauthorized")

class HttpException(val code: Int) : IOException("http $code")

/** Minimal client for the Tyche mobile endpoints. */
class TycheApi(private val baseUrl: String) {

    /** Exchanges the one time sign in code for a device token. */
    fun exchangeCode(code: String, verifier: String): String {
        val res = post(
            "/api/mobile/token",
            JSONObject().put("code", code).put("verifier", verifier),
            token = null,
        )
        return res.getString("token")
    }

    /** Uploads a batch, returns the per message statuses in the same order. */
    fun uploadSms(token: String, messages: List<PendingSms>): List<String> {
        val arr = JSONArray()
        messages.forEach {
            arr.put(
                JSONObject()
                    .put("sender", it.sender)
                    .put("body", it.body)
                    .put("received_at", Instant.ofEpochMilli(it.receivedAtMillis).toString()),
            )
        }
        val res = post("/api/sms", JSONObject().put("messages", arr), token)
        val results = res.getJSONArray("results")
        return List(results.length()) { results.getJSONObject(it).getString("status") }
    }

    private fun post(path: String, body: JSONObject, token: String?): JSONObject {
        val conn = URL(baseUrl + path).openConnection() as HttpURLConnection
        try {
            conn.requestMethod = "POST"
            conn.connectTimeout = TIMEOUT_MS
            conn.readTimeout = TIMEOUT_MS
            conn.doOutput = true
            conn.setRequestProperty("Content-Type", "application/json")
            conn.setRequestProperty("Accept", "application/json")
            token?.let { conn.setRequestProperty("Authorization", "Bearer $it") }

            conn.outputStream.use { it.write(body.toString().toByteArray()) }

            return when (val code = conn.responseCode) {
                HttpURLConnection.HTTP_OK ->
                    JSONObject(conn.inputStream.bufferedReader().use { it.readText() })
                HttpURLConnection.HTTP_UNAUTHORIZED -> throw UnauthorizedException()
                else -> throw HttpException(code)
            }
        } finally {
            conn.disconnect()
        }
    }

    companion object {
        private const val TIMEOUT_MS = 20_000
    }
}
