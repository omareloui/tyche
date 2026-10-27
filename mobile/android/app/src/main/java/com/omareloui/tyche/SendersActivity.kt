package com.omareloui.tyche

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.os.Bundle
import android.provider.Telephony
import android.view.View
import android.view.ViewGroup
import android.widget.BaseAdapter
import android.widget.Button
import android.widget.CheckBox
import android.widget.ListView
import android.widget.TextView
import android.widget.Toast
import androidx.activity.result.contract.ActivityResultContracts
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

data class Sender(val address: String, val count: Int, val lastBody: String)

/** Lists everyone in the SMS inbox to pick the bank senders from. */
class SendersActivity : AppCompatActivity() {

    private lateinit var settings: Settings
    private lateinit var adapter: SendersAdapter

    private val requestReadSms =
        registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
            if (granted) load() else {
                Toast.makeText(this, "SMS permission is needed to list the senders", Toast.LENGTH_LONG).show()
                finish()
            }
        }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.activity_senders)
        supportActionBar?.setDisplayHomeAsUpEnabled(true)
        settings = Settings(this)

        adapter = SendersAdapter(this, settings.senders.toMutableSet())
        findViewById<ListView>(R.id.list).apply {
            adapter = this@SendersActivity.adapter
            setOnItemClickListener { _, _, position, _ -> this@SendersActivity.adapter.toggle(position) }
        }

        findViewById<Button>(R.id.save).setOnClickListener {
            settings.senders = adapter.selected
            Toast.makeText(this, "Saved", Toast.LENGTH_SHORT).show()
            finish()
        }

        val granted = ContextCompat.checkSelfPermission(this, Manifest.permission.READ_SMS) ==
            PackageManager.PERMISSION_GRANTED
        if (granted) load() else requestReadSms.launch(Manifest.permission.READ_SMS)
    }

    override fun onSupportNavigateUp(): Boolean {
        finish()
        return true
    }

    private fun load() {
        lifecycleScope.launch {
            val senders = withContext(Dispatchers.IO) { readSenders() }
            adapter.setSenders(senders)
        }
    }

    /** Senders in the inbox, the most recent first. */
    private fun readSenders(): List<Sender> {
        val counts = LinkedHashMap<String, Int>()
        val lastBodies = HashMap<String, String>()

        contentResolver.query(
            Telephony.Sms.Inbox.CONTENT_URI,
            arrayOf(Telephony.Sms.ADDRESS, Telephony.Sms.BODY),
            null, null,
            "${Telephony.Sms.DATE} DESC",
        )?.use { c ->
            while (c.moveToNext()) {
                val address = c.getString(0)?.trim().orEmpty()
                if (address.isEmpty()) continue
                counts[address] = (counts[address] ?: 0) + 1
                lastBodies.getOrPut(address) { c.getString(1).orEmpty() }
            }
        }

        return counts.map { (address, count) -> Sender(address, count, lastBodies[address].orEmpty()) }
    }
}

private class SendersAdapter(
    private val context: Context,
    private val selectedSet: MutableSet<String>,
) : BaseAdapter() {

    private var senders: List<Sender> = emptyList()

    val selected: Set<String> get() = selectedSet.toSet()

    /** Selected senders not in the inbox anymore stay listed so they can be unselected. */
    fun setSenders(inbox: List<Sender>) {
        val missing = selectedSet
            .filter { s -> inbox.none { it.address.equals(s, ignoreCase = true) } }
            .map { Sender(it, 0, "") }
        // Selected first so they're easy to review.
        senders = (inbox + missing).sortedByDescending { isSelected(it) }
        notifyDataSetChanged()
    }

    fun toggle(position: Int) {
        val address = senders[position].address
        val existing = selectedSet.firstOrNull { it.equals(address, ignoreCase = true) }
        if (existing != null) selectedSet.remove(existing) else selectedSet.add(address)
        notifyDataSetChanged()
    }

    private fun isSelected(s: Sender) = selectedSet.any { it.equals(s.address, ignoreCase = true) }

    override fun getCount() = senders.size
    override fun getItem(position: Int) = senders[position]
    override fun getItemId(position: Int) = position.toLong()

    override fun getView(position: Int, convertView: View?, parent: ViewGroup): View {
        val view = convertView
            ?: android.view.LayoutInflater.from(context).inflate(R.layout.item_sender, parent, false)
        val sender = senders[position]

        val supported = Settings.SUPPORTED_SENDERS.any { it.equals(sender.address, ignoreCase = true) }
        view.findViewById<TextView>(R.id.name).text = sender.address
        view.findViewById<CheckBox>(R.id.checked).isChecked = isSelected(sender)
        view.findViewById<TextView>(R.id.preview).text = buildString {
            append(
                context.resources.getQuantityString(
                    R.plurals.sender_summary,
                    sender.count,
                    sender.count,
                    if (supported) context.getString(R.string.sender_supported) else "",
                ),
            )
            if (sender.lastBody.isNotEmpty()) append("\n").append(sender.lastBody.replace('\n', ' '))
        }
        return view
    }
}
