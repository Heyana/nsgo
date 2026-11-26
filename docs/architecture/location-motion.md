# Location and motion sensors

`core.Scope` owns location requests and motion streams (#56). Apple uses Core Location and Core Motion through purego; Android uses the framework's LocationManager and SensorManager, without Google Play services. The approved contract is fresh one-shot fixes by default, explicit cached age, opt-in background location, raw and fused motion, separate capabilities and latest-value delivery with drop counts (decided by the user, 2026-11-15). NSH v0.6.1 has no corresponding sensor service; NSGo follows its scoped service and capability boundaries with its own protocol and tests.

## API and ownership

- The permissions are `core.PermissionLocation`, `PermissionBackgroundLocation`, `PermissionMotion` and `PermissionActivityRecognition` ([permissions](services.md#permissions)), so the service adds no permission API of its own: `PermissionStatus` reads one without prompting and `RequestPermission` asks for it. Core carries both as the sensor service's `ItemSensor` read and request, which the backends answer from the providers' own authorization, and ends a request with the scope that made it; only `OpenPermissionSettings` goes out as `ItemPermission`. Restricted answers `PermissionDenied`.
- `Locate(options, done)` waits for a new fix unless `MaxAge` explicitly allows a cached one, then stops. Its foreground timeout defaults to 30 seconds. `Stop` cancels silently.
- `WatchLocation(options, sample, failed)` and `WatchMotion(options, sample, failed)` return streams owned by their requesting scope. Disposal stops providers and suppresses pending callbacks, including permission answers. The explicitly passed scope owns them even when called inside another scope’s handler.
- Callbacks run on the engine goroutine. Failures are stable `SensorError` codes: `not-allowed`, `unavailable`, `not-configured`, `timeout` or `failed`; none contains coordinates.
- TSX's `Sensors` exposes `permission` and `requestPermission`, which resolve with the permission's status name, `locate`, `watchLocation` and `watchMotion`; subscriptions return a stop function. TSX durations and timestamps use milliseconds.

Foreground streams keep running while inactive and pause in background. A paused one-shot waits with its remaining timeout. Resuming increments its generation, rejects queued samples from before the pause and marks the next delivered sample `Gap`. Explicit background location and bounded background motion keep running until stopped or expired. No service restores a killed process.

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

Apple declarations live in `apple.info`: `NSLocationWhenInUseUsageDescription`, macOS's `NSLocationUsageDescription` where needed, and `NSMotionUsageDescription`. Hardened-runtime and store signing add the location entitlement only with a nonempty location usage description. Arrays and dictionaries encode as native plist values, including `UIBackgroundModes: [location]` and temporary-accuracy purpose dictionaries; `RequestPreciseLocation` explicitly requests temporary full accuracy using a declared purpose key. Background updates need the declared mode and a manager supporting background delivery; When In Use may continue with the native indicator. This service provides no region-monitoring relaunch guarantee.

Android declares `ACCESS_COARSE_LOCATION`, optional `ACCESS_FINE_LOCATION`, and for background streams `FOREGROUND_SERVICE` and `FOREGROUND_SERVICE_LOCATION`, the latter of which makes `nsgo build` declare `LocationService` in the manifest, so no other app carries a location foreground service for Google Play to question; without both, `PermissionBackgroundLocation` is unavailable. A location foreground service continues an explicitly started foreground grant. Apps additionally declaring `ACCESS_BACKGROUND_LOCATION` request it separately, after foreground authorization; Android 11+ opens the app's settings only on that explicit request. A background stream requires the app's notification title, with an optional body. Starting its foreground service can still be refused by the system's background-start rules.

`LocationService` is non-exported and non-sticky. It owns the visible notification while background streams exist. A standalone Activity destroyed during tracking keeps its engine's one view off screen, as it does for overlays over other apps (`NsgoView.keepOffScreen`), and the next Activity shows that same view again; the last stopped stream, or the last closed overlay, ends a process nothing else keeps. Embedded hosts keep ownership of their engines. The prebuilt runtime's dex and jar ship with the Java sources and `runtime/build.sh` that rebuilds them.

## Verification and remaining acceptance

Core tests cover separate grants, scope cancellation, one-shot completion, inactive/background transitions, generation rejection, resume gaps, timeout pause and rate validation. Driver tests exercise concurrent latest-value delivery and drop accounting; Android tests cover wire values, per-sensor coalescing and JVM provider/cache/rate fixtures. Apple tests use constructed CLLocation fixtures, never live positions, and check struct returns, cache rejection, missing values, acceleration units and frame conversion. TSX and the gallery keep the permissions in `nsgotest`'s `Permissions` and `PermissionAnswers`, as every permission's, with `LocationCache`, `FeedLocation` and `FeedMotion`.

Sensor settings navigation (2026-11-23) ran through `OpenPermissionSettings` for all three sensor keys: on macOS, scoped accessibility inspection confirmed Location Services for foreground/background location and Motion & Fitness for motion; on an isolated Android API 35 emulator, the resumed Settings activity named the owned test package and its App info page showed that app with no permissions requested. The Android runtime was rebuilt from the Java under test. `internal/testhost/testdata/sensorsettings` keeps the opt-in fixture and observation steps. No permission was requested or changed.

`internal/testhost/testdata/phonesensors` runs the native providers against an injected location on an iOS simulator and an Android emulator: a granted fix and stream, a denied fix and the accelerometer, which simulators answer unavailable (2026-11-26).

Device acceptance remains necessary for native authorization changes, reduced accuracy, service notifications, Activity destruction/reopening, suspended Apple delivery and sensor calibration/rates on actual hardware. Tests must use injected simulator locations or recorded fixtures, never a user's live location. Constructed Objective-C providers exercise native steps/activity/pressure blocks, region ownership, precise-authorization completion and early/finite execution-lease cleanup. An owned Android API 35 emulator with injected pressure runs real SensorManager samples and a three-second shortService, observed still active after Home and removed at expiry. These fixtures do not prove physical fitness accuracy, real region wake-up or suspended Apple execution; those remain in the roadmap.

## Scoped fitness and region services

`WatchSteps`, `WatchActivity` and `WatchPressure` belong to `AccessMotion`. Steps and activity use `PermissionActivityRecognition` (`activity-recognition` in the driver, `activityRecognition` in TSX); raw `PermissionMotion` remains separate on Android. Apple pressure also requires the fitness grant. Starting any stream never asks for permission. Explicit `RequestPermission(PermissionActivityRecognition, ...)` starts Apple's pedometer authorization operation or requests Android's declared `ACTIVITY_RECOGNITION`.

| Service | Apple phones | Android framework | Other targets |
| --- | --- | --- | --- |
| Steps | `CMPedometer` live window | `TYPE_STEP_COUNTER`, baseline from first event | Unavailable |
| Activity | `CMMotionActivityManager` bitset and confidence | Unavailable: no framework activity classifier | Unavailable |
| Pressure | `CMAltimeter`, kPa converted to Pa; relative meters | `TYPE_PRESSURE`, hPa converted to Pa; no inferred altitude | Unavailable |
| Geofences | `CLCircularRegion` delegate states | `LocationManager.addProximityAlert` explicit receiver | macOS region monitoring where supported; otherwise unavailable |
| Precise authorization | Temporary full accuracy with declared purpose key | Explicit coarse/fine permission upgrade | Unavailable |
| Bounded background motion | Cancellable UIKit execution lease | Explicit `shortService`, at most three minutes | Unavailable |

`StepSample.Count` belongs to its `Since` observation window. Pausing starts a new window with `Gap`; Android counter resets also start a new window. Distances and floors are optional, with valid zero distinguished from absence. Activity may have several flags at once. Pressure uses pascals, a monotonic timestamp and actual delivery interval; relative altitude is optional and never calculated from pressure.

`WatchGeofences` copies one to twenty circles, validates distinct nonempty IDs and finite WGS 84 coordinates/radii, and owns unique native registrations. Deliveries coalesce per region, so one busy region cannot hide another. Foreground watches pause; explicit background watches require background access and platform declarations, Apple's Always authorization, and Android's foreground location notification. Stop removes only this watch's regions. No Go subscription survives process death: Apple removes stale runtime-prefixed registrations on the next watch, and Android's non-exported receiver cancels stale PendingIntents without starting an engine. This is scoped region monitoring, with no persistent `CLMonitor` restoration contract.

`RequestPreciseLocation(purpose, done)` returns only an authorization boolean, never a position. Apple validates a nonempty string in `NSLocationTemporaryUsageDescriptionDictionary` before calling the native API. Android needs both coarse and fine declarations and asks explicitly; false can retain an approximate grant.

`WatchBackgroundMotion` requires both `AccessMotion` and `AccessBackgroundMotion`, a positive duration and the usual motion grant. Duration or early OS expiration reports `SensorTimedOut`. iOS uses `beginBackgroundTaskWithName:expirationHandler:`; this is an expiring live stream, not `CMSensorRecorder`, whose recording cannot be cancelled by disposing a scope. Android apps explicitly set `android.background_motion: true`, declare `FOREGROUND_SERVICE` and supply a notification title. The nonsticky short service has an independent three-minute cap that additional watches cannot extend. No stream records after process termination. Apple Watch has no UIKit lease; unsupported providers report unavailable.

TSX mirrors all six methods, using milliseconds and a separate signed-package `backgroundMotion` capability. `nsgotest` adds `FeedSteps`, `FeedActivity`, `FeedPressure`, `FeedGeofence` and `PreciseLocationAnswer`. Android advertises `CAP_EXTENDED_SENSORS` in the environment's fourth capability byte, written after the opened URL; Go rejects advanced requests locally against an older runtime, preventing fitness authorization from being mistaken for location authorization. The old wire layout stays unchanged.
