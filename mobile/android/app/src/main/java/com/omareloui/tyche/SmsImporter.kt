package com.omareloui.tyche

import android.content.Context
import android.provider.Telephony

/** Queues the bank SMS already in the inbox, the server skips the ones it has. */
object SmsImporter {

    fun importSince(context: Context, sinceMillis: Long): Int {
        val settings = Settings(context)
        val queue = SmsQueue(context)
        var count = 0

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
                queue.add(sender, body, sent)
                count++
            }
        }

        if (count > 0) UploadWorker.enqueue(context)
        return count
    }
}
