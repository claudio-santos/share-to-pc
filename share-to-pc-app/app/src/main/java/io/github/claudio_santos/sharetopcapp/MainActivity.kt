package io.github.claudio_santos.sharetopcapp

import android.Manifest
import android.content.Intent
import android.content.SharedPreferences
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.ArrowDropDown
import androidx.compose.material.icons.filled.Forward10
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Replay10
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Stop
import androidx.compose.material.icons.filled.Tune
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.RadioButton
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.lifecycleScope
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import io.github.claudio_santos.sharetopcapp.ui.theme.ShareToPcAppTheme
import io.github.claudio_santos.sharetopcapp.ui.theme.StatusOkDark
import io.github.claudio_santos.sharetopcapp.ui.theme.StatusOkLight
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

data class UiStatus(val text: String, val isOk: Boolean = false, val isError: Boolean = false)
data class ShareResult(val message: String, val mode: String)
data class PlayerInfo(
    val available: Boolean = false,
    val running: Boolean = false,
    val playing: Boolean = false,
    val title: String = "",
    val pos: Float = 0f,
    val duration: Float = 0f,
    val speed: Float = 1f,
    val source: String = "mpv"
)

data class Track(
    val id: Int,
    val lang: String = "",
    val title: String = "",
    val selected: Boolean = false
)

data class TracksResult(
    val video: List<Track> = emptyList(),
    val audio: List<Track> = emptyList(),
    val sub: List<Track> = emptyList(),
    val editions: List<Track> = emptyList()
)

enum class Screen { Share, Remote }

class MainActivity : ComponentActivity() {

    private val status = mutableStateOf(UiStatus(""))
    private val isTesting = mutableStateOf(false)
    private val ipInput = mutableStateOf("")
    private val portInput = mutableStateOf("")
    private val screen = mutableStateOf(Screen.Share)
    private val playerInfo = mutableStateOf(PlayerInfo())
    private val openMode = mutableStateOf("ask")
    private val pendingShareUrl = mutableStateOf<String?>(null)

    private lateinit var prefs: SharedPreferences

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        prefs = getSharedPreferences("share_to_pc", MODE_PRIVATE)
        ipInput.value = prefs.getString("ip", "") ?: ""
        portInput.value = prefs.getString("port", "8888") ?: "8888"
        openMode.value = prefs.getString("open_mode", "ask") ?: "ask"

        enableEdgeToEdge()
        handleIntent(intent)
        setContent {
            ShareToPcAppTheme {
                when (screen.value) {
                    Screen.Share -> ShareScreen(
                        ip = ipInput,
                        port = portInput,
                        status = status,
                        isTesting = isTesting,
                        openMode = openMode.value,
                        onOpenModeChange = ::setOpenMode,
                        onSave = ::savePrefs,
                        onTest = ::testConnection,
                        onScan = ::applyQr,
                        onScanCancel = { toast("Scan cancelled") },
                        onPermissionDenied = {
                            toast("Camera permission required - enable it in the phone Settings")
                        },
                        onRemote = { screen.value = Screen.Remote }
                    )
                    Screen.Remote -> RemoteScreen(
                        base = buildBase(),
                        playerInfo = playerInfo,
                        onBack = { screen.value = Screen.Share },
                        onCommand = { b, c, v ->
                            val ok = sendRemoteCommand(b, c, v)
                            if (!ok) toast("Command failed")
                            ok
                        },
                        onSetTrack = ::setTrack
                    )
                }
                val pending = pendingShareUrl.value
                if (pending != null) {
                    ShareModeDialog(
                        url = pending,
                        onSelect = { mode, rememberChoice ->
                            pendingShareUrl.value = null
                            if (rememberChoice) setOpenMode(mode)
                            val base = buildBase()
                            if (base != null) doShare(base, pending, mode)
                        },
                        onDismiss = {
                            pendingShareUrl.value = null
                            finish()
                        }
                    )
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        handleIntent(intent)
    }

    private fun handleIntent(intent: Intent?) {
        if (intent?.action != Intent.ACTION_SEND) return
        val text = intent.getStringExtra(Intent.EXTRA_TEXT)?.trim()
        if (text.isNullOrEmpty()) {
            toastThenFinish("No text shared")
            return
        }
        handleShare(text)
    }

    private fun handleShare(text: String) {
        val base = buildBase()
        if (base == null) {
            status.value = UiStatus("Set the PC IP and port first")
            return
        }
        val url = extractUrl(text)
        if (url == null) {
            toastThenFinish("No URL found")
            return
        }
        val mode = prefs.getString("open_mode", "ask") ?: "ask"
        if (mode == "ask") {
            pendingShareUrl.value = url
        } else {
            doShare(base, url, mode)
        }
    }

    private fun doShare(base: String, url: String, mode: String) {
        Thread {
            val result = postShare(base, url, mode)
            if (isDestroyed) return@Thread
            if (result.mode == "mpv" || result.mode == "browser") {
                toast(result.message)
                runOnUiThread { if (!isDestroyed) screen.value = Screen.Remote }
            } else {
                toastThenFinish(result.message)
            }
        }.start()
    }

    private fun setOpenMode(mode: String) {
        openMode.value = mode
        prefs.edit().putString("open_mode", mode).apply()
    }

    private fun buildBase(): String? {
        val ip = ipInput.value.trim()
        val port = portInput.value.trim()
        return if (ip.isEmpty() || port.isEmpty()) null else "http://$ip:$port"
    }

    private fun extractUrl(text: String): String? {
        val trimmed = text.trim()
        if (trimmed.startsWith("http://") || trimmed.startsWith("https://")) return trimmed
        val match = Regex("https?://\\S+").find(text)
        return match?.value?.trimEnd('.', ',', ')', '"', '\'')
    }

    private fun postShare(base: String, url: String, mode: String): ShareResult {
        val conn = URL("$base/share").openConnection() as HttpURLConnection
        return try {
            conn.requestMethod = "POST"
            conn.setRequestProperty("Content-Type", "application/json")
            conn.doOutput = true
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            val body = JSONObject().put("url", url).put("mode", mode).toString()
            conn.outputStream.use { it.write(body.toByteArray()) }
            val code = conn.responseCode
            if (code == 200) {
                val json = JSONObject(conn.inputStream.bufferedReader().readText())
                val resultMode = json.optString("mode", "browser")
                val message = if (resultMode == "mpv") "Playing on PC" else "Opened in browser"
                ShareResult(message, resultMode)
            } else {
                conn.errorStream?.close()
                ShareResult("Failed: HTTP $code", "error")
            }
        } catch (e: Exception) {
            ShareResult("Failed: ${e.message ?: "connection failed"}", "error")
        } finally {
            conn.disconnect()
        }
    }

    private fun testConnection() {
        val base = buildBase()
        if (base == null) {
            status.value = UiStatus("Set the PC IP and port first")
            return
        }
        if (isTesting.value) return
        isTesting.value = true
        status.value = UiStatus("Testing...")
        lifecycleScope.launch {
            val reached = withContext(Dispatchers.IO) {
                try {
                    val conn = URL("$base/api/config").openConnection() as HttpURLConnection
                    try {
                        conn.connectTimeout = 5000
                        conn.readTimeout = 5000
                        if (conn.responseCode == 200) {
                            true
                        } else {
                            conn.errorStream?.close()
                            false
                        }
                    } finally {
                        conn.disconnect()
                    }
                } catch (e: Exception) {
                    false
                }
            }
            isTesting.value = false
            status.value =
                if (reached) UiStatus("PC reachable", isOk = true) else UiStatus("PC not reachable", isError = true)
        }
    }

    private fun savePrefs() {
        val ip = ipInput.value.trim()
        val port = portInput.value.trim()
        val portNumber = port.toIntOrNull()
        if (ip.isEmpty() || portNumber == null || portNumber < 1 || portNumber > 65535) {
            status.value = UiStatus("Enter a valid IP and port", isError = true)
            return
        }
        prefs.edit()
            .putString("ip", ip)
            .putString("port", portNumber.toString())
            .apply()
        status.value = UiStatus("Saved", isOk = true)
    }

    private fun applyQr(text: String) {
        try {
            val url = URL(text.trim())
            if (url.protocol != "http" && url.protocol != "https") {
                toast("Invalid QR: not a ShareToPC address")
                return
            }
            val host = url.host
            if (host.isEmpty()) {
                toast("Invalid QR: no PC address")
                return
            }
            val port = if (url.port != -1) url.port.toString() else "8888"
            ipInput.value = host
            portInput.value = port
            savePrefs()
            status.value = UiStatus("Configured from QR: $host:$port", isOk = true)
            testConnection()
        } catch (e: Exception) {
            toast("Invalid QR: ${e.message ?: "could not read address"}")
        }
    }

    private fun sendRemoteCommand(base: String, cmd: String, value: Float?): Boolean {
        val conn = URL("$base/remote/cmd").openConnection() as HttpURLConnection
        return try {
            conn.requestMethod = "POST"
            conn.setRequestProperty("Content-Type", "application/json")
            conn.doOutput = true
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            val json = JSONObject().put("cmd", cmd)
            if (value != null) json.put("value", value.toDouble())
            conn.outputStream.use { it.write(json.toString().toByteArray()) }
            conn.responseCode == 200
        } catch (e: Exception) {
            false
        } finally {
            conn.disconnect()
        }
    }

    private fun setTrack(base: String, type: String, id: Int) {
        Thread {
            val ok = try {
                val conn = URL("$base/remote/cmd").openConnection() as HttpURLConnection
                try {
                    conn.requestMethod = "POST"
                    conn.setRequestProperty("Content-Type", "application/json")
                    conn.doOutput = true
                    conn.connectTimeout = 5000
                    conn.readTimeout = 5000
                    val body = JSONObject()
                        .put("cmd", "set_track")
                        .put("type", type)
                        .put("value", id)
                        .toString()
                    conn.outputStream.use { it.write(body.toByteArray()) }
                    conn.responseCode == 200
                } finally {
                    conn.disconnect()
                }
            } catch (e: Exception) {
                false
            }
            if (!ok) toast("Track change failed")
        }.start()
    }

    private fun toastThenFinish(message: String) {
        Handler(Looper.getMainLooper()).postDelayed({
            Toast.makeText(applicationContext, message, Toast.LENGTH_SHORT).show()
            if (!isDestroyed) finish()
        }, 1000)
    }

    private fun toast(message: String) {
        Handler(Looper.getMainLooper()).post {
            Toast.makeText(applicationContext, message, Toast.LENGTH_SHORT).show()
        }
    }
}

@Composable
fun ShareScreen(
    ip: MutableState<String>,
    port: MutableState<String>,
    status: MutableState<UiStatus>,
    isTesting: MutableState<Boolean>,
    openMode: String,
    onOpenModeChange: (String) -> Unit,
    onSave: () -> Unit,
    onTest: () -> Unit,
    onScan: (String) -> Unit,
    onScanCancel: () -> Unit,
    onPermissionDenied: () -> Unit,
    onRemote: () -> Unit
) {
    val scanLauncher = rememberLauncherForActivityResult(ScanContract()) { result ->
        val content = result?.contents
        if (content != null) onScan(content) else onScanCancel()
    }
    val cameraPermLauncher = rememberLauncherForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        if (granted) {
            scanLauncher.launch(
                ScanOptions().apply {
                    setCaptureActivity(PortraitCaptureActivity::class.java)
                    setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                    setBeepEnabled(true)
                    setOrientationLocked(false)
                    setPrompt("Point at the QR code from the ShareToPC page")
                }
            )
        } else {
            onPermissionDenied()
        }
    }
    val statusColor = when {
        status.value.isError -> MaterialTheme.colorScheme.error
        status.value.isOk -> if (isSystemInDarkTheme()) StatusOkDark else StatusOkLight
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }

    Scaffold(modifier = Modifier.fillMaxSize()) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(24.dp)
                .verticalScroll(rememberScrollState())
                .imePadding()
        ) {
            Text(
                "ShareToPC",
                style = MaterialTheme.typography.headlineLarge,
                color = MaterialTheme.colorScheme.primary
            )
            Spacer(Modifier.height(4.dp))
            Text(
                "Send links from your phone to this PC",
                style = MaterialTheme.typography.bodyMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(Modifier.height(24.dp))

            Card(modifier = Modifier.fillMaxWidth()) {
                Column(Modifier.padding(16.dp)) {
                    Text("PC Settings", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(12.dp))

                    OutlinedTextField(
                        value = ip.value,
                        onValueChange = { ip.value = it },
                        label = { Text("PC IP") },
                        singleLine = true,
                        modifier = Modifier.fillMaxWidth()
                    )
                    Spacer(Modifier.height(8.dp))

                    OutlinedTextField(
                        value = port.value,
                        onValueChange = { port.value = it },
                        label = { Text("Port") },
                        singleLine = true,
                        keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
                        modifier = Modifier.fillMaxWidth()
                    )
                    Spacer(Modifier.height(16.dp))

                    Button(
                        onClick = { cameraPermLauncher.launch(Manifest.permission.CAMERA) },
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("Scan QR Code")
                    }
                    Spacer(Modifier.height(8.dp))

                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                        OutlinedButton(
                            onClick = onSave,
                            modifier = Modifier.weight(1f)
                        ) {
                            Text("Save")
                        }
                        Button(
                            onClick = onTest,
                            enabled = !isTesting.value,
                            modifier = Modifier.weight(1f)
                        ) {
                            if (isTesting.value) {
                                CircularProgressIndicator(
                                    modifier = Modifier.size(18.dp),
                                    strokeWidth = 2.dp,
                                    color = MaterialTheme.colorScheme.onPrimary
                                )
                                Spacer(Modifier.width(8.dp))
                                Text("Testing...")
                            } else {
                                Text("Test")
                            }
                        }
                    }
                    Spacer(Modifier.height(16.dp))

                    Text(
                        "Open mode: ask each share or always use one",
                        style = MaterialTheme.typography.titleSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    Spacer(Modifier.height(8.dp))

                    Row(
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        modifier = Modifier.horizontalScroll(rememberScrollState())
                    ) {
                        listOf("ask" to "Ask", "mpv" to "mpv", "browser" to "Browser").forEach { (value, label) ->
                            FilterChip(
                                selected = openMode == value,
                                onClick = { onOpenModeChange(value) },
                                label = { Text(label) }
                            )
                        }
                    }
                }
            }
            Spacer(Modifier.height(16.dp))

            OutlinedButton(
                onClick = onRemote,
                modifier = Modifier.fillMaxWidth()
            ) {
                Text("Remote")
            }
            Spacer(Modifier.height(16.dp))

            if (status.value.text.isNotEmpty()) {
                Text(
                    status.value.text,
                    style = MaterialTheme.typography.bodyMedium,
                    color = statusColor
                )
            }
        }
    }
}

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun RemoteScreen(
    base: String?,
    playerInfo: MutableState<PlayerInfo>,
    onBack: () -> Unit,
    onCommand: (String, String, Float?) -> Boolean,
    onSetTrack: (String, String, Int) -> Unit
) {
    val pollingJob = remember { mutableStateOf<Job?>(null) }
    val scrubbing = remember { mutableStateOf(false) }
    val scrubValue = remember { mutableStateOf(0f) }
    val pendingSeek = remember { mutableStateOf<Float?>(null) }
    val pendingSeekTime = remember { mutableStateOf(0L) }
    val showTracks = remember { mutableStateOf(false) }
    val tracks = remember { mutableStateOf<TracksResult?>(null) }
    val tracksLoading = remember { mutableStateOf(false) }
    val tracksVersion = remember { mutableStateOf(0) }
    val showStopConfirm = remember { mutableStateOf(false) }
    val speedExpanded = remember { mutableStateOf(false) }

    val lifecycleOwner = LocalLifecycleOwner.current

    androidx.compose.runtime.DisposableEffect(lifecycleOwner, base) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                if (pollingJob.value?.isActive != true && base != null) {
                    pollingJob.value = lifecycleOwner.lifecycleScope.launch {
                        while (isActive) {
                            withContext(Dispatchers.IO) {
                                try {
                                    val conn = URL("$base/api/player").openConnection() as HttpURLConnection
                                    try {
                                        conn.connectTimeout = 3000
                                        conn.readTimeout = 3000
                                        if (conn.responseCode == 200) {
                                            val json = JSONObject(conn.inputStream.bufferedReader().readText())
                                            playerInfo.value = PlayerInfo(
                                                available = json.optBoolean("available", false),
                                                running = json.optBoolean("running", false),
                                                playing = json.optBoolean("playing", false),
                                                title = json.optString("title", ""),
                                                pos = json.optDouble("pos", 0.0).toFloat(),
                                                duration = json.optDouble("duration", 0.0).toFloat(),
                                                speed = json.optDouble("speed", 1.0).toFloat(),
                                                source = json.optString("source", "mpv")
                                            )
                                            if (!playerInfo.value.running) {
                                                pendingSeek.value = null
                                            } else {
                                                val target = pendingSeek.value
                                                if (target != null &&
                                                    (Math.abs(playerInfo.value.pos - target) <= 1.5f ||
                                                        System.currentTimeMillis() - pendingSeekTime.value > 4000)
                                                ) {
                                                    pendingSeek.value = null
                                                }
                                            }
                                        } else {
                                            conn.errorStream?.close()
                                        }
                                    } finally {
                                        conn.disconnect()
                                    }
                                } catch (e: Exception) {
                                    Log.w("ShareToPC", "player polling error", e)
                                }
                            }
                            delay(1000)
                        }
                    }
                }
            } else if (event == Lifecycle.Event.ON_PAUSE) {
                pollingJob.value?.cancel()
                pollingJob.value = null
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose {
            lifecycleOwner.lifecycle.removeObserver(observer)
            pollingJob.value?.cancel()
        }
    }

    LaunchedEffect(showTracks.value, base, tracksVersion.value) {
        if (showTracks.value && base != null) {
            if (tracksVersion.value > 0) delay(600)
            var lastSig = -1
            repeat(6) { round ->
                tracksLoading.value = true
                tracks.value = withContext(Dispatchers.IO) { fetchTracks(base) }
                tracksLoading.value = false
                val t = tracks.value
                val sig = t?.let {
                    it.video.size + it.audio.size + it.sub.size + it.editions.size
                } ?: -1
                if (sig != -1 && sig == lastSig) {
                    return@LaunchedEffect
                }
                lastSig = sig
                if (round < 5) delay(2000)
            }
        }
    }

    val info = playerInfo.value
    val statusColor = when {
        !info.available -> MaterialTheme.colorScheme.error
        info.running -> if (isSystemInDarkTheme()) StatusOkDark else StatusOkLight
        else -> MaterialTheme.colorScheme.onSurfaceVariant
    }
    val statusText = when {
        !info.available ->
            if (info.source == "browser") "Player: no Chromium browser on the PC" else "Player: mpv not installed on the PC"
        !info.running -> "Player: not running"
        info.playing -> "Playing"
        else -> "Paused"
    }
    val seekMax = if (info.duration > 0f) info.duration else 1f
    val isLive = info.duration <= 0f && info.running

    Scaffold(modifier = Modifier.fillMaxSize()) { innerPadding ->
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(innerPadding)
                .padding(horizontal = 16.dp, vertical = 16.dp)
                .imePadding(),
            horizontalAlignment = Alignment.CenterHorizontally
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically
            ) {
                IconButton(onClick = onBack) {
                    Icon(
                        Icons.AutoMirrored.Filled.ArrowBack,
                        contentDescription = "Back",
                        tint = MaterialTheme.colorScheme.primary
                    )
                }
                Text(
                    "Remote",
                    style = MaterialTheme.typography.headlineLarge,
                    color = MaterialTheme.colorScheme.primary
                )
            }
            Spacer(Modifier.height(16.dp))

            if (base == null) {
                Text(
                    "Set the PC IP and port first",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
                return@Scaffold
            }

            Card(modifier = Modifier.fillMaxWidth()) {
                Column(Modifier.padding(16.dp)) {
                    Text("Player", style = MaterialTheme.typography.titleMedium)
                    Spacer(Modifier.height(8.dp))

                    Text(
                        info.title.ifEmpty { "No title" },
                        style = MaterialTheme.typography.bodyMedium,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                    Spacer(Modifier.height(4.dp))
                    Row(verticalAlignment = Alignment.CenterVertically) {
                        if (info.running) {
                            Icon(
                                imageVector = if (info.playing) Icons.Filled.PlayArrow else Icons.Filled.Pause,
                                contentDescription = null,
                                modifier = Modifier.size(16.dp),
                                tint = statusColor
                            )
                            Spacer(Modifier.width(6.dp))
                        }
                        Text(
                            statusText,
                            style = MaterialTheme.typography.bodyMedium,
                            color = statusColor
                        )
                    }
                }
            }
            Spacer(Modifier.height(16.dp))

            if (info.running && !isLive) {
                val displayValue = when {
                    scrubbing.value -> scrubValue.value
                    pendingSeek.value != null -> pendingSeek.value!!
                    else -> info.pos
                }
                Slider(
                    value = displayValue,
                    onValueChange = { v ->
                        scrubbing.value = true
                        scrubValue.value = v
                    },
                    onValueChangeFinished = {
                        scrubbing.value = false
                        pendingSeek.value = scrubValue.value
                        pendingSeekTime.value = System.currentTimeMillis()
                        Thread { onCommand(base, "seek_to", scrubValue.value) }.start()
                    },
                    valueRange = 0f..seekMax,
                    modifier = Modifier.fillMaxWidth()
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(
                        formatTime(displayValue),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    Text(
                        formatTime(info.duration),
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
                Spacer(Modifier.height(8.dp))
            }

            if (isLive) {
                Text(
                    "Live",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    textAlign = TextAlign.Center,
                    modifier = Modifier.fillMaxWidth()
                )
                Spacer(Modifier.height(8.dp))
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                OutlinedButton(
                    onClick = {
                        Thread { onCommand(base, "seek", -10f) }.start()
                    },
                    enabled = info.running,
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(
                        Icons.Filled.Replay10,
                        contentDescription = "Rewind 10 seconds",
                        modifier = Modifier.size(22.dp)
                    )
                }
                Button(
                    onClick = {
                        Thread { onCommand(base, "toggle", null) }.start()
                    },
                    enabled = info.running,
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(
                        imageVector = if (info.playing) Icons.Filled.Pause else Icons.Filled.PlayArrow,
                        contentDescription = if (info.playing) "Pause" else "Play",
                        modifier = Modifier.size(28.dp)
                    )
                }
                OutlinedButton(
                    onClick = {
                        Thread { onCommand(base, "seek", 10f) }.start()
                    },
                    enabled = info.running,
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(
                        Icons.Filled.Forward10,
                        contentDescription = "Forward 10 seconds",
                        modifier = Modifier.size(22.dp)
                    )
                }
            }
            Spacer(Modifier.height(12.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                OutlinedButton(
                    onClick = {
                        Thread { onCommand(base, "fullscreen", null) }.start()
                    },
                    enabled = info.running,
                    modifier = Modifier.weight(0.2f)
                ) {
                    Icon(
                        Icons.Filled.Fullscreen,
                        contentDescription = "Toggle fullscreen",
                        modifier = Modifier.size(22.dp)
                    )
                }
                Box(modifier = Modifier.weight(0.8f)) {
                    OutlinedButton(
                        onClick = { speedExpanded.value = true },
                        enabled = info.running,
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text("Speed: ${formatSpeed(info.speed)}x")
                        Spacer(Modifier.width(6.dp))
                        Icon(Icons.Filled.ArrowDropDown, contentDescription = null)
                    }
                    DropdownMenu(
                        expanded = speedExpanded.value,
                        onDismissRequest = { speedExpanded.value = false }
                    ) {
                        listOf(0.5f, 1.0f, 1.25f, 1.5f, 2.0f).forEach { s ->
                            DropdownMenuItem(
                                text = { Text("${formatSpeed(s)}x") },
                                onClick = {
                                    speedExpanded.value = false
                                    Thread { onCommand(base, "set_speed", s) }.start()
                                }
                            )
                        }
                    }
                }
            }
            Spacer(Modifier.height(12.dp))

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                if (info.source != "browser") {
                    OutlinedButton(
                        onClick = {
                            showTracks.value = true
                            tracksLoading.value = true
                        },
                        enabled = info.running,
                        modifier = Modifier.weight(1f)
                    ) {
                        Icon(Icons.Filled.Tune, contentDescription = null, modifier = Modifier.size(18.dp))
                        Spacer(Modifier.width(6.dp))
                        Text("Tracks")
                    }
                }
                OutlinedButton(
                    onClick = { showStopConfirm.value = true },
                    enabled = info.running,
                    modifier = Modifier.weight(1f)
                ) {
                    Icon(Icons.Filled.Stop, contentDescription = null, modifier = Modifier.size(18.dp))
                    Spacer(Modifier.width(6.dp))
                    Text("Stop")
                }
            }

            if (showStopConfirm.value) {
                AlertDialog(
                    onDismissRequest = { showStopConfirm.value = false },
                    title = { Text("Stop playback?") },
                    text = { Text("The video will close on the PC.") },
                    confirmButton = {
                        TextButton(onClick = {
                            showStopConfirm.value = false
                            showTracks.value = false
                            Thread { onCommand(base, "quit", null) }.start()
                        }) { Text("Stop") }
                    },
                    dismissButton = {
                        TextButton(onClick = { showStopConfirm.value = false }) { Text("Cancel") }
                    }
                )
            }

            if (showTracks.value) {
                ModalBottomSheet(onDismissRequest = { showTracks.value = false }) {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .verticalScroll(rememberScrollState())
                            .padding(start = 16.dp, end = 16.dp, bottom = 24.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Text("Tracks", style = MaterialTheme.typography.titleLarge)
                            IconButton(onClick = { tracksVersion.value++ }) {
                                Icon(
                                    Icons.Filled.Refresh,
                                    contentDescription = "Refresh tracks"
                                )
                            }
                        }
                        Spacer(Modifier.height(12.dp))
                        val t = tracks.value
                        when {
                            tracksLoading.value && t == null -> CircularProgressIndicator(modifier = Modifier.size(24.dp))
                            t == null -> Text(
                                "Failed to load tracks",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                            t.video.isEmpty() && t.audio.isEmpty() && t.sub.isEmpty() && t.editions.isEmpty() -> Text(
                                "No tracks reported by the player",
                                style = MaterialTheme.typography.bodyMedium,
                                color = MaterialTheme.colorScheme.onSurfaceVariant
                            )
                            else -> {
                                if (t.editions.isNotEmpty()) {
                                    Text(
                                        "Quality",
                                        style = MaterialTheme.typography.titleSmall,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                    Spacer(Modifier.height(4.dp))
                                    t.editions.forEach { e ->
                                        TrackRow(
                                            label = e.title.ifBlank { "Edition ${e.id}" },
                                            selected = e.selected,
                                            onClick = {
                                                onSetTrack(base, "edition", e.id)
                                                tracksVersion.value++
                                            }
                                        )
                                    }
                                    Spacer(Modifier.height(12.dp))
                                }
                                val groups = listOf(
                                    Triple("Audio", t.audio, "audio"),
                                    Triple("Subtitles", t.sub, "sub"),
                                    Triple("Video", t.video, "video")
                                )
                                groups.forEach { (label, list, type) ->
                                    if (list.isEmpty()) return@forEach
                                    Text(
                                        label,
                                        style = MaterialTheme.typography.titleSmall,
                                        color = MaterialTheme.colorScheme.onSurfaceVariant
                                    )
                                    Spacer(Modifier.height(4.dp))
                                    if (type != "video") {
                                        TrackRow(
                                            label = "None",
                                            selected = list.none { it.selected },
                                            onClick = {
                                                onSetTrack(base, type, 0)
                                                tracksVersion.value++
                                            }
                                        )
                                    }
                                    list.forEach { trk ->
                                        TrackRow(
                                            label = listOfNotNull(trk.title, trk.lang)
                                                .filter { it.isNotBlank() }
                                                .joinToString(" \u2014 ")
                                                .ifEmpty { "Track ${trk.id}" },
                                            selected = trk.selected,
                                            onClick = {
                                                onSetTrack(base, type, trk.id)
                                                tracksVersion.value++
                                            }
                                        )
                                    }
                                    Spacer(Modifier.height(12.dp))
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun ShareModeDialog(
    url: String,
    onSelect: (String, Boolean) -> Unit,
    onDismiss: () -> Unit
) {
    val rememberChoice = remember { mutableStateOf(false) }
    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Open this link in?") },
        text = {
            Column {
                Text(
                    url,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                    maxLines = 3,
                    overflow = TextOverflow.Ellipsis
                )
                Spacer(Modifier.height(12.dp))
                Row(verticalAlignment = Alignment.CenterVertically) {
                    Checkbox(
                        checked = rememberChoice.value,
                        onCheckedChange = { rememberChoice.value = it }
                    )
                    Spacer(Modifier.width(4.dp))
                    Text("Remember my choice", style = MaterialTheme.typography.bodyMedium)
                }
            }
        },
        confirmButton = {
            Row {
                TextButton(onClick = { onSelect("browser", rememberChoice.value) }) { Text("Browser") }
                Spacer(Modifier.width(8.dp))
                TextButton(onClick = { onSelect("mpv", rememberChoice.value) }) { Text("mpv") }
            }
        },
        dismissButton = {
            TextButton(onClick = onDismiss) { Text("Cancel") }
        }
    )
}

private fun formatTime(seconds: Float): String {
    val total = seconds.toInt()
    val h = total / 3600
    val m = (total % 3600) / 60
    val s = total % 60
    return if (h > 0) String.format("%d:%02d:%02d", h, m, s)
    else String.format("%d:%02d", m, s)
}

private fun formatSpeed(speed: Float): String {
    val s = String.format("%.2f", speed).trimEnd('0').trimEnd('.')
    return if (s == "1") "1.0" else s
}

@Composable
private fun TrackRow(
    label: String,
    selected: Boolean,
    onClick: () -> Unit
) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .clip(RoundedCornerShape(8.dp))
            .clickable(onClick = onClick)
            .padding(horizontal = 8.dp, vertical = 6.dp),
        verticalAlignment = Alignment.CenterVertically
    ) {
        RadioButton(selected = selected, onClick = onClick)
        Spacer(Modifier.width(12.dp))
        Text(
            label,
            style = MaterialTheme.typography.bodyMedium,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis
        )
    }
}

private fun fetchTracks(base: String): TracksResult? {
    val conn = URL("$base/api/tracks").openConnection() as HttpURLConnection
    return try {
        conn.connectTimeout = 5000
        conn.readTimeout = 5000
        if (conn.responseCode != 200) {
            conn.errorStream?.close()
            null
        } else {
            val json = JSONObject(conn.inputStream.bufferedReader().readText())
            fun parseList(key: String): List<Track> {
                val arr = json.optJSONArray(key) ?: return emptyList()
                return (0 until arr.length()).mapNotNull { i ->
                    val o = arr.optJSONObject(i) ?: return@mapNotNull null
                    Track(
                        id = o.optInt("id", 0),
                        lang = o.optString("lang", ""),
                        title = o.optString("title", ""),
                        selected = o.optBoolean("selected", false)
                    )
                }
            }
            TracksResult(
                video = parseList("video"),
                audio = parseList("audio"),
                sub = parseList("sub"),
                editions = parseList("editions")
            )
        }
    } catch (e: Exception) {
        null
    } finally {
        conn.disconnect()
    }
}
