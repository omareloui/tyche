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
        val log = ActivityLog.get(context)
        val parts = Telephony.Sms.Intents.getMessagesFromIntent(intent) ?: return
        val queue = SmsQueue(context)
        var queued = false

        // A long SMS arrives as several parts in the same intent.
        parts.groupBy { it.originatingAddress.orEmpty() }.forEach { (sender, msgs) ->
            if (!settings.isBankSender(sender)) {
                // No body, it's not a bank's message.
                log.add(LogLevel.MUTED, "SMS from $sender ignored", "Not one of your bank senders.")
                return@forEach
            }
            val body = msgs.joinToString("") { it.messageBody.orEmpty() }
            if (!settings.isSignedIn) {
                log.add(
                    LogLevel.WARN,
                    "SMS from $sender not queued, signed out",
                    ActivityLog.snippet(body) + if (settings.lastScanAt > 0) {
                        "\n→ Picked up from the inbox after signing in again."
                    } else {
                        "\n→ Import it from the inbox after signing in."
                    },
                )
                return@forEach
            }
            // The service center timestamp, same as the inbox's date_sent used
            // when importing, so the server dedups both.
            if (queue.add(sender, body, msgs.first().timestampMillis)) {
                log.add(LogLevel.INFO, "SMS from $sender queued", ActivityLog.snippet(body))
                queued = true
            }
        }

        if (queued) UploadWorker.enqueue(context)
        // After an app update nothing else may have scheduled it yet.
        CatchUpWorker.schedule(context)
    }
}
