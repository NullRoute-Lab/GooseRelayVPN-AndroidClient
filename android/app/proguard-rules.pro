# ProGuard rules for GooseRelayVPN Android

# Keep Go mobile bindings
-keep class mobile.** { *; }
-keep class go.** { *; }

# Keep Hilt
-keep class dagger.hilt.** { *; }

# Keep Room entities
-keep class com.gooserelay.gooserelayvpn.data.local.** { *; }
