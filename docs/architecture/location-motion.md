# Location and motion sensors

`core.Scope` owns location requests and motion streams (#56). Apple uses Core Location and Core Motion through purego; Android uses the framework's LocationManager and SensorManager, without Google Play services. The approved contract is fresh one-shot fixes by default, explicit cached age, opt-in background location, raw and fused motion, separate capabilities and latest-value delivery with drop counts (decided by the user, 2026-11-15). NSH v0.5.1 has no corresponding sensor service; NSGo follows its scoped service and capability boundaries with its own protocol and tests.

## API and ownership

- The permissions are `core.PermissionLocation`, `PermissionBackgroundLocation` and `PermissionMotion` ([permissions](services.md#permissions)), so the service adds no permission API of its own: `PermissionStatus` reads one without prompting and `RequestPermission` asks for it. Core carries both as the sensor service's `ItemSensor` read and request, which the backends answer from the providers' own authorization, and ends a request with the scope that made it; only `OpenPermissionSettings` goes out as `ItemPermission`. Restricted answers `PermissionDenied`.
- `Locate(options, done)` waits for a new fix unless `MaxAge` explicitly allows a cached one, then stops. Its foreground timeout defaults to 30 seconds. `Stop` cancels silently.
- `WatchLocation(options, sample, failed)` and `WatchMotion(options, sample, failed)` return streams owned by their requesting scope. Disposal stops providers and suppresses pending callbacks, including permission answers.
- Callbacks run on the engine goroutine. Failures are stable `SensorError` codes: `not-allowed`, `unavailable`, `not-configured`, `timeout` or `failed`; none contains coordinates.
- TSX's `Sensors` exposes `permission` and `requestPermission`, which resolve with the permission's status name, `locate`, `watchLocation` and `watchMotion`; subscriptions return a stop function. TSX durations and timestamps use milliseconds.

Foreground streams keep running while inactive and pause in background. A paused one-shot waits with its remaining timeout. Resuming increments its generation, rejects queued samples from before the pause and marks the next delivered sample `Gap`. Explicit background location keeps running; motion always pauses. No service restores a killed process.

## Providers

| Target | Location | Motion |
| --- | --- | --- |
| iOS, watchOS | Core Location fixes and streams | Available Core Motion raw sensors and fused device motion |
| macOS | Core Location fixes and streams | Unavailable |
| tvOS | One-shot `requestLocation`; streams unavailable | Unavailable |
| Android | API 31+ framework fused provider when enabled; otherwise GPS with precise permission or network, with coarse accuracy preferring network; API 30+ `getCurrentLocation`, older `requestSingleUpdate` | SensorManager accelerometer, gyroscope, magnetometer and rotation vector with gravity, linear acceleration and gyro readings |
| Web | Unavailable | `DeviceMotionEvent` and `DeviceOrientationEvent` in secure mobile browser contexts, with acceleration, rotation and fused readings where supplied |
| Windows, Linux, HarmonyOS, mini programs | Unavailable | Unavailable |

`LocationOptions` chooses desired accuracy in meters, minimum distance, interval, cache age, timeout and background policy. Providers may give a worse accuracy or slower interval; samples carry actual accuracy and motion intervals. Apple does not offer a minimum-time location filter. The API neither geocodes nor infers place names. Missing sensors fail explicitly; fused motion requires its companion sensors on Android. Android serves an app with approximate location alone a coarse fix it refreshes at most every ten minutes: `getCurrentLocation` answers it nothing, so a `Locate` without `MaxAge` fails there, while one with `MaxAge` gets the coarse fix as `Cached` and `Approximate` (Pixel 3 XL, Android 12, 2026-11-16), and a stream's fixes come as seldom.

## Location values

`LocationFix` uses WGS 84 coordinates, meters, m/s and course in degrees. `SensorNumber.Valid` distinguishes a missing field from zero. `Altitude` is ellipsoidal; `MSLAltitude` is above mean sea level, available from Apple's altitude and Android API 34's MSL altitude where supplied. Older Apple versions without ellipsoidal altitude leave it absent.

`Approximate` says the user allowed only an approximate location as the fix came: Apple's reduced accuracy, or Android's coarse location without the fine one; it is how the app learns the precise or approximate answer.

`Timestamp` is provider wall time. Android additionally supplies monotonic time since boot with `HasMonotonic`; Apple supplies none. Cache decisions use monotonic time on Android and provider wall time on Apple. `Cached` identifies a fix predating the request and `Age` reports its delivery age. A future cached monotonic timestamp is rejected. Updates preserve provider ordering except samples explicitly replaced under back-pressure.

## Motion values

`MotionOptions.Sensors` is a set of `SensorAccelerometer`, `SensorGyroscope`, `SensorMagnetometer` and `SensorDeviceMotion`. Zero requests accelerometer. The default interval is 20 ms, clamped to 10 ms–1 s and then to hardware limits. Each `MotionSample` identifies its sensor, monotonic timestamp, interval, accuracy where known, gap and drop count.

Vectors use natural, right-handed device axes: x toward screen right, y toward top, z out of the face, without an orientation-dependent transform. Acceleration uses m/s² with Android's sign; face-up at rest is +9.80665 on z. Core Motion acceleration, gravity and user acceleration multiply by -9.80665. Rotation is rad/s and magnetic field µT. `Vector` belongs to raw readings; fused samples use `Gravity`, `UserAcceleration`, `RotationRate` and `Attitude`.

Attitude rotates device axes into east-north-up world axes when `HeadingReferenced` is true. Apple prefers magnetic-north reference and rotates its north-west-up frame a quarter turn about z; otherwise its arbitrary horizontal frame reports no heading reference. Android uses its rotation vector's world frame. The contract promises neither identical noise nor synchronized companion readings.

The browser uses the page's monotonic event clock. It converts rotation rates from degrees per second, normalizes WebKit's iOS acceleration sign and combines the latest orientation with motion readings when all fused fields are available. The last stream ending detaches its listeners.

## Back-pressure

A `driver.SensorPacket` holds the latest copied sample per stream/sensor under a mutex and queues at most one engine wake-up. Replacement accumulates `Dropped` since the preceding delivery. Permission and terminal reports never replace samples. Android also keeps only the latest sample per stream/sensor before writing the pipe, adding those drops to the engine's count. Providers may independently sample slower than requested; the count measures replaced deliveries rather than estimating unseen hardware samples.

## Permissions and packaging

`AccessLocation`, `AccessBackgroundLocation` and `AccessMotion` are separate capabilities, mirrored by TSX package grants `location`, `backgroundLocation` and `motion`; they hold the sensors' permissions too, in place of `AccessPermissions`, so a package granted location may read and ask for it. Background sampling also needs foreground location access. Starting a stream never requests a grant implicitly: one without the permission fails with `not-allowed`.

On Safari call `RequestPermission(PermissionMotion, done)` from a press handler. Other mobile browsers apply their motion site setting. The web reports unavailable on desktops and for magnetometer streams.

Apple declarations live in `apple.info`: `NSLocationWhenInUseUsageDescription`, macOS's `NSLocationUsageDescription` where needed, and `NSMotionUsageDescription`. Hardened-runtime and store signing add the location entitlement only with a nonempty location usage description. Arrays and dictionaries encode as native plist values, including `UIBackgroundModes: [location]` and temporary-accuracy purpose dictionaries; requesting temporary full accuracy is a later API. Background updates need the declared mode and a manager supporting background delivery; When In Use may continue with the native indicator. This service provides no region-monitoring relaunch guarantee.

Android declares `ACCESS_COARSE_LOCATION`, optional `ACCESS_FINE_LOCATION`, and for background streams `FOREGROUND_SERVICE` and `FOREGROUND_SERVICE_LOCATION`, the latter of which makes `nsgo build` declare `LocationService` in the manifest, so no other app carries a location foreground service for Google Play to question; without both, `PermissionBackgroundLocation` is unavailable. A location foreground service continues an explicitly started foreground grant. Apps additionally declaring `ACCESS_BACKGROUND_LOCATION` request it separately, after foreground authorization; Android 11+ opens the app's settings only on that explicit request. A background stream requires the app's notification title, with an optional body. Starting its foreground service can still be refused by the system's background-start rules.

`LocationService` is non-exported and non-sticky. It owns the visible notification while background streams exist. A standalone Activity destroyed during tracking keeps its engine's one view off screen, as it does for overlays over other apps (`NsgoView.keepOffScreen`), and the next Activity shows that same view again; the last stopped stream, or the last closed overlay, ends a process nothing else keeps. Embedded hosts keep ownership of their engines. The prebuilt runtime's dex and jar ship with the Java sources and `runtime/build.sh` that rebuilds them.

## Verification and remaining acceptance

Core tests cover separate grants, scope cancellation, one-shot completion, inactive/background transitions, generation rejection, resume gaps, timeout pause and rate validation. Driver tests exercise concurrent latest-value delivery and drop accounting; Android tests cover wire values, per-sensor coalescing and JVM provider/cache/rate fixtures. Apple tests use constructed CLLocation fixtures, never live positions, and check struct returns, cache rejection, missing values, acceleration units and frame conversion. TSX and the gallery keep the permissions in `nsgotest`'s `Permissions` and `PermissionAnswers`, as every permission's, with `LocationCache`, `FeedLocation` and `FeedMotion`.

Device acceptance remains necessary for native authorization changes, reduced accuracy, service notifications, Activity destruction/reopening, suspended Apple delivery and sensor calibration/rates on actual hardware. Tests must use injected simulator locations or recorded fixtures, never a user's live location. Other providers, geofencing, steps, barometer, activity recognition and bounded background motion remain in the roadmap.
