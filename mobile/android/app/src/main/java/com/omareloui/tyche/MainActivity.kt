package com.omareloui.tyche

import android.Manifest
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.view.View
import android.widget.Button
import android.widget.EditText
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.browser.customtabs.CustomTabsIntent
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.concurrent.TimeUnit

class MainActivity : AppCompatActivity() {

    private lateinit var settings: Settings
    private lateinit var status: TextView
    private lateinit var serverUrl: EditText
    private lateinit var senders: TextView

    private val requestSmsPermissions =
        registerForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) { render() }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_main)
        settings = Settings(this)

        status = findViewById(R.id.status)
        serverUrl = findViewById(R.id.server_url)
        senders = findViewById(R.id.senders)

        serverUrl.setText(settings.serverUrl)

        findViewById<Button>(R.id.sign_in).setOnClickListener { signIn() }
        findViewById<Button>(R.id.sign_out).setOnClickListener { signOut() }
        findViewById<Button>(R.id.sync_now).setOnClickListener {
            UploadWorker.enqueue(this)
            toast("Syncing")
        }
        findViewById<Button>(R.id.import_past).setOnClickListener {
            importSince(System.currentTimeMillis() - TimeUnit.DAYS.toMillis(IMPORT_DAYS))
        }
        findViewById<Button>(R.id.import_all).setOnClickListener { importSince(0) }
        findViewById<Button>(R.id.choose_senders).setOnClickListener {
            startActivity(Intent(this, SendersActivity::class.java))
        }

        handleAuthRedirect(intent)
        render()
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
        CustomTabsIntent.Builder().build().launchUrl(this, uri)
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
            toast("Sign in failed, try again")
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
                    toast("Signed in")
                    askForSmsPermissions()
                }
                .onFailure { toast("Sign in failed: ${it.message}") }
            render()
        }
    }

    private fun signOut() {
        settings.token = null
        settings.authError = false
        SmsQueue(this).clear()
        toast("Signed out. Revoke the device from Tyche's SMS page to fully remove it.")
        render()
    }

    private fun importSince(sinceMillis: Long) {
        if (!hasPermission(Manifest.permission.READ_SMS)) {
            askForSmsPermissions()
            return
        }
        lifecycleScope.launch {
            val count = withContext(Dispatchers.IO) { SmsImporter.importSince(this@MainActivity, sinceMillis) }
            toast("Queued $count SMS")
            render()
        }
    }

    private fun askForSmsPermissions() {
        requestSmsPermissions.launch(
            arrayOf(Manifest.permission.RECEIVE_SMS, Manifest.permission.READ_SMS),
        )
    }

    private fun hasPermission(permission: String) =
        ContextCompat.checkSelfPermission(this, permission) == PackageManager.PERMISSION_GRANTED

    private fun render() {
        val signedIn = settings.isSignedIn
        findViewById<View>(R.id.signed_out).visibility = if (signedIn) View.GONE else View.VISIBLE
        findViewById<View>(R.id.signed_in).visibility = if (signedIn) View.VISIBLE else View.GONE

        if (!signedIn) {
            status.text = "Not signed in"
            return
        }

        senders.text = getString(
            R.string.senders_selected,
            settings.senders.sorted().joinToString(", ").ifEmpty { getString(R.string.senders_none) },
        )

        lifecycleScope.launch {
            val pending = withContext(Dispatchers.IO) { SmsQueue(this@MainActivity).count() }
            status.text = buildString {
                appendLine("Signed in to ${settings.serverUrl}")
                appendLine("Pending SMS: $pending")
                appendLine("Last sync: ${settings.lastSync ?: "never"}")
                if (!hasPermission(Manifest.permission.RECEIVE_SMS)) {
                    appendLine("⚠ SMS permission missing, new SMS won't be sent")
                }
                if (settings.authError) {
                    appendLine("⚠ Tyche rejected this device, sign out and sign in again")
                }
            }
        }
    }

    private fun toast(msg: String) = Toast.makeText(this, msg, Toast.LENGTH_LONG).show()

    companion object {
        private const val IMPORT_DAYS = 30L
    }
}
