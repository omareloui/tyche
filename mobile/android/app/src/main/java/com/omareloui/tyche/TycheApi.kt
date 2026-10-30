package com.omareloui.tyche

import org.json.JSONArray
import org.json.JSONObject
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.time.Instant

class UnauthorizedException : IOException("unauthorized")

class HttpException(val code: Int) : IOException("http $code")

/** What Tyche did with an uploaded SMS. */
data class SmsResult(val status: String, val error: String?, val transactionId: String?)

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

    /** Pings Tyche's health endpoint, returns how long it took to answer in ms. */
    fun health(): Long {
        val started = System.nanoTime()
        val conn = URL("$baseUrl/health").openConnection() as HttpURLConnection
        try {
            conn.connectTimeout = HEALTH_TIMEOUT_MS
            conn.readTimeout = HEALTH_TIMEOUT_MS
            val code = conn.responseCode
            if (code != HttpURLConnection.HTTP_OK) throw HttpException(code)
            val status = JSONObject(conn.inputStream.bufferedReader().use { it.readText() }).optString("status")
            if (status != "ok") throw IOException("unhealthy: ${status.ifEmpty { "no status" }}")
        } finally {
            conn.disconnect()
        }
        return (System.nanoTime() - started) / 1_000_000
    }

    /** Uploads a batch, returns the per message results in the same order. */
    fun uploadSms(token: String, messages: List<PendingSms>): List<SmsResult> {
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
        return List(results.length()) {
            val r = results.getJSONObject(it)
            SmsResult(
                status = r.getString("status"),
                error = r.optString("error").ifEmpty { null },
                transactionId = r.optString("transaction_id").ifEmpty { null },
            )
        }
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
        private const val HEALTH_TIMEOUT_MS = 5_000
    }
}
