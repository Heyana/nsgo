import type { JSX } from "./types";

/** The document metadata while this component is visible. Later Heads
 * override the fields they set; hiding or removing one restores earlier
 * values. Native targets and embedded apps leave their host alone. */
export interface HeadProps {
  title?: string;
  description?: string;
}
export declare function Head(props: HeadProps): JSX.Element;
