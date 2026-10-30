package com.omareloui.tyche

import android.content.ContentValues
import android.content.Context
import android.database.sqlite.SQLiteDatabase
import android.database.sqlite.SQLiteOpenHelper
import android.util.Log
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

enum class LogLevel { MUTED, INFO, SUCCESS, WARN, ERROR }

data class LogEntry(
    val id: Long,
    val atMillis: Long,
    val level: LogLevel,
    val title: String,
    val detail: String?,
)

/**
 * What the app did, newest first, shown on the main screen so it's clear
 * where every SMS went. Kept to the last [MAX_ENTRIES].
 */
class ActivityLog private constructor(context: Context) :
    SQLiteOpenHelper(context.applicationContext, "activity_log.db", null, 1) {

    override fun onCreate(db: SQLiteDatabase) {
        db.execSQL(
            """
            CREATE TABLE entries (
                id INTEGER PRIMARY KEY AUTOINCREMENT,
                at INTEGER NOT NULL,
                level TEXT NOT NULL,
                title TEXT NOT NULL,
                detail TEXT
            )
            """.trimIndent(),
        )
    }

    override fun onUpgrade(db: SQLiteDatabase, oldVersion: Int, newVersion: Int) = Unit

    fun add(level: LogLevel, title: String, detail: String? = null) {
        Log.println(
            when (level) {
                LogLevel.ERROR -> Log.ERROR
                LogLevel.WARN -> Log.WARN
                else -> Log.INFO
            },
            TAG,
            if (detail == null) title else "$title: $detail",
        )
        writableDatabase.run {
            insert(
                "entries",
                null,
                ContentValues().apply {
                    put("at", System.currentTimeMillis())
                    put("level", level.name)
                    put("title", title)
                    put("detail", detail)
                },
            )
            execSQL(
                "DELETE FROM entries WHERE id <= (SELECT MAX(id) FROM entries) - ?",
                arrayOf(MAX_ENTRIES),
            )
        }
        _changes.value++
    }

    fun recent(limit: Int): List<LogEntry> =
        readableDatabase.query(
            "entries", arrayOf("id", "at", "level", "title", "detail"),
            null, null, null, null, "id DESC", limit.toString(),
        ).use { c ->
            buildList {
                while (c.moveToNext()) {
                    add(
                        LogEntry(
                            id = c.getLong(0),
                            atMillis = c.getLong(1),
                            level = runCatching { LogLevel.valueOf(c.getString(2)) }.getOrDefault(LogLevel.INFO),
                            title = c.getString(3),
                            detail = if (c.isNull(4)) null else c.getString(4),
                        ),
                    )
                }
            }
        }

    fun clear() {
        writableDatabase.delete("entries", null, null)
        _changes.value++
    }

    companion object {
        private const val TAG = "Tyche"
        private const val MAX_ENTRIES = 300

        @Volatile
        private var instance: ActivityLog? = null

        private val _changes = MutableStateFlow(0L)

        /** Bumped on every change, the UI re-reads the log on it. */
        val changes: StateFlow<Long> = _changes

        fun get(context: Context): ActivityLog =
            instance ?: synchronized(this) {
                instance ?: ActivityLog(context).also { instance = it }
            }

        /** Shortens an SMS body for a log line. */
        fun snippet(body: String, max: Int = 160): String {
            val flat = body.replace(Regex("\\s+"), " ").trim()
            return if (flat.length <= max) flat else flat.take(max - 1) + "…"
        }
    }
}
