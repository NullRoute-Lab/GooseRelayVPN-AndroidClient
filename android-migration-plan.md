# GooseRelayVPN Android Migration Plan (Implemented)

## What changed
- Copied `masterdns/android` into `GooseRelayVPN/android`.
- Renamed Android namespace from `com.masterdns.vpn` to `com.gooserelay.gooserelayvpn`.
- Switched profile schema from MasterDNS DNS/resolver fields to GooseRelay JSON fields.
- Replaced TOML-oriented config generation with GooseRelay `client_config.json` generation.
- Reworked Profile and Profile Settings flows around `debug_timing`, `socks_host`, `socks_port`, `google_host`, `sni`, `script_keys`, `tunnel_key`.
- Replaced import/export format from TOML to JSON.
- Removed DNS/resolver-specific generation logic from VPN startup config path.
- Updated app branding text and moved launcher foreground asset to `logo/logo.png`.

## Profile defaults
```json
{
  "debug_timing": false,
  "socks_host": "127.0.0.1",
  "socks_port": 1080,
  "google_host": "216.239.38.120",
  "sni": ["www.google.com", "mail.google.com", "accounts.google.com"],
  "script_keys": [
    "REPLACE_WITH_DEPLOYMENT_ID",
    "OPTIONAL_SECOND_DEPLOYMENT_ID"
  ],
  "tunnel_key": "REPLACE_WITH_OUTPUT_OF_scripts_gen-key.sh"
}
```

## Folder organization for future core updates
- Android project remains isolated under `GooseRelayVPN/android`.
- Core config handoff is centralized in `util/ConfigGenerator.kt`.
- VPN startup/config write path is centralized in `service/MasterDnsVpnService.kt`.
- Profile persistence is centralized in `data/local/ProfileEntity.kt`.

## Important notes
- Room DB was reset via version bump + destructive migration fallback to match the new profile schema.
- UI layout style remains Compose-based and consistent with the existing app structure, with major field changes focused on Profile/Profile Settings.
