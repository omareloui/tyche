package com.omareloui.tyche

import androidx.work.WorkInfo

enum class Tone { OK, BUSY, WAITING, WARN, ERROR }

enum class StatusAction { NONE, GRANT_SMS, SENDERS, SIGN_IN_AGAIN }

/** The headline of the main screen: whether SMS reach Tyche, and if not why. */
data class SyncStatus(
    val tone: Tone,
    val label: String,
    val title: String,
    val detail: String,
    val action: StatusAction = StatusAction.NONE,
)

data class SyncState(
    val now: Long,
    val canReceiveSms: Boolean,
    val hasSenders: Boolean,
    val authError: Boolean,
    val serverHost: String,
    val pending: Long,
    val oldestQueuedAt: Long?,
    val lastSyncAt: Long,
    val lastError: String?,
    val lastErrorAt: Long,
    val work: WorkInfo?,
    /** Why Tyche's health check fails, null when it's up or unchecked. */
    val serverDown: String?,
)

object SyncStatusResolver {
    /** Queued SMS older than this count as stuck. */
    private const val STUCK_AFTER_MS = 10 * 60 * 1000L

    fun resolve(s: SyncState): SyncStatus {
        val waiting = s.oldestQueuedAt?.let { s.now - it } ?: 0L
        val stuck = s.pending > 0 && waiting > STUCK_AFTER_MS
        val queued = "${s.pending} SMS"

        if (s.authError) {
            return SyncStatus(
                Tone.ERROR, "Stuck", "Tyche rejected this phone",
                "The device was revoked from Tyche's SMS page, or signed in again elsewhere." +
                    (if (s.pending > 0) " $queued are waiting and will be sent once you sign in again." else "") +
                    " Sign out, then sign in.",
                StatusAction.SIGN_IN_AGAIN,
            )
        }

        if (s.pending == 0L) {
            return when {
                !s.canReceiveSms -> SyncStatus(
                    Tone.WARN, "Needs permission", "Can't see new SMS",
                    "Allow SMS access so bank messages get forwarded as they arrive.",
                    StatusAction.GRANT_SMS,
                )
                !s.hasSenders -> SyncStatus(
                    Tone.WARN, "Needs setup", "No bank senders picked",
                    "Pick who sends your bank SMS, nothing is forwarded until then.",
                    StatusAction.SENDERS,
                )
                s.serverDown != null -> SyncStatus(
                    Tone.WARN, "Server down", "Tyche is unreachable",
                    "${s.serverDown}. New bank SMS wait here and are sent once it's back.",
                )
                else -> SyncStatus(
                    Tone.OK, "All good", "All synced",
                    if (s.lastSyncAt > 0) {
                        "Last sync with ${s.serverHost} ${relative(s.lastSyncAt, s.now)}. New bank SMS are sent right away."
                    } else {
                        "Waiting for bank SMS. They're sent to ${s.serverHost} right away."
                    },
                )
            }
        }

        val since = "waiting ${formatDuration(waiting)}"
        val nextTry = s.work?.nextScheduleTimeMillis
            ?.takeIf { it != Long.MAX_VALUE && it > s.now }
            ?.let { "Next try ${relative(it, s.now)}." }
            ?: ""
        val error = s.lastError?.takeIf { s.lastErrorAt > 0 }

        if (s.serverDown != null && s.work?.state != WorkInfo.State.RUNNING) {
            return SyncStatus(
                if (stuck) Tone.ERROR else Tone.WARN,
                if (stuck) "Stuck for ${formatDuration(waiting)}" else "Server down",
                "$queued waiting, Tyche is unreachable",
                "${s.serverDown}. They're sent as soon as it's back.",
            )
        }

        return when (s.work?.state) {
            WorkInfo.State.RUNNING -> SyncStatus(
                Tone.BUSY, "Sending", "Sending $queued…",
                "Uploading to ${s.serverHost}." + (error?.let { " Last try failed: $it." } ?: ""),
            )

            WorkInfo.State.ENQUEUED, WorkInfo.State.BLOCKED -> when {
                error != null -> SyncStatus(
                    if (stuck) Tone.ERROR else Tone.WARN,
                    if (stuck) "Stuck for ${formatDuration(waiting)}" else "Retrying",
                    if (stuck) "$queued can't get through" else "$queued waiting to retry",
                    "$error ${relative(s.lastErrorAt, s.now)}. $nextTry".trim(),
                )
                // Never ran yet: WorkManager waits for a network.
                s.work.runAttemptCount == 0 && waiting > 60_000 -> SyncStatus(
                    if (stuck) Tone.ERROR else Tone.WAITING,
                    if (stuck) "Stuck for ${formatDuration(waiting)}" else "Waiting",
                    "Waiting for internet",
                    "$queued will be sent once the phone is online ($since).",
                )
                // Backing off after a failure from before errors were recorded.
                s.work.runAttemptCount > 0 -> SyncStatus(
                    if (stuck) Tone.ERROR else Tone.WARN,
                    if (stuck) "Stuck for ${formatDuration(waiting)}" else "Retrying",
                    if (stuck) "$queued can't get through" else "$queued waiting to retry",
                    "Earlier tries failed. $nextTry Tap Sync now to try right away.".replace("  ", " "),
                )
                else -> SyncStatus(Tone.BUSY, "Queued", "Sending $queued shortly", "Queued for ${s.serverHost}.")
            }

            // Nothing scheduled to send them: the last run gave up or never ran.
            else -> SyncStatus(
                if (stuck || error != null) Tone.ERROR else Tone.WARN,
                if (stuck) "Stuck for ${formatDuration(waiting)}" else "Not sending",
                "$queued aren't being sent",
                (error?.let { "Last try failed: $it. " } ?: "No upload is scheduled. ") + "Tap Sync now to try again.",
            )
        }
    }
}
