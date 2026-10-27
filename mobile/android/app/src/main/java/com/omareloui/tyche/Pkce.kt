package com.omareloui.tyche

import android.util.Base64
import java.security.MessageDigest
import java.security.SecureRandom

object Pkce {
    private const val FLAGS = Base64.URL_SAFE or Base64.NO_PADDING or Base64.NO_WRAP

    fun verifier(): String {
        val bytes = ByteArray(32).also { SecureRandom().nextBytes(it) }
        return Base64.encodeToString(bytes, FLAGS)
    }

    /** S256 challenge: base64url(sha256(verifier)), 43 chars. */
    fun challenge(verifier: String): String {
        val digest = MessageDigest.getInstance("SHA-256").digest(verifier.toByteArray())
        return Base64.encodeToString(digest, FLAGS)
    }
}
