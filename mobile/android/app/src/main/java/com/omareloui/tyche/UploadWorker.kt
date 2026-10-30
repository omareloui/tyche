package com.omareloui.tyche

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.io.IOException
import java.net.ConnectException
import java.net.SocketTimeoutException
import java.net.UnknownHostException
import java.util.concurrent.TimeUnit
import javax.net.ssl.SSLException
import kotlin.math.min

/** Uploads the queued SMS to Tyche until the queue is empty. */
class UploadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val settings = Settings(applicationContext)
        val log = ActivityLog.get(applicationContext)
        val queue = SmsQueue(applicationContext)

        val token = settings.token
        if (token == null || settings.serverUrl.isEmpty()) {
            log.add(LogLevel.WARN, "Not sending, signed out", "${queue.count()} SMS stay queued until you sign in again.")
            return@withContext Result.failure()
        }
        if (settings.authError) {
            log.add(LogLevel.ERROR, "Not sending, Tyche rejected this device", "Sign out and sign in again.")
            return@withContext Result.failure()
        }

        val total = queue.count()
        if (total == 0L) {
            log.add(LogLevel.MUTED, "Nothing to send, all caught up")
            settings.lastSyncAt = System.currentTimeMillis()
            settings.lastError = null
            return@withContext Result.success()
        }

        val api = TycheApi(settings.serverUrl)
        val attempt = if (runAttemptCount > 0) " (try ${runAttemptCount + 1})" else ""
        log.add(LogLevel.INFO, "Sending $total SMS to ${settings.serverHost}$attempt")

        try {
            var sent = 0
            while (true) {
                val batch = queue.next(BATCH_SIZE)
                if (batch.isEmpty()) break

                val results = api.uploadSms(token, batch)
                logResults(log, batch, results)
                // Every message is stored server side whatever its status, so
                // they're all done.
                queue.remove(batch.map { it.id })
                sent += batch.size
                settings.sentTotal += batch.size
            }
            settings.lastSyncAt = System.currentTimeMillis()
            settings.lastError = null
            log.add(LogLevel.SUCCESS, "Synced, $sent SMS sent")
            Result.success()
        } catch (e: UnauthorizedException) {
            fail(settings, log, "Tyche rejected this device", "It was revoked from Tyche's SMS page or signed in elsewhere. Sign out and sign in again.")
            settings.authError = true
            Result.failure()
        } catch (e: HttpException) {
            if (e.code >= 500) {
                fail(settings, log, "Tyche had a server error (${e.code})", retryHint())
                Result.retry()
            } else {
                fail(settings, log, "Tyche refused the upload (${e.code})", "Not retrying on its own. Check the server logs, then tap Sync now.")
                Result.failure()
            }
        } catch (e: IOException) {
            fail(settings, log, describe(e, settings), retryHint())
            Result.retry()
        }
    }

    /** Logs every message in small batches, a summary plus the problems in big ones. */
    private fun logResults(log: ActivityLog, batch: List<PendingSms>, results: List<SmsResult>) {
        val detailed = batch.size <= DETAILED_BATCH
        batch.zip(results).forEach { (sms, res) ->
            val ok = res.status == "parsed" || res.status == "duplicate" || res.status == "ignored"
            if (detailed || !ok) {
                log.add(levelOf(res.status), "${sms.sender}: ${describeStatus(res.status)}", buildString {
                    append(ActivityLog.snippet(sms.body))
                    res.error?.let { append("\n→ ").append(it) }
                })
            }
        }
        if (!detailed) {
            val counts = results.groupingBy { it.status }.eachCount()
            log.add(
                LogLevel.INFO,
                "Sent ${batch.size} SMS",
                counts.entries.joinToString(", ") { (st, n) -> "$n ${shortStatus(st)}" },
            )
        }
    }

    private fun fail(settings: Settings, log: ActivityLog, title: String, detail: String) {
        settings.lastError = title
        settings.lastErrorAt = System.currentTimeMillis()
        log.add(LogLevel.ERROR, title, detail)
    }

    /** When WorkManager runs the next try, following the backoff policy. */
    private fun retryHint(): String {
        val delay = min(BACKOFF_SECONDS shl min(runAttemptCount, 20), MAX_BACKOFF_SECONDS)
        return "Retrying in ~${formatDuration(delay * 1000)}, or tap Sync now."
    }

    companion object {
        private const val WORK_NAME = "upload_sms"
        private const val BATCH_SIZE = 50
        private const val DETAILED_BATCH = 10
        private const val BACKOFF_SECONDS = 30L
        private const val MAX_BACKOFF_SECONDS = 5 * 60 * 60L

        const val UNIQUE_NAME = WORK_NAME

        fun enqueue(context: Context) {
            val request = OneTimeWorkRequestBuilder<UploadWorker>()
                .setConstraints(
                    Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build(),
                )
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, BACKOFF_SECONDS, TimeUnit.SECONDS)
                .build()
            // Replace, not append: appended work waits behind a run backing off
            // after failures (up to hours), so new SMS and "Sync now" would be
            // stuck. Cancelling a run mid-upload is fine, the queue only drops
            // uploaded messages and the server dedups.
            WorkManager.getInstance(context)
                .enqueueUniqueWork(WORK_NAME, ExistingWorkPolicy.REPLACE, request)
        }

        /** A network failure in words, with how to fix the common local setup. */
        fun describe(e: IOException, settings: Settings): String {
            val host = settings.serverHost
            return when (e) {
                is ConnectException ->
                    if (host.startsWith("localhost") || host.startsWith("127.0.0.1")) {
                        val port = host.substringAfter(':', "80")
                        "Can't reach $host. Is Tyche running, and the USB tunnel up? (adb reverse tcp:$port tcp:$port)"
                    } else {
                        "Can't reach $host, the connection was refused"
                    }
                is UnknownHostException -> "Can't find $host, check the URL or your internet"
                is SocketTimeoutException -> "$host took too long to answer"
                is SSLException -> "Secure connection to $host failed (${e.message})"
                else -> "Upload to $host failed: ${e.message ?: e.javaClass.simpleName}"
            }
        }

        fun describeStatus(status: String) = when (status) {
            "parsed" -> "became a transaction"
            "duplicate" -> "already on Tyche"
            "ignored" -> "not a transaction, skipped"
            "unparsed" -> "Tyche couldn't read it"
            "unmatched" -> "no account matched it"
            "failed" -> "couldn't create the transaction"
            else -> status
        }

        private fun shortStatus(status: String) = when (status) {
            "parsed" -> "transactions"
            "duplicate" -> "already there"
            "ignored" -> "skipped"
            "unparsed" -> "unreadable"
            "unmatched" -> "unmatched"
            else -> status
        }

        private fun levelOf(status: String) = when (status) {
            "parsed" -> LogLevel.SUCCESS
            "duplicate", "ignored" -> LogLevel.MUTED
            "failed" -> LogLevel.ERROR
            else -> LogLevel.WARN
        }
    }
}
