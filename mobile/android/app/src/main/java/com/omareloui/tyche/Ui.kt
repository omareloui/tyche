package com.omareloui.tyche

import android.content.Context
import android.content.res.ColorStateList
import android.graphics.drawable.GradientDrawable
import android.view.View
import android.widget.ImageView
import androidx.annotation.ColorRes
import androidx.annotation.DrawableRes
import androidx.core.content.ContextCompat
import androidx.core.graphics.ColorUtils
import java.text.SimpleDateFormat
import java.util.Calendar
import java.util.Date
import java.util.Locale

fun Context.dp(v: Int): Int = (v * resources.displayMetrics.density).toInt()

fun Context.color(@ColorRes id: Int): Int = ContextCompat.getColor(this, id)

/** A rounded tile tinted with the accent, like the web app's icon tiles. */
fun View.tintedTile(accent: Int, radiusDp: Int, alpha: Float = 0.15f) {
    background = GradientDrawable().apply {
        cornerRadius = context.dp(radiusDp).toFloat()
        setColor(ColorUtils.setAlphaComponent(accent, (alpha * 255).toInt()))
    }
}

fun ImageView.icon(@DrawableRes id: Int, color: Int) {
    setImageResource(id)
    imageTintList = ColorStateList.valueOf(color)
}

@ColorRes
fun Tone.colorRes(): Int = when (this) {
    Tone.OK -> R.color.emerald
    Tone.BUSY -> R.color.sky
    Tone.WAITING -> R.color.violet
    Tone.WARN -> R.color.amber
    Tone.ERROR -> R.color.red
}

@DrawableRes
fun Tone.iconRes(): Int = when (this) {
    Tone.OK -> R.drawable.ic_check_circle
    Tone.BUSY -> R.drawable.ic_upload
    Tone.WAITING -> R.drawable.ic_wifi_off
    Tone.WARN -> R.drawable.ic_alert
    Tone.ERROR -> R.drawable.ic_x_circle
}

@ColorRes
fun LogLevel.colorRes(): Int = when (this) {
    LogLevel.MUTED -> R.color.dim
    LogLevel.INFO -> R.color.sky
    LogLevel.SUCCESS -> R.color.emerald
    LogLevel.WARN -> R.color.amber
    LogLevel.ERROR -> R.color.red
}

@DrawableRes
fun LogLevel.iconRes(): Int = when (this) {
    LogLevel.MUTED -> R.drawable.ic_dot
    LogLevel.INFO -> R.drawable.ic_message
    LogLevel.SUCCESS -> R.drawable.ic_check
    LogLevel.WARN -> R.drawable.ic_alert
    LogLevel.ERROR -> R.drawable.ic_x_circle
}

/** e.g. "45s", "12m", "3h 5m", "2d". */
fun formatDuration(ms: Long): String {
    val s = ms / 1000
    return when {
        s < 60 -> "${s.coerceAtLeast(1)}s"
        s < 3600 -> "${s / 60}m"
        s < 86_400 -> if ((s % 3600) / 60 == 0L) "${s / 3600}h" else "${s / 3600}h ${(s % 3600) / 60}m"
        else -> "${s / 86_400}d"
    }
}

/** e.g. "just now", "5m ago", "in 3m". */
fun relative(at: Long, now: Long = System.currentTimeMillis()): String {
    val diff = at - now
    return when {
        diff > 5_000 -> "in ${formatDuration(diff)}"
        diff > -10_000 -> "just now"
        else -> "${formatDuration(-diff)} ago"
    }
}

/** A log entry's time: the clock today, the date before. */
fun formatLogTime(at: Long, now: Long = System.currentTimeMillis()): String {
    val day = Calendar.getInstance().apply { timeInMillis = now }
    val then = Calendar.getInstance().apply { timeInMillis = at }
    val sameDay = day.get(Calendar.YEAR) == then.get(Calendar.YEAR) &&
        day.get(Calendar.DAY_OF_YEAR) == then.get(Calendar.DAY_OF_YEAR)
    val pattern = if (sameDay) "HH:mm" else "MMM d, HH:mm"
    return SimpleDateFormat(pattern, Locale.getDefault()).format(Date(at))
}
