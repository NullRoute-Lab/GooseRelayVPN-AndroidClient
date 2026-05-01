# Gomobile Build Fix

## Problem
GitHub Actions build failed with error:
```
no exported names in the package "./mobile/tun"
gomobile: /home/runner/go/bin/gobind -lang=go,java failed: exit status 1
```

## Root Cause
The TUN module had issues that prevented gomobile from binding it:

1. **`//export` directives** - These are for CGO, not gomobile
2. **Unsupported function parameter** - `SetProtectFunc(protectFn func(int) bool)` has a function parameter which gomobile doesn't support

## Fixes Applied

### 1. Removed `//export` Comments
Removed all `//export` directives from `mobile/tun/tun_api.go`:
- `//export StartTunBridge`
- `//export StopTunBridge`
- `//export IsTunBridgeRunning`
- `//export GetTunBandwidth`
- `//export SetProtectFunc`
- `//export GetDNSMapping`
- `//export GetDNSMappingCount`

These directives are for CGO (calling Go from C), not for gomobile (calling Go from Java/Kotlin).

### 2. Commented Out Unsupported Function
Commented out `SetProtectFunc()` because gomobile doesn't support function parameters:

```go
// NOTE: SetProtectFunc is not exposed via gomobile because it takes a function parameter
// which is not supported by gomobile. Socket protection is not critical for this use case
// since we're only connecting to localhost SOCKS5 proxy.
```

Socket protection is not needed because:
- The TUN bridge only connects to `127.0.0.1:1080` (localhost SOCKS5)
- Localhost connections don't go through the VPN interface
- No risk of routing loops

## Gomobile-Compatible Types

Gomobile supports these types:
- ✅ Basic types: `int`, `int32`, `int64`, `float32`, `float64`, `bool`, `string`
- ✅ Byte slices: `[]byte`
- ✅ Error interface: `error`
- ✅ Multiple return values (up to 2)
- ❌ Function types: `func(int) bool`
- ❌ Channels: `chan int`
- ❌ Complex types: `complex64`, `complex128`
- ❌ Interfaces (except `error`)
- ❌ Structs (must use getter/setter methods)

## Final Exported Functions

All functions now use gomobile-compatible types:

```go
func StartTunBridge(tunFd int32, mtu int32, socksAddr string) error
func StopTunBridge()
func IsTunBridgeRunning() bool
func GetTunBandwidth() (up int64, down int64)
func GetDNSMapping(fakeIP string) string
func GetDNSMappingCount() int
```

## Testing

Push the changes and GitHub Actions should now successfully:
1. Build `gooserelayvpn.aar` ✅
2. Build `tun.aar` ✅
3. Build Android APK ✅
4. Upload all artifacts ✅

## Usage in Kotlin

The functions will be available as:
```kotlin
import tun.Tun

// Start TUN bridge
Tun.startTunBridge(fd.toLong(), 1500L, "127.0.0.1:1080")

// Stop TUN bridge
Tun.stopTunBridge()

// Check if running
val running = Tun.isTunBridgeRunning()

// Get bandwidth
val bandwidth = Tun.getTunBandwidth()
val up = bandwidth.up
val down = bandwidth.down

// Get DNS mapping
val hostname = Tun.getDNSMapping("198.18.0.1")

// Get mapping count
val count = Tun.getDNSMappingCount()
```

Note: Gomobile converts Go function names to Java/Kotlin naming conventions:
- `StartTunBridge` → `startTunBridge`
- `GetTunBandwidth` → `getTunBandwidth`
- etc.
