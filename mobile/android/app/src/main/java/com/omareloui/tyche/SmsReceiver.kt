package com.omareloui.tyche

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.provider.Telephony

/** Queues incoming SMS from the bank senders and schedules an upload. */
class SmsReceiver : BroadcastReceiver() {

    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Telephony.Sms.Intents.SMS_RECEIVED_ACTION) return

        val settings = Settings(context)
        if (!settings.isSignedIn) return

        val parts = Telephony.Sms.Intents.getMessagesFromIntent(intent) ?: return
        val queue = SmsQueue(context)
        var queued = false

        // A long SMS arrives as several parts in the same intent.
        parts.groupBy { it.originatingAddress.orEmpty() }.forEach { (sender, msgs) ->
            if (!settings.isBankSender(sender)) return@forEach
            val body = msgs.joinToString("") { it.messageBody.orEmpty() }
            // The service center timestamp, same as the inbox's date_sent used
            // when importing, so the server dedups both.
            queue.add(sender, body, msgs.first().timestampMillis)
            queued = true
        }

        if (queued) UploadWorker.enqueue(context)
    }
}
