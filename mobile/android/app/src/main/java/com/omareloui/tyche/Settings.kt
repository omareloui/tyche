package com.omareloui.tyche

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
import java.util.UUID
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/** App settings; the device token is encrypted with a key kept in the Android Keystore. */
class Settings(context: Context) {
    private val prefs = context.getSharedPreferences("tyche", Context.MODE_PRIVATE)

    var serverUrl: String
        get() = prefs.getString(KEY_SERVER, "") ?: ""
        set(value) = prefs.edit().putString(KEY_SERVER, value.trim().trimEnd('/')).apply()

    var senders: Set<String>
        get() = prefs.getString(KEY_SENDERS, null)?.let(::parseSenders) ?: DEFAULT_SENDERS
        set(value) = prefs.edit().putString(KEY_SENDERS, value.joinToString(",")).apply()

    /** Stable id of this install, lets Tyche reuse the same device on signing in again. */
    val installId: String
        get() = prefs.getString(KEY_INSTALL_ID, null)
            ?: UUID.randomUUID().toString().also { prefs.edit().putString(KEY_INSTALL_ID, it).apply() }

    /** PKCE verifier of the sign in in progress. */
    var pendingVerifier: String?
        get() = prefs.getString(KEY_VERIFIER, null)
        set(value) = prefs.edit().putString(KEY_VERIFIER, value).apply()

    /** Set when the server rejected the token, e.g. the device got revoked. */
    var authError: Boolean
        get() = prefs.getBoolean(KEY_AUTH_ERROR, false)
        set(value) = prefs.edit().putBoolean(KEY_AUTH_ERROR, value).apply()

    /** When the queue was last fully uploaded, 0 if never. */
    var lastSyncAt: Long
        get() = prefs.getLong(KEY_LAST_SYNC_AT, 0)
        set(value) = prefs.edit().putLong(KEY_LAST_SYNC_AT, value).apply()

    /** How many SMS were uploaded from this install. */
    var sentTotal: Long
        get() = prefs.getLong(KEY_SENT_TOTAL, 0)
        set(value) = prefs.edit().putLong(KEY_SENT_TOTAL, value).apply()

    /** Why the last upload failed, cleared once one succeeds. */
    var lastError: String?
        get() = prefs.getString(KEY_LAST_ERROR, null)
        set(value) = prefs.edit().putString(KEY_LAST_ERROR, value).apply()

    var lastErrorAt: Long
        get() = prefs.getLong(KEY_LAST_ERROR_AT, 0)
        set(value) = prefs.edit().putLong(KEY_LAST_ERROR_AT, value).apply()

    /** Up to when (inbox received time) the catch-up looked for missed SMS, 0 if never. */
    var lastScanAt: Long
        get() = prefs.getLong(KEY_LAST_SCAN_AT, 0)
        set(value) = prefs.edit().putLong(KEY_LAST_SCAN_AT, value).apply()

    /** Whether the app asked once to be left out of battery optimization. */
    var batteryAsked: Boolean
        get() = prefs.getBoolean(KEY_BATTERY_ASKED, false)
        set(value) = prefs.edit().putBoolean(KEY_BATTERY_ASKED, value).apply()

    /** Shows the dev tools sending fake SMS, toggled by tapping the header 7 times. */
    var devMode: Boolean
        get() = prefs.getBoolean(KEY_DEV_MODE, false)
        set(value) = prefs.edit().putBoolean(KEY_DEV_MODE, value).apply()

    /** Account reference filled into the dev tools' transaction SMS. */
    var devAccountRef: String
        get() = prefs.getString(KEY_DEV_ACCOUNT_REF, null) ?: DEFAULT_DEV_ACCOUNT_REF
        set(value) = prefs.edit().putString(KEY_DEV_ACCOUNT_REF, value.trim()).apply()

    /** The server as host[:port], for messages. */
    val serverHost: String
        get() = runCatching {
            val uri = java.net.URI(serverUrl)
            if (uri.port > 0) "${uri.host}:${uri.port}" else uri.host
        }.getOrNull() ?: serverUrl

    var token: String?
        get() = prefs.getString(KEY_TOKEN, null)?.let(::decrypt)
        set(value) {
            if (value == null) prefs.edit().remove(KEY_TOKEN).apply()
            else prefs.edit().putString(KEY_TOKEN, encrypt(value)).apply()
        }

    val isSignedIn: Boolean get() = token != null && serverUrl.isNotEmpty()

    fun isBankSender(sender: String): Boolean =
        senders.any { it.equals(sender.trim(), ignoreCase = true) }

    private fun key(): SecretKey {
        val ks = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        (ks.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, KEYSTORE).run {
            init(
                KeyGenParameterSpec.Builder(
                    KEY_ALIAS,
                    KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
                )
                    .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                    .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                    .build(),
            )
            generateKey()
        }
    }

    private fun encrypt(plain: String): String {
        val cipher = Cipher.getInstance(TRANSFORMATION).apply { init(Cipher.ENCRYPT_MODE, key()) }
        val out = cipher.iv + cipher.doFinal(plain.toByteArray())
        return Base64.encodeToString(out, Base64.NO_WRAP)
    }

    private fun decrypt(encoded: String): String? = runCatching {
        val data = Base64.decode(encoded, Base64.NO_WRAP)
        val cipher = Cipher.getInstance(TRANSFORMATION).apply {
            init(Cipher.DECRYPT_MODE, key(), GCMParameterSpec(128, data, 0, IV_SIZE))
        }
        String(cipher.doFinal(data, IV_SIZE, data.size - IV_SIZE))
    }.getOrNull()

    companion object {
        /** Senders Tyche has parsers for, keep in sync with smsparser.BankFromSender. */
        val SUPPORTED_SENDERS = setOf("HSBC", "CIB", "AAIB", "Arab Bank")
        val DEFAULT_SENDERS = SUPPORTED_SENDERS

        fun parseSenders(s: String): Set<String> =
            s.split(',').map { it.trim() }.filter { it.isNotEmpty() }.toSet()

        private const val KEY_SERVER = "server_url"
        private const val KEY_SENDERS = "senders"
        private const val KEY_VERIFIER = "pkce_verifier"
        private const val KEY_AUTH_ERROR = "auth_error"
        private const val KEY_LAST_SYNC_AT = "last_sync_at"
        private const val KEY_LAST_ERROR = "last_error"
        private const val KEY_SENT_TOTAL = "sent_total"
        private const val KEY_LAST_ERROR_AT = "last_error_at"
        private const val KEY_TOKEN = "device_token"
        private const val KEY_INSTALL_ID = "install_id"
        private const val KEY_LAST_SCAN_AT = "last_scan_at"
        private const val KEY_BATTERY_ASKED = "battery_asked"
        private const val KEY_DEV_MODE = "dev_mode"
        private const val KEY_DEV_ACCOUNT_REF = "dev_account_ref"
        private const val DEFAULT_DEV_ACCOUNT_REF = "789"

        private const val KEYSTORE = "AndroidKeyStore"
        private const val KEY_ALIAS = "tyche_token_key"
        private const val TRANSFORMATION = "AES/GCM/NoPadding"
        private const val IV_SIZE = 12
    }
}
