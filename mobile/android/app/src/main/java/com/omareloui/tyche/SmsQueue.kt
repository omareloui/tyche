package com.omareloui.tyche

import android.content.ContentValues
import android.content.Context
import android.database.DatabaseUtils
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper

data class PendingSms(val id: Long, val sender: String, val body: String, val receivedAtMillis: Long)

/** Durable queue of SMS waiting to be uploaded, survives reboots and offline periods. */
class SmsQueue(context: Context) :
    SQLiteOpenHelper(context.applicationContext, "sms_queue.db", null, 3) {

    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            """
            CREATE TABLE pending_sms (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                sender TEXT NOT NULL,
                body TEXT NOT NULL,
                received_at INTEGER NOT NULL,
                queued_at INTEGER,
                UNIQUE (sender, body, received_at)
            )
            """.trimIndent(),
        )
        createSent(db)
    }

    /** What got uploaded, so the inbox catch-up doesn't send it again. */
    private fun createSent(db: SQLiteDatabase) {
        db.execSQL(
            """
            CREATE TABLE sent_sms (
                sender TEXT NOT NULL,
                body TEXT NOT NULL,
                received_at INTEGER NOT NULL,
                sent_at INTEGER NOT NULL,
                PRIMARY KEY (sender, body, received_at)
            )
            """.trimIndent(),
        )
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) {
        if (oldVersion < 2) {
            db.execSQL("ALTER TABLE pending_sms ADD COLUMN queued_at INTEGER")
            // Unknown for the ones queued before, their SMS time is close enough.
            db.execSQL("UPDATE pending_sms SET queued_at = received_at")
        }
        if (oldVersion < 3) createSent(db)
    }

    /**
     * Queues the SMS, false if it's already queued, or already sent when
     * [skipSent] (the manual import sends them again, e.g. after a reset on Tyche).
     */
    fun add(sender: String, body: String, receivedAtMillis: Long, skipSent: Boolean = false): Boolean {
        if (skipSent && wasSent(sender, body, receivedAtMillis)) return false
        return writableDatabase.insertWithOnConflict(
            "pending_sms",
            null,
            ContentValues().apply {
                put("sender", sender)
                put("body", body)
                put("received_at", receivedAtMillis)
                put("queued_at", System.currentTimeMillis())
            },
            SQLiteDatabase.CONFLICT_IGNORE,
        ) != -1L
    }

    private fun wasSent(sender: String, body: String, receivedAtMillis: Long): Boolean =
        DatabaseUtils.queryNumEntries(
            readableDatabase, "sent_sms", "sender = ? AND body = ? AND received_at = ?",
            arrayOf(sender, body, receivedAtMillis.toString()),
        ) > 0

    fun next(limit: Int): List<PendingSms> =
        readableDatabase.query(
            "pending_sms", arrayOf("id", "sender", "body", "received_at"),
            null, null, null, null, "id", limit.toString(),
        ).use { c ->
            buildList {
                while (c.moveToNext()) {
                    add(PendingSms(c.getLong(0), c.getString(1), c.getString(2), c.getLong(3)))
                }
            }
        }

    /** Removes the uploaded SMS from the queue, remembering them as sent. */
    fun markSent(ids: List<Long>) {
        if (ids.isEmpty()) return
        val now = System.currentTimeMillis()
        val inIds = "id IN (${ids.joinToString(",") { "?" }})"
        val args = ids.map { it.toString() }.toTypedArray()
        val db = writableDatabase
        db.beginTransaction()
        try {
            db.execSQL(
                "INSERT OR REPLACE INTO sent_sms (sender, body, received_at, sent_at) " +
                    "SELECT sender, body, received_at, $now FROM pending_sms WHERE $inIds",
                args,
            )
            db.delete("pending_sms", inIds, args)
            // The catch-up only looks back a few hours, older ones aren't needed.
            db.delete("sent_sms", "sent_at < ?", arrayOf((now - SENT_KEEP_MS).toString()))
            db.setTransactionSuccessful()
        } finally {
            db.endTransaction()
        }
    }

    fun count(): Long =
        readableDatabase.compileStatement("SELECT COUNT(*) FROM pending_sms").simpleQueryForLong()

    /** When the oldest queued SMS got queued (its id order), null if none. */
    fun oldestQueuedAt(): Long? =
        readableDatabase.rawQuery("SELECT queued_at FROM pending_sms ORDER BY id LIMIT 1", null).use { c ->
            if (c.moveToFirst() && !c.isNull(0)) c.getLong(0) else null
        }

    companion object {
        private const val SENT_KEEP_MS = 30L * 24 * 60 * 60 * 1000
    }
}
