// The nsgo/audio module: short sounds decoded once and played many times, overlapping, gated by the "audio" package capability ("network" too for an http(s) source).

/** Why a sound failed to load, as an Error's `reason`. */
export type AudioErrorReason = "not-allowed" | "network" | "decode" | "not-supported" | "quota-exceeded" | "unavailable" | "failed";

/** How one playback of a sound plays. */
export interface Voice {
  /** From 0 (silent) to 1, the default. */
  volume?: number;
  /** From 0.5 to 2; 1 by default. */
  rate?: number;
  /** From -1 (full left) to 1 (full right); 0, centered, by default. */
  pan?: number;
  /** Repeats the playback until it or its sound stops, or the sound closes. */
  loop?: boolean;
  /** Starts the playback silent; `setVolume` can raise it. */
  muted?: boolean;
}

/** One play of a sound, changed or stopped alone. */
export interface Playback {
  /** Whether it played to its end, was stopped, was replaced at the playback limit or could not start. Reading it tracks it. */
  readonly ended: boolean;
  setVolume(volume: number): void;
  setRate(rate: number): void;
  stop(): void;
}

/** A sound decoded into memory. */
export interface Sound {
  /** How long it plays, in milliseconds. */
  readonly duration: number;
  play(voice?: Voice): Playback;
  /** Stops every playback of the sound. */
  stop(): void;
  /** Lets the sound go; the view's sounds go with it. */
  close(): void;
}

/** Reports whether the platform plays audio and the package is granted the "audio" capability. */
export declare function supported(): boolean;

/**
 * Loads a sound from an "asset:" path, a data: URL or an http(s) URL. A package's sounds keep the default limits: 64 sounds, 16 MiB decoded, 8 MiB encoded and 5 seconds each, and 32 overlapping playbacks. Rejects with an Error whose `reason` is an AudioErrorReason, and whose `code` is "ERR_ACCESS_DENIED" without the capability and "ERR_NSH_QUOTA_EXCEEDED" past a limit.
 */
export declare function loadSound(uri: string): Promise<Sound>;
