// The nsgo/gamepad module: connected game controllers and per-frame button and
// axis readings in the W3C Standard Gamepad layout, gated by the "gamepad"
// package capability.

export interface Pad {
  /** Stable identifier of the connected controller. */
  id: number;
  /** Human-readable controller name reported by the system. */
  name: string;
  /** Button values in the W3C Standard Gamepad layout (0..16), each in [0, 1]. */
  buttons: number[];
  /** Analog stick axes in the W3C Standard Gamepad layout (0..3), each in [-1, 1]. */
  axes: number[];
}

/** W3C Standard Gamepad button indices. */
export declare const Button: {
  readonly South: 0;
  readonly A: 0;
  readonly East: 1;
  readonly B: 1;
  readonly West: 2;
  readonly X: 2;
  readonly North: 3;
  readonly Y: 3;
  readonly LB: 4;
  readonly RB: 5;
  readonly LT: 6;
  readonly RT: 7;
  readonly Back: 8;
  readonly Select: 8;
  readonly Start: 9;
  readonly LeftStick: 10;
  readonly RightStick: 11;
  readonly DPadUp: 12;
  readonly DPadDown: 13;
  readonly DPadLeft: 14;
  readonly DPadRight: 15;
  readonly Home: 16;
  readonly Count: 17;
};

/** W3C Standard Gamepad analog stick axis indices. */
export declare const Axis: {
  readonly LeftX: 0;
  readonly LeftY: 1;
  readonly RightX: 2;
  readonly RightY: 3;
  readonly Count: 4;
};

export type ButtonIndex = (typeof Button)[Exclude<keyof typeof Button, "Count">];
export type AxisIndex = (typeof Axis)[Exclude<keyof typeof Axis, "Count">];

/**
 * Reports whether the current platform supports gamepads and the package is
 * granted the "gamepad" capability.
 */
export declare function supported(): boolean;

/**
 * Returns the currently connected game controllers, sorted by id.
 * Tracks controller connect/disconnect changes reactively without rerendering
 * on button or axis updates. Throws ERR_ACCESS_DENIED without the "gamepad"
 * capability.
 */
export declare function pads(): Pad[];

/**
 * Returns one controller's buttons and axes as of the current animation frame,
 * or null if no controller with that id is connected. Throws ERR_ACCESS_DENIED
 * without the "gamepad" capability.
 */
export declare function state(id: number): Pad | null;
