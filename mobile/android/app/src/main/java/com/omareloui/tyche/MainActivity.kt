package com.omareloui.tyche

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.view.LayoutInflater
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.browser.customtabs.CustomTabColorSchemeParams
import androidx.browser.customtabs.CustomTabsIntent
import androidx.core.content.ContextCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import androidx.work.WorkInfo
import androidx.work.WorkManager
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

class MainActivity : AppCompatActivity() {

    private lateinit var settings: Settings
    private lateinit var log: ActivityLog
    private lateinit var serverUrl: EditText

    /** The upload work, observed to tell sending, retrying and stuck apart. */
    private var work: WorkInfo? = null

    /** The last health check of the server: null while unchecked. */
    private var health: Health? = null

    /** Whether the upload got restarted on this screen, done once. */
    private var kicked = false

    /** Log entries showing their whole detail. */
    private val expanded = mutableSetOf<Long>()

    private val requestSmsPermissions =
        registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { granted ->
            if (granted.values.all { it }) {
                log.add(LogLevel.SUCCESS, "SMS access allowed", "New bank SMS are forwarded as they arrive.")
            } else {
                log.add(LogLevel.WARN, "SMS access denied", "New bank SMS won't be forwarded.")
            }
            render()
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        settings = Settings(this)
        log = ActivityLog.get(this)

        serverUrl = findViewById(R.id.server_url)
        serverUrl.setText(settings.serverUrl)

        findViewById<Button>(R.id.sign_in).setOnClickListener { signIn() }
        findViewById<Button>(R.id.sign_out).setOnClickListener { signOut() }
        findViewById<Button>(R.id.sync_now).setOnClickListener {
            log.add(LogLevel.MUTED, "Sync requested")
            UploadWorker.enqueue(this)
        }
        findViewById<Button>(R.id.import_past).setOnClickListener {
            importSince(System.currentTimeMillis() - TimeUnit.DAYS.toMillis(IMPORT_DAYS), "the last $IMPORT_DAYS days")
        }
        findViewById<Button>(R.id.import_all).setOnClickListener { importSince(0, "the whole inbox") }
        findViewById<Button>(R.id.choose_senders).setOnClickListener { openSenders() }
        findViewById<View>(R.id.health_pill).setOnClickListener { lifecycleScope.launch { checkHealth() } }
        findViewById<TextView>(R.id.clear_activity).setOnClickListener {
            lifecycleScope.launch(Dispatchers.IO) { log.clear() }
        }

        WorkManager.getInstance(this)
            .getWorkInfosForUniqueWorkLiveData(UploadWorker.UNIQUE_NAME)
            .observe(this) { infos ->
                // REPLACE leaves the cancelled runs around, the live one matters.
                work = infos.firstOrNull { !it.state.isFinished } ?: infos.lastOrNull()
                render()
            }

        lifecycleScope.launch {
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                launch { ActivityLog.changes.collect { render() } }
                launch {
                    while (true) {
                        checkHealth()
                        delay(HEALTH_MS)
                    }
                }
                // Keeps the relative times and the stuck detection current.
                launch {
                    while (true) {
                        delay(REFRESH_MS)
                        render()
                    }
                }
            }
        }

        handleAuthRedirect(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleAuthRedirect(intent)
    }

    override fun onResume() {
        super.onResume()
        render()
    }

    private fun signIn() {
        val url = serverUrl.text.toString().trim().trimEnd('/')
        if (!url.startsWith("https://") && !url.startsWith("http://")) {
            toast("Enter the Tyche URL, e.g. https://tyche.example.com")
            return
        }
        settings.serverUrl = url

        val verifier = Pkce.verifier()
        settings.pendingVerifier = verifier

        val uri = Uri.parse("$url/oauth/mobile").buildUpon()
            .appendQueryParameter("challenge", Pkce.challenge(verifier))
            .appendQueryParameter("device_name", "${Build.MANUFACTURER} ${Build.MODEL}")
            .appendQueryParameter("install_id", settings.installId)
            .build()
        log.add(LogLevel.INFO, "Signing in to ${settings.serverHost}", "Continue in the browser.")
        CustomTabsIntent.Builder()
            .setDefaultColorSchemeParams(
                CustomTabColorSchemeParams.Builder().setToolbarColor(color(R.color.bg)).build(),
            )
            .build()
            .launchUrl(this, uri)
    }

    /** Handles tyche://auth?code=... after signing in on the browser. */
    private fun handleAuthRedirect(intent: Intent?) {
        val data = intent?.data ?: return
        if (data.scheme != "tyche" || data.host != "auth") return
        // Don't handle the same redirect again on recreation.
        intent.data = null

        val code = data.getQueryParameter("code")
        val verifier = settings.pendingVerifier
        if (code.isNullOrEmpty() || verifier == null) {
            log.add(LogLevel.ERROR, "Sign in failed", "The browser came back without a code, try again.")
            return
        }

        lifecycleScope.launch {
            val result = withContext(Dispatchers.IO) {
                runCatching { TycheApi(settings.serverUrl).exchangeCode(code, verifier) }
            }
            settings.pendingVerifier = null
            result
                .onSuccess { token ->
                    settings.token = token
                    settings.authError = false
                    settings.lastError = null
                    log.add(LogLevel.SUCCESS, "Signed in to ${settings.serverHost}")
                    UploadWorker.enqueue(this@MainActivity)
                    askForSmsPermissions()
                }
                .onFailure {
                    val why = (it as? java.io.IOException)?.let { e -> UploadWorker.describe(e, settings) } ?: it.message
                    log.add(LogLevel.ERROR, "Sign in failed", why)
                }
            render()
        }
    }

    private fun signOut() {
        settings.token = null
        settings.authError = false
        // Keep the queued SMS, they're uploaded after signing in again.
        log.add(
            LogLevel.INFO,
            "Signed out",
            "Queued SMS are kept for the next sign in. Revoke this phone from Tyche's SMS page to remove it there.",
        )
        render()
    }

    private fun importSince(sinceMillis: Long, label: String) {
        if (!hasPermission(Manifest.permission.READ_SMS)) {
            askForSmsPermissions()
            return
        }
        lifecycleScope.launch(Dispatchers.IO) { SmsImporter.importSince(this@MainActivity, sinceMillis, label) }
    }

    private fun openSenders() = startActivity(Intent(this, SendersActivity::class.java))

    private fun askForSmsPermissions() {
        requestSmsPermissions.launch(
            arrayOf(Manifest.permission.RECEIVE_SMS, Manifest.permission.READ_SMS),
        )
    }

    private fun hasPermission(permission: String) =
        ContextCompat.checkSelfPermission(this, permission) == PackageManager.PERMISSION_GRANTED

    private fun render() {
        if (isFinishing) return
        val signedIn = settings.isSignedIn
        findViewById<View>(R.id.signed_out).visibility = if (signedIn) View.GONE else View.VISIBLE
        findViewById<View>(R.id.signed_in).visibility = if (signedIn) View.VISIBLE else View.GONE
        findViewById<TextView>(R.id.header_sub).text =
            if (signedIn) "Forwarding bank SMS to ${settings.serverHost}" else "Not signed in"
        renderHealth()

        if (!signedIn) return

        lifecycleScope.launch {
            val (pending, oldest, entries) = withContext(Dispatchers.IO) {
                val queue = SmsQueue(this@MainActivity)
                Triple(queue.count(), queue.oldestQueuedAt(), log.recent(ACTIVITY_LIMIT))
            }
            val now = System.currentTimeMillis()
            val status = SyncStatusResolver.resolve(
                SyncState(
                    now = now,
                    canReceiveSms = hasPermission(Manifest.permission.RECEIVE_SMS),
                    hasSenders = settings.senders.isNotEmpty(),
                    authError = settings.authError,
                    serverHost = settings.serverHost,
                    pending = pending,
                    oldestQueuedAt = oldest,
                    lastSyncAt = settings.lastSyncAt,
                    lastError = settings.lastError,
                    lastErrorAt = settings.lastErrorAt,
                    work = work,
                    serverDown = (health as? Health.Down)?.reason,
                ),
            )
            kickIfStalled(pending, now)
            renderStatus(status)
            renderStats(pending, now)
            findViewById<TextView>(R.id.senders).text =
                settings.senders.sorted().joinToString(" · ").ifEmpty { getString(R.string.senders_none) }
            renderActivity(entries, now)
        }
    }

    /** Pings Tyche, logging when it goes down or comes back. */
    private suspend fun checkHealth() {
        if (settings.serverUrl.isEmpty()) return
        val url = settings.serverUrl
        val result = withContext(Dispatchers.IO) { runCatching { TycheApi(url).health() } }
        val prev = health
        val now = System.currentTimeMillis()
        health = result.fold(
            onSuccess = { Health.Up(it) },
            onFailure = { e ->
                val reason = (e as? java.io.IOException)?.let { UploadWorker.describe(it, settings) } ?: e.toString()
                Health.Down(reason, (prev as? Health.Down)?.since ?: now)
            },
        )
        when (val h = health) {
            is Health.Down -> if (prev !is Health.Down) {
                log.add(LogLevel.ERROR, "Tyche is unreachable", h.reason)
            }
            is Health.Up -> if (prev is Health.Down) {
                log.add(LogLevel.SUCCESS, "Tyche is reachable again", "It was down for ${formatDuration(now - prev.since)}.")
                // Don't wait for the backoff to send what piled up.
                if (settings.isSignedIn && !settings.authError) UploadWorker.enqueue(this)
            }
            null -> Unit
        }
        render()
    }

    private fun renderHealth() {
        val pill = findViewById<View>(R.id.health_pill)
        if (settings.serverUrl.isEmpty()) {
            pill.visibility = View.GONE
            return
        }
        pill.visibility = View.VISIBLE
        val host = settings.serverHost
        val (colorRes, text) = when (val h = health) {
            null -> R.color.dim to "$host · Checking…"
            is Health.Up -> R.color.emerald to "$host · Online · ${h.latencyMs}ms"
            is Health.Down -> R.color.red to "$host · Offline for ${formatDuration(System.currentTimeMillis() - h.since)}"
        }
        val accent = color(colorRes)
        pill.background = android.graphics.drawable.GradientDrawable().apply {
            cornerRadius = dp(999).toFloat()
            setColor(androidx.core.graphics.ColorUtils.setAlphaComponent(accent, 26))
        }
        findViewById<View>(R.id.health_dot).background = android.graphics.drawable.GradientDrawable().apply {
            shape = android.graphics.drawable.GradientDrawable.OVAL
            setColor(accent)
        }
        findViewById<TextView>(R.id.health_text).text = text
    }

    /**
     * Restarts the upload when SMS wait with nothing about to send them, e.g.
     * updating the app drops the scheduled job while WorkManager still lists
     * it as enqueued.
     */
    private fun kickIfStalled(pending: Long, now: Long) {
        if (kicked || pending == 0L || settings.authError) return
        val w = work
        val stalled = when {
            w == null || w.state.isFinished -> settings.lastError == null
            w.state == WorkInfo.State.ENQUEUED ->
                w.nextScheduleTimeMillis != Long.MAX_VALUE && w.nextScheduleTimeMillis < now - STALL_GRACE_MS
            else -> false
        }
        if (!stalled) return
        kicked = true
        log.add(LogLevel.WARN, "Upload was stalled, restarted it", "$pending SMS were waiting with nothing sending them.")
        UploadWorker.enqueue(this)
    }

    private fun renderStatus(status: SyncStatus) {
        val accent = color(status.tone.colorRes())
        findViewById<View>(R.id.status_tile).tintedTile(accent, 16)
        findViewById<ImageView>(R.id.status_icon).icon(status.tone.iconRes(), accent)
        findViewById<TextView>(R.id.status_label).apply {
            text = status.label
            setTextColor(accent)
        }
        findViewById<TextView>(R.id.status_title).text = status.title
        findViewById<TextView>(R.id.status_detail).text = status.detail

        val sync = findViewById<Button>(R.id.sync_now)
        sync.isEnabled = work?.state != WorkInfo.State.RUNNING && !settings.authError
        sync.alpha = if (sync.isEnabled) 1f else 0.5f
        sync.text = if (status.tone == Tone.ERROR || status.tone == Tone.WARN) "Retry now" else getString(R.string.sync_now)

        val action = findViewById<Button>(R.id.status_action)
        val (label, onClick) = when (status.action) {
            StatusAction.GRANT_SMS -> "Allow SMS" to { askForSmsPermissions() }
            StatusAction.SENDERS -> "Pick senders" to { openSenders() }
            StatusAction.SIGN_IN_AGAIN -> "Sign out" to { signOut() }
            StatusAction.NONE -> null to {}
        }
        action.visibility = if (label == null) View.GONE else View.VISIBLE
        action.text = label
        action.setOnClickListener { onClick() }
    }

    private fun renderStats(pending: Long, now: Long) {
        stat(R.id.stat_queued, pending.toString(), "Queued", if (pending > 0) R.color.amber else R.color.text)
        stat(R.id.stat_sent, settings.sentTotal.toString(), "Sent", R.color.text)
        val last = settings.lastSyncAt
        val ago = when {
            last == 0L -> "—"
            now - last < 10_000 -> "now"
            else -> formatDuration(now - last)
        }
        stat(R.id.stat_last, ago, "Last sync", R.color.text)
    }

    private fun stat(id: Int, value: String, label: String, valueColor: Int) {
        val root = findViewById<View>(id)
        root.findViewById<TextView>(R.id.value).apply {
            text = value
            setTextColor(color(valueColor))
        }
        root.findViewById<TextView>(R.id.label).text = label
    }

    private fun renderActivity(entries: List<LogEntry>, now: Long) {
        val list = findViewById<LinearLayout>(R.id.activity_list)
        list.removeAllViews()
        if (entries.isEmpty()) {
            list.addView(
                TextView(this).apply {
                    text = getString(R.string.activity_empty)
                    setTextColor(color(R.color.dim))
                    textSize = 15f
                    setPadding(dp(16), dp(14), dp(16), dp(14))
                },
            )
            return
        }
        val inflater = LayoutInflater.from(this)
        entries.forEach { e ->
            val row = inflater.inflate(R.layout.item_activity, list, false)
            val accent = color(e.level.colorRes())
            row.findViewById<View>(R.id.tile).tintedTile(accent, 10, if (e.level == LogLevel.MUTED) 0.08f else 0.14f)
            row.findViewById<ImageView>(R.id.icon).icon(e.level.iconRes(), accent)
            row.findViewById<TextView>(R.id.title).apply {
                text = e.title
                setTextColor(color(if (e.level == LogLevel.MUTED) R.color.muted else R.color.text_soft))
            }
            row.findViewById<TextView>(R.id.time).text = formatLogTime(e.atMillis, now)
            val detail = row.findViewById<TextView>(R.id.detail)
            if (e.detail.isNullOrEmpty()) {
                detail.visibility = View.GONE
            } else {
                detail.text = e.detail
                detail.maxLines = if (e.id in expanded) Int.MAX_VALUE else 2
                row.setOnClickListener {
                    if (!expanded.add(e.id)) expanded.remove(e.id)
                    detail.maxLines = if (e.id in expanded) Int.MAX_VALUE else 2
                }
            }
            list.addView(row)
        }
    }

    private fun toast(msg: String) = Toast.makeText(this, msg, Toast.LENGTH_LONG).show()

    companion object {
        private const val IMPORT_DAYS = 30L
        private const val ACTIVITY_LIMIT = 80
        private const val REFRESH_MS = 5_000L
        private const val STALL_GRACE_MS = 2 * 60 * 1000L
        private const val HEALTH_MS = 15_000L
    }
}

private sealed interface Health {
    data class Up(val latencyMs: Long) : Health

    data class Down(val reason: String, val since: Long) : Health
}
