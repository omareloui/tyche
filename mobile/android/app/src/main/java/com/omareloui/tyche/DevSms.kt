package com.omareloui.tyche

import java.text.SimpleDateFormat
import java.util.Date
import java.util.Locale
import kotlin.random.Random

enum class DevSmsKind { VALID, IGNORED, UNRECOGNIZED }

data class DevPreset(val kind: DevSmsKind, val label: String, val sender: String, val template: String)

/**
 * Fake bank SMS for the dev tools, from the shared templates' samples in
 * internal/smsparser/seed.go (keep them in sync with it). Tokens filled when
 * sending, so the transaction lands today with a new amount:
 * {date:<SimpleDateFormat pattern>}, {time}, {amount}, {acc} (the account
 * reference) and {ref} (a random reference).
 */
object DevSms {
    val PRESETS = listOf(
        DevPreset(
            DevSmsKind.VALID, "HSBC debit purchase", "HSBC",
            "From HSBC: {date:ddMMMyy} DEV SHOP Purchase from 123-456***-{acc} EGP {amount}- Your available balance is EGP 12,345.67",
        ),
        DevPreset(
            DevSmsKind.VALID, "HSBC salary", "HSBC",
            "From HSBC: {date:ddMMMyy} Salary to 123-456***-{acc} EGP {amount}+ Your available balance is EGP 12,345.67",
        ),
        DevPreset(
            DevSmsKind.VALID, "HSBC credit card purchase", "HSBC",
            "Your Credit Card ending with *** {acc} has been used for EGP {amount} on {date:dd/MM/yyyy} at DEV CAFE. Your available limit is EGP 50000.00",
        ),
        DevPreset(
            DevSmsKind.VALID, "CIB debit purchase", "CIB",
            "تم خصم مبلغ  EGP {amount} من بطاقة الخصم المباشر المنتهية بـ **{acc} عند Dev Store في {date:dd/MM/yy} {time} ، الرصيد المتاح 5432.10 EGP",
        ),
        DevPreset(
            DevSmsKind.VALID, "CIB instant transfer in", "CIB",
            "يرجى العلم انه تم تنفيذ تحويل لحظي بمبلغ {amount} جم إلى حسابك المنتهي بـ ********{acc} من DEV FRIEND برقم مرجعي {ref} بتاريخ {date:dd-MM-yyyy} {time} للمزيد، برجاء الاتصال بـ 19666",
        ),
        DevPreset(
            DevSmsKind.IGNORED, "HSBC birthday", "HSBC",
            "From HSBC: The HSBC Team wishes you a Happy Birthday and many happy returns of the day!",
        ),
        DevPreset(
            DevSmsKind.IGNORED, "HSBC invalid PIN", "HSBC",
            "Transaction EGP {amount} on Card ending *** {acc} was declined due to invalid PIN. Enter the correct PIN or call the number at the back of your Card to reset it.",
        ),
        DevPreset(
            DevSmsKind.IGNORED, "CIB card statement", "CIB",
            "كشف حساب شهر أكتوبر 2026  لبطاقتك الائتمانية المنتهية بـ {acc}",
        ),
        DevPreset(
            DevSmsKind.UNRECOGNIZED, "HSBC unknown format", "HSBC",
            "From HSBC: Dev test {ref} of EGP {amount}, no template reads this.",
        ),
        DevPreset(
            DevSmsKind.UNRECOGNIZED, "CIB unknown format", "CIB",
            "رسالة تجريبية {ref} بمبلغ {amount} جم لا يقرأها أي قالب",
        ),
    )

    private val TOKEN = Regex("""\{(date:[^}]+|time|amount|acc|ref)\}""")

    fun render(template: String, nowMillis: Long, accountRef: String): String {
        val now = Date(nowMillis)
        return TOKEN.replace(template) { m ->
            val token = m.groupValues[1]
            when {
                // Uppercased for HSBC's 05NOV26, digits don't care.
                token.startsWith("date:") ->
                    SimpleDateFormat(token.removePrefix("date:"), Locale.US).format(now).uppercase(Locale.US)
                token == "time" -> SimpleDateFormat("HH:mm", Locale.US).format(now)
                token == "amount" -> String.format(Locale.US, "%.2f", Random.nextInt(1_000, 100_000) / 100.0)
                token == "acc" -> accountRef
                else -> Random.nextLong(0x10000000L, 0xffffffffL).toString(16)
            }
        }
    }
}
