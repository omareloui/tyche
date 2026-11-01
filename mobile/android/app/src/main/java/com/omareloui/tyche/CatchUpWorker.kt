package com.omareloui.tyche

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import androidx.core.content.ContextCompat
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

/**
 * Picks up the bank SMS the receiver missed (app force stopped or killed by
 * the battery saver, signed out) from the inbox, then uploads the queue.
 */
class CatchUpWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val ctx = applicationContext
        val settings = Settings(ctx)
        // Signed out SMS stay in the inbox for the run after signing in.
        if (!settings.isSignedIn || settings.authError) return@withContext Result.success()
        if (ContextCompat.checkSelfPermission(ctx, Manifest.permission.READ_SMS) != PackageManager.PERMISSION_GRANTED) {
            return@withContext Result.success()
        }

        val now = System.currentTimeMillis()
        val last = settings.lastScanAt
        if (last == 0L) {
            // First run: the past is the manual import's job.
            settings.lastScanAt = now
            return@withContext Result.success()
        }

        // Overlap a bit, an SMS can land in the inbox right after the last scan's query.
        val (missed, _) = SmsImporter.scan(ctx, last - OVERLAP_MS, skipSent = true)
        settings.lastScanAt = now
        if (missed > 0) {
            ActivityLog.get(ctx).add(
                LogLevel.WARN,
                "Picked up $missed missed SMS from the inbox",
                "They arrived while the app couldn't forward them, e.g. stopped by the battery saver.",
            )
        }
        // Also restarts an upload that gave up or got dropped.
        if (SmsQueue(ctx).count() > 0) UploadWorker.enqueue(ctx)
        Result.success()
    }

    companion object {
        private const val PERIODIC_NAME = "catch_up_sms"
        private const val ONCE_NAME = "catch_up_sms_now"
        private const val INTERVAL_HOURS = 6L
        private val OVERLAP_MS = TimeUnit.HOURS.toMillis(1)

        /** Schedules the periodic run, keeping the one already scheduled. */
        fun schedule(context: Context) {
            val request = PeriodicWorkRequestBuilder<CatchUpWorker>(INTERVAL_HOURS, TimeUnit.HOURS).build()
            WorkManager.getInstance(context)
                .enqueueUniquePeriodicWork(PERIODIC_NAME, ExistingPeriodicWorkPolicy.KEEP, request)
        }

        /** Runs it now, e.g. on opening the app after it was force stopped. */
        fun runNow(context: Context) {
            WorkManager.getInstance(context)
                .enqueueUniqueWork(ONCE_NAME, ExistingWorkPolicy.KEEP, OneTimeWorkRequestBuilder<CatchUpWorker>().build())
        }
    }
}
