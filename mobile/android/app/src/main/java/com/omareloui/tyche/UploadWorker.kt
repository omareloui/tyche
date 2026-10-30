package com.omareloui.tyche

import android.content.Context
import android.util.Log
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
import java.time.Instant
import java.util.concurrent.TimeUnit

/** Uploads the queued SMS to Tyche until the queue is empty. */
class UploadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {

    override suspend fun doWork(): Result = withContext(Dispatchers.IO) {
        val settings = Settings(applicationContext)
        val token = settings.token
        if (token == null || settings.serverUrl.isEmpty() || settings.authError) {
            return@withContext Result.failure()
        }

        val api = TycheApi(settings.serverUrl)
        val queue = SmsQueue(applicationContext)

        try {
            while (true) {
                val batch = queue.next(BATCH_SIZE)
                if (batch.isEmpty()) break

                val statuses = api.uploadSms(token, batch)
                Log.i(TAG, "uploaded ${batch.size} sms: ${statuses.groupingBy { it }.eachCount()}")
                // Every message is stored server side whatever its status, so
                // they're all done.
                queue.remove(batch.map { it.id })
            }
            settings.lastSync = Instant.now().toString()
            Result.success()
        } catch (e: UnauthorizedException) {
            Log.w(TAG, "device token rejected, needs signing in again")
            settings.authError = true
            Result.failure()
        } catch (e: HttpException) {
            Log.w(TAG, "upload failed with ${e.code}")
            if (e.code >= 500) Result.retry() else Result.failure()
        } catch (e: IOException) {
            Log.w(TAG, "upload failed", e)
            Result.retry()
        }
    }

    companion object {
        private const val TAG = "TycheUpload"
        private const val WORK_NAME = "upload_sms"
        private const val BATCH_SIZE = 50

        fun enqueue(context: Context) {
            val request = OneTimeWorkRequestBuilder<UploadWorker>()
                .setConstraints(
                    Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build(),
                )
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
                .build()
            // Replace, not append: appended work waits behind a run backing off
            // after failures (up to hours), so new SMS and "Sync now" would be
            // stuck. Cancelling a run mid-upload is fine, the queue only drops
            // uploaded messages and the server dedups.
            WorkManager.getInstance(context)
                .enqueueUniqueWork(WORK_NAME, ExistingWorkPolicy.REPLACE, request)
        }
    }
}
