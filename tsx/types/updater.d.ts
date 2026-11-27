// The nsgo/updater module: checking for app updates, reading updater
// capability and status, and persisting automatic-update preferences on the
// same platform updaters Go's updater package uses.

/** Who updates this installed copy, without contacting an update feed. */
export type UpdaterManaged = "unavailable" | "self" | "page" | "development" | "channel" | "store";

/** What an in-app update is doing on Linux and Android. */
export type UpdatePhase = "idle" | "downloading" | "installing";

/** How this copy updates. `store` names its installer when `managed` is `"store"`. */
export interface UpdaterCapability {
  managed: UpdaterManaged;
  store: string | null;
}

/**
 * The updater's latest knowledge. Reading `status()` inside a reactive
 * expression or effect tracks changes. `automaticDownloads` is supported only
 * by Sparkle on macOS. `phase`, `loaded`, `total` and `fraction` follow an
 * in-app update that downloads and installs on Linux and Android.
 */
export interface UpdaterState {
  capability: UpdaterCapability;
  automaticChecks: boolean;
  automaticDownloads: boolean;
  /** The newer version reported by the platform updater, or null when none is known. */
  available: string | null;
  /** Release notes for `available`, or null when none were reported. */
  notes: string | null;
  /** Time of the last check in milliseconds since the Unix epoch, or null if none. */
  lastCheck: number | null;
  phase: UpdatePhase;
  loaded: number;
  total: number;
  /** Download progress from 0 to 1, or 0 when `total` is unknown or no bytes have arrived yet. */
  fraction: number;
}

/**
 * Checks for a newer version now, as Go's `updater.Check` and a
 * "Check for Updates…" menu item do, and shows what it finds using the
 * platform's updater. Does nothing in Steam builds.
 */
export declare function checkForUpdate(): void;

/**
 * Checks quietly without showing anything unless a newer version is found, as
 * Go's `updater.CheckInBackground` does. Does nothing in Steam builds.
 */
export declare function checkInBackground(): void;

/**
 * Checks in the background at most once a day when `status().automaticChecks`
 * is true (retrying failed checks after an hour), as Go's `updater.CheckDaily`
 * does. Does nothing in Steam builds.
 */
export declare function checkDaily(): void;

/**
 * Synchronously reports how this copy updates without a network request, as
 * Go's `updater.CapabilityOf` does.
 */
export declare function capability(): UpdaterCapability;

/**
 * Returns tracked updater state and preferences without checking for updates,
 * as Go's `updater.Status` does. Calling it inside a reactive expression or
 * effect subscribes to changes.
 */
export declare function status(): UpdaterState;

/**
 * Persists whether automatic update checks are enabled, as Go's
 * `updater.SetAutomaticChecks` does. Native updaters own their preferences;
 * Linux, Android and iOS store the choice in a private storage partition.
 */
export declare function setAutomaticChecks(on: boolean): void;

/**
 * Persists Sparkle's automatic-downloads setting on macOS, as Go's
 * `updater.SetAutomaticDownloads` does. Does nothing on other platforms.
 */
export declare function setAutomaticDownloads(on: boolean): void;
