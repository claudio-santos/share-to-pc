package io.github.claudio_santos.sharetopcapp

import android.Manifest
import android.content.Intent
import android.content.SharedPreferences
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.widget.Toast
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.MutableState
import androidx.compose.runtime.mutableStateOf
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import androidx.lifecycle.lifecycleScope
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import io.github.claudio_santos.sharetopcapp.ui.theme.ShareToPcAppTheme
import io.github.claudio_santos.sharetopcapp.ui.theme.StatusOkDark
import io.github.claudio_santos.sharetopcapp.ui.theme.StatusOkLight
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

data class UiStatus(val text: String, val isOk: Boolean = false, val isError: Boolean = false)

class MainActivity : ComponentActivity() {

    private val status = mutableStateOf(UiStatus(""))
    private val isTesting = mutableStateOf(false)
    private val ipInput = mutableStateOf("")
    private val portInput = mutableStateOf("")

    private lateinit var prefs: SharedPreferences

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        prefs = getSharedPreferences("share_to_pc", MODE_PRIVATE)
        ipInput.value = prefs.getString("ip", "") ?: ""
        portInput.value = prefs.getString("port", "8888") ?: "8888"

        enableEdgeToEdge()
        handleIntent(intent)
        setContent {
            ShareToPcAppTheme {
                ShareScreen(
                    ip = ipInput,
                    port = portInput,
                    status = status,
                    isTesting = isTesting,
                    onSave = ::savePrefs,
                    onTest = ::testConnection,
                    onScan = ::applyQr,
                    onScanCancel = { toast("Scan cancelled") },
                    onPermissionDenied = {
                        toast("Camera permission required - enable it in the phone Settings")
                    }
                )
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
            toast("No text shared")
            finish()
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
            toast("No URL found")
            finish()
            return
        }
        Thread {
            val result = postShare(base, url)
            toast(result)
        }.start()
        finish()
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

    private fun postShare(base: String, url: String): String {
        return try {
            val conn = URL("$base/share").openConnection() as HttpURLConnection
            conn.requestMethod = "POST"
            conn.setRequestProperty("Content-Type", "application/json")
            conn.doOutput = true
            conn.connectTimeout = 5000
            conn.readTimeout = 5000
            val body = JSONObject().put("url", url).toString()
            conn.outputStream.use { it.write(body.toByteArray()) }
            val code = conn.responseCode
            if (code == 200) "Opened on PC" else "Failed: HTTP $code"
        } catch (e: Exception) {
            "Failed: ${e.message ?: "connection failed"}"
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
                    conn.connectTimeout = 5000
                    conn.readTimeout = 5000
                    conn.responseCode == 200
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
    onSave: () -> Unit,
    onTest: () -> Unit,
    onScan: (String) -> Unit,
    onScanCancel: () -> Unit,
    onPermissionDenied: () -> Unit
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
                            } else {
                                Text("Test Connection")
                            }
                        }
                    }
                }
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