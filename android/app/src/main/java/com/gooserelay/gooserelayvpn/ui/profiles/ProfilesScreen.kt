package com.gooserelay.gooserelayvpn.ui.profiles

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Scaffold
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.collectAsState
import androidx.compose.ui.Modifier
import androidx.compose.ui.unit.dp
import androidx.hilt.navigation.compose.hiltViewModel
import com.gooserelay.gooserelayvpn.data.local.ProfileEntity
import com.gooserelay.gooserelayvpn.ui.components.mdv.controls.MdvBackTopAppBar

@Composable
fun ProfilesScreen(
    viewModel: ProfilesViewModel = hiltViewModel(),
    onBack: () -> Unit,
    onOpenSettings: (Long) -> Unit
) {
    val profiles by viewModel.profiles.collectAsState()
    var showEditor by remember { mutableStateOf(false) }
    var editing by remember { mutableStateOf<ProfileEntity?>(null) }

    Scaffold(
        topBar = {
            MdvBackTopAppBar(
                title = "Profiles",
                onBack = onBack,
                actions = {
                    IconButton(onClick = { editing = null; showEditor = true }) {
                        Icon(Icons.Filled.Add, contentDescription = "Add")
                    }
                }
            )
        }
    ) { padding ->
        LazyColumn(
            modifier = Modifier
                .fillMaxSize()
                .padding(padding)
                .padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            items(profiles) { profile ->
                Card(onClick = { viewModel.selectProfile(profile.id) }, modifier = Modifier.fillMaxWidth()) {
                    Column(Modifier.padding(12.dp)) {
                        Text(profile.name)
                        Text("${profile.socksHost}:${profile.socksPort}")
                        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.End) {
                            IconButton(onClick = { editing = profile; showEditor = true }) {
                                Icon(Icons.Filled.Edit, contentDescription = "Edit")
                            }
                            IconButton(onClick = { onOpenSettings(profile.id) }) {
                                Icon(Icons.Filled.Settings, contentDescription = "Settings")
                            }
                            IconButton(onClick = { viewModel.deleteProfile(profile) }) {
                                Icon(Icons.Filled.Delete, contentDescription = "Delete")
                            }
                        }
                    }
                }
            }
        }
    }

    if (showEditor) {
        ProfileEditorDialog(
            profile = editing,
            onSave = {
                if (editing == null) viewModel.addProfile(it) else viewModel.updateProfile(it)
                showEditor = false
                editing = null
            },
            onDismiss = { showEditor = false; editing = null }
        )
    }
}

@Composable
private fun ProfileEditorDialog(
    profile: ProfileEntity?,
    onSave: (ProfileEntity) -> Unit,
    onDismiss: () -> Unit
) {
    var name by remember { mutableStateOf(profile?.name ?: "Default") }
    var debugTiming by remember { mutableStateOf(profile?.debugTiming ?: false) }
    var socksHost by remember { mutableStateOf(profile?.socksHost ?: "127.0.0.1") }
    var socksPort by remember { mutableStateOf((profile?.socksPort ?: 1080).toString()) }
    var googleHost by remember { mutableStateOf(profile?.googleHost ?: "216.239.38.120") }
    var sniCsv by remember { mutableStateOf((profile?.sniJson ?: "[\"www.google.com\",\"mail.google.com\",\"accounts.google.com\"]").replace("[", "").replace("]", "").replace("\"", "")) }
    var scriptKeysText by remember { mutableStateOf(profile?.scriptKeysText ?: "REPLACE_WITH_DEPLOYMENT_ID\nOPTIONAL_SECOND_DEPLOYMENT_ID") }
    var tunnelKey by remember { mutableStateOf(profile?.tunnelKey ?: "REPLACE_WITH_OUTPUT_OF_scripts_gen-key.sh") }

    AlertDialog(
        onDismissRequest = onDismiss,
        confirmButton = {
            Button(onClick = {
                val sniJson = sniCsv.split(",").map { it.trim() }.filter { it.isNotBlank() }
                    .joinToString(prefix = "[\"", postfix = "\"]", separator = "\",\"")
                onSave(
                    ProfileEntity(
                        id = profile?.id ?: 0,
                        name = name,
                        debugTiming = debugTiming,
                        socksHost = socksHost,
                        socksPort = socksPort.toIntOrNull()?.coerceIn(1, 65535) ?: 1080,
                        googleHost = googleHost,
                        sniJson = sniJson,
                        scriptKeysText = scriptKeysText,
                        tunnelKey = tunnelKey,
                        isSelected = profile?.isSelected ?: false,
                        createdAt = profile?.createdAt ?: System.currentTimeMillis()
                    )
                )
            }) { Text("Save") }
        },
        dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
        title = { Text(if (profile == null) "New Profile" else "Edit Profile") },
        text = {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(value = name, onValueChange = { name = it }, label = { Text("Profile Name") })
                Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                    Text("Debug Timing")
                    Switch(checked = debugTiming, onCheckedChange = { debugTiming = it })
                }
                OutlinedTextField(value = socksHost, onValueChange = { socksHost = it }, label = { Text("socks_host") })
                OutlinedTextField(value = socksPort, onValueChange = { socksPort = it.filter(Char::isDigit) }, label = { Text("socks_port") })
                OutlinedTextField(value = googleHost, onValueChange = { googleHost = it }, label = { Text("google_host") })
                OutlinedTextField(value = sniCsv, onValueChange = { sniCsv = it }, label = { Text("sni (comma separated)") })
                OutlinedTextField(value = scriptKeysText, onValueChange = { scriptKeysText = it }, label = { Text("script_keys (one per line)") }, minLines = 3)
                OutlinedTextField(value = tunnelKey, onValueChange = { tunnelKey = it }, label = { Text("tunnel_key") })
                Spacer(Modifier.height(2.dp))
            }
        }
    )
}
