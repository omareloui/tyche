package com.omareloui.tyche

import android.os.Bundle
import android.widget.Button
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

/**
 * Sends fake bank SMS through the receiver's path (sender check, queue,
 * upload) to test the parser, the SMS page, reviews and the PWA badge.
 */
class DevActivity : AppCompatActivity() {

    private lateinit var settings: Settings
    private lateinit var accountRef: EditText
    private lateinit var sender: EditText
    private lateinit var body: EditText

    /** The preset in the fields and its body as rendered, to render it again after sending. */
    private var preset: DevPreset? = null
    private var rendered: String? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_dev)
        supportActionBar?.setDisplayHomeAsUpEnabled(true)
        settings = Settings(this)

        accountRef = findViewById(R.id.account_ref)
        sender = findViewById(R.id.sender)
        body = findViewById(R.id.body)
        accountRef.setText(settings.devAccountRef)
        findViewById<TextView>(R.id.warning).text =
            "Fake SMS become real transactions on ${settings.serverHost.ifEmpty { "Tyche" }}."

        addPresets(R.id.presets_valid, DevSmsKind.VALID)
        addPresets(R.id.presets_ignored, DevSmsKind.IGNORED)
        addPresets(R.id.presets_unrecognized, DevSmsKind.UNRECOGNIZED)
        DevSms.PRESETS.firstOrNull()?.let(::fill)

        findViewById<Button>(R.id.send).setOnClickListener { send() }
        findViewById<Button>(R.id.turn_off).setOnClickListener {
            settings.devMode = false
            ActivityLog.get(this).add(LogLevel.MUTED, "Dev mode off")
            finish()
        }
    }

    override fun onSupportNavigateUp(): Boolean {
        finish()
        return true
    }

    private fun addPresets(containerId: Int, kind: DevSmsKind) {
        val container = findViewById<LinearLayout>(containerId)
        DevSms.PRESETS.filter { it.kind == kind }.forEach { p ->
            container.addView(
                Button(this, null, 0, R.style.Tyche_Button_Ghost).apply {
                    text = p.label
                    setOnClickListener { fill(p) }
                },
                LinearLayout.LayoutParams(LinearLayout.LayoutParams.MATCH_PARENT, LinearLayout.LayoutParams.WRAP_CONTENT)
                    .apply { topMargin = dp(8) },
            )
        }
    }

    private fun fill(p: DevPreset) {
        preset = p
        rendered = DevSms.render(p.template, System.currentTimeMillis(), accountRef.text.toString().trim())
        sender.setText(p.sender)
        body.setText(rendered)
    }

    private fun send() {
        val from = sender.text.toString().trim()
        val text = body.text.toString()
        if (from.isEmpty() || text.isBlank()) {
            toast("Enter a sender and a message")
            return
        }
        settings.devAccountRef = accountRef.text.toString()
        val label = preset?.takeIf { rendered == text }?.label ?: "Custom SMS"
        lifecycleScope.launch {
            val queued = withContext(Dispatchers.IO) {
                val log = ActivityLog.get(this@DevActivity)
                // Now, so the server never takes it for a duplicate.
                SmsReceiver.handle(settings, log, SmsQueue(this@DevActivity), from, text, System.currentTimeMillis())
                    .also { if (it) log.add(LogLevel.MUTED, "Dev SMS sent", label) }
            }
            when {
                queued -> {
                    UploadWorker.enqueue(this@DevActivity)
                    toast("Sent, see the activity")
                    // A new amount and time for the next one.
                    preset?.takeIf { rendered == text }?.let(::fill)
                }
                !settings.isBankSender(from) -> toast("$from isn't one of your bank senders")
                !settings.isSignedIn -> toast("Signed out, sign in first")
                else -> toast("Not queued")
            }
        }
    }

    private fun toast(msg: String) = Toast.makeText(this, msg, Toast.LENGTH_SHORT).show()
}
