package com.omareloui.tyche

import android.content.Context
import android.provider.Telephony

/** Queues the bank SMS already in the inbox, the server skips the ones it has. */
object SmsImporter {

    data class Result(val queued: Int, val alreadyQueued: Int)

    fun importSince(context: Context, sinceMillis: Long, label: String): Result {
        val settings = Settings(context)
        val (count, already) = scan(context, sinceMillis, skipSent = false)

        val log = ActivityLog.get(context)
        when {
            count > 0 -> log.add(
                LogLevel.INFO,
                "Imported $count bank SMS from $label",
                "Queued to send. Tyche skips the ones it already has." +
                    if (already > 0) " $already were already queued." else "",
            )
            already > 0 -> log.add(LogLevel.INFO, "Nothing new to import from $label", "All $already are already queued.")
            else -> log.add(
                LogLevel.WARN,
                "No bank SMS found in $label",
                "Senders: ${settings.senders.sorted().joinToString(", ")}. Check them if that's unexpected.",
            )
        }

        if (count > 0) UploadWorker.enqueue(context)
        return Result(count, already)
    }

    /**
     * Queues the bank SMS received since [sinceMillis] without uploading them,
     * leaving out the ones already uploaded when [skipSent].
     */
    fun scan(context: Context, sinceMillis: Long, skipSent: Boolean): Result {
        val settings = Settings(context)
        val queue = SmsQueue(context)
        var count = 0
        var already = 0

        context.contentResolver.query(
            Telephony.Sms.Inbox.CONTENT_URI,
            arrayOf(Telephony.Sms.ADDRESS, Telephony.Sms.BODY, Telephony.Sms.DATE, Telephony.Sms.DATE_SENT),
            "${Telephony.Sms.DATE} >= ?",
            arrayOf(sinceMillis.toString()),
            "${Telephony.Sms.DATE} ASC",
        )?.use { c ->
            while (c.moveToNext()) {
                val sender = c.getString(0).orEmpty()
                if (!settings.isBankSender(sender)) continue
                val body = c.getString(1).orEmpty()
                // date_sent matches SmsMessage.timestampMillis of the live
                // receiver, fall back to the received date when missing.
                val sent = c.getLong(3).takeIf { it > 0 } ?: c.getLong(2)
                if (queue.add(sender, body, sent, skipSent)) count++ else already++
            }
        }
        return Result(count, already)
    }
}
