package com.omareloui.tyche

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import java.security.KeyStore
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

    /** PKCE verifier of the sign in in progress. */
    var pendingVerifier: String?
        get() = prefs.getString(KEY_VERIFIER, null)
        set(value) = prefs.edit().putString(KEY_VERIFIER, value).apply()

    /** Set when the server rejected the token, e.g. the device got revoked. */
    var authError: Boolean
        get() = prefs.getBoolean(KEY_AUTH_ERROR, false)
        set(value) = prefs.edit().putBoolean(KEY_AUTH_ERROR, value).apply()

    var lastSync: String?
        get() = prefs.getString(KEY_LAST_SYNC, null)
        set(value) = prefs.edit().putString(KEY_LAST_SYNC, value).apply()

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
        private const val KEY_LAST_SYNC = "last_sync"
        private const val KEY_TOKEN = "device_token"

        private const val KEYSTORE = "AndroidKeyStore"
        private const val KEY_ALIAS = "tyche_token_key"
        private const val TRANSFORMATION = "AES/GCM/NoPadding"
        private const val IV_SIZE = 12
    }
}
