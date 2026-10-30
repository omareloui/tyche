package com.omareloui.tyche

import android.content.ContentValues
import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper

data class PendingSms(val id: Long, val sender: String, val body: String, val receivedAtMillis: Long)

/** Durable queue of SMS waiting to be uploaded, survives reboots and offline periods. */
class SmsQueue(context: Context) :
    SQLiteOpenHelper(context.applicationContext, "sms_queue.db", null, 1) {

    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            """
            CREATE TABLE pending_sms (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                sender TEXT NOT NULL,
                body TEXT NOT NULL,
                received_at INTEGER NOT NULL,
                UNIQUE (sender, body, received_at)
            )
            """.trimIndent(),
        )
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) = Unit

    fun add(sender: String, body: String, receivedAtMillis: Long) {
        writableDatabase.insertWithOnConflict(
            "pending_sms",
            null,
            ContentValues().apply {
                put("sender", sender)
                put("body", body)
                put("received_at", receivedAtMillis)
            },
            SQLiteDatabase.CONFLICT_IGNORE,
        )
    }

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

    fun remove(ids: List<Long>) {
        if (ids.isEmpty()) return
        writableDatabase.delete(
            "pending_sms",
            "id IN (${ids.joinToString(",") { "?" }})",
            ids.map { it.toString() }.toTypedArray(),
        )
    }

    fun count(): Long =
        readableDatabase.compileStatement("SELECT COUNT(*) FROM pending_sms").simpleQueryForLong()
}
