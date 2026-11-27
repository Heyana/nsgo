import { webview } from '@kit.ArkWeb';
import { image } from '@kit.ImageKit';
import { util } from '@kit.ArkTS';
import { i18n, resourceManager } from '@kit.LocalizationKit';
import { deviceInfo, pasteboard, settings } from '@kit.BasicServicesKit';
import { photoAccessHelper } from '@kit.MediaLibraryKit';
import { camera, cameraPicker } from '@kit.CameraKit';
import { fileIo as fs, fileUri, picker } from '@kit.CoreFileKit';
import { accessibility } from '@kit.AccessibilityKit';
import { formProvider, formBindingData } from '@kit.FormKit';
import { display, window, typeNode, BuilderNode, NodeController, FrameNode, UIContext, FrameCallback, componentSnapshot, LengthMetricsUnit } from '@kit.ArkUI';
import { displaySync, text } from '@kit.ArkGraphics2D';
import { common, ConfigurationConstant, abilityAccessCtrl, bundleManager } from '@kit.AbilityKit';
import type { Permissions, PermissionRequestResult } from '@kit.AbilityKit';
import { userAuth } from '@kit.UserAuthenticationKit';
import MeasureText, { MeasureOptions } from '@ohos.measure';
import { SandboxFiles } from './SandboxFiles';
import { SpanBackgroundPaint, SpanRectangle, backgroundBand, spanLineRange, joinSpanBoxes } from './SpanBackground';
import {
  WebHeaderPair,
  WebConfigOp,
  WebLoadOp,
  parseWebConfigOp,
  parseWebLoadOp,
  resolveWebUserAgent,
  webHooksKey,
  webReloadKey,
  webDocumentStartScripts,
  webDocumentEndScripts,
  webSourceNeedsFileAccess,
  webNavigable,
  refusedNavigationMessage,
  webMediaWant,
  sanitizeDownloadName,
  normalizeReportedWebURL,
  webShouldReportError,
  WEB_COOKIES_OP_SET,
  WEB_COOKIES_OP_LIST,
  WEB_COOKIES_OP_CLEAR,
  NO_WEB_COOKIE_DOMAINS,
  WebCookieWire,
  parseWebCookiesReq,
  isValidWebCookieURL,
  validateWebCookieSetItems,
  mapWebCookieSameSite,
  formatWebCookieRefused
} from './WebViewConfig';

// The host the harmonyos target builds: the app's engine runs in WebAssembly
// in a 1x1 Web view, and this page draws what it mounts with ArkUI views.
// internal/platform/harmonyos writes the operations, one JSON array a batch.

const ORIGIN: string = 'https://nsgo.local/';
const ROOT: string = 'web/';
// internal/driver's ViewKind.
const KIND_TEXT: number = 2;
const KIND_INPUT: number = 3;
const KIND_SCROLL: number = 4;
const KIND_IMAGE: number = 5;
const KIND_MODAL: number = 6;
const KIND_CANVAS: number = 7;
const KIND_WEB: number = 8;
const KIND_VIDEO: number = 9;
const KIND_NATIVE: number = 10;

// DENSITY is the screen's pixels per vp, which a canvas needs to size an
// image drawn from its pixels. The device reading sets it, and density
// reads the display if a canvas draws first.
let DENSITY: number = 0;

// WidgetData is what the app hands its home-screen card: the kind it
// updated and the text its 2x2 card shows.
interface WidgetData {
  kind: string;
  title: string;
}

// FolderItem is one entry the host lists in a folder the user picked.
interface FolderItem {
  n: string;
  d: boolean;
  s: number;
}

// PickedFolderInfo is a folder the folder picker gave: its name and the
// URI the host reads its files through.
interface PickedFolderInfo {
  n: string;
  u: string;
}

// AlertSpec is a native alert the engine sends: its ID, title, message,
// whether it is a sheet, whether it may be cancelled, and its buttons with
// AlertRole numbers (0 default, 1 cancel, 2 destructive).
interface AlertButton {
  l: string;
  r: number;
}

interface AlertSpec {
  id: number;
  t: string;
  m: string;
  s: boolean;
  c: boolean;
  a: AlertButton[];
}

// PickedImage is one image the photo picker returned: its name, media type
// and bytes, base64-encoded, as the engine's PickedFile.
interface PickedImage {
  n: string;
  m: string;
  d: string;
}

// PickedFileInfo is one file the document picker returned: its name, media
// type, size and the path it was copied to in the app's cache.
interface PickedFileInfo {
  n: string;
  m: string;
  s: number;
  p: string;
}

// edgeEnd is how far into the window an avoid area that starts at start and spans size reaches, or 0 for an empty one.
function edgeEnd(start: number, size: number): number {
  return size > 0 ? start + size : 0;
}

// edgeFromEnd is how far in from the far edge (the right or the bottom) of a window extent long an avoid area that starts at start and spans size reaches, or 0 for an empty one.
function edgeFromEnd(start: number, size: number, extent: number): number {
  return size > 0 ? Math.max(0, extent - start) : 0;
}

// BarRequest is one of the window's system bar requests. It is made only when what it asks for changes, because avoidAreaChange fires when an edge swipe reveals hidden bars for a moment and asking again then would hide them at once. A failed request is made again only when retry is set, which the page does for a batch that carries the engine's status bar, never for an avoid area change, so a call the device keeps refusing cannot re-hide revealed bars or run on every frame. A failure that arrives after a newer request was made is ignored, so it cannot undo that request.
class BarRequest {
  asked: string | undefined = undefined;
  failed: boolean = false;

  make(want: string, retry: boolean, call: () => Promise<void>): void {
    if (want === this.asked && !(retry && this.failed)) {
      return;
    }
    this.asked = want;
    this.failed = false;
    const fail = (e: Error): void => {
      if (this.asked === want) {
        this.failed = true;
      }
      console.error('NSGO harmony: system bars ' + want + ': ' + e);
    };
    try {
      call().catch(fail);
    } catch (e) {
      fail(e as Error);
    }
  }
}

// mimeOf guesses a file's media type from its extension.
function mimeOf(path: string): string {
  const dot = path.lastIndexOf('.');
  const ext = dot >= 0 ? path.slice(dot + 1).toLowerCase() : '';
  switch (ext) {
    case 'txt':
      return 'text/plain';
    case 'json':
      return 'application/json';
    case 'pdf':
      return 'application/pdf';
    case 'png':
      return 'image/png';
    case 'jpg':
    case 'jpeg':
      return 'image/jpeg';
    case 'zip':
      return 'application/zip';
  }
  return 'application/octet-stream';
}

// Launch is what EntryAbility stores from the Want that opened the app:
// nsgo- parameters as the engine's flags, and the link that opened it.
interface Launch {
  args: string[];
  url: string;
}

// Device is what the engine reads as its environment, in deviceJSON's
// field names. Every value is the system's, never a guess of the page's.
interface Device {
  Width: number;
  Height: number;
  SafeTop: number;
  SafeRight: number;
  SafeBottom: number;
  SafeLeft: number;
  Keyboard: number;
  PixelRatio: number;
  FontScale: number;
  Language: string;
  OS: string;
  DeviceModel: string;
  OSMajor: number;
  OSMinor: number;
  Dark: boolean;
  Tablet: boolean;
  RTL: boolean;
  ScreenReader: boolean;
  Lifecycle: number;
  OpenedURL: string;
  // CanvasAtlas tells Go this player draws an atlas from one op (case 41).
  CanvasAtlas: boolean;
  Display: DisplayData;
}

// DisplayData is the display the window is on, in deviceData's Display.
interface DisplayData {
  ID: number;
  Width: number;
  Height: number;
  Main: boolean;
  RefreshRate: number;
  MaxRefreshRate: number;
}

// CanvasOp is one drawing operation the engine sent: internal/driver's
// DrawOp, short keys and omitted zeros.
// TextRun is one styled run of a text with spans.
interface TextRun {
  t: string;
  c?: string;
  s?: number;
  w?: number;
  i?: boolean;
  f?: string;
  b?: SpanBackgroundPaint;
}

// TintedImage is a canvas image's copy multiplied by a tint, with the paint that last drew it.
interface TintedImage {
  pm: image.PixelMap;
  paint: number;
}

interface CanvasOp {
  k: number;
  imgk?: string;
  imgd?: string;
  // pxd is a pixel copy's premultiplied RGBA, base64, pw by ph pixels, with the first op of a drawing that draws it.
  pxd?: string;
  pw?: number;
  ph?: number;
  tint?: number;
  x?: number;
  y?: number;
  x1?: number;
  y1?: number;
  x2?: number;
  y2?: number;
  w?: number;
  h?: number;
  r?: number;
  a?: number;
  a0?: number;
  a1?: number;
  ry?: number;
  ccw?: boolean;
  c?: string;
  cap?: number;
  join?: number;
  eo?: boolean;
  t?: string;
  ff?: string;
  fs?: number;
  fw?: number;
  fi?: boolean;
  g?: CanvasGradientData;
  co?: number;
  d?: number[];
  img?: string;
  n?: boolean;
  sp?: number[];
  v?: number[];
  va?: number[];
  e?: number[];
}

interface CanvasGradientData {
  radial?: boolean;
  x1?: number;
  y1?: number;
  r1?: number;
  x2?: number;
  y2?: number;
  r2?: number;
  s?: CanvasStop[];
}

interface CanvasStop {
  o: number;
  c: string;
}

// TabBarItemSpec, TabBarBadgeSpec and TabBarSpec are the JSON payload of op "tb"
// for a native core.TabBar mounted as KIND_NATIVE.
interface TabBarItemSpec {
  t: string;
  i?: string;
  d?: string;
}

interface TabBarBadgeSpec {
  d?: boolean;
  t?: string;
  a?: string;
}

interface TabBarSpec {
  items?: TabBarItemSpec[];
  badges?: TabBarBadgeSpec[];
  selected?: number;
  edge?: string;
  label?: string;
  tint?: string;
  bg?: string;
  inactive?: string;
  indicator?: string;
  active?: string;
  badgeColor?: string;
  dark?: boolean;
  insetBottom?: number;
  insetLeft?: number;
  insetRight?: number;
}

// fitOf turns the engine's resize mode into ArkUI's fit.
function fitOf(fit: string): ImageFit {
  switch (fit) {
    case 'contain':
      return ImageFit.Contain;
    case 'fill':
      return ImageFit.Fill;
    case 'none':
      return ImageFit.None;
    case 'scale-down':
      return ImageFit.ScaleDown;
  }
  return ImageFit.Cover;
}

// interpolationOf turns the engine's image sampling into ArkUI's interpolation: none for nearest neighbour, else ArkUI's default.
function interpolationOf(nearest: boolean): ImageInterpolation {
  return nearest ? ImageInterpolation.None : ImageInterpolation.Low;
}

class VNode {
  id: number = 0;
  kind: number = 1;
  parent: number = 0;
  x: number = 0;
  y: number = 0;
  w: number = 0;
  h: number = 0;
  // The engine's absolute frame, for presses, and the view a press here
  // goes to: itself when it reports presses, else its nearest ancestor.
  ax: number = 0;
  ay: number = 0;
  hit: number = 0;
  bg: string = '';
  radius: number = 0;
  bw: number = 0;
  bc: string = '';
  op: number = 1;
  hidden: boolean = false;
  clip: boolean = false;
  // needFocus and needBlur ask the renderer to move or take the field's
  // keyboard focus once, as ItemFocus and ItemBlur say.
  needFocus: boolean = false;
  needBlur: boolean = false;
  // selStart and selEnd are the selection ItemSelection asked for, -1 for
  // none: setTextSelection applies it once, after the value.
  selStart: number = -1;
  selEnd: number = -1;
  // testID, pointer and accessibility fields are ItemViewProps': the id a UI
  // test finds the view by, how it takes hits, and what assistive technology says.
  testID: string = '';
  pointer: number = 0;
  label: string = '';
  accDesc: string = '';
  propDisabled: boolean = false;
  accLevel: string = 'auto';
  accGroup: boolean = false;
  // A screen's system header: its title, whether it shows, and a back
  // button to the screen below. Its height and colors are the core's:
  // a large title's header is taller, and a transparent one leaves its
  // content showing.
  isScreen: boolean = false;
  scrTitle: string = '';
  scrHidden: boolean = false;
  scrCanGoBack: boolean = false;
  scrBackTitle: string = '';
  scrHeight: number = 44;
  scrTransparent: boolean = false;
  scrBg: string = '';
  scrTitleColor: string = '';
  scrTintColor: string = '';
  // anim is a modal's appearance: driver.ModalAnimation's slide, fade or
  // none. It is 1 (fade) for every other kind.
  anim: number = 1;
  text: string = '';
  fs: number = 16;
  fw: number = 400;
  italic: boolean = false;
  family: string = '';
  color: string = '#000000';
  phColor: string = '';
  lh: number = 0;
  align: number = 0;
  maxLines: number = 0;
  ph: string = '';
  value: string = '';
  masked: boolean = false;
  multiline: boolean = false;
  readonly: boolean = false;
  disabled: boolean = false;
  // A text field's content box in its frame: where its text goes, inside its padding and border.
  inX: number = 0;
  inY: number = 0;
  inW: number = -1;
  inH: number = -1;
  // An image's URI, its fit, the bytes it embeds, base64, and whether it scales with nearest-neighbour sampling.
  src: string = '';
  fit: string = 'cover';
  data: string = '';
  nearest: boolean = false;
  // pv counts the pixel copies an image view was sent ('ip'); the copy itself is in viewPixels.
  pv: number = 0;
  // A canvas's drawing, as operations to replay.
  drawing: string = '';
  // A native core.TabBar's JSON config from op "tb".
  tabBar: string = '';
  // A web view's configuration, queued commands, and live controller state.
  webSrc: string = '';
  webHTML: string = '';
  webBaseURL: string = '';
  noJS: boolean = false;
  webBefore: string = '';
  webAfter: string = '';
  webUA: string = '';
  webUASuffix: string = '';
  webHeaders: WebHeaderPair[] = [];
  webIncognito: boolean = false;
  webNoScroll: boolean = false;
  webMuted: boolean = false;
  webPinchZoom: boolean = false;
  webDownloadDir: string = '';
  webRestricted: boolean = false;
  webHosts: string[] = [];
  webSchemes: string[] = [];
  webAppSchemes: string[] = [];
  webMedia: number = 0;
  webFileAccess: boolean = false;
  webCmds: WebCmd[] = [];
  webState: WebViewState | undefined = undefined;
  // A video's source, how it plays and where it is, and its controller.
  videoSrc: string = '';
  videoPaused: boolean = false;
  videoMuted: boolean = false;
  videoRepeat: boolean = false;
  videoControls: boolean = false;
  videoFit: string = 'contain';
  videoSeek: number = -1;
  videoPausedShown: boolean = false;
  videoController: VideoController | undefined = undefined;
  // Animated values folded into the view's look.
  tx: number = 0;
  ty: number = 0;
  scx: number = 1;
  scy: number = 1;
  rot: number = 0;
  // A text's styled runs, when it has spans.
  runs: TextRun[] = [];
  press: boolean = false;
  long: boolean = false;
  // A scroll view's refresh: whether it refreshes, whether it is, and how
  // far a pull must go before releasing it refreshes.
  refreshable: boolean = false;
  refreshing: boolean = false;
  refreshAt: number = 64;
  // A scroll view's content size, its offsets as the engine set them, and
  // how it scrolls.
  cw: number = 0;
  ch: number = 0;
  sx: number = 0;
  sy: number = 0;
  skey: string = '';
  // zmin, zmax and zoom are a zoomable scroll view's range and scale;
  // pinch says whether fingers zoom it, and zcx, zcy where an engine
  // zoom asked to keep.
  zmin: number = 1;
  zmax: number = 1;
  zoom: number = 1;
  pinch: boolean = true;
  zcx: number = 0;
  zcy: number = 0;
  // zset says a zoom request or a pinch set the scale, so a later
  // scroll config's starting scale does not reset it.
  zset: boolean = false;
  horizontal: boolean = false;
  hideBar: boolean = false;
  // matchFrame is where the element this view replaces was drawn, so a
  // shared-element transition starts there.
  matchFrame: MatchFrame | undefined = undefined;
  // dirty says a batch changed this view, so a frame writes its attributes
  // again: a batch that names one view must not walk the whole tree's.
  dirty: boolean = true;
  // queued says the batch already listed this view to write.
  queued: boolean = false;
  children: number[] = [];
}

// MatchFrame is a view's frame on the page.
interface MatchFrame {
  x: number;
  y: number;
  w: number;
  h: number;
}

// PinchState is a two-finger pinch in progress on a zoomable scroll
// view: which one, the fingers' first distance and the scale it began
// at.
interface PinchState {
  id: number;
  dist: number;
  scale: number;
}

// StickyCell is one cell of a scroll view that stays at its top, in the
// engine's main-axis offsets: from s it travels with the offset to e.
interface StickyCell {
  c: number;
  s: number;
  e: number;
}

// StickyShift is how far a sticky cell travels from its frame and the
// axis it travels along, kept so a batch that rewrote its position does
// not move it back.
class StickyShift {
  d: number = 0;
  across: boolean = false;
}

class Tree {
  nodes: Map<number, VNode> = new Map();
  root: number = -1;
  // sticky are the cells each scroll view pins as it scrolls.
  sticky: Map<number, StickyCell[]> = new Map();
  // The status bar: a driver.StatusBarStyle, whether it shows, and whether the window is immersive, hiding every system bar.
  sbStyle: number = 1;
  sbHidden: boolean = false;
  sbImmersive: boolean = false;
  // sbSent is set by a batch that carries the status bar, so the page makes a failed system bar request again for it.
  sbSent: boolean = false;
  // orientation is the driver.Orientation mask the app asks for, zero for its usual policy, and orientationSent is set by a batch that carries it.
  orientation: number = 0;
  orientationSent: boolean = false;
  // appMode is the driver.Appearance the app shows in.
  appMode: number = 0;
  // layoutMs and layoutCurve are this batch's layout animation, which a
  // shared-element match runs over.
  layoutMs: number = 300;
  layoutCurve: number = 3;
  // gone collects the ids this batch deletes, whose FrameNodes the
  // renderer drops before it draws, since the engine reuses ids.
  gone: number[] = [];
  // zoomed collects the scroll views this batch zoomed, which the renderer
  // scales once it has drawn.
  zoomed: number[] = [];
  // structural says the batch created, moved or removed views, so the tree
  // is walked; geometry says a frame or content size changed, so the
  // absolute frames are measured again. A batch of only changed views skips
  // both, and the renderer writes just those the batch names.
  structural: boolean = false;
  geometry: boolean = false;
  dirtyIDs: number[] = [];
  // parents collects the views whose children this batch inserted, moved or removed, and rootChanged a new root: a structural batch syncs those parents' children rather than writing every view again.
  parents: number[] = [];
  rootChanged: boolean = false;

  // scrollAt returns the refreshable scroll view of id's subtree that
  // holds the point, the last drawn first, or found when none does.
  // scrollNodeAt returns the scroll view of id's subtree that holds the
  // point, the last drawn first, or found when none does.
  scrollNodeAt(id: number, x: number, y: number, found: number): number {
    const v = this.nodes.get(id);
    if (v === undefined || v.hidden || x < v.ax || y < v.ay || x >= v.ax + v.w || y >= v.ay + v.h) {
      return found;
    }
    let at = v.kind === KIND_SCROLL ? v.id : found;
    for (let i = v.children.length - 1; i >= 0; i--) {
      at = this.scrollNodeAt(v.children[i], x, y, at);
    }
    return at;
  }

  scrollAt(id: number, x: number, y: number, found: number): number {
    const v = this.nodes.get(id);
    if (v === undefined || v.hidden || x < v.ax || y < v.ay || x >= v.ax + v.w || y >= v.ay + v.h) {
      return found;
    }
    let at = v.kind === KIND_SCROLL && v.refreshable ? v.id : found;
    for (let i = v.children.length - 1; i >= 0; i--) {
      at = this.scrollAt(v.children[i], x, y, at);
    }
    return at;
  }

  // takeGone returns the ids the last batch deleted and clears them.
  takeGone(): number[] {
    const g = this.gone;
    this.gone = [];
    return g;
  }

  // takeZoomed returns the ids the last batch zoomed and clears them.
  takeZoomed(): number[] {
    const z = this.zoomed;
    this.zoomed = [];
    return z;
  }

  // takeDirty returns the views the last batch changed and clears them.
  takeDirty(): number[] {
    const d = this.dirtyIDs;
    this.dirtyIDs = [];
    for (let i = 0; i < d.length; i++) {
      const v = this.nodes.get(d[i]);
      if (v !== undefined) {
        v.queued = false;
      }
    }
    return d;
  }

  // mark queues a view to be written: a header or sticky cells the batch changed are drawn when their view is, and a structural batch no longer writes every view.
  mark(v: VNode): void {
    v.dirty = true;
    if (!v.queued) {
      v.queued = true;
      this.dirtyIDs.push(v.id);
    }
  }

  // takeParents returns the views whose children the last batch changed and clears them.
  takeParents(): number[] {
    const p = this.parents;
    this.parents = [];
    return p;
  }

  takeRootChanged(): boolean {
    const r = this.rootChanged;
    this.rootChanged = false;
    return r;
  }

  // takeStructural and takeGeometry report and clear what the last batch
  // changed of the tree's shape and frames.
  takeStructural(): boolean {
    const s = this.structural;
    this.structural = false;
    return s;
  }

  takeGeometry(): boolean {
    const g = this.geometry;
    this.geometry = false;
    return g;
  }

  add(ops: string): void {
    const batch = JSON.parse(ops) as Object[];
    for (let i = 0; i < batch.length; i++) {
      const o = batch[i] as Object[];
      const code = o[0] as string;
      const id = o[1] as number;
      if (code === 'c') {
        const v = new VNode();
        v.id = id;
        v.kind = o[2] as number;
        this.nodes.set(id, v);
        this.structural = true;
        this.geometry = true;
        const g = this.gone.indexOf(id);
        if (g >= 0) {
          this.gone.splice(g, 1);
        }
      } else if (code === 'root') {
        this.root = id;
        this.rootChanged = true;
        this.structural = true;
        this.geometry = true;
      } else if (code === 'app') {
        this.appMode = o[2] as number;
      } else if (code === 'sb') {
        this.sbStyle = o[2] as number;
        this.sbHidden = o[3] as boolean;
        this.sbImmersive = o[4] as boolean;
        this.sbSent = true;
      } else if (code === 'or') {
        this.orientation = o[2] as number;
        this.orientationSent = true;
      } else if (code === 'la') {
        this.layoutMs = o[2] as number;
        this.layoutCurve = o[3] as number;
      } else if (code === 'ik') {
        canvasImageCache.keep(o[2] as string);
      } else if (code === 'ir') {
        canvasImageCache.release(o[2] as string);
      } else if (code === 'il') {
        canvasImageCache.setLimit(o[2] as number);
      } else if (code === 'sticky') {
        this.sticky.set(id, JSON.parse(o[2] as string) as StickyCell[]);
        this.geometry = true;
        const n = this.nodes.get(id);
        if (n !== undefined) {
          this.mark(n);
        }
      } else if (code === 'scr') {
        const n = this.nodes.get(id);
        if (n !== undefined) {
          n.isScreen = true;
          n.scrTitle = o[2] as string;
          n.scrHidden = o[3] as boolean;
          n.scrCanGoBack = o[4] as boolean;
          n.scrBackTitle = o[5] as string;
          n.scrHeight = o[6] as number;
          n.scrTransparent = o[7] as boolean;
          n.scrBg = o[8] as string;
          n.scrTitleColor = o[9] as string;
          n.scrTintColor = o[10] as string;
          this.mark(n);
        }
      } else if (code === 'i') {
        const v = this.nodes.get(id);
        if (v === undefined) {
          continue;
        }
        this.structural = true;
        this.geometry = true;
        this.detach(v);
        const parentId = o[2] as number;
        const before = o[3] as number;
        v.parent = parentId;
        this.parents.push(parentId);
        const p = this.nodes.get(parentId);
        if (p === undefined) {
          continue;
        }
        // Children are replaced rather than changed in place, so the view
        // that draws them sees a new array and lays them out again.
        if (before === 0) {
          p.children = p.children.concat([id]);
        } else {
          const at = p.children.indexOf(before);
          if (at < 0) {
            p.children = p.children.concat([id]);
          } else {
            p.children = p.children.slice(0, at).concat([id], p.children.slice(at));
          }
        }
      } else if (code === 'r' || code === 'd') {
        const v = this.nodes.get(id);
        if (v === undefined) {
          continue;
        }
        this.structural = true;
        this.geometry = true;
        this.detach(v);
        if (code === 'd') {
          setViewPixels(id, 0, 0, '');
          this.nodes.delete(id);
          this.sticky.delete(id);
          this.gone.push(id);
        }
      } else {
        const v = this.nodes.get(id);
        if (v === undefined) {
          continue;
        }
        v.dirty = true
        if (!v.queued) {
          v.queued = true;
          this.dirtyIDs.push(id);
        }
        if (code === 'f' || code === 'ss' || code === 'sc') {
          this.geometry = true;
        }
        if (code === 'match') {
          // The batch has not deleted Before yet, so its frame is here
          // for the new view to start from. Both are the page's, since
          // the two views need not share a parent.
          const before = this.nodes.get(o[2] as number);
          if (before !== undefined) {
            v.matchFrame = { x: before.ax, y: before.ay, w: before.w, h: before.h };
          }
          continue;
        }
        if (code === 'f') {
          v.x = o[2] as number;
          v.y = o[3] as number;
          v.w = o[4] as number;
          v.h = o[5] as number;
        } else if (code === 's') {
          v.bg = o[2] as string;
          v.radius = o[3] as number;
          v.bw = o[4] as number;
          v.bc = o[5] as string;
          v.op = o[6] as number;
          v.hidden = o[7] as boolean;
          v.clip = o[8] as boolean;
          v.tx = o[9] as number;
          v.ty = o[10] as number;
          v.scx = o[11] as number;
          v.scy = o[12] as number;
          v.rot = o[13] as number;
        } else if (code === 't') {
          v.text = o[2] as string;
          v.fs = o[3] as number;
          v.fw = o[4] as number;
          v.italic = o[5] as boolean;
          v.family = o[6] as string;
          v.color = o[7] as string;
          v.lh = o[8] as number;
          v.align = o[9] as number;
          v.maxLines = o[10] as number;
        } else if (code === 'e') {
          v.press = o[2] as boolean;
          v.long = o[3] as boolean;
        } else if (code === 'in') {
          v.ph = o[2] as string;
          v.fs = o[3] as number;
          v.fw = o[4] as number;
          v.color = o[5] as string;
          v.phColor = o[6] as string;
          v.masked = o[7] as boolean;
          v.multiline = o[8] as boolean;
          v.inX = o[9] as number;
          v.inY = o[10] as number;
          v.inW = o[11] as number;
          v.inH = o[12] as number;
          v.readonly = o[13] as boolean;
          v.disabled = o[14] as boolean;
          v.family = o[20] as string;
          v.italic = o[21] as boolean;
          v.lh = o[22] as number;
          v.align = o[23] as number;
        } else if (code === 'iv') {
          v.value = o[2] as string;
        } else if (code === 'im') {
          v.src = o[2] as string;
          v.fit = o[3] as string;
          v.data = o[4] as string;
          v.nearest = o[5] as boolean;
        } else if (code === 'ip') {
          setViewPixels(id, o[2] as number, o[3] as number, o[4] as string);
          v.pv++;
        } else if (code === 'cv') {
          v.drawing = o[2] as string;
        } else if (code === 'tb') {
          v.tabBar = o[2] as string;
        } else if (code === 'tr') {
          v.runs = JSON.parse(o[2] as string) as TextRun[];
        } else if (code === 'sc') {
          v.horizontal = o[2] as boolean;
          v.hideBar = o[3] as boolean;
          v.refreshable = o[4] as boolean;
          v.refreshing = o[5] as boolean;
          v.refreshAt = o[6] as number;
          v.zmin = o[7] as number;
          v.zmax = o[8] as number;
          if (!v.zset) {
            v.zoom = o[9] as number;
          }
          v.pinch = o[10] as boolean;
        } else if (code === 'zoom') {
          v.zoom = o[2] as number;
          v.zcx = o[3] as number;
          v.zcy = o[4] as number;
          v.zset = true;
          this.zoomed.push(id);
        } else if (code === 'ss') {
          v.cw = o[2] as number;
          v.ch = o[3] as number;
        } else if (code === 'web') {
          const cfg: WebConfigOp = parseWebConfigOp(o[2] as string);
          v.webSrc = cfg.src ?? '';
          v.webHTML = cfg.html ?? '';
          v.webBaseURL = cfg.base ?? '';
          v.noJS = cfg.noJS ?? false;
          v.webBefore = cfg.before ?? '';
          v.webAfter = cfg.after ?? '';
          v.webUA = cfg.ua ?? '';
          v.webUASuffix = cfg.uas ?? '';
          v.webHeaders = cfg.headers ?? [];
          v.webIncognito = cfg.incognito ?? false;
          v.webNoScroll = cfg.noScroll ?? false;
          v.webMuted = cfg.muted ?? false;
          v.webPinchZoom = cfg.pinchZoom ?? false;
          v.webDownloadDir = cfg.downloadDir ?? '';
          v.webRestricted = cfg.restricted ?? false;
          v.webHosts = cfg.hosts ?? [];
          v.webSchemes = cfg.schemes ?? [];
          v.webAppSchemes = cfg.appSchemes ?? [];
          v.webMedia = cfg.media ?? 0;
          v.webFileAccess = webSourceNeedsFileAccess(v.webSrc, v.webHTML, v.webBaseURL);
        } else if (code === 'wcmd') {
          v.webCmds.push({ cmd: o[2] as number, value: o[3] as string });
        } else if (code === 'video') {
          v.videoSrc = o[2] as string;
          v.videoPaused = o[3] as boolean;
          v.videoMuted = o[4] as boolean;
          v.videoRepeat = o[5] as boolean;
          v.videoControls = o[6] as boolean;
          v.videoFit = o[7] as string;
        } else if (code === 'vseek') {
          v.videoSeek = o[2] as number;
        } else if (code === 'al') {
          v.label = o[2] as string;
        } else if (code === 'vp') {
          v.testID = o[2] as string;
          v.pointer = o[3] as number;
          v.label = o[4] as string;
          v.accDesc = o[5] as string;
          v.propDisabled = o[6] as boolean;
          v.accLevel = o[7] as string;
          v.accGroup = o[8] as boolean;
        } else if (code === 'sel') {
          v.selStart = o[2] as number;
          v.selEnd = o[3] as number;
        } else if (code === 'fo') {
          v.needFocus = true;
        } else if (code === 'bl') {
          v.needBlur = true;
        } else if (code === 'mo') {
          v.anim = o[2] as number;
        } else if (code === 'so') {
          v.sx = o[2] as number;
          v.sy = o[3] as number;
          v.skey = v.sx + ',' + v.sy;
        }
      }
    }
  }

  detach(v: VNode): void {
    if (v.parent === 0) {
      return;
    }
    const p = this.nodes.get(v.parent);
    if (p !== undefined) {
      p.children = p.children.filter((c: number) => c !== v.id);
    }
    this.parents.push(v.parent);
    v.parent = 0;
  }

  // measure walks the tree from its root, giving each view its frame in the
  // window and the view a press on it goes to.
  measure(): void {
    if (this.root >= 0) {
      this.walk(this.root, 0, 0, 0);
    }
  }

  walk(id: number, ox: number, oy: number, hit: number): void {
    const v = this.nodes.get(id);
    if (v === undefined) {
      return;
    }
    v.ax = ox + v.x;
    v.ay = oy + v.y;
    v.hit = v.press ? v.id : hit;
    for (let i = 0; i < v.children.length; i++) {
      this.walk(v.children[i], v.ax, v.ay, v.hit);
    }
  }

  at(id: number): VNode | undefined {
    return this.nodes.get(id);
  }

  top(): VNode | undefined {
    return this.root < 0 ? undefined : this.nodes.get(this.root);
  }
}

// Bridge is what the module calls in this page, as nsgoHarmony.
class Bridge {
  host: Index | undefined = undefined;

  ready(): void {
    console.info('NSGO harmony: engine ready');
    // The module is up, so what was measured before it started can be sent.
    if (this.host !== undefined) {
      this.host.readEnv();
    }
  }

  submit(ops: string): void {
    if (this.host !== undefined) {
      this.host.apply(ops);
    }
  }

  tick(): void {
    if (this.host !== undefined) {
      this.host.tick();
    }
  }

  idle(): void {
    if (this.host !== undefined) {
      this.host.idle();
    }
  }

  perf(text: string): void {
    if (this.host !== undefined) {
      this.host.setPerf(text);
    }
  }

  snapshot(count: number): void {
    if (this.host !== undefined) {
      this.host.snapshot(count);
    }
  }

  scrollBy(id: number, dx: number, dy: number): void {
    const sc = this.host?.renderer.scrollers.get(id);
    if (sc !== undefined) {
      sc.scrollBy(dx, dy);
    }
  }

  typeInput(id: number, value: string): void {
    const host = this.host;
    if (host === undefined) {
      return;
    }
    const v = host.tree.at(id);
    const node = host.renderer.nodes.get(id);
    if (v === undefined || node === undefined) {
      return;
    }
    v.value = value;
    host.renderer.shown.delete(id);
    host.renderer.input(v, node);
  }

  device(): string {
    const host = this.host;
    if (host === undefined) {
      return '{}';
    }
    return host.deviceJSON();
  }

  // launch answers the page before the module starts, so a Want's nsgo-
  // flags reach go.argv without a second build for each run.
  launch(): string {
    return AppStorage.get<string>('nsgoLaunch') ?? '{"args":[],"url":""}';
  }

  sandbox(operation: string, request: string): string {
    return this.host?.files?.call(operation, request) ?? '{"error":"ENOSYS"}';
  }

  alert(json: string): void {
    if (this.host !== undefined) {
      this.host.alert(json);
    }
  }

  clipread(kind: number, id: number): void {
    if (this.host !== undefined) {
      this.host.clipread(kind, id);
    }
  }

  clipset(text: string): void {
    if (this.host !== undefined) {
      this.host.clipset(text);
    }
  }

  clipimage(data: string): void {
    if (this.host !== undefined) {
      this.host.clipimage(data);
    }
  }

  dismisskeyboard(): void {
    if (this.host !== undefined) {
      this.host.dismissKeyboard();
    }
  }

  announce(text: string): void {
    if (this.host !== undefined) {
      this.host.announce(text);
    }
  }

  title(label: string): void {
    if (this.host !== undefined) {
      this.host.title(label);
    }
  }

  updatewidget(data: string): void {
    if (this.host !== undefined) {
      this.host.updatewidget(data);
    }
  }

  listdir(uri: string, name: string): string {
    return this.host === undefined ? '[]' : this.host.listdir(uri, name);
  }

  readfile(uri: string, name: string): string {
    return this.host === undefined ? '' : this.host.readfile(uri, name);
  }

  pickdirectories(id: number, limit: number): void {
    if (this.host !== undefined) {
      this.host.pickdirectories(id, limit);
    }
  }

  imageinfo(id: number, uri: string, data: string): void {
    if (this.host !== undefined) {
      this.host.imageinfo(id, uri, data);
    }
  }

  openurl(id: number, url: string): void {
    if (this.host !== undefined) {
      this.host.openurl(id, url);
    }
  }

  pickimages(id: number, limit: number): void {
    if (this.host !== undefined) {
      this.host.pickimages(id, limit);
    }
  }

  pickfiles(id: number, limit: number): void {
    if (this.host !== undefined) {
      this.host.pickfiles(id, limit);
    }
  }

  takephoto(id: number): void {
    if (this.host !== undefined) {
      this.host.takephoto(id);
    }
  }

  keepawake(id: number, display: boolean): void {
    if (this.host !== undefined) {
      this.host.keepawake(id, display);
    }
  }

  releaseawake(id: number): void {
    if (this.host !== undefined) {
      this.host.releaseawake(id);
    }
  }

  bioavail(id: number): void {
    if (this.host !== undefined) {
      this.host.bioavail(id);
    }
  }

  bioauth(id: number, reason: string): void {
    if (this.host !== undefined) {
      this.host.bioauth(id, reason);
    }
  }

  permission(id: number, command: number, name: string): void {
    if (this.host !== undefined) {
      this.host.permission(id, command, name);
    }
  }

  webcookies(id: number, req: string): void {
    if (this.host !== undefined) {
      this.host.webcookies(id, req);
    }
  }

  savefile(id: number, name: string, mime: string, data: string): void {
    if (this.host !== undefined) {
      this.host.savefile(id, name, mime, data);
    }
  }

  // saved answers a save that never reached the dialog, with the reason.
  saved(id: number, ok: boolean, why: string): void {
    if (this.host !== undefined) {
      this.host.send('saved', id, ok ? 1 : 0, 0, why);
    }
  }

  openfile(id: number, path: string, mime: string): void {
    if (this.host !== undefined) {
      this.host.openfile(id, path, mime);
    }
  }

  // backunhandled says the engine left the back gesture to the system.
  backunhandled(): void {
    if (this.host !== undefined) {
      this.host.finish();
    }
  }

  measure(text: string, size: number, weight: number, italic: boolean, family: string, width: number): string {
    const opts = {
      textContent: text,
      fontSize: size,
      fontWeight: weight,
      fontStyle: italic ? FontStyle.Italic : FontStyle.Normal
    } as MeasureOptions;
    if (width > 0) {
      const wrapped = {
        textContent: text,
        fontSize: size,
        fontWeight: weight,
        fontStyle: italic ? FontStyle.Italic : FontStyle.Normal,
        constraintWidth: width
      } as MeasureOptions;
      // measureTextSize answers the constraint's width rather than the
      // text's own, so a label measured for its line's room came back as wide
      // as the room and every centering it sat in started at the left edge.
      // The call without a constraint says which width is the text's.
      const sw = MeasureText.measureTextSize(wrapped);
      const own = MeasureText.measureTextSize(opts);
      const w = Math.min(Number(sw.width), Number(own.width));
      return '[' + px2vp(w) + ',' + px2vp(Number(sw.height)) + ']';
    }
    const s = MeasureText.measureTextSize(opts);
    return '[' + px2vp(Number(s.width)) + ',' + px2vp(Number(s.height)) + ']';
  }
}

@Entry
@Component
struct Index {
  controller: webview.WebviewController = new webview.WebviewController();
  bridge: Bridge = new Bridge();
  tree: Tree = new Tree();
  // The renderer mounts the engine's tree on FrameNodes; its controller is
  // the NodeContainer the page shows.
  renderer: Renderer = new Renderer();
  renderController: RenderController = new RenderController();
  private res: resourceManager.ResourceManager | undefined = undefined;
  private hostCtx: common.UIAbilityContext | undefined = undefined;
  files: SandboxFiles | undefined = undefined;
  private win: window.Window | undefined = undefined;
  // awake holds the power assertions keeping the screen on.
  private awake: Set<number> = new Set();
  private safeTop: number = 0;
  private safeRight: number = 0;
  private safeBottom: number = 0;
  private safeLeft: number = 0;
  private keyboard: number = 0;
  // bars and indicator are the window's two system bar requests: the bars setWindowSystemBarEnable shows, and whether setSpecificSystemBarEnabled shows the navigation indicator. Each is tracked on its own, so one the device refuses does not make the other again.
  private bars: BarRequest = new BarRequest();
  private indicator: BarRequest = new BarRequest();
  // The display clock that runs while something animates.
  private sync: displaySync.DisplaySync | undefined = undefined;
  // framePending is a frame the page has not finished. The display clock fires every vsync whether or not the engine kept up, and each runJavaScript queues behind the last, so a frame slower than the vsync let dozens queue in the page (over a hundred at launch on a phone) and every touch waited behind them. A vsync that finds a frame pending is skipped; the engine's clock reads the time, so the next frame catches up.
  private framePending: boolean = false;
  // pinchState is the pinch in progress over a zoomable scroll view.
  private pinchState: PinchState | undefined = undefined;
  private ticking: boolean = false;
  // appMode is the appearance the app last asked for, -1 before any.
  // systemDark is the system's, and dark is the one the status bar's
  // automatic style follows: the app's when it forces one.
  private appMode: number = -1;
  private systemDark: boolean = false;
  private dark: boolean = false;
  // The performance monitor's readings, shown when --nsgo-fps asks.
  @State private perfText: string = '';
  // picking is set while a system picker shows: its back closes it rather
  // than the app's back gesture, which would leave the app.
  private picking: boolean = false;
  // A new Want, a system configuration change or a lifecycle move bumps
  // this; the watcher asks the engine to read its environment again.
  @StorageLink('nsgoLaunchVer') @Watch('onLaunchChange') launchVer: number = 0;

  onLaunchChange(): void {
    this.readEnv();
  }


  // clipread answers ItemReadClipboard: the clipboard's text, or whether it
  // holds text at all without reading it.
  clipread(kind: number, id: number): void {
    const pb = pasteboard.getSystemPasteboard();
    if (kind === 1) {
      let has = false;
      try {
        has = pb.hasDataSync();
      } catch (e) {
        console.error('NSGO harmony: clipboard: ' + e);
      }
      this.send('clip', id, 1, 0, has ? '1' : '0');
      return;
    }
    if (kind !== 0) {
      this.send('clip', id, 0, 0, '');
      return;
    }
    pb.getData().then((data: pasteboard.PasteData) => {
      let text = '';
      try {
        text = data.getPrimaryText();
      } catch (e) {
        text = '';
      }
      this.send('clip', id, 0, 0, text);
    }).catch(() => {
      this.send('clip', id, 0, 0, '');
    });
  }

  // clipset replaces the clipboard with text.
  clipset(text: string): void {
    const data = pasteboard.createData(pasteboard.MIMETYPE_TEXT_PLAIN, text);
    pasteboard.getSystemPasteboard().setData(data).catch((e: Error) => {
      console.error('NSGO harmony: clipboard: ' + e);
    });
  }

  // clipimage puts an image on the system clipboard, decoding the app's
  // bytes into a PixelMap first.
  clipimage(data: string): void {
    try {
      const bytes = new util.Base64Helper().decodeSync(data);
      const source = image.createImageSource(bytes.buffer);
      source.createPixelMap().then((pm: image.PixelMap) => {
        const paste = pasteboard.createData(pasteboard.MIMETYPE_PIXELMAP, pm);
        pasteboard.getSystemPasteboard().setData(paste).then(() => {
          source.release();
        }).catch((e: Error) => {
          console.error('NSGO harmony: clipboard image: ' + e);
          source.release();
        });
      }).catch((e: Error) => {
        console.error('NSGO harmony: clipboard image: ' + e);
      });
    } catch (e) {
      console.error('NSGO harmony: clipboard image: ' + e);
    }
  }

  // openurl hands a URL to the system, which opens it in whichever app
  // takes it, and answers ItemOpenURL with whether one did.
  openurl(id: number, url: string): void {
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      this.send('openurl', id, 0, 0, '');
      return;
    }
    ctx.startAbility({
      action: 'ohos.want.action.viewData',
      uri: url,
      entities: ['entity.system.browsable']
    }).then(() => {
      this.send('openurl', id, 1, 0, '');
    }).catch((e: Error) => {
      console.error('NSGO harmony: open URL: ' + e);
      this.send('openurl', id, 0, 0, '');
    });
  }
  setPerf(text: string): void {
    this.perfText = text;
  }

  // pinchTouch zooms the zoomable scroll view under a pinch's middle and
  // reports whether it took the event. A pinch ends with the fingers.
  private pinchTouch(e: TouchEvent): boolean {
    if (e.touches.length !== 2) {
      this.pinchState = undefined;
      return false;
    }
    const a = e.touches[0];
    const b = e.touches[1];
    const dx = a.x - b.x;
    const dy = a.y - b.y;
    const dist = Math.sqrt(dx * dx + dy * dy);
    const mx = (a.x + b.x) / 2;
    const my = (a.y + b.y) / 2;
    const state = this.pinchState;
    if (state === undefined) {
      const id = this.tree.scrollNodeAt(this.tree.root, mx, my, 0);
      const v = this.tree.nodes.get(id);
      if (v === undefined || !v.pinch || v.zmax <= v.zmin) {
        return false;
      }
      this.pinchState = { id: id, dist: dist, scale: v.zoom };
      return true;
    }
    const v = this.tree.nodes.get(state.id);
    if (v === undefined) {
      this.pinchState = undefined;
      return false;
    }
    if (state.dist > 0) {
      const z = Math.min(Math.max(state.scale * dist / state.dist, v.zmin), v.zmax);
      this.renderer.zoomTo(v, z, mx, my);
    }
    return true;
  }

  // snapshot takes the view's picture for a test: the root's pixels as a
  // PNG, base64, answered on the page's own global.
  snapshot(count: number): void {
    componentSnapshot.get('nsgo-root').then((px: image.PixelMap) => {
      const packer = image.createImagePacker();
      packer.packToData(px, { format: 'image/png', quality: 100 }).then((buf: ArrayBuffer) => {
        const b64 = new util.Base64Helper().encodeToStringSync(new Uint8Array(buf));
        this.controller.runJavaScript('nsgoSnapshotReply(' + count + ',"' + b64 + '")');
        packer.release();
        px.release();
      }).catch((e: Error) => {
        console.error('NSGO harmony: snapshot pack failed: ' + e.message);
        this.controller.runJavaScript('nsgoSnapshotReply(' + count + ',"")');
      });
    }).catch((e: Error) => {
      console.error('NSGO harmony: snapshot failed: ' + e.message);
      this.controller.runJavaScript('nsgoSnapshotReply(' + count + ',"")');
    });
  }

  // alert shows the engine's alert as the system's dialog, as the other
  // platforms do; its buttons answer with EventAlert carrying the alert's
  // ID and the button's index, or -1 when the user cancels it.
  alert(json: string): void {
    const a = JSON.parse(json) as AlertSpec;
    const buttons: AlertDialogButtonOptions[] = [];
    for (let i = 0; i < a.a.length; i++) {
      const at = i;
      if (a.a[i].r === 2) {
        buttons.push({ value: a.a[i].l, fontColor: '#d32f2f', action: () => { this.answer(a.id, at); } });
      } else {
        buttons.push({ value: a.a[i].l, action: () => { this.answer(a.id, at); } });
      }
    }
    if (a.c) {
      AlertDialog.show({ title: a.t, message: a.m, buttons: buttons, autoCancel: true,
        cancel: () => { this.answer(a.id, -1); } });
    } else {
      AlertDialog.show({ title: a.t, message: a.m, buttons: buttons, autoCancel: false });
    }
  }

  answer(id: number, index: number): void {
    this.send('alert', id, index, 0, '');
  }

  // pickimages shows the system's photo picker and answers ItemPickImages
  // with the bytes of what the user picked, base64-encoded.
  pickimages(id: number, limit: number): void {
    this.picking = true;
    const picker = new photoAccessHelper.PhotoViewPicker();
    picker.select({
      MIMEType: photoAccessHelper.PhotoViewMIMETypes.IMAGE_TYPE,
      maxSelectNumber: limit > 0 ? limit : 1
    }).then((result: photoAccessHelper.PhotoSelectResult) => {
      const files: PickedImage[] = [];
      const uris = result.photoUris;
      for (let i = 0; i < uris.length; i++) {
        try {
          const f = fs.openSync(uris[i], fs.OpenMode.READ_ONLY);
          const size = fs.statSync(f.fd).size;
          const buf = new ArrayBuffer(size);
          fs.readSync(f.fd, buf);
          fs.closeSync(f);
          const data = new util.Base64Helper().encodeToStringSync(new Uint8Array(buf));
          files.push({ n: 'image-' + i + '.jpg', m: 'image/jpeg', d: data });
        } catch (e) {
          console.error('NSGO harmony: pick image: ' + e);
        }
      }
      this.picking = false;
      this.send('picked', id, 0, 0, JSON.stringify(files));
    }).catch((e: Error) => {
      console.error('NSGO harmony: picker: ' + e);
      this.picking = false;
      this.send('picked', id, 0, 0, '[]');
    });
  }

  // pickfiles shows the system's document picker and copies what the user
  // picked into the app's cache, answering ItemPickFiles with the paths.
  pickfiles(id: number, limit: number): void {
    this.picking = true;
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      this.picking = false;
      this.send('pickedfiles', id, 0, 0, '[]');
      return;
    }
    const p = new picker.DocumentViewPicker();
    p.select({ maxSelectNumber: limit > 0 ? limit : 1 }).then((uris: Array<string>) => {
      const files: PickedFileInfo[] = [];
      for (let i = 0; i < uris.length; i++) {
        const uri = uris[i];
        try {
          const src = fs.openSync(uri, fs.OpenMode.READ_ONLY);
          const size = fs.statSync(src.fd).size;
          const dest = ctx.cacheDir + '/nsgo-pick-' + Date.now() + '-' + i;
          fs.copyFileSync(src.fd, dest);
          fs.closeSync(src);
          files.push({ n: 'picked-' + i, m: mimeOf(dest), s: size, p: dest });
        } catch (e) {
          console.error('NSGO harmony: pick file: ' + e);
        }
      }
      this.picking = false;
      this.send('pickedfiles', id, 0, 0, JSON.stringify(files));
    }).catch((e: Error) => {
      console.error('NSGO harmony: picker: ' + e);
      this.picking = false;
      this.send('pickedfiles', id, 0, 0, '[]');
    });
  }

  // takephoto shows the system's camera and answers ItemTakePhoto with the
  // photo it took, as PickImages answers its images.
  takephoto(id: number): void {
    this.picking = true;
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      this.picking = false;
      this.send('picked', id, 0, 0, '[]');
      return;
    }
    cameraPicker.pick(ctx, [cameraPicker.PickerMediaType.PHOTO], {
      cameraPosition: camera.CameraPosition.CAMERA_POSITION_BACK
    }).then((result: cameraPicker.PickerResult) => {
      const files: PickedImage[] = [];
      if (result.resultUri !== '') {
        try {
          const f = fs.openSync(result.resultUri, fs.OpenMode.READ_ONLY);
          const size = fs.statSync(f.fd).size;
          const buf = new ArrayBuffer(size);
          fs.readSync(f.fd, buf);
          fs.closeSync(f);
          files.push({ n: 'photo.jpg', m: 'image/jpeg', d: new util.Base64Helper().encodeToStringSync(new Uint8Array(buf)) });
        } catch (e) {
          console.error('NSGO harmony: take photo: ' + e);
        }
      }
      this.picking = false;
      this.send('picked', id, 0, 0, JSON.stringify(files));
    }).catch((e: Error) => {
      console.error('NSGO harmony: camera: ' + e);
      this.picking = false;
      this.send('picked', id, 0, 0, '[]');
    });
  }

  // savefile shows the system's save dialog and writes the bytes there.
  savefile(id: number, name: string, mime: string, data: string): void {
    this.picking = true;
    const p = new picker.DocumentViewPicker();
    p.save({ newFileNames: [name] }).then((uris: Array<string>) => {
      if (uris.length === 0) {
        this.picking = false;
        this.send('saved', id, 0, 0, '');
        return;
      }
      try {
        const bytes = new util.Base64Helper().decodeSync(data);
        const f = fs.openSync(uris[0], fs.OpenMode.READ_WRITE | fs.OpenMode.CREATE | fs.OpenMode.TRUNC);
        fs.writeSync(f.fd, bytes.buffer);
        fs.closeSync(f);
        this.picking = false;
        this.send('saved', id, 1, 0, '');
      } catch (e) {
        this.picking = false;
        this.send('saved', id, 0, 0, '' + e);
      }
    }).catch((e: Error) => {
      this.picking = false;
      this.send('saved', id, 0, 0, e.message);
    });
  }

  // openfile hands a file to the app the system associates with it.
  openfile(id: number, path: string, mime: string): void {
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      this.send('opened', id, 0, 0, '');
      return;
    }
    ctx.startAbility({
      action: 'ohos.want.action.viewData',
      uri: fileUri.getUriFromPath(path),
      type: mime
    }).then(() => {
      this.send('opened', id, 1, 0, '');
    }).catch((e: Error) => {
      console.error('NSGO harmony: open file: ' + e);
      this.send('opened', id, 0, 0, e.message);
    });
  }

  // dismissKeyboard takes keyboard focus away, which closes the keyboard.
  dismissKeyboard(): void {
    this.getUIContext().getFocusController().clearFocus();
  }

  // imageinfo loads an image ahead of showing it and answers ItemImageInfo
  // with its size in pixels, or why it could not.
  imageinfo(id: number, uri: string, data: string): void {
    let source: image.ImageSource | undefined = undefined;
    try {
      if (data !== '') {
        const bytes = new util.Base64Helper().decodeSync(data);
        source = image.createImageSource(bytes.buffer);
      } else {
        source = image.createImageSource(uri);
      }
    } catch (e) {
      this.send('imageinfo', id, 0, 0, '' + e);
      return;
    }
    const s = source;
    if (s === undefined) {
      this.send('imageinfo', id, 0, 0, 'no image');
      return;
    }
    s.getImageInfo().then((info: image.ImageInfo) => {
      this.send('imageinfo', id, info.size.width, info.size.height, '');
      s.release();
    }).catch((e: Error) => {
      this.send('imageinfo', id, 0, 0, e.message);
      s.release();
    });
  }

  // keepawake holds the screen on for a power assertion, as iOS's
  // idleTimerDisabled: one open hold keeps it on, and the last to go lets
  // it sleep again.
  keepawake(id: number, display: boolean): void {
    this.awake.add(id);
    this.applyAwake();
  }

  releaseawake(id: number): void {
    this.awake.delete(id);
    this.applyAwake();
  }

  private applyAwake(): void {
    const w = this.win;
    if (w === undefined) {
      return;
    }
    w.setWindowKeepScreenOn(this.awake.size > 0).catch((e: Error) => {
      console.error('NSGO harmony: keep awake: ' + e);
    });
  }

  // bioavail answers ItemBiometricAvailability with the enrolled
  // credential's kind, driver.Biometric's.
  bioavail(id: number): void {
    const at = abilityAccessCtrl.createAtManager();
    const token = this.tokenID();
    // Face and fingerprint, as driver's kinds; the system tells which is
    // enrolled by whether the credential is there at all.
    const kinds: number[] = [userAuth.UserAuthType.FACE, userAuth.UserAuthType.FINGERPRINT];
    const mapped: number[] = [2, 3];
    let found = 0;
    for (let i = 0; i < kinds.length; i++) {
      try {
        at.checkAccessTokenSync(token, 'ohos.permission.ACCESS_BIOMETRIC');
      } catch (e) {
        console.error('NSGO harmony: biometric: ' + e);
      }
      try {
        userAuth.getAvailableStatus(kinds[i], userAuth.AuthTrustLevel.ATL1);
        found = mapped[i];
        break;
      } catch (e) {
        // Not enrolled: the next kind may be.
      }
    }
    this.send('bio', id, found, 0, '');
  }

  // bioauth asks for the user's biometric and answers
  // ItemBiometricAuthenticate with driver.BiometricResult's kind.
  bioauth(id: number, reason: string): void {
    const ctx = this.hostCtx;
    if (ctx === undefined || reason === '') {
      this.send('bio', id, 4, 0, '');
      return;
    }
    let auth: userAuth.UserAuthInstance;
    try {
      auth = userAuth.getUserAuthInstance({
        challenge: new Uint8Array([]),
        authType: [userAuth.UserAuthType.FACE, userAuth.UserAuthType.FINGERPRINT],
        authTrustLevel: userAuth.AuthTrustLevel.ATL1
      }, { title: reason, uiContext: ctx });
    } catch (e) {
      console.error('NSGO harmony: biometric auth: ' + e);
      this.send('bio', id, 4, 0, '');
      return;
    }
    auth.on('result', {
      onResult: (result: userAuth.UserAuthResult) => {
        // Success, failure, cancel or unavailable, as driver's kinds.
        let mapped = 2;
        if (result.result === userAuth.UserAuthResultCode.SUCCESS) {
          mapped = 1;
        } else if (result.result === userAuth.UserAuthResultCode.CANCELED) {
          mapped = 3;
        } else if (result.result === userAuth.UserAuthResultCode.NOT_ENROLLED ||
          result.result === userAuth.UserAuthResultCode.TYPE_NOT_SUPPORT) {
          mapped = 4;
        }
        this.send('bio', id, mapped, 0, '');
      }
    });
    auth.start();
  }

  // tokenID is this app's access token, which the permission checks ask
  // about.
  private tokenID(): number {
    try {
      // The token is in the application info, which the default flags
      // leave out.
      const info = bundleManager.getBundleInfoForSelfSync(bundleManager.BundleFlag.GET_BUNDLE_INFO_WITH_APPLICATION);
      return info.appInfo.accessTokenId;
    } catch (e) {
      console.error('NSGO harmony: token: ' + e);
      return 0;
    }
  }

  // permission answers ItemPermission numbered id for a key this system
  // keeps: the microphone or the camera.
  permission(id: number, command: number, name: string): void {
    const perm: string = name === 'microphone' ? 'ohos.permission.MICROPHONE' : 'ohos.permission.CAMERA';
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      this.send('perm', id, 5, 0, '');
      return;
    }
    const at = abilityAccessCtrl.createAtManager();
    const token = this.tokenID();
    const status = (): number => {
      try {
        const s = at.checkAccessTokenSync(token, perm as Permissions);
        return s === abilityAccessCtrl.GrantStatus.PERMISSION_GRANTED ? 1 : 2;
      } catch (e) {
        console.error('NSGO harmony: permission: ' + e);
        return 5;
      }
    };
    // driver.PermissionCommand: check, ask, open settings, guide, stop.
    if (command === 1) {
      at.requestPermissionsFromUser(ctx, [perm as Permissions]).then((r: PermissionRequestResult) => {
        const granted = r.authResults.length > 0 && r.authResults[0] === 0;
        this.send('perm', id, granted ? 1 : 2, 0, '');
      }).catch((e: Error) => {
        console.error('NSGO harmony: permission ask: ' + e);
        this.send('perm', id, status(), 0, '');
      });
      return;
    }
    if (command === 2 || command === 3) {
      // The app's own page in the system's settings, where the user
      // changes a permission it refused.
      try {
        settings.openAppDetailSettingsPage(ctx, ctx.applicationInfo.name);
      } catch (e) {
        console.error('NSGO harmony: permission settings: ' + e);
        this.send('perm', id, status(), 0, '');
        return;
      }
      this.send('perm', id, status(), command === 2 ? 1 : 0, '');
      return;
    }
    this.send('perm', id, status(), 0, '');
  }

  // webcookies answers ItemWebCookies through ArkWeb's persistent WebCookieManager
  // and WebStorage (incognito=false), leaving Incognito web views isolated.
  webcookies(id: number, raw: string): void {
    const req = parseWebCookiesReq(raw);
    if (req === undefined || req.op === undefined) {
      this.send('webcookies', id, 0, 0, '', 'HarmonyOS: invalid web cookies request');
      return;
    }
    if (req.op === WEB_COOKIES_OP_SET) {
      const items = req.cookies ?? [];
      if (items.length === 0) {
        this.send('webcookies', id, 0, 0, '', '');
        return;
      }
      const invalid = validateWebCookieSetItems(items);
      if (invalid !== '') {
        this.send('webcookies', id, 0, 0, '', invalid);
        return;
      }
      let failure = '';
      let wrote = false;
      for (let i = 0; i < items.length; i++) {
        const item = items[i];
        try {
          webview.WebCookieManager.configCookieSync(item.u, item.v, false, true);
          wrote = true;
        } catch (e) {
          failure = formatWebCookieRefused(item.n, '' + e);
          break;
        }
      }
      if (!wrote) {
        this.send('webcookies', id, 0, 0, '', failure);
        return;
      }
      try {
        webview.WebCookieManager.saveCookieAsync().then(() => {
          this.send('webcookies', id, 0, 0, '', failure);
        }).catch((e: Error) => {
          this.send('webcookies', id, 0, 0, '', failure !== '' ? failure : ('' + e));
        });
      } catch (e) {
        this.send('webcookies', id, 0, 0, '', failure !== '' ? failure : ('' + e));
      }
      return;
    }
    if (req.op === WEB_COOKIES_OP_LIST) {
      const reqURL = req.url ?? '';
      if (reqURL !== '' && !isValidWebCookieURL(reqURL)) {
        this.send('webcookies', id, 0, 0, '', 'HarmonyOS: invalid web cookie filter URL');
        return;
      }
      try {
        webview.WebCookieManager.fetchAllCookies(false).then((list: Array<webview.WebHttpCookie>) => {
          const out: WebCookieWire[] = [];
          for (let i = 0; i < list.length; i++) {
            const c = list[i];
            out.push({
              n: c.name,
              v: c.value,
              d: c.domain,
              p: c.path,
              e: c.isSessionCookie ? '' : c.expiresDate,
              s: c.isSessionCookie,
              sec: c.isSecure,
              http: c.isHttpOnly,
              ss: mapWebCookieSameSite(Number(c.samesitePolicy), c.isSecure)
            });
          }
          this.send('webcookies', id, 1, 0, JSON.stringify(out), '');
        }).catch((_e: Error) => {
          this.webcookiesFetchByURL(id, reqURL);
        });
      } catch (_e) {
        this.webcookiesFetchByURL(id, reqURL);
      }
      return;
    }
    if (req.op === WEB_COOKIES_OP_CLEAR) {
      try {
        webview.WebCookieManager.clearAllCookiesSync(false);
        webview.WebStorage.deleteAllData(false);
        try {
          webview.WebviewController.removeAllCache(true);
        } catch (_e) {
        }
        webview.WebCookieManager.saveCookieAsync().then(() => {
          this.send('webcookies', id, 0, 0, '', '');
        }).catch((e: Error) => {
          this.send('webcookies', id, 0, 0, '', '' + e);
        });
      } catch (e) {
        this.send('webcookies', id, 0, 0, '', '' + e);
      }
      return;
    }
    this.send('webcookies', id, 0, 0, '', 'HarmonyOS: unsupported web cookies op ' + req.op);
  }

  // webcookiesFetchByURL falls back to API 11+ fetchCookieSync when fetchAllCookies (API 23+) is unavailable.
  private webcookiesFetchByURL(id: number, reqURL: string): void {
    if (reqURL === '') {
      this.send('webcookies', id, 0, 0, '', NO_WEB_COOKIE_DOMAINS);
      return;
    }
    try {
      const header: string = webview.WebCookieManager.fetchCookieSync(reqURL, false);
      this.send('webcookies', id, 2, 0, header, '');
    } catch (e) {
      this.send('webcookies', id, 0, 0, '', '' + e);
    }
  }

  // hasWebPermissions checks whether the app holds all requested media capture
  // permissions (microphone / camera).
  hasWebPermissions(perms: Permissions[]): boolean {
    if (this.hostCtx === undefined || perms.length === 0) {
      return false;
    }
    const at = abilityAccessCtrl.createAtManager();
    const token = this.tokenID();
    for (let i = 0; i < perms.length; i++) {
      try {
        if (at.checkAccessTokenSync(token, perms[i]) !== abilityAccessCtrl.GrantStatus.PERMISSION_GRANTED) {
          return false;
        }
      } catch (_e) {
        return false;
      }
    }
    return true;
  }

  // requestWebPermissions asks the user for missing media capture permissions
  // on behalf of a web view's getUserMedia request.
  requestWebPermissions(perms: Permissions[], done: (granted: boolean) => void): void {
    const ctx = this.hostCtx;
    if (ctx === undefined || perms.length === 0) {
      done(false);
      return;
    }
    const at = abilityAccessCtrl.createAtManager();
    at.requestPermissionsFromUser(ctx, perms).then((r: PermissionRequestResult) => {
      if (r.authResults.length < perms.length) {
        done(false);
        return;
      }
      for (let i = 0; i < r.authResults.length; i++) {
        if (r.authResults[i] !== 0) {
          done(false);
          return;
        }
      }
      done(this.hasWebPermissions(perms));
    }).catch((e: Error) => {
      console.error('NSGO harmony: web permission ask: ' + e);
      done(false);
    });
  }

  // title names the app in the task switcher, which has no window title.
  title(label: string): void {
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      return;
    }
    ctx.setMissionLabel(label).catch((e: Error) => {
      console.error('NSGO harmony: title: ' + e);
    });
  }

  // updatewidget writes what the app last gave its home-screen card into
  // the app's files, which the card's provider reads, and refreshes any
  // card already on the home screen. A card has no Web view and runs in
  // another process, so the data goes through the file they share.
  updatewidget(data: string): void {
    console.info('NSGO harmony: updatewidget ' + data);
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      return;
    }
    const dir = ctx.getApplicationContext().filesDir;
    const path = dir + '/nsgo-widget-data.json';
    try {
      const f = fs.openSync(path, fs.OpenMode.CREATE | fs.OpenMode.TRUNC | fs.OpenMode.WRITE_ONLY);
      fs.writeSync(f.fd, data);
      fs.closeSync(f);
    } catch (e) {
      console.error('NSGO harmony: widget data: ' + e);
    }
    let ids: string[] = [];
    try {
      const f = fs.openSync(dir + '/nsgo-form-ids.json', fs.OpenMode.READ_ONLY);
      const stat = fs.statSync(f.fd);
      const buf = new ArrayBuffer(stat.size);
      fs.readSync(f.fd, buf);
      fs.closeSync(f);
      ids = JSON.parse(new util.TextDecoder().decodeToString(new Uint8Array(buf))) as string[];
    } catch (e) {
      // No card has been added yet, so there is nothing to refresh.
      return;
    }
    try {
      const cfg = JSON.parse(data) as WidgetData;
      const binding = formBindingData.createFormBindingData({ title: cfg.title });
      for (let i = 0; i < ids.length; i++) {
        formProvider.updateForm(ids[i], binding).catch((e: Error) => {
          console.error('NSGO harmony: update card: ' + e);
        });
      }
    } catch (e) {
      console.error('NSGO harmony: refresh cards: ' + e);
    }
  }

  // listdir lists one folder of a picked directory. The Go side calls it
  // synchronously as the app walks the folder as an fs.FS.
  listdir(uri: string, name: string): string {
    try {
      const base = name === '.' ? uri : uri + '/' + name;
      const names = fs.listFileSync(base);
      const out: FolderItem[] = [];
      for (let i = 0; i < names.length; i++) {
        try {
          const st = fs.statSync(base + '/' + names[i]);
          out.push({ n: names[i], d: st.isDirectory(), s: st.size });
        } catch (e) {
          // A vanished entry is simply left out of the listing.
        }
      }
      return JSON.stringify(out);
    } catch (e) {
      console.error('NSGO harmony: list folder: ' + e);
      return '[]';
    }
  }

  // readfile gives the Go side a picked folder's file as base64.
  readfile(uri: string, name: string): string {
    try {
      const f = fs.openSync(uri + '/' + name, fs.OpenMode.READ_ONLY);
      const st = fs.statSync(f.fd);
      const buf = new ArrayBuffer(st.size);
      fs.readSync(f.fd, buf);
      fs.closeSync(f);
      return new util.Base64Helper().encodeToStringSync(new Uint8Array(buf));
    } catch (e) {
      console.error('NSGO harmony: read folder file: ' + e);
      return '';
    }
  }

  // pickdirectories shows the system's folder picker. It reports the
  // folders as name/URI pairs; the Go side reads their contents through
  // listdir and readfile as the app asks for them.
  pickdirectories(id: number, limit: number): void {
    this.picking = true;
    const p = new picker.DocumentViewPicker();
    p.select({ selectMode: picker.DocumentSelectMode.FOLDER,
      maxSelectNumber: limit > 0 ? limit : 1 }).then((uris: Array<string>) => {
      const out: PickedFolderInfo[] = [];
      for (let i = 0; i < uris.length; i++) {
        const u = uris[i];
        out.push({ n: u.substring(u.lastIndexOf('/') + 1), u: u });
      }
      this.picking = false;
      this.send('pickeddirectories', id, 0, 0, JSON.stringify(out));
    }).catch((e: Error) => {
      console.error('NSGO harmony: folder picker: ' + e);
      this.picking = false;
      this.send('pickeddirectories', id, 0, 0, '[]');
    });
  }

  // announce has the screen reader say a message, where one runs.
  announce(text: string): void {
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      return;
    }
    try {
      const info = new accessibility.EventInfo('announceForAccessibility', ctx.applicationInfo.name, 'focus');
      info.textAnnouncedForAccessibility = text;
      accessibility.sendAccessibilityEvent(info).catch((e: Error) => {
        console.error('NSGO harmony: announce: ' + e);
      });
    } catch (e) {
      console.error('NSGO harmony: announce: ' + e);
    }
  }

  // finish leaves the app when the engine has nothing left to take the
  // back gesture, as a back press does on the other targets.
  finish(): void {
    const ctx = this.hostCtx;
    if (ctx !== undefined) {
      ctx.terminateSelf().catch(() => {});
    }
  }

  // onBackPress sends the system's back gesture to the engine first: it
  // closes the topmost modal, pops a screen or lets an app's handler take
  // it, and answers with ItemBackUnhandled when nothing does.
  onBackPress(): boolean {
    if (this.picking) {
      this.picking = false;
      return true;
    }
    if (this.tree.root >= 0) {
      this.send('back', 0, 0, 0, '');
    }
    return true;
  }

  aboutToAppear(): void {
    const host = this.getUIContext().getHostContext() as common.UIAbilityContext;
    if (host) {
      this.hostCtx = host;
      this.res = host.resourceManager;
      this.files = new SandboxFiles(host.filesDir, host.cacheDir, host.tempDir);
    }
    this.bridge.host = this;
    this.renderer.host = this;
    nsgoHost = this;
    this.renderController.renderer = this.renderer;
    this.watchWindow();
  }

  aboutToDisappear(): void {
    this.files?.close();
    this.files = undefined;
  }

  // watchWindow measures what the system bars cover, and measures again when
  // they or the window change, telling the engine through nsgoEnv.
  async watchWindow(): Promise<void> {
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      return;
    }
    try {
      const w: window.Window = await window.getLastWindow(ctx);
      this.win = w;
      this.readEnv();
      // A request that arrived before the window applies now.
      this.applyOrientation();
      w.on('avoidAreaChange', () => {
        this.readEnv();
      });
      w.on('windowSizeChange', () => {
        this.readEnv();
      });
      // The window moving to another display, or its display changing mode or rate, changes the display the engine reports.
      w.on('displayIdChange', () => {
        this.readEnv();
      });
      display.on('change', () => {
        this.readEnv();
      });
    } catch (e) {
      console.error('NSGO harmony: window failed: ' + e);
    }
  }

  readEnv(): void {
    if (this.win !== undefined) {
      try {
        // The display's cutout stays an inset when immersive mode hides the status bar over it, as on the other targets, on whichever side the rotation puts it. Its areas are the cutout's bounds, which need not start at the window's edge.
        const cut = this.win.getWindowAvoidArea(window.AvoidAreaType.TYPE_CUTOUT);
        const rect = this.win.getWindowProperties().windowRect;
        let top = edgeEnd(cut.topRect.top, cut.topRect.height);
        let left = edgeEnd(cut.leftRect.left, cut.leftRect.width);
        let right = edgeFromEnd(cut.rightRect.left, cut.rightRect.width, rect.width);
        let bottom = edgeFromEnd(cut.bottomRect.top, cut.bottomRect.height, rect.height);
        // While immersive mode hides the bars, an edge swipe reveals them over the content for a moment, as on Android, so only the cutout insets the window: the areas the revealed bars report would move the whole layout once as they show and again as they hide. A refused request leaves the bars showing, so their areas count again.
        if (!this.tree.sbImmersive || this.bars.failed) {
          const bars = this.win.getWindowAvoidArea(window.AvoidAreaType.TYPE_SYSTEM);
          // A full-screen window's system bar area carries the status bar but not the navigation indicator, whose own area is what the bottom of the tree must stay above. In landscape the bars' side areas inset the sides.
          const nav = this.win.getWindowAvoidArea(window.AvoidAreaType.TYPE_NAVIGATION_INDICATOR);
          top = Math.max(top, bars.topRect.height);
          left = Math.max(left, edgeEnd(bars.leftRect.left, bars.leftRect.width));
          right = Math.max(right, edgeFromEnd(bars.rightRect.left, bars.rightRect.width, rect.width));
          bottom = Math.max(bottom, bars.bottomRect.height, nav.bottomRect.height);
        }
        this.safeTop = px2vp(top);
        this.renderer.safeTop = this.safeTop;
        this.safeLeft = px2vp(left);
        this.safeRight = px2vp(right);
        this.safeBottom = px2vp(bottom);
        const kb = this.win.getWindowAvoidArea(window.AvoidAreaType.TYPE_KEYBOARD);
        this.keyboard = px2vp(kb.bottomRect.height);
      } catch (e) {
        console.error('NSGO harmony: avoid area failed: ' + e);
      }
    }
    try {
      this.controller.runJavaScript('nsgoEnv()');
    } catch (e) {
      console.error('NSGO harmony: env report failed: ' + e);
    }
    this.applyStatusBar();
  }

  // displayData describes the display the window is on, the default display until the window is known. supportedRefreshRates is ascending; a system before API 20 leaves it out, and the highest rate reads as unknown.
  displayData(main: display.Display): DisplayData {
    let d = main;
    const id = this.win?.getWindowProperties().displayId;
    if (id !== undefined && id !== main.id) {
      try {
        d = display.getDisplayByIdSync(id);
      } catch (e) {
        console.error('NSGO harmony: display ' + id + ' failed: ' + e);
      }
    }
    const rates = d.supportedRefreshRates ?? [];
    return {
      ID: d.id, Width: px2vp(d.width), Height: px2vp(d.height), Main: d.id === main.id,
      RefreshRate: d.refreshRate, MaxRefreshRate: rates.length > 0 ? rates[rates.length - 1] : 0
    };
  }

  deviceJSON(): string {
    const d = display.getDefaultDisplaySync();
    const config = this.hostCtx?.config;
    const locale = i18n.System.getSystemLocale().replace('_', '-');
    let screenReader = false;
    try {
      screenReader = accessibility.isScreenReaderOpenSync();
    } catch (e) {
      console.error('NSGO harmony: screen reader failed: ' + e);
    }
    let launch: Launch = { args: [], url: '' };
    try {
      launch = JSON.parse(AppStorage.get<string>('nsgoLaunch') ?? '{"args":[],"url":""}') as Launch;
    } catch (e) {
      console.error('NSGO harmony: launch failed: ' + e);
    }
    DENSITY = d.densityPixels;
    const dark = config?.colorMode === ConfigurationConstant.ColorMode.COLOR_MODE_DARK;
    this.systemDark = dark;
    this.dark = this.appMode === 2 ? true : (this.appMode === 1 ? false : dark);
    const out: Device = {
      Width: px2vp(d.width), Height: px2vp(d.height),
      SafeTop: this.safeTop, SafeRight: this.safeRight, SafeBottom: this.safeBottom, SafeLeft: this.safeLeft,
      Keyboard: this.keyboard,
      PixelRatio: d.densityPixels, FontScale: config?.fontSizeScale ?? 1,
      Language: locale, OS: 'ohos',
      DeviceModel: deviceInfo.marketName || deviceInfo.productModel,
      OSMajor: deviceInfo.majorVersion, OSMinor: deviceInfo.seniorVersion,
      Dark: dark,
      Tablet: deviceInfo.deviceType === 'tablet',
      RTL: i18n.isRTL(locale), ScreenReader: screenReader,
      Lifecycle: AppStorage.get<number>('nsgoLifecycle') ?? 0,
      OpenedURL: launch.url ?? '',
      CanvasAtlas: true,
      Display: this.displayData(d)
    };
    return JSON.stringify(out);
  }

  apply(ops: string): void {
    const immersive = this.tree.sbImmersive;
    this.renderer.apply(ops);
    this.applyStatusBar();
    this.applyAppearance();
    this.applyOrientation();
    // readEnv counts the bars' areas only outside immersive mode, so the safe area is read again when it starts or ends.
    if (this.tree.sbImmersive !== immersive) {
      setTimeout(() => { this.readEnv(); }, 0);
    }
  }

  // touch reports the fingers on the surface, which the engine hit-tests
  // against its frames and hands to the views that listen for them. A
  // finger that lifted may be in the changed ones alone, so both lists
  // make up the event.
  touch(e: TouchEvent): void {
    if (this.pinchTouch(e)) {
      return;
    }
    let phase = 2;
    if (e.type === TouchType.Down) {
      phase = 1;
    } else if (e.type === TouchType.Up) {
      phase = 3;
    } else if (e.type === TouchType.Cancel) {
      phase = 4;
    }
    this.renderer.pull(e, phase);
    const moved = new Map<number, boolean>();
    const changed = e.changedTouches;
    for (let i = 0; i < changed.length; i++) {
      moved.set(changed[i].id, true);
    }
    const parts: string[] = [];
    const down = e.touches;
    for (let i = 0; i < down.length; i++) {
      const t = down[i];
      parts.push('{"i":' + t.id + ',"x":' + t.x + ',"y":' + t.y +
        ',"c":' + (moved.get(t.id) === true ? 'true' : 'false') + '}');
    }
    for (let i = 0; i < changed.length; i++) {
      const t = changed[i];
      let seen = false;
      for (let k = 0; k < down.length; k++) {
        if (down[k].id === t.id) {
          seen = true;
          break;
        }
      }
      if (!seen) {
        parts.push('{"i":' + t.id + ',"x":' + t.x + ',"y":' + t.y + ',"c":true}');
      }
    }
    this.controller.runJavaScript(`nsgoTouches(${phase},${Date.now()},'[` + parts.join(',') + `]')`);
  }

  // applyAppearance shows the app in the appearance it asks for, which the
  // system's configuration then carries back to the engine with the rest of
  // the environment.
  applyAppearance(): void {
    const mode = this.tree.appMode;
    if (mode === this.appMode) {
      return;
    }
    this.appMode = mode;
    const ctx = this.hostCtx;
    if (ctx === undefined) {
      return;
    }
    const m = mode === 1 ? ConfigurationConstant.ColorMode.COLOR_MODE_LIGHT :
      (mode === 2 ? ConfigurationConstant.ColorMode.COLOR_MODE_DARK :
        ConfigurationConstant.ColorMode.COLOR_MODE_NOT_SET);
    try {
      ctx.setColorMode(m);
    } catch (e) {
      console.error('NSGO harmony: color mode failed: ' + e);
    }
    // The bar's automatic style follows the appearance at once, and the
    // configuration the change brings follows with the rest of it.
    this.dark = mode === 2 ? true : (mode === 1 ? false : this.systemDark);
    this.applyStatusBar();
    setTimeout(() => { this.readEnv(); }, 0);
  }

  // applyStatusBar gives the window the system bars the engine asked for. Immersive hides the status bar, the navigation bar and the navigation indicator; ending it shows them again, the status bar only if the engine does not hide it. Outside immersive mode the navigation bar stays, as three-button navigation needs it. EntryAbility lays the window out full screen from the start, so the content already runs under the bars in either state; readEnv counts their areas as insets outside immersive mode and only the cutout's within it.
  applyStatusBar(): void {
    const w = this.win;
    if (w === undefined) {
      return;
    }
    const immersive = this.tree.sbImmersive;
    const names: Array<'status' | 'navigation'> = immersive ? [] : (this.tree.sbHidden ? ['navigation'] : ['status', 'navigation']);
    // A batch that carries the status bar makes a failed request again; an avoid area change or an appearance change does not.
    const retry = this.tree.sbSent;
    this.tree.sbSent = false;
    this.bars.make(names.join(','), retry, () => w.setWindowSystemBarEnable(names));
    this.indicator.make(immersive ? 'hide' : 'show', retry, () => w.setSpecificSystemBarEnabled('navigationIndicator', !immersive));
    try {
      // Auto follows the appearance, as the engine's StatusBarAuto says.
      const light = this.tree.sbStyle === 3 || (this.tree.sbStyle === 1 && this.dark);
      w.setWindowSystemBarProperties({
        statusBarColor: light ? '#000000' : '#ffffff',
        statusBarContentColor: light ? '#ffffff' : '#000000'
      });
    } catch (e) {
      console.error('NSGO harmony: status bar: ' + e);
    }
  }

  // applyOrientation asks the window for the orientations the engine's latest request allows, as Android's launcher activity does: both directions of an axis or all four rotate with the device, all four only while the rotation lock is off, and a set that names no axis whole keeps the direction shown if it may, else takes one in the same axis. Zero returns to the usual policy. The window's size change brings the new size and safe area to the engine.
  applyOrientation(): void {
    const w = this.win;
    if (w === undefined || !this.tree.orientationSent) {
      return;
    }
    this.tree.orientationSent = false;
    const mask = this.tree.orientation;
    const portrait = 1, upsideDown = 2, left = 4, right = 8;
    const bothPortrait = portrait | upsideDown, landscape = left | right;
    let o = window.Orientation.UNSPECIFIED;
    if (mask === bothPortrait) {
      o = window.Orientation.AUTO_ROTATION_PORTRAIT;
    } else if (mask === landscape) {
      o = window.Orientation.AUTO_ROTATION_LANDSCAPE;
    } else if (mask === (bothPortrait | landscape)) {
      o = window.Orientation.AUTO_ROTATION_RESTRICTED;
    } else if (mask !== 0) {
      // display.Rotation counts clockwise quarter turns: Orientation.LANDSCAPE, which puts the device's top on the left as Android's SCREEN_ORIENTATION_LANDSCAPE does, shows as the third and LANDSCAPE_INVERTED as the first.
      const rotation = display.getDefaultDisplaySync().rotation;
      const current = [portrait, right, upsideDown, left][rotation] ?? portrait;
      const sameAxis = mask & ((current & bothPortrait) !== 0 ? bothPortrait : landscape);
      const allowed = (current & mask) !== 0 ? current : (sameAxis !== 0 ? sameAxis : mask);
      const chosen = allowed & -allowed;
      o = chosen === portrait ? window.Orientation.PORTRAIT : (chosen === upsideDown ? window.Orientation.PORTRAIT_INVERTED :
        (chosen === left ? window.Orientation.LANDSCAPE : window.Orientation.LANDSCAPE_INVERTED));
    }
    w.setPreferredOrientation(o).catch((e: Error) => {
      console.error('NSGO harmony: orientation failed: ' + e.message);
    });
  }

  // frame hands the engine a display frame with when it shows, in nanoseconds on displaySync's clock.
  frame(target: number): void {
    if (this.framePending) {
      return;
    }
    this.framePending = true;
    this.controller.runJavaScript('nsgoFrame(' + target + ')').then(() => {
      this.framePending = false;
    }).catch(() => {
      this.framePending = false;
    });
  }

  // tick keeps the display clock running: each frame asks the engine for
  // one, and idle stops it once nothing animates.
  tick(): void {
    if (this.sync === undefined) {
      const sync = displaySync.create();
      sync.setExpectedFrameRateRange({ min: 30, max: 120, expected: 60 });
      sync.on('frame', (info: displaySync.IntervalInfo) => {
        this.frame(info.targetTimestamp);
      });
      this.sync = sync;
    }
    if (!this.ticking) {
      this.ticking = true;
      const sync = this.sync;
      if (sync !== undefined) {
        sync.start();
      }
    }
  }

  idle(): void {
    if (this.ticking) {
      this.ticking = false;
      const sync = this.sync;
      if (sync !== undefined) {
        sync.stop();
      }
    }
  }

  // scrolled reports where a scroll view is, as the user left it.
  scrolled(id: number, x: number, y: number): void {
    this.send('scroll', id, x, y, '');
  }

  // zoomed reports a scroll view's scale and the offset a pinch left it
  // at.
  zoomed(id: number, z: number, x: number, y: number): void {
    this.controller.runJavaScript('nsgoEvent("zoom",' + id + ',' + z + ',' + x + ',' + y + ')');
  }

  send(name: string, id: number, x: number, y: number, text: string, detail: string = ''): void {
    let js = 'nsgoEvent("' + name + '",' + id + ',' + x + ',' + y;
    if (text !== '' || detail !== '') {
      js += ',' + JSON.stringify(text);
    }
    if (detail !== '') {
      js += ',' + JSON.stringify(detail);
    }
    js += ')';
    this.controller.runJavaScript(js);
  }

  serve(url: string): WebResourceResponse {
    let rel = ROOT + 'index.html';
    const i = url.indexOf(ROOT);
    if (i >= 0) {
      rel = url.substring(i);
      const q = rel.indexOf('?');
      if (q >= 0) {
        rel = rel.substring(0, q);
      }
    }
    let type = 'text/html';
    if (rel.endsWith('.wasm')) {
      type = 'application/wasm';
    } else if (rel.endsWith('.js')) {
      type = 'application/javascript';
    } else if (rel.endsWith('.json')) {
      type = 'application/json';
    }
    const resp = new WebResourceResponse();
    resp.setResponseCode(200);
    resp.setResponseHeader([
      { headerKey: 'Content-Type', headerValue: type },
      { headerKey: 'Access-Control-Allow-Origin', headerValue: '*' }
    ]);
    if (this.res) {
      try {
        resp.setResponseData(this.res.getRawFileContentSync(rel).buffer);
      } catch (e) {
        // A file the page asks for may be absent, such as a test's or the
        // monitor's: the page reads a 404 as none.
        resp.setResponseCode(404);
        resp.setResponseHeader([{ headerKey: 'Content-Type', headerValue: 'text/plain' }]);
      }
    }
    return resp;
  }

  build() {
    Stack({ alignContent: Alignment.TopStart }) {
      NodeContainer(this.renderController)
        .id('nsgo-root')

      Web({ src: ORIGIN + ROOT + 'index.html', controller: this.controller })
        .javaScriptAccess(true)
        .domStorageAccess(true)
        .javaScriptProxy({
          object: this.bridge,
          name: 'nsgoHarmony',
          methodList: ['ready', 'submit', 'tick', 'idle', 'device', 'measure', 'perf', 'snapshot', 'scrollBy', 'typeInput', 'launch', 'sandbox', 'alert', 'backunhandled', 'clipread', 'clipset', 'clipimage', 'dismisskeyboard', 'announce', 'title', 'updatewidget', 'listdir', 'readfile', 'pickdirectories', 'imageinfo', 'openurl', 'pickimages', 'pickfiles', 'savefile', 'saved', 'openfile', 'takephoto', 'keepawake', 'releaseawake', 'bioavail', 'bioauth', 'permission', 'webcookies'],
          controller: this.controller
        })
        .onInterceptRequest((event) => {
          return this.serve(event.request.getRequestUrl());
        })
          .onConsole((event) => {
            console.info('NSGO harmony page: ' + event.message.getMessage());
            return false;
          })
        .width(1)
        .height(1)
        .position({ x: 0, y: 0 })

      if (this.perfText !== '') {
        Text(this.perfText)
          .fontSize(10)
          .fontColor('#ffffff')
          .backgroundColor('rgba(0,0,0,0.65)')
          .padding(6)
          .borderRadius(4)
          .position({ right: 8, top: 44 })
      }
    }
    .width('100%')
    .height('100%')
    .backgroundColor('#000000')
    .focusable(true)
    .onKeyEvent((e: KeyEvent) => {
      // The engine's keymap is on the Go side, so the page sends the raw
      // key and its modifiers: driver.FindKey matches it there.
      let mods = 0;
      if (e.getModifierKeyState !== undefined) {
        if (e.getModifierKeyState(['Ctrl'])) {
          mods |= 1;
        }
        if (e.getModifierKeyState(['Alt'])) {
          mods |= 2;
        }
        if (e.getModifierKeyState(['Shift'])) {
          mods |= 4;
        }
      }
      if (e.metaKey === 1) {
        mods |= 8;
      }
      this.send('key', e.keyCode, e.type === KeyType.Up ? 1 : 0, mods, e.keyText);
      return false;
    })
  }
}

// pixelMapOf makes a PixelMap of a pixel copy, w by h premultiplied RGBA in base64, or undefined when it cannot.
function pixelMapOf(w: number, h: number, data: string): image.PixelMap | undefined {
  if (w <= 0 || h <= 0 || data === '') {
    return undefined;
  }
  try {
    const bytes = new util.Base64Helper().decodeSync(data);
    const buf = bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength) as ArrayBuffer;
    return image.createPixelMapSync(buf, {
      size: { width: w, height: h },
      srcPixelFormat: image.PixelMapFormat.RGBA_8888,
      pixelFormat: image.PixelMapFormat.RGBA_8888,
      alphaType: image.AlphaType.PREMUL
    });
  } catch (e) {
    console.error('NSGO harmony: pixels failed: ' + e);
    return undefined;
  }
}

// ViewPixels is the pixel copy an image view shows and the one it showed before, released only once a newer copy replaces it, since the Image may still be drawing it.
interface ViewPixels {
  cur?: image.PixelMap;
  prev?: image.PixelMap;
}

const viewPixels: Map<number, ViewPixels> = new Map();

// setViewPixels gives a view the copy an 'ip' op sends; an empty one, or the view's deletion, releases what it held.
function setViewPixels(id: number, w: number, h: number, data: string): void {
  const held: ViewPixels = viewPixels.get(id) ?? {};
  const pm = pixelMapOf(w, h, data);
  if (pm === undefined) {
    held.prev?.release();
    held.cur?.release();
    viewPixels.delete(id);
    return;
  }
  held.prev?.release();
  held.prev = held.cur;
  held.cur = pm;
  viewPixels.set(id, held);
}

// NodeImageView draws an image the engine mounted, from the bytes it embeds
// (base64, decoded into a PixelMap), from the app's resources ("asset:",
// read out of the HAP's rawfile directory) or from the URI the app gave.
@Component
struct NodeImageView {
  @Prop @Watch('reload') src: string = '';
  @Prop @Watch('reload') fit: string = 'cover';
  @Prop @Watch('reload') data: string = '';
  @Prop nearest: boolean = false;
  // vid is the view's id and pv counts the pixel copies it was sent, which viewPixels holds.
  @Prop vid: number = 0;
  @Prop @Watch('reload') pv: number = 0;
  @State pixels: image.PixelMap | undefined = undefined;
  private showsCopy: boolean = false;

  aboutToAppear(): void {
    this.decode();
  }

  reload(): void {
    this.decode();
  }

  async decode(): Promise<void> {
    const copy = viewPixels.get(this.vid)?.cur;
    if (copy !== undefined) {
      this.pixels = copy;
      this.showsCopy = true;
      return;
    }
    if (this.showsCopy) {
      this.pixels = undefined;
      this.showsCopy = false;
    }
    if (this.data !== '') {
      try {
        const bytes = new util.Base64Helper().decodeSync(this.data);
        const source = image.createImageSource(bytes.buffer as ArrayBuffer);
        this.pixels = await source.createPixelMap();
        source.release();
      } catch (e) {
        console.error('NSGO harmony: image failed: ' + e);
      }
      return;
    }
    if (!this.src.startsWith('asset:')) {
      return;
    }
    // The build puts the app's resources beside the page, and the HAP
    // carries them in its rawfile directory, which only the host reads.
    const rel = this.src.substring('asset:'.length).replace(/^\/+/, '');
    try {
      const ctx = getContext(this) as common.UIAbilityContext;
      const bytes = ctx.resourceManager.getRawFileContentSync('web/resources/' + rel);
      const source = image.createImageSource(bytes.buffer as ArrayBuffer);
      this.pixels = await source.createPixelMap();
      source.release();
    } catch (e) {
      console.error('NSGO harmony: image ' + this.src + ' failed: ' + e);
    }
  }

  build() {
    if (this.pixels !== undefined) {
      Image(this.pixels)
        .objectFit(fitOf(this.fit))
        .interpolation(interpolationOf(this.nearest))
        .width('100%')
        .height('100%')
    } else if (this.src !== '' && !this.src.startsWith('asset:')) {
      Image(this.src)
        .objectFit(fitOf(this.fit))
        .interpolation(interpolationOf(this.nearest))
        .width('100%')
        .height('100%')
        .onError((e: ImageError) => {
          console.error('NSGO harmony: image ' + this.src + ' failed: ' +
            e.message + ' ' + e.componentWidth + 'x' + e.componentHeight);
        })
    }
  }
}


// CanvasView replays a canvas's drawing into ArkUI's Canvas. The engine
// sends the whole drawing again when it changes, and the canvas redraws.
interface CanvasImageEntry {
  waiting: Array<() => void>;
  pm?: image.PixelMap;
  bytes: number;
  used: number;
  pending: boolean;
  failed: boolean;
}

// CanvasImageCache bounds decoded canvas PixelMaps across the page by both count (64 unkept entries) and encoded-byte budget (default 32 MiB), keeping pinned (KeepImage) entries, in-flight decodes, and entries used in the current paint pass out of eviction.
class CanvasImageCache {
  private static readonly MAX_ENTRIES: number = 64;
  private entries: Map<string, CanvasImageEntry> = new Map();
  private kept: Map<string, number> = new Map();
  private limitBytes: number = 32 * 1024 * 1024;
  private use: number = 0;
  private pass: number = 0;

  keep(key: string): void {
    if (key === '') {
      return;
    }
    this.kept.set(key, (this.kept.get(key) ?? 0) + 1);
  }

  release(key: string): void {
    if (key === '') {
      return;
    }
    const n = this.kept.get(key) ?? 0;
    if (n <= 0) {
      return;
    }
    if (n === 1) {
      this.kept.delete(key);
    } else {
      this.kept.set(key, n - 1);
    }
    this.pass = this.use;
    this.evict(0);
  }

  setLimit(maxBytes: number): void {
    this.limitBytes = maxBytes < 0 ? 0 : maxBytes;
    this.pass = this.use;
    this.evict(0);
  }

  endPass(): void {
    this.pass = this.use;
    if (this.limitBytes === 0) {
      this.evict(0);
    }
  }

  isKept(key: string): boolean {
    return (this.kept.get(key) ?? 0) > 0;
  }

  get(key: string): image.PixelMap | undefined {
    const e = this.entries.get(key);
    if (e === undefined) {
      return undefined;
    }
    this.use++;
    e.used = this.use;
    return e.pm;
  }

  has(key: string): boolean {
    return this.entries.has(key);
  }

  addPending(key: string): void {
    this.use++;
    const reserve = this.isKept(key) ? 0 : 1;
    this.evict(reserve);
    this.entries.set(key, { waiting: [], bytes: 0, used: this.use, pending: true, failed: false });
  }

  wait(key: string, redraw: () => void): void {
    const e = this.entries.get(key);
    if (e !== undefined && e.pending && e.waiting.indexOf(redraw) < 0) {
      e.waiting.push(redraw);
    }
  }

  complete(key: string, pm: image.PixelMap, bytes: number): void {
    const e = this.entries.get(key);
    if (e === undefined) {
      pm.release();
      return;
    }
    e.pm = pm;
    e.bytes = bytes < 0 ? 0 : bytes;
    e.pending = false;
    e.failed = false;
    const waiting = e.waiting;
    e.waiting = [];
    waiting.forEach((redraw) => { redraw(); });
    if (this.unkeptBytes() > this.limitBytes || this.limitBytes === 0) {
      this.evict(0);
    }
  }

  fail(key: string): void {
    const e = this.entries.get(key);
    if (e === undefined) {
      return;
    }
    e.pending = false;
    e.failed = true;
    e.waiting = [];
    if (this.limitBytes === 0) {
      this.evict(0);
    }
  }

  private unkeptCount(): number {
    let n = 0;
    this.entries.forEach((_e, k) => {
      if (!this.isKept(k)) {
        n++;
      }
    });
    return n;
  }

  private unkeptBytes(): number {
    let total = 0;
    this.entries.forEach((e, k) => {
      if (!this.isKept(k)) {
        total += e.bytes;
      }
    });
    return total;
  }

  private evict(reserve: number): void {
    let unkept = this.unkeptCount();
    let bytes = this.unkeptBytes();
    while (unkept + reserve > CanvasImageCache.MAX_ENTRIES || bytes > this.limitBytes || (this.limitBytes === 0 && unkept > 0)) {
      let oldestKey = '';
      let oldestUse = Number.MAX_SAFE_INTEGER;
      this.entries.forEach((e, k) => {
        if (!this.isKept(k) && !e.pending && e.used < oldestUse) {
          oldestKey = k;
          oldestUse = e.used;
        }
      });
      if (oldestKey === '' || oldestUse > this.pass) {
        break;
      }
      const old = this.entries.get(oldestKey);
      if (old !== undefined) {
        if (old.pm !== undefined) {
          old.pm.release();
        }
        bytes -= old.bytes;
      }
      this.entries.delete(oldestKey);
      unkept--;
    }
  }
}

const canvasImageCache: CanvasImageCache = new CanvasImageCache();

@Component
struct CanvasView {
  @Prop @Watch('paint') drawing: string = '';
  private ctx: CanvasRenderingContext2D = new CanvasRenderingContext2D(new RenderingContextSettings(true));
  private ready: boolean = false;
  private redraw: () => void = () => { this.paint(); };
  // tinted holds each image's tinted copies by tint. A copy no paint drew for two paints is released; one the last paint drew is kept, since the Canvas may still read it.
  private tinted: Map<image.PixelMap, Map<number, TintedImage>> = new Map();
  private paints: number = 0;

  aboutToAppear(): void {
  }

  aboutToDisappear(): void {
    this.ready = false;
    this.releaseTints(this.paints + 1);
  }

  // roundRect adds a rounded rectangle to the current path.
  private roundRect(x: number, y: number, w: number, h: number, r: number): void {
    const ctx = this.ctx;
    const rr = Math.min(r, Math.min(w, h) / 2);
    ctx.moveTo(x + rr, y);
    ctx.lineTo(x + w - rr, y);
    ctx.quadraticCurveTo(x + w, y, x + w, y + rr);
    ctx.lineTo(x + w, y + h - rr);
    ctx.quadraticCurveTo(x + w, y + h, x + w - rr, y + h);
    ctx.lineTo(x + rr, y + h);
    ctx.quadraticCurveTo(x, y + h, x, y + h - rr);
    ctx.lineTo(x, y + rr);
    ctx.quadraticCurveTo(x, y, x + rr, y);
    ctx.closePath();
  }

  // tintedImage multiplies the source's RGBA pixels in a separate canvas, reusing the copy for that image and tint while paints draw it.
  private tintedImage(pm: image.PixelMap, tint: number): image.PixelMap {
    let byTint = this.tinted.get(pm);
    if (byTint === undefined) {
      byTint = new Map<number, TintedImage>();
      this.tinted.set(pm, byTint);
    }
    const cached = byTint.get(tint);
    if (cached !== undefined) {
      cached.paint = this.paints;
      return cached.pm;
    }
    const size = pm.getImageInfoSync().size;
    const ctx = new OffscreenCanvasRenderingContext2D(size.width, size.height,
      new RenderingContextSettings(true), LengthMetricsUnit.PX);
    ctx.drawImage(pm, 0, 0, size.width, size.height);
    const pixels = ctx.getImageData(0, 0, size.width, size.height);
    const r = tint >>> 24;
    const g = (tint >>> 16) & 255;
    const b = (tint >>> 8) & 255;
    const a = tint & 255;
    for (let i = 0; i < pixels.data.length; i += 4) {
      pixels.data[i] = Math.round(pixels.data[i] * r / 255);
      pixels.data[i + 1] = Math.round(pixels.data[i + 1] * g / 255);
      pixels.data[i + 2] = Math.round(pixels.data[i + 2] * b / 255);
      pixels.data[i + 3] = Math.round(pixels.data[i + 3] * a / 255);
    }
    ctx.putImageData(pixels, 0, 0);
    const tinted = ctx.getPixelMap(0, 0, size.width, size.height);
    const entry: TintedImage = { pm: tinted, paint: this.paints };
    byTint.set(tint, entry);
    return tinted;
  }

  // releaseTints releases the tinted copies no paint since the paint before drew.
  private releaseTints(before: number): void {
    this.tinted.forEach((byTint: Map<number, TintedImage>, src: image.PixelMap) => {
      byTint.forEach((t: TintedImage, tint: number) => {
        if (t.paint < before) {
          t.pm.release();
          byTint.delete(tint);
        }
      });
      if (byTint.size === 0) {
        this.tinted.delete(src);
      }
    });
  }

  // drawImage draws an image op: at its natural size when W and H are 0, else into that rectangle, from the part X1, Y1, X2, Y2 of it when the source rectangle is given, smoothed unless the op samples nearest. The op is in points and the pixels are not.
  private drawImage(ctx: CanvasRenderingContext2D, pm: image.PixelMap, o: CanvasOp): void {
    ctx.imageSmoothingEnabled = !(o.n ?? false);
    const d = density();
    const size = pm.getImageInfoSync().size;
    const nw = size.width / d;
    const nh = size.height / d;
    const w = o.w ?? 0;
    const h = o.h ?? 0;
    const dw = w === 0 && h === 0 ? nw : w;
    const dh = w === 0 && h === 0 ? nh : h;
    const x = o.x ?? 0;
    const y = o.y ?? 0;
    if ((o.x2 ?? 0) === 0 && (o.y2 ?? 0) === 0) {
      ctx.drawImage(pm, x, y, dw, dh);
      return;
    }
    ctx.drawImage(pm, (o.x1 ?? 0) * d, (o.y1 ?? 0) * d, (o.x2 ?? 0) * d, (o.y2 ?? 0) * d, x, y, dw, dh);
  }

  // drawAtlas draws an atlas op's sprites from one image, nine numbers
  // each: the source rectangle in the image's points, the scaled rotation
  // and translation, and the opacity factor of the current alpha.
  private drawAtlas(ctx: CanvasRenderingContext2D, pm: image.PixelMap, sp: number[]): void {
    const d = density();
    const alpha = ctx.globalAlpha;
    for (let i = 0; i + 9 <= sp.length; i += 9) {
      const sx = sp[i] * d;
      const sy = sp[i + 1] * d;
      const w = sp[i + 2];
      const h = sp[i + 3];
      const c = sp[i + 4];
      const s = sp[i + 5];
      ctx.globalAlpha = alpha * sp[i + 8];
      if (s === 0 && c > 0) {
        ctx.drawImage(pm, sx, sy, w * d, h * d, sp[i + 6], sp[i + 7], w * c, h * c);
        continue;
      }
      ctx.save();
      ctx.transform(c, s, -s, c, sp[i + 6], sp[i + 7]);
      ctx.drawImage(pm, sx, sy, w * d, h * d, 0, 0, w, h);
      ctx.restore();
    }
    ctx.globalAlpha = alpha;
  }

  // drawVertices draws a mesh op's triangles, x, y, u, v at each corner, each clipped to itself and drawn from the image through the affine transform its corners make. A triangle whose texture coordinates meet at one point fills with that texel, and one whose coordinates lie on a line samples a quarter of an image pixel off it. An opaque triangle grows by half a device pixel along the edges it shares with another opaque one, the bits of `e`, so neighbors leave no seam; the mesh's outline and translucent triangles keep to their corners, since a strip drawn twice would show. A Path2D clips, so the drawing's own path is kept. Vertex colors play only as each triangle's mean alpha.
  private drawVertices(ctx: CanvasRenderingContext2D, pm: image.PixelMap, o: CanvasOp): void {
    const v = o.v ?? [];
    const alphas = o.va ?? [];
    const seams = o.e ?? [];
    const d = density();
    const size = pm.getImageInfoSync().size;
    const nw = size.width / d;
    const nh = size.height / d;
    const m = ctx.getTransform();
    const scale = Math.sqrt(Math.abs((m.scaleX ?? 1) * (m.scaleY ?? 1) - (m.rotateX ?? 0) * (m.rotateY ?? 0)));
    const grow = scale > 0 ? 0.5 / (d * scale) : 0;
    const alpha = ctx.globalAlpha;
    const xs: number[] = [0, 0, 0];
    const ys: number[] = [0, 0, 0];
    const us: number[] = [0, 0, 0];
    const ts: number[] = [0, 0, 0];
    for (let i = 0; i + 11 < v.length; i += 12) {
      for (let k = 0; k < 3; k++) {
        xs[k] = v[i + 4 * k];
        ys[k] = v[i + 4 * k + 1];
        us[k] = v[i + 4 * k + 2];
        ts[k] = v[i + 4 * k + 3];
      }
      const a = alpha * (alphas[i / 12] ?? 1);
      ctx.save();
      ctx.clip(this.meshClip(xs, ys, a >= 1 ? (seams[i / 12] ?? 0) : 0, grow));
      ctx.globalAlpha = a;
      if (us[0] === us[1] && us[1] === us[2] && ts[0] === ts[1] && ts[1] === ts[2]) {
        const sx = Math.min(Math.max(Math.floor(us[0] * d), 0), size.width - 1);
        const sy = Math.min(Math.max(Math.floor(ts[0] * d), 0), size.height - 1);
        const pad = 4 * grow;
        const x = Math.min(xs[0], xs[1], xs[2]) - pad;
        const y = Math.min(ys[0], ys[1], ys[2]) - pad;
        ctx.imageSmoothingEnabled = false;
        ctx.drawImage(pm, sx, sy, 1, 1, x, y, Math.max(xs[0], xs[1], xs[2]) + pad - x, Math.max(ys[0], ys[1], ys[2]) + pad - y);
        ctx.restore();
        continue;
      }
      let du1 = us[1] - us[0], dt1 = ts[1] - ts[0], du2 = us[2] - us[0], dt2 = ts[2] - ts[0];
      let det = du1 * dt2 - du2 * dt1;
      if (det === 0) {
        // The corner between the other two moves off their line, so the transform is invertible.
        let k = 0;
        let far = -1;
        for (let j = 0; j < 3; j++) {
          const dist = Math.hypot(us[(j + 2) % 3] - us[(j + 1) % 3], ts[(j + 2) % 3] - ts[(j + 1) % 3]);
          if (dist > far) {
            far = dist;
            k = j;
          }
        }
        const p = (k + 1) % 3;
        const q = (k + 2) % 3;
        const off = 0.25 / d / far;
        us[k] -= (ts[q] - ts[p]) * off;
        ts[k] += (us[q] - us[p]) * off;
        du1 = us[1] - us[0];
        dt1 = ts[1] - ts[0];
        du2 = us[2] - us[0];
        dt2 = ts[2] - ts[0];
        det = du1 * dt2 - du2 * dt1;
      }
      const dx1 = xs[1] - xs[0], dy1 = ys[1] - ys[0], dx2 = xs[2] - xs[0], dy2 = ys[2] - ys[0];
      const ma = (dx1 * dt2 - dx2 * dt1) / det;
      const mb = (dy1 * dt2 - dy2 * dt1) / det;
      const mc = (dx2 * du1 - dx1 * du2) / det;
      const md = (dy2 * du1 - dy1 * du2) / det;
      ctx.imageSmoothingEnabled = o.n !== true;
      ctx.transform(ma, mb, mc, md, xs[0] - ma * us[0] - mc * ts[0], ys[0] - mb * us[0] - md * ts[0]);
      ctx.drawImage(pm, 0, 0, nw, nh);
      ctx.restore();
    }
  }

  // meshClip is a triangle's path with each edge from corner k to the next moved out by grow where bit k of mask is set. Each corner moves to where its two edges' lines meet, at most four times grow on a sliver.
  private meshClip(xs: number[], ys: number[], mask: number, grow: number): Path2D {
    const area = (xs[1] - xs[0]) * (ys[2] - ys[0]) - (xs[2] - xs[0]) * (ys[1] - ys[0]);
    const sign: number = area > 0 ? 1 : -1;
    const nx: number[] = [0, 0, 0];
    const ny: number[] = [0, 0, 0];
    const off: number[] = [0, 0, 0];
    for (let k = 0; k < 3; k++) {
      const j = (k + 1) % 3;
      const ex = xs[j] - xs[k];
      const ey = ys[j] - ys[k];
      const len = Math.hypot(ex, ey);
      if (len > 0) {
        nx[k] = sign * ey / len;
        ny[k] = -sign * ex / len;
      }
      off[k] = ((mask >> k) & 1) !== 0 ? grow : 0;
    }
    const limit = 4 * grow;
    const path = new Path2D();
    for (let k = 0; k < 3; k++) {
      const p = (k + 2) % 3;
      let px = xs[k];
      let py = ys[k];
      const det = nx[p] * ny[k] - ny[p] * nx[k];
      if ((off[p] !== 0 || off[k] !== 0) && det !== 0) {
        let dx = (off[p] * ny[k] - off[k] * ny[p]) / det;
        let dy = (nx[p] * off[k] - nx[k] * off[p]) / det;
        const len = Math.hypot(dx, dy);
        if (len > limit) {
          dx *= limit / len;
          dy *= limit / len;
        }
        px += dx;
        py += dy;
      }
      if (k === 0) {
        path.moveTo(px, py);
      } else {
        path.lineTo(px, py);
      }
    }
    path.closePath();
    return path;
  }

  // loadImage decodes an image op once; a repaint draws it when it comes.
  private loadImage(key: string, uri: string, data: string): void {
    if (canvasImageCache.has(key)) {
      canvasImageCache.wait(key, this.redraw);
      return;
    }
    if (data === '' && !uri.startsWith('asset:')) {
      canvasImageCache.addPending(key);
      canvasImageCache.fail(key);
      console.warn('NSGO harmony: canvas image ' + uri + ' is not drawn: a URL is not loaded here');
      return;
    }
    canvasImageCache.addPending(key);
    canvasImageCache.wait(key, this.redraw);
    this.decodeImage(key, uri, data);
  }

  private async decodeImage(key: string, uri: string, data: string): Promise<void> {
    try {
      let bytes: Uint8Array = new Uint8Array(0);
      if (data !== '') {
        bytes = new util.Base64Helper().decodeSync(data);
      } else {
        const rel = uri.substring('asset:'.length).replace(/^\/+/, '');
        const ctx = getContext(this) as common.UIAbilityContext;
        bytes = ctx.resourceManager.getRawFileContentSync('web/resources/' + rel);
      }
      const source = image.createImageSource(bytes.buffer as ArrayBuffer);
      const pm = await source.createPixelMap();
      source.release();
      canvasImageCache.complete(key, pm, bytes.byteLength);
    } catch (e) {
      canvasImageCache.fail(key);
      console.error('NSGO harmony: canvas image failed: ' + e);
    }
  }

  // gradientOf builds a canvas gradient, or undefined when it is conic.
  private gradientOf(g: CanvasGradientData): CanvasGradient | undefined {
    const ctx = this.ctx;
    let grad: CanvasGradient;
    if (g.radial === true) {
      grad = ctx.createRadialGradient(g.x1 ?? 0, g.y1 ?? 0, g.r1 ?? 0, g.x2 ?? 0, g.y2 ?? 0, g.r2 ?? 0);
    } else {
      grad = ctx.createLinearGradient(g.x1 ?? 0, g.y1 ?? 0, g.x2 ?? 0, g.y2 ?? 0);
    }
    const stops = g.s ?? [];
    for (let i = 0; i < stops.length; i++) {
      grad.addColorStop(stops[i].o, stops[i].c);
    }
    return grad;
  }

  paint(): void {
    if (!this.ready || this.drawing === '') {
      return;
    }
    const ctx = this.ctx;
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.globalAlpha = 1;
    ctx.lineWidth = 1;
    ctx.lineCap = 'butt';
    ctx.lineJoin = 'miter';
    ctx.miterLimit = 10;
    ctx.setLineDash([]);
    ctx.lineDashOffset = 0;
    ctx.shadowColor = 'rgba(0,0,0,0)';
    ctx.shadowBlur = 0;
    ctx.shadowOffsetX = 0;
    ctx.shadowOffsetY = 0;
    ctx.globalCompositeOperation = 'source-over';
    ctx.imageSmoothingEnabled = true;
    ctx.clearRect(0, 0, 100000, 100000);
    let ops: CanvasOp[] = [];
    try {
      ops = JSON.parse(this.drawing) as CanvasOp[];
    } catch (e) {
      console.error('NSGO harmony: canvas failed: ' + e);
      return;
    }
    this.paints++;
    const caps: CanvasLineCap[] = ['butt', 'round', 'square'];
    const joins: CanvasLineJoin[] = ['miter', 'round', 'bevel'];
    const comps: string[] = ['source-over', 'source-in', 'source-out', 'source-atop',
      'destination-over', 'destination-in', 'destination-out', 'destination-atop',
      'lighter', 'copy', 'xor', 'multiply', 'screen', 'overlay', 'darken', 'lighten',
      'color-dodge', 'color-burn', 'hard-light', 'soft-light'];
    let fill: string | CanvasGradient = '#000000';
    let stroke: string | CanvasGradient = '#000000';
    for (let i = 0; i < ops.length; i++) {
      const o = ops[i];
      switch (o.k) {
        case 1:
          ctx.save();
          break;
        case 2:
          ctx.restore();
          break;
        case 3:
          ctx.translate(o.x ?? 0, o.y ?? 0);
          break;
        case 4:
          ctx.scale(o.x ?? 1, o.y ?? 1);
          break;
        case 5:
          ctx.rotate(o.a ?? 0);
          break;
        case 6:
          ctx.beginPath();
          break;
        case 7:
          ctx.moveTo(o.x ?? 0, o.y ?? 0);
          break;
        case 8:
          ctx.lineTo(o.x ?? 0, o.y ?? 0);
          break;
        case 9:
          ctx.quadraticCurveTo(o.x1 ?? 0, o.y1 ?? 0, o.x ?? 0, o.y ?? 0);
          break;
        case 10:
          ctx.bezierCurveTo(o.x1 ?? 0, o.y1 ?? 0, o.x2 ?? 0, o.y2 ?? 0, o.x ?? 0, o.y ?? 0);
          break;
        case 11:
          ctx.arc(o.x ?? 0, o.y ?? 0, o.r ?? 0, o.a0 ?? 0, o.a1 ?? 0, o.ccw === true);
          break;
        case 12:
          ctx.rect(o.x ?? 0, o.y ?? 0, o.w ?? 0, o.h ?? 0);
          break;
        case 13:
          this.roundRect(o.x ?? 0, o.y ?? 0, o.w ?? 0, o.h ?? 0, o.r ?? 0);
          break;
        case 14:
          ctx.ellipse(o.x ?? 0, o.y ?? 0, o.r ?? 0, o.ry ?? 0, 0, 0, Math.PI * 2);
          break;
        case 15:
          ctx.closePath();
          break;
        case 16:
          ctx.fill(o.eo === true ? 'evenodd' : 'nonzero');
          break;
        case 17:
          ctx.stroke();
          break;
        case 18:
          ctx.clip(o.eo === true ? 'evenodd' : 'nonzero');
          break;
        case 19:
          fill = o.c ?? '#000000';
          ctx.fillStyle = fill;
          break;
        case 20:
          stroke = o.c ?? '#000000';
          ctx.strokeStyle = stroke;
          break;
        case 21:
          ctx.lineWidth = o.w ?? 1;
          break;
        case 22:
          ctx.lineCap = caps[(o.cap ?? 1) - 1] ?? 'butt';
          break;
        case 23:
          ctx.lineJoin = joins[(o.join ?? 1) - 1] ?? 'miter';
          break;
        case 24:
          ctx.globalAlpha = o.a ?? 1;
          break;
        case 25:
        case 36:
          ctx.font = (o.fi === true ? 'italic ' : '') + (o.fw ?? 400) + ' ' +
            (o.fs ?? 16) + 'px ' + (o.ff !== undefined && o.ff !== '' ? o.ff : 'sans-serif');
          if (o.k === 25) {
            ctx.fillStyle = fill;
            ctx.fillText(o.t ?? '', (o.x ?? 0) + (o.x1 ?? 0), (o.y ?? 0) + (o.y1 ?? 0));
          } else {
            ctx.strokeStyle = stroke;
            ctx.strokeText(o.t ?? '', (o.x ?? 0) + (o.x1 ?? 0), (o.y ?? 0) + (o.y1 ?? 0));
          }
          break;
        case 26:
          ctx.clearRect(o.x ?? 0, o.y ?? 0, o.w ?? 0, o.h ?? 0);
          break;
        case 27:
          if (o.g !== undefined) {
            const g = this.gradientOf(o.g);
            if (g !== undefined) {
              fill = g;
              ctx.fillStyle = g;
            }
          }
          break;
        case 28:
          if (o.g !== undefined) {
            const g = this.gradientOf(o.g);
            if (g !== undefined) {
              stroke = g;
              ctx.strokeStyle = g;
            }
          }
          break;
        case 29:
          ctx.transform(o.x1 ?? 1, o.y1 ?? 0, o.x2 ?? 0, o.y2 ?? 1, o.x ?? 0, o.y ?? 0);
          break;
        case 30:
          ctx.setTransform(o.x1 ?? 1, o.y1 ?? 0, o.x2 ?? 0, o.y2 ?? 1, o.x ?? 0, o.y ?? 0);
          break;
        case 31:
          ctx.globalCompositeOperation = comps[(o.co ?? 1) - 1];
          break;
        case 32:
          ctx.setLineDash(o.d ?? []);
          break;
        case 33:
          ctx.lineDashOffset = o.w ?? 0;
          break;
        case 34:
          ctx.miterLimit = o.w ?? 10;
          break;
        case 35:
          ctx.shadowColor = o.c ?? 'rgba(0,0,0,0)';
          ctx.shadowOffsetX = o.x ?? 0;
          ctx.shadowOffsetY = o.y ?? 0;
          ctx.shadowBlur = o.r ?? 0;
          break;
        case 37:
        case 40:
        case 41: {
          const uri = o.img ?? '';
          const data = o.imgd ?? '';
          const key = o.imgk ?? (data !== '' ? data : uri);
          if (key === '') {
            break;
          }
          if (o.pxd !== undefined && !canvasImageCache.has(key)) {
            const copy = pixelMapOf(o.pw ?? 0, o.ph ?? 0, o.pxd);
            if (copy !== undefined) {
              canvasImageCache.addPending(key);
              canvasImageCache.complete(key, copy, (o.pw ?? 0) * (o.ph ?? 0) * 4);
            }
          }
          const pm = canvasImageCache.get(key);
          if (pm === undefined) {
            // A pixel copy comes with its drawing's bytes, so there is nothing to load.
            if (o.pxd === undefined && !key.startsWith('px:')) {
              this.loadImage(key, uri, data);
            }
            break;
          }
          if (o.k === 41) {
            ctx.imageSmoothingEnabled = !(o.n ?? false);
            this.drawAtlas(ctx, pm, o.sp ?? []);
          } else if (o.k === 40) {
            this.drawVertices(ctx, pm, o);
          } else {
            const src = o.tint === undefined || o.tint === 0xffffffff ? pm : this.tintedImage(pm, o.tint);
            this.drawImage(ctx, src, o);
          }
          break;
        }
      }
    }
    canvasImageCache.endPass();
    this.releaseTints(this.paints - 1);
  }

  build() {
    Canvas(this.ctx)
      .width('100%')
      .height('100%')
      .onReady(() => {
        this.ready = true;
        this.paint();
      })
  }
}


// nsgoHost is the page a WebView reports to, set as it appears.
let nsgoHost: Index | undefined = undefined;


// VideoView draws a video's frame and controls, and reports where it is.
@Component
struct VideoView {
  v: VNode = new VNode();
  @State private duration: number = 0;

  build() {
    Video({ src: this.v.videoSrc, controller: this.v.videoController })
      .muted(this.v.videoMuted)
      .controls(this.v.videoControls)
      .loop(this.v.videoRepeat)
      .autoPlay(!this.v.videoPaused)
      .objectFit(fitOf(this.v.videoFit))
      .width('100%')
      .height('100%')
      .onPrepared((e: PreparedInfo) => {
        this.duration = e.duration;
        const host = nsgoHost;
        if (host !== undefined) {
          host.send('videoload', this.v.id, e.duration, 0, '');
        }
      })
      .onUpdate((e: PlaybackInfo) => {
        const host = nsgoHost;
        if (host !== undefined) {
          host.send('videoprogress', this.v.id, e.time, this.duration, '');
        }
      })
      .onFinish(() => {
        const host = nsgoHost;
        if (host !== undefined) {
          host.send('videoend', this.v.id, 0, 0, '');
        }
      })
      .onError(() => {
        const host = nsgoHost;
        if (host !== undefined) {
          host.send('videoerror', this.v.id, 0, 0, 'playback failed');
        }
      })
  }
}
interface WebCmd {
  cmd: number;
  value: string;
}

interface WebPendingMedia {
  request: PermissionRequest;
  want: number;
}

interface WebDownloadMeta {
  id: string;
  url: string;
  filename: string;
  path: string;
}

interface WebPendingLoad {
  url: string;
  isHTML: boolean;
}

interface WebDownloadPayload {
  state: number;
  id: string;
  url: string;
  filename: string;
  path: string;
  bytes: number;
  message: string;
}

let webTokenSerial: number = 0;
let webDownloadSerial: number = 0;

// WebMessageProxy receives window.nsgo.postMessage calls from the page's top
// frame, verified against the WebViewState's per-instance token.
class WebMessageProxy {
  private state: WebViewState;

  constructor(state: WebViewState) {
    this.state = state;
  }

  post(token: string, data: string): void {
    this.state.handleMessage(token, data);
  }
}

// WebViewState holds the live ArkWeb controller, applied settings, navigation
// state, active downloads, and pending media permission requests for one WebView.
class WebViewState {
  id: number;
  controller: webview.WebviewController = new webview.WebviewController();
  token: string;
  attached: boolean = false;
  closed: boolean = false;
  defaultUA: string = '';
  appliedUA: string = '';
  appliedMuted: boolean = false;
  appliedNoScroll: boolean = false;
  loading: boolean = false;
  currentURL: string = '';
  currentTitle: string = '';
  loadedHTMLNoBase: boolean = false;
  pendingAppLoad: WebPendingLoad | undefined = undefined;
  allowAppHistory: boolean = false;
  pendingMedia: WebPendingMedia[] = [];
  activeDownloads: Map<string, webview.WebDownloadItem> = new Map();
  downloadMeta: Map<string, WebDownloadMeta> = new Map();
  proxy: WebMessageProxy;
  delegate: webview.WebDownloadDelegate = new webview.WebDownloadDelegate();

  constructor(id: number) {
    this.id = id;
    webTokenSerial++;
    this.token = 'nsgo-' + webTokenSerial + '-' + Date.now().toString(36);
    this.proxy = new WebMessageProxy(this);
  }

  currentHost(): Index | undefined {
    if (this.closed) {
      return undefined;
    }
    const host = nsgoHost;
    if (host !== undefined && host.tree.at(this.id)?.webState === this) {
      return host;
    }
    return undefined;
  }

  navFlags(loading: boolean): number {
    let flags = 0;
    try {
      if (this.controller.accessBackward()) {
        flags |= 1;
      }
      if (this.controller.accessForward()) {
        flags |= 2;
      }
    } catch (_e) {
    }
    if (loading) {
      flags |= 4;
    }
    return flags;
  }

  effectiveURL(): string {
    if (this.currentURL !== '') {
      return this.currentURL;
    }
    try {
      const raw = this.controller.getUrl();
      return normalizeReportedWebURL(raw, this.loadedHTMLNoBase);
    } catch (_e) {
      return '';
    }
  }

  handleMessage(token: string, data: string): void {
    if (this.closed || token !== this.token) {
      return;
    }
    const host = this.currentHost();
    if (host === undefined) {
      return;
    }
    host.send('webmessage', this.id, 0, 0, data, this.effectiveURL());
  }

  retireMedia(): void {
    const list = this.pendingMedia;
    this.pendingMedia = [];
    for (let i = 0; i < list.length; i++) {
      try {
        list[i].request.deny();
      } catch (_e) {
      }
    }
  }

  retireMediaDisallowed(allowedMedia: number): void {
    const keep: WebPendingMedia[] = [];
    for (let i = 0; i < this.pendingMedia.length; i++) {
      const item = this.pendingMedia[i];
      if ((allowedMedia & item.want) !== item.want) {
        try {
          item.request.deny();
        } catch (_e) {
        }
      } else {
        keep.push(item);
      }
    }
    this.pendingMedia = keep;
  }

  handlePermission(v: VNode, host: Index, req: PermissionRequest): void {
    if (this.closed) {
      req.deny();
      return;
    }
    const info = webMediaWant(req.getAccessibleResource());
    if (info.want === 0 || (v.webMedia & info.want) !== info.want) {
      req.deny();
      return;
    }
    const perms: Permissions[] = info.perms as Permissions[];
    if (host.hasWebPermissions(perms)) {
      req.grant(info.resources);
      return;
    }
    const pending: WebPendingMedia = { request: req, want: info.want };
    this.pendingMedia.push(pending);
    host.requestWebPermissions(perms, (granted: boolean) => {
      const idx = this.pendingMedia.indexOf(pending);
      if (idx < 0) {
        return;
      }
      this.pendingMedia.splice(idx, 1);
      const current = this.currentHost();
      const node = current?.tree.at(this.id);
      if (granted && !this.closed && node !== undefined && (node.webMedia & pending.want) === pending.want) {
        try {
          req.grant(info.resources);
        } catch (_e) {
        }
      } else {
        try {
          req.deny();
        } catch (_e) {
        }
      }
    });
  }

  reportDownload(host: Index, payload: WebDownloadPayload): void {
    if (this.closed) {
      return;
    }
    host.send('webdownload', this.id, 0, 0, JSON.stringify(payload));
  }

  installDownloadDelegate(): void {
    this.delegate.onBeforeDownload((item: webview.WebDownloadItem) => {
      const host = this.currentHost();
      const v = host?.tree.at(this.id);
      if (host === undefined || v === undefined || v.webDownloadDir === '') {
        try {
          item.cancel();
        } catch (_e) {
        }
        return;
      }
      webDownloadSerial++;
      const dlID = Date.now().toString(36) + '-' + webDownloadSerial.toString(36);
      const url = item.getUrl();
      const filename = sanitizeDownloadName(item.getSuggestedFileName());
      const dir = v.webDownloadDir + '/' + dlID;
      const dest = dir + '/' + filename;
      const guid = item.getGuid();
      try {
        fs.mkdirSync(dir, true);
      } catch (e) {
        this.reportDownload(host, {
          state: 2,
          id: dlID,
          url: url,
          filename: filename,
          path: dest,
          bytes: 0,
          message: 'Cannot create download directory: ' + e
        });
        try {
          item.cancel();
        } catch (_e) {
        }
        return;
      }
      this.activeDownloads.set(guid, item);
      this.downloadMeta.set(guid, { id: dlID, url: url, filename: filename, path: dest });
      this.reportDownload(host, {
        state: 0,
        id: dlID,
        url: url,
        filename: filename,
        path: dest,
        bytes: 0,
        message: ''
      });
      try {
        item.start(dest);
      } catch (e) {
        this.activeDownloads.delete(guid);
        this.downloadMeta.delete(guid);
        this.reportDownload(host, {
          state: 2,
          id: dlID,
          url: url,
          filename: filename,
          path: dest,
          bytes: 0,
          message: '' + e
        });
      }
    });
    this.delegate.onDownloadFinish((item: webview.WebDownloadItem) => {
      const guid = item.getGuid();
      this.activeDownloads.delete(guid);
      const meta = this.downloadMeta.get(guid);
      this.downloadMeta.delete(guid);
      const host = this.currentHost();
      if (host === undefined || meta === undefined) {
        return;
      }
      this.reportDownload(host, {
        state: 1,
        id: meta.id,
        url: meta.url,
        filename: meta.filename,
        path: item.getFullPath() !== '' ? item.getFullPath() : meta.path,
        bytes: item.getReceivedBytes(),
        message: ''
      });
    });
    this.delegate.onDownloadFailed((item: webview.WebDownloadItem) => {
      const guid = item.getGuid();
      this.activeDownloads.delete(guid);
      const meta = this.downloadMeta.get(guid);
      this.downloadMeta.delete(guid);
      const host = this.currentHost();
      if (host === undefined || meta === undefined) {
        return;
      }
      this.reportDownload(host, {
        state: 2,
        id: meta.id,
        url: meta.url,
        filename: meta.filename,
        path: meta.path,
        bytes: item.getReceivedBytes(),
        message: 'download failed (code ' + item.getLastErrorCode() + ')'
      });
    });
    try {
      this.controller.setDownloadDelegate(this.delegate);
    } catch (e) {
      console.error('NSGO harmony: setDownloadDelegate: ' + e);
    }
  }

  applyLiveSettings(v: VNode, force: boolean = false): void {
    this.retireMediaDisallowed(v.webMedia);
    if (!this.attached || this.closed) {
      return;
    }
    const wantUA = resolveWebUserAgent(this.defaultUA, v.webUA, v.webUASuffix);
    if (force || wantUA !== this.appliedUA) {
      this.appliedUA = wantUA;
      try {
        this.controller.setCustomUserAgent(wantUA);
      } catch (e) {
        console.error('NSGO harmony: setCustomUserAgent: ' + e);
      }
    }
    if (force || v.webMuted !== this.appliedMuted) {
      this.appliedMuted = v.webMuted;
      try {
        this.controller.setAudioMuted(v.webMuted);
      } catch (e) {
        console.error('NSGO harmony: setAudioMuted: ' + e);
      }
    }
    if (force || v.webNoScroll !== this.appliedNoScroll) {
      this.appliedNoScroll = v.webNoScroll;
      try {
        this.controller.setScrollable(!v.webNoScroll);
      } catch (e) {
        console.error('NSGO harmony: setScrollable: ' + e);
      }
    }
  }

  loadSource(v: VNode, src: string, html: string, baseURL: string): void {
    if (!this.attached || this.closed) {
      return;
    }
    this.retireMedia();
    this.loadedHTMLNoBase = html !== '' && baseURL === '';
    try {
      if (html !== '') {
        this.pendingAppLoad = { url: baseURL !== '' ? baseURL : 'data:', isHTML: true };
        if (baseURL !== '') {
          this.controller.loadData(html, 'text/html', 'UTF-8', baseURL, baseURL);
        } else {
          this.controller.loadData(encodeURIComponent(html), 'text/html', 'UTF-8');
        }
      } else if (src !== '') {
        this.pendingAppLoad = { url: src, isHTML: false };
        if (v.webHeaders.length > 0) {
          const headers: Array<webview.WebHeader> = [];
          for (let i = 0; i < v.webHeaders.length; i++) {
            headers.push({ headerKey: v.webHeaders[i].k, headerValue: v.webHeaders[i].v });
          }
          this.controller.loadUrl(src, headers);
        } else {
          this.controller.loadUrl(src);
        }
      }
    } catch (e) {
      console.error('NSGO harmony: web load: ' + e);
    }
  }

  drainCommands(v: VNode, updateLeaf: () => void): void {
    if (!this.attached || this.closed || v.webCmds.length === 0) {
      return;
    }
    const cmds = v.webCmds;
    v.webCmds = [];
    for (let i = 0; i < cmds.length; i++) {
      const item = cmds[i];
      try {
        if (item.cmd === 1) {
          this.allowAppHistory = true;
          this.controller.refresh();
        } else if (item.cmd === 2) {
          if (this.controller.accessBackward()) {
            this.allowAppHistory = true;
            this.controller.backward();
          }
        } else if (item.cmd === 3) {
          if (this.controller.accessForward()) {
            this.allowAppHistory = true;
            this.controller.forward();
          }
        } else if (item.cmd === 4) {
          this.controller.stop();
        } else if (item.cmd === 5 || item.cmd === 6) {
          this.controller.runJavaScript(item.value);
        } else if (item.cmd === 8) {
          const load: WebLoadOp = parseWebLoadOp(item.value);
          const loadSrc = load.src ?? '';
          const loadHTML = load.html ?? '';
          const loadBase = load.base ?? '';
          const needsFile = webSourceNeedsFileAccess(loadSrc, loadHTML, loadBase);
          if (needsFile !== v.webFileAccess) {
            v.webFileAccess = needsFile;
            updateLeaf();
          }
          this.loadSource(v, loadSrc, loadHTML, loadBase);
        }
      } catch (e) {
        console.error('NSGO harmony: web command ' + item.cmd + ': ' + e);
      }
    }
  }

  close(): void {
    this.closed = true;
    this.retireMedia();
    this.activeDownloads.forEach((item: webview.WebDownloadItem) => {
      try {
        item.cancel();
      } catch (_e) {
      }
    });
    this.activeDownloads.clear();
    this.downloadMeta.clear();
  }
}

function webPropsKey(v: VNode): string {
  return JSON.stringify([v.webNoScroll, v.webPinchZoom, v.webFileAccess]);
}

function noWebController(): webview.WebviewController | null {
  return null;
}

// WebView draws a web view's page: the engine gives it its source, headers,
// user agent, injected scripts, media/navigation/download policy, and commands.
@Component
struct WebView {
  v: VNode = new VNode();
  state: WebViewState | undefined = undefined;
  host: Index | undefined = undefined;
  @Prop noScroll: boolean = false;
  @Prop pinchZoom: boolean = false;
  @Prop fileAccess: boolean = false;

  startScripts(state: WebViewState): ScriptItem[] {
    const list = webDocumentStartScripts(this.v.noJS, state.token, this.v.webBefore);
    const out: ScriptItem[] = [];
    for (let i = 0; i < list.length; i++) {
      out.push({ script: list[i], scriptRules: ['*'] });
    }
    return out;
  }

  endScripts(): ScriptItem[] {
    const list = webDocumentEndScripts(this.v.noJS, this.v.webAfter);
    const out: ScriptItem[] = [];
    for (let i = 0; i < list.length; i++) {
      out.push({ script: list[i], scriptRules: ['*'] });
    }
    return out;
  }

  build() {
    if (this.state !== undefined) {
      Web({ src: '', controller: this.state.controller, incognitoMode: this.v.webIncognito })
        .width('100%')
        .height('100%')
        .javaScriptAccess(!this.v.noJS)
        .domStorageAccess(true)
        .fileAccess(this.fileAccess)
        .zoomAccess(this.pinchZoom)
        .pinchSmooth(this.pinchZoom)
        .verticalScrollBarAccess(!this.noScroll)
        .horizontalScrollBarAccess(!this.noScroll)
        .multiWindowAccess(true)
        .allowWindowOpenMethod(true)
        .javaScriptProxy({
          object: this.state.proxy,
          name: '__nsgoWebBridge',
          methodList: ['post'],
          controller: this.state.controller
        })
        .javaScriptOnDocumentStart(this.startScripts(this.state))
        .javaScriptOnDocumentEnd(this.endScripts())
        .onControllerAttached(() => {
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined) {
            return;
          }
          try {
            st.defaultUA = st.controller.getUserAgent();
          } catch (_e) {
            st.defaultUA = '';
          }
          st.attached = true;
          st.installDownloadDelegate();
          st.applyLiveSettings(this.v, true);
          st.loadSource(this.v, this.v.webSrc, this.v.webHTML, this.v.webBaseURL);
          st.drainCommands(this.v, () => {});
        })
        .onLoadIntercept((e: OnLoadInterceptEvent): boolean => {
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined) {
            return false;
          }
          const req = e.data;
          if (!req.isMainFrame()) {
            return false;
          }
          const url = req.getRequestUrl();
          let isAppSource = false;
          const pending = st.pendingAppLoad;
          if (!req.isRedirect() && pending !== undefined) {
            const matched = pending.isHTML
              ? (url === pending.url || url.toLowerCase().startsWith('data:'))
              : url === pending.url;
            if (matched) {
              st.pendingAppLoad = undefined;
              isAppSource = true;
            }
          }
          if (!isAppSource && !req.isRedirect() && st.allowAppHistory) {
            st.allowAppHistory = false;
            isAppSource = true;
          }
          if (!isAppSource) {
            st.pendingAppLoad = undefined;
          }
          if (!webNavigable(this.v.webRestricted, this.v.webHosts, this.v.webSchemes, this.v.webAppSchemes, url, isAppSource)) {
            host.send('weberror', this.v.id, 0, 0, url, refusedNavigationMessage(url));
            return true;
          }
          if (!isAppSource || !(this.v.webHTML !== '' && this.v.webBaseURL === '')) {
            if (!url.toLowerCase().startsWith('data:')) {
              st.loadedHTMLNoBase = false;
            }
          }
          return false;
        })
        .onPageBegin((e: OnPageBeginEvent) => {
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined) {
            return;
          }
          st.retireMedia();
          st.loading = true;
          const url = normalizeReportedWebURL(e.url, st.loadedHTMLNoBase);
          st.currentURL = url;
          host.send('webloadstart', this.v.id, st.navFlags(false), 0, url, st.currentTitle);
        })
        .onPageEnd((e: OnPageEndEvent) => {
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined) {
            return;
          }
          st.loading = false;
          const url = normalizeReportedWebURL(e.url, st.loadedHTMLNoBase);
          st.currentURL = url;
          let title = st.currentTitle;
          try {
            title = st.controller.getTitle();
          } catch (_e) {
          }
          st.currentTitle = title;
          host.send('webload', this.v.id, st.navFlags(false), 0, url, title);
        })
        .onTitleReceive((e: OnTitleReceiveEvent) => {
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined) {
            return;
          }
          st.currentTitle = e.title;
          host.send('webnav', this.v.id, st.navFlags(st.loading), 0, st.effectiveURL(), e.title);
        })
        .onNavigationEntryCommitted((e: LoadCommittedDetails) => {
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined || !e.isMainFrame) {
            return;
          }
          const url = normalizeReportedWebURL(e.url, st.loadedHTMLNoBase);
          st.currentURL = url;
          if (e.isSameDocument) {
            host.send('webnav', this.v.id, st.navFlags(st.loading), 0, url, st.currentTitle);
          }
        })
        .onErrorReceive((e: OnErrorReceiveEvent) => {
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined) {
            return;
          }
          const req = e.request;
          const err = e.error;
          if (!webShouldReportError(req.isMainFrame(), err.getErrorCode())) {
            return;
          }
          host.send('weberror', this.v.id, err.getErrorCode(), 1, req.getRequestUrl(), err.getErrorInfo());
        })
        .onWindowNew((e: OnWindowNewEvent) => {
          e.handler.setWebController(noWebController() as webview.WebviewController);
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined) {
            return;
          }
          host.send('webopenwindow', this.v.id, e.isUserTrigger ? 1 : 0, 0, e.targetUrl);
        })
        .onPermissionRequest((e: OnPermissionRequestEvent) => {
          const st = this.state;
          const host = st?.currentHost();
          if (st === undefined || host === undefined) {
            e.request.deny();
            return;
          }
          st.handlePermission(this.v, host, e.request);
        })
    }
  }
}

// The builders for the leaves typeNode cannot make: rich text (Span), an
// image drawn from its bytes, and a canvas.
@Builder function spanBuilder(v: VNode) {
  SpanTextView({ runs: v.runs, textAlignment: v.align, maxLines: v.maxLines })
}

// Native layout is available after the frame's layout task. Redraw once there rather than measuring the span again or moving its glyphs.
class SpanBackgroundFrame extends FrameCallback {
  private paint: () => void;

  constructor(paint: () => void) {
    super();
    this.paint = paint;
  }

  onIdle(_time: number): void {
    this.paint();
  }
}

// SpanTextView paints each native line's band below the Text component, keeping selection and glyphs above it. Neither Canvas nor the bands take input or change the text's layout.
@Component
struct SpanTextView {
  @Prop @Watch('schedulePaint') runs: TextRun[] = [];
  @Prop @Watch('schedulePaint') textAlignment: number = 0;
  @Prop @Watch('schedulePaint') maxLines: number = 0;
  private controller: TextController = new TextController();
  private ctx: CanvasRenderingContext2D = new CanvasRenderingContext2D(new RenderingContextSettings(true));
  private ready: boolean = false;
  private pending: boolean = false;

  private hasBackground(): boolean {
    return this.runs.some((r: TextRun) => r.b !== undefined);
  }

  aboutToDisappear(): void {
    this.ready = false;
  }

  private schedulePaint(): void {
    if (!this.ready || this.pending) {
      return;
    }
    this.pending = true;
    this.getUIContext().postFrameCallback(new SpanBackgroundFrame(() => {
      this.pending = false;
      if (this.ready) {
        this.paint();
      }
    }));
  }

  private paint(): void {
    const ctx = this.ctx;
    ctx.clearRect(0, 0, ctx.width, ctx.height);
    const layout = this.controller.getLayoutManager();
    if (layout === undefined) {
      return;
    }
    let value = '';
    this.runs.forEach((r: TextRun) => { value += r.t; });
    let start = 0;
    const count = Math.min(layout.getLineCount(), this.maxLines > 0 ? this.maxLines : 100);
    for (let i = 0; i < this.runs.length; i++) {
      const run = this.runs[i];
      const end = start + run.t.length;
      const bg = run.b;
      if (bg !== undefined) {
        for (let line = 0; line < count; line++) {
          const metrics = layout.getLineMetrics(line);
          const range = spanLineRange(value, metrics.startIndex, metrics.endIndex, start, end);
          if (range === undefined) {
            continue;
          }
          const boxes = layout.getRectsForRange(range, text.RectWidthStyle.TIGHT, text.RectHeightStyle.TIGHT);
          const rects = boxes.map((box: text.TextBox): SpanRectangle => {
            return { left: box.rect.left, top: box.rect.top, right: box.rect.right, bottom: box.rect.bottom };
          });
          joinSpanBoxes(rects).forEach((rect: SpanRectangle) => {
            const ui = this.getUIContext();
            const band = backgroundBand(ui.px2vp(rect.left), ui.px2vp(rect.top),
              ui.px2vp(rect.right), ui.px2vp(rect.bottom), bg);
            if (band === undefined) {
              return;
            }
            const x = band.x;
            const y = band.y;
            const w = band.width;
            const h = band.height;
            const r = band.radius;
            ctx.beginPath();
            ctx.moveTo(x + r, y);
            ctx.lineTo(x + w - r, y);
            ctx.quadraticCurveTo(x + w, y, x + w, y + r);
            ctx.lineTo(x + w, y + h - r);
            ctx.quadraticCurveTo(x + w, y + h, x + w - r, y + h);
            ctx.lineTo(x + r, y + h);
            ctx.quadraticCurveTo(x, y + h, x, y + h - r);
            ctx.lineTo(x, y + r);
            ctx.quadraticCurveTo(x, y, x + r, y);
            ctx.closePath();
            ctx.fillStyle = band.color;
            ctx.fill();
          });
        }
      }
      start = end;
    }
  }

  build() {
    Stack({ alignContent: Alignment.TopStart }) {
      if (this.hasBackground()) {
        Canvas(this.ctx)
          .width('100%')
          .height('100%')
          .hitTestBehavior(HitTestMode.None)
          .onReady(() => {
            this.ready = true;
            this.schedulePaint();
          })
          .onAreaChange(() => { this.schedulePaint(); })
          .onDisAppear(() => { this.ready = false; })
      }
      Text(undefined, { controller: this.controller }) {
        // Key by paint too so same-size text and background changes rebuild.
        ForEach(this.runs, (r: TextRun) => {
          Span(r.t)
            .fontSize(r.s ?? 16)
            .fontWeight(r.w ?? 400)
            .fontColor(r.c ?? '#000000')
            .fontFamily(r.f ?? '')
            .fontStyle(r.i === true ? FontStyle.Italic : FontStyle.Normal)
        }, (r: TextRun, i: number) => i + ':' + JSON.stringify(r))
      }
      .textAlign(this.textAlignment === 1 ? TextAlign.Center :
        (this.textAlignment === 2 ? TextAlign.End : TextAlign.Start))
      .maxLines(this.maxLines > 0 ? this.maxLines : 100)
      .textOverflow({ overflow: TextOverflow.Ellipsis })
      .width('100%')
      .onAreaChange(() => { this.schedulePaint(); })
    }
    .width('100%')
    .height('100%')
  }
}

@Builder function imageBuilder(v: VNode) {
  NodeImageView({ src: v.src, fit: v.fit, data: v.data, nearest: v.nearest, vid: v.id, pv: v.pv })
}

@Builder function canvasBuilder(v: VNode) {
  CanvasView({ drawing: v.drawing })
}

@Builder function webBuilder(v: VNode) {
  WebView({
    v: v,
    state: v.webState,
    host: nsgoHost,
    noScroll: v.webNoScroll,
    pinchZoom: v.webPinchZoom,
    fileAccess: v.webFileAccess
  })
}

// HeaderView draws a screen's system header: the bar with its title, and a
// back button to the screen below. The header covers the bar and the part
// of the status bar the screen reaches under, so its background fills that
// whole area and the bar sits at its bottom.
@Component
struct HeaderView {
  v: VNode = new VNode();

  build() {
    Stack({ alignContent: Alignment.TopStart }) {
      Stack({ alignContent: Alignment.Center }) {
        Text(this.v.scrTitle)
          .fontSize(17)
          .fontWeight(FontWeight.Medium)
          .fontColor(this.v.scrTitleColor === '' ? '#000000' : this.v.scrTitleColor)
          .maxLines(1)
          .textOverflow({ overflow: TextOverflow.Ellipsis })
          .width('60%')
          .textAlign(TextAlign.Center)
        if (this.v.scrCanGoBack) {
          Row() {
            Text('‹').fontSize(28).fontColor(this.v.scrTintColor === '' ? '#007dff' : this.v.scrTintColor)
            Text(this.v.scrBackTitle)
              .fontSize(17)
              .fontColor(this.v.scrTintColor === '' ? '#007dff' : this.v.scrTintColor)
          }
          .position({ x: 8, y: 0 })
          .height('100%')
          .onClick(() => {
            const host = nsgoHost;
            if (host !== undefined) {
              host.send('back', this.v.id, 0, 0, '');
            }
          })
        }
      }
      .width('100%')
      .height(44)
      .position({ x: 0, y: this.v.scrHeight > 44 ? this.v.scrHeight - 44 : 0 })
    }
    .width('100%')
    .height(this.v.scrHeight > 0 ? this.v.scrHeight : 44)
    .backgroundColor(this.v.scrTransparent ? Color.Transparent :
      (this.v.scrBg === '' ? '#f7f7f7' : this.v.scrBg))
  }
}

@Builder function videoBuilder(v: VNode) {
  VideoView({ v: v })
}

@Builder function headerBuilder(v: VNode) {
  HeaderView({ v: v })
}

// TabIconView draws a 24x24 tab bar SVG icon replaying CanvasOp[] tinted in
// the tab's active or idle color, matching iOS template images and Android's
// tinted ImageView without extra PixelMap allocations.
@Component
struct TabIconView {
  @Prop @Watch('paint') drawing: string = '';
  @Prop @Watch('paint') color: string = '#000000';
  private ctx: CanvasRenderingContext2D = new CanvasRenderingContext2D(new RenderingContextSettings(true));
  private ready: boolean = false;
  private cachedSrc: string = '';
  private cachedOps: CanvasOp[] = [];

  aboutToDisappear(): void {
    this.ready = false;
  }

  private roundRect(x: number, y: number, w: number, h: number, r: number): void {
    const ctx = this.ctx;
    const rr = Math.min(r, Math.min(w, h) / 2);
    ctx.moveTo(x + rr, y);
    ctx.lineTo(x + w - rr, y);
    ctx.quadraticCurveTo(x + w, y, x + w, y + rr);
    ctx.lineTo(x + w, y + h - rr);
    ctx.quadraticCurveTo(x + w, y + h, x + w - rr, y + h);
    ctx.lineTo(x + rr, y + h);
    ctx.quadraticCurveTo(x, y + h, x, y + h - rr);
    ctx.lineTo(x, y + rr);
    ctx.quadraticCurveTo(x, y, x + rr, y);
    ctx.closePath();
  }

  private paint(): void {
    if (!this.ready || this.drawing === '') {
      return;
    }
    if (this.drawing !== this.cachedSrc) {
      this.cachedSrc = this.drawing;
      try {
        this.cachedOps = JSON.parse(this.drawing) as CanvasOp[];
      } catch (e) {
        console.error('NSGO harmony: tab icon failed: ' + e);
        this.cachedOps = [];
        return;
      }
    }
    const ctx = this.ctx;
    ctx.setTransform(1, 0, 0, 1, 0, 0);
    ctx.globalAlpha = 1;
    ctx.lineWidth = 1;
    ctx.lineCap = 'butt';
    ctx.lineJoin = 'miter';
    ctx.miterLimit = 10;
    ctx.setLineDash([]);
    ctx.lineDashOffset = 0;
    ctx.clearRect(0, 0, 24, 24);
    ctx.fillStyle = this.color;
    ctx.strokeStyle = this.color;
    const caps: CanvasLineCap[] = ['butt', 'round', 'square'];
    const joins: CanvasLineJoin[] = ['miter', 'round', 'bevel'];
    const ops = this.cachedOps;
    for (let i = 0; i < ops.length; i++) {
      const o = ops[i];
      switch (o.k) {
        case 1:
          ctx.save();
          break;
        case 2:
          ctx.restore();
          break;
        case 3:
          ctx.translate(o.x ?? 0, o.y ?? 0);
          break;
        case 4:
          ctx.scale(o.x ?? 1, o.y ?? 1);
          break;
        case 5:
          ctx.rotate(o.a ?? 0);
          break;
        case 6:
          ctx.beginPath();
          break;
        case 7:
          ctx.moveTo(o.x ?? 0, o.y ?? 0);
          break;
        case 8:
          ctx.lineTo(o.x ?? 0, o.y ?? 0);
          break;
        case 9:
          ctx.quadraticCurveTo(o.x1 ?? 0, o.y1 ?? 0, o.x ?? 0, o.y ?? 0);
          break;
        case 10:
          ctx.bezierCurveTo(o.x1 ?? 0, o.y1 ?? 0, o.x2 ?? 0, o.y2 ?? 0, o.x ?? 0, o.y ?? 0);
          break;
        case 11:
          ctx.arc(o.x ?? 0, o.y ?? 0, o.r ?? 0, o.a0 ?? 0, o.a1 ?? 0, o.ccw === true);
          break;
        case 12:
          ctx.rect(o.x ?? 0, o.y ?? 0, o.w ?? 0, o.h ?? 0);
          break;
        case 13:
          this.roundRect(o.x ?? 0, o.y ?? 0, o.w ?? 0, o.h ?? 0, o.r ?? 0);
          break;
        case 14:
          ctx.ellipse(o.x ?? 0, o.y ?? 0, o.r ?? 0, o.ry ?? 0, 0, 0, Math.PI * 2);
          break;
        case 15:
          ctx.closePath();
          break;
        case 16:
          ctx.fill(o.eo === true ? 'evenodd' : 'nonzero');
          break;
        case 17:
          ctx.stroke();
          break;
        case 18:
          ctx.clip(o.eo === true ? 'evenodd' : 'nonzero');
          break;
        case 19:
          ctx.fillStyle = this.color;
          break;
        case 20:
          ctx.strokeStyle = this.color;
          break;
        case 21:
          ctx.lineWidth = o.w ?? 1;
          break;
        case 22:
          ctx.lineCap = caps[(o.cap ?? 1) - 1] ?? 'butt';
          break;
        case 23:
          ctx.lineJoin = joins[(o.join ?? 1) - 1] ?? 'miter';
          break;
        case 24:
          ctx.globalAlpha = o.a ?? 1;
          break;
        case 26:
          ctx.clearRect(o.x ?? 0, o.y ?? 0, o.w ?? 0, o.h ?? 0);
          break;
        case 29:
          ctx.transform(o.x1 ?? 1, o.y1 ?? 0, o.x2 ?? 0, o.y2 ?? 1, o.x ?? 0, o.y ?? 0);
          break;
        case 30:
          ctx.setTransform(o.x1 ?? 1, o.y1 ?? 0, o.x2 ?? 0, o.y2 ?? 1, o.x ?? 0, o.y ?? 0);
          break;
        case 32:
          ctx.setLineDash(o.d ?? []);
          break;
        case 33:
          ctx.lineDashOffset = o.w ?? 0;
          break;
        case 34:
          ctx.miterLimit = o.w ?? 10;
          break;
      }
    }
  }

  build() {
    Canvas(this.ctx)
      .width(24)
      .height(24)
      .onReady(() => {
        this.ready = true;
        this.paint();
      })
  }
}

// TabBarView renders core.TabBar with ArkUI's native Tabs, TabContent and
// Badge components and reports tab taps back to Go as "tabselect".
@Component
struct TabBarView {
  @Prop vid: number = 0;
  @Prop @Watch('onSpecChange') spec: string = '';
  @State private items: TabBarItemSpec[] = [];
  @State private badges: TabBarBadgeSpec[] = [];
  @State private selected: number = 0;
  @State private edge: string = 'bottom';
  @State private barLabel: string = '';
  @State private tint: string = '';
  @State private bg: string = '';
  @State private inactive: string = '';
  @State private indicator: string = '';
  @State private activeContent: string = '';
  @State private badgeColor: string = '';
  @State private dark: boolean = false;
  @State private insetBottom: number = 0;
  @State private insetLeft: number = 0;
  @State private insetRight: number = 0;
  private controller: TabsController = new TabsController();

  aboutToAppear(): void {
    this.onSpecChange();
  }

  private onSpecChange(): void {
    if (this.spec === '') {
      return;
    }
    try {
      const parsed = JSON.parse(this.spec) as TabBarSpec;
      this.items = parsed.items ?? [];
      this.badges = parsed.badges ?? [];
      this.selected = parsed.selected ?? 0;
      this.edge = parsed.edge !== undefined && parsed.edge !== '' ? parsed.edge : 'bottom';
      this.barLabel = parsed.label ?? '';
      this.tint = parsed.tint ?? '';
      this.bg = parsed.bg ?? '';
      this.inactive = parsed.inactive ?? '';
      this.indicator = parsed.indicator ?? '';
      this.activeContent = parsed.active ?? '';
      this.badgeColor = parsed.badgeColor ?? '';
      this.dark = parsed.dark === true;
      this.insetBottom = parsed.insetBottom ?? 0;
      this.insetLeft = parsed.insetLeft ?? 0;
      this.insetRight = parsed.insetRight ?? 0;
      if (this.items.length > 0 && this.selected >= 0 && this.selected < this.items.length) {
        try {
          this.controller.changeIndex(this.selected);
        } catch (_e) {
          // Controller attaches when Tabs builds on the first render.
        }
      }
    } catch (e) {
      console.error('NSGO harmony: tabbar failed: ' + e);
    }
  }

  private isVertical(): boolean {
    return this.edge === 'leading' || this.edge === 'trailing';
  }

  private barPos(): BarPosition {
    return this.edge === 'leading' ? BarPosition.Start : BarPosition.End;
  }

  private tabsIndex(): number {
    return this.selected >= 0 && this.selected < this.items.length ? this.selected : 0;
  }

  private bgColor(): string {
    if (this.bg !== '') {
      return this.bg;
    }
    return this.dark ? '#1c1c1e' : '#f1f3f5';
  }

  private dividerColor(): string {
    return this.dark ? '#2c2c2e' : '#e5e5ea';
  }

  private activeColor(): string {
    if (this.activeContent !== '') {
      return this.activeContent;
    }
    if (this.tint !== '') {
      return this.tint;
    }
    return this.dark ? '#317af7' : '#0a59f7';
  }

  private idleColor(): string {
    if (this.inactive !== '') {
      return this.inactive;
    }
    return this.dark ? '#99ffffff' : '#99182431';
  }

  private itemColor(idx: number): string {
    return idx === this.selected ? this.activeColor() : this.idleColor();
  }

  private itemPillColor(idx: number): ResourceColor {
    if (idx === this.selected && this.indicator !== '') {
      return this.indicator;
    }
    return Color.Transparent;
  }

  private resolvedBadgeColor(): string {
    return this.badgeColor !== '' ? this.badgeColor : '#e84026';
  }

  private badgeAt(idx: number): TabBarBadgeSpec | undefined {
    return idx >= 0 && idx < this.badges.length ? this.badges[idx] : undefined;
  }

  private hasTextBadge(idx: number): boolean {
    const b = this.badgeAt(idx);
    return b !== undefined && b.t !== undefined && b.t !== '';
  }

  private hasDotBadge(idx: number): boolean {
    const b = this.badgeAt(idx);
    return b !== undefined && (b.t === undefined || b.t === '') && b.d === true;
  }

  private badgeText(idx: number): string {
    const b = this.badgeAt(idx);
    return b !== undefined && b.t !== undefined ? b.t : '';
  }

  private tabAccessibility(item: TabBarItemSpec, idx: number): string {
    const b = this.badgeAt(idx);
    if (b !== undefined && b.a !== undefined && b.a !== '') {
      return item.t + ', ' + b.a;
    }
    return item.t;
  }

  @Builder
  tabLabel(item: TabBarItemSpec, idx: number) {
    Text(item.t)
      .fontSize(10)
      .fontWeight(idx === this.selected ? FontWeight.Medium : FontWeight.Normal)
      .fontColor(this.itemColor(idx))
      .maxLines(1)
      .textOverflow({ overflow: TextOverflow.Ellipsis })
  }

  @Builder
  tabBadgedIcon(item: TabBarItemSpec, idx: number) {
    if (this.hasTextBadge(idx)) {
      Badge({
        value: this.badgeText(idx),
        position: BadgePosition.RightTop,
        style: {
          badgeColor: this.resolvedBadgeColor(),
          color: '#ffffff',
          fontSize: 10,
          badgeSize: 16
        }
      }) {
        TabIconView({ drawing: item.d ?? '', color: this.itemColor(idx) })
      }
    } else if (this.hasDotBadge(idx)) {
      Badge({
        value: '',
        position: BadgePosition.RightTop,
        style: {
          badgeColor: this.resolvedBadgeColor(),
          badgeSize: 6
        }
      }) {
        TabIconView({ drawing: item.d ?? '', color: this.itemColor(idx) })
      }
    } else {
      TabIconView({ drawing: item.d ?? '', color: this.itemColor(idx) })
    }
  }

  @Builder
  tabBadgedLabel(item: TabBarItemSpec, idx: number) {
    if (this.hasTextBadge(idx)) {
      Badge({
        value: this.badgeText(idx),
        position: BadgePosition.RightTop,
        style: {
          badgeColor: this.resolvedBadgeColor(),
          color: '#ffffff',
          fontSize: 10,
          badgeSize: 16
        }
      }) {
        this.tabLabel(item, idx)
      }
    } else if (this.hasDotBadge(idx)) {
      Badge({
        value: '',
        position: BadgePosition.RightTop,
        style: {
          badgeColor: this.resolvedBadgeColor(),
          badgeSize: 6
        }
      }) {
        this.tabLabel(item, idx)
      }
    } else {
      this.tabLabel(item, idx)
    }
  }

  @Builder
  tabBarItem(item: TabBarItemSpec, idx: number) {
    Column({ space: 2 }) {
      if (item.d !== undefined && item.d !== '') {
        Stack({ alignContent: Alignment.Center }) {
          this.tabBadgedIcon(item, idx)
        }
        .width(48)
        .height(28)
        .borderRadius(14)
        .backgroundColor(this.itemPillColor(idx))
        this.tabLabel(item, idx)
      } else {
        this.tabBadgedLabel(item, idx)
      }
    }
    .width('100%')
    .height('100%')
    .justifyContent(FlexAlign.Center)
    .alignItems(HorizontalAlign.Center)
    .accessibilityText(this.tabAccessibility(item, idx))
  }

  build() {
    Stack({ alignContent: Alignment.TopStart }) {
      if (this.items.length > 0) {
        Tabs({ barPosition: this.barPos(), index: this.tabsIndex(), controller: this.controller }) {
          ForEach(this.items, (item: TabBarItemSpec, idx: number) => {
            TabContent() {
            }
            .tabBar(this.tabBarItem(item, idx))
          }, (item: TabBarItemSpec, idx: number) => idx + ':' + item.t + ':' + (item.d ?? '').length)
        }
        .vertical(this.isVertical())
        .barMode(BarMode.Fixed)
        .barWidth('100%')
        .barHeight('100%')
        .barBackgroundColor(Color.Transparent)
        .scrollable(false)
        .animationDuration(0)
        .width('100%')
        .height('100%')
        .onTabBarClick((index: number) => {
          this.selected = index;
          const host = nsgoHost;
          if (host !== undefined) {
            host.send('tabselect', this.vid, index, 0, '');
          }
        })
      }
    }
    .width('100%')
    .height('100%')
    .backgroundColor(this.bgColor())
    .border({
      width: this.isVertical()
        ? (this.edge === 'leading' ? { right: 0.5 } : { left: 0.5 })
        : { top: 0.5 },
      color: this.dividerColor()
    })
    .padding({ bottom: this.insetBottom, left: this.insetLeft, right: this.insetRight })
    .accessibilityText(this.barLabel)
  }
}

@Builder function tabBarBuilder(v: VNode) {
  TabBarView({ vid: v.id, spec: v.tabBar })
}

// density is the screen's pixels per vp.
function density(): number {
  if (DENSITY <= 0) {
    DENSITY = display.getDefaultDisplaySync().densityPixels;
  }
  return DENSITY;
}

// layoutCurve maps a driver.LayoutCurve to ArkUI's, whose imperative
// animation takes one of its curves rather than the driver's spring or
// bezier.
function layoutCurve(c: number): Curve {
  if (c === 0) {
    return Curve.Linear;
  }
  if (c === 1) {
    return Curve.EaseIn;
  }
  if (c === 2) {
    return Curve.EaseOut;
  }
  if (c === 4) {
    return Curve.Friction;
  }
  return Curve.EaseInOut;
}

// Renderer mounts the engine's views on FrameNodes, one node per view: a
// container is a Stack, a scroll is a Scroll around a content Stack, and a
// text, input or image is the ArkUI component of its kind. A batch changes
// the nodes it names, so no statement reads a batch version any more.
class Renderer {
  host: Index | undefined = undefined;
  ui: UIContext | undefined = undefined;
  root: FrameNode | null = null;
  nodes: Map<number, FrameNode> = new Map();
  contents: Map<number, FrameNode> = new Map();
  scrollers: Map<number, Scroller> = new Map();
  leaves: Map<number, BuilderNode<[VNode]>> = new Map();
  // webStates are the web views' live states (controllers, applied settings, downloads, media permissions).
  webStates: Map<number, WebViewState> = new Map();
  webShown: Map<number, string> = new Map();
  // webHooksShown are the hooks each web view's ArkWeb view was made with (webHooksKey).
  webHooksShown: Map<number, string> = new Map();
  webPropsShown: Map<number, string> = new Map();
  videoShown: Map<number, string> = new Map();
  // headers are the system headers the screens draw, and their builders.
  headers: Map<number, FrameNode> = new Map();
  headerBuilders: Map<number, BuilderNode<[VNode]>> = new Map();
  // controllers are the text fields' TextInputControllers, which select.
  controllers: Map<number, TextInputController> = new Map();
  // shown is the text or value a node was last given, so a batch that did
  // not change it does not reset a field's caret.
  shown: Map<number, string> = new Map();
  // scrollStates are the ScrollState each scroll view was last in, so its
  // transitions say when a drag began, glided and came to rest.
  scrollStates: Map<number, number> = new Map();
  // pullId is the refreshable scroll view a finger is pulling down, and
  // pullFrom where it started.
  pullId: number = 0;
  pullFrom: number = 0;
  // cellShifts are the sticky cells' travel from their frames.
  cellShifts: Map<number, StickyShift> = new Map();
  // safeTop is the status bar's height; the tree is drawn below it.
  safeTop: number = 0;
  // applied is the scroll offset a scroll node was last given.
  applied: Map<number, string> = new Map();
  // zoomShown is the scale a zoomable scroll view's content was last
  // given.
  zoomShown: Map<number, number> = new Map();
  // styled is what style last wrote to each view's node.
  styled: Map<number, Styled> = new Map();

  attach(ui: UIContext, root: FrameNode): void {
    this.ui = ui;
    this.root = root;
    this.sync();
  }

  // apply puts a batch into the tree and draws it. A frame that fails keeps
  // what it drew last and says so, since the next batch is the engine's
  // chance to set it right.
  apply(ops: string): void {
    const host = this.host;
    if (host === undefined) {
      return;
    }
    try {
      host.tree.add(ops);
      const gone = host.tree.takeGone();
      for (let i = 0; i < gone.length; i++) {
        this.forget(gone[i]);
      }
      const structural = host.tree.takeStructural();
      const geometry = host.tree.takeGeometry();
      if (geometry) {
        host.tree.measure();
      }
      const parents = host.tree.takeParents();
      if (host.tree.takeRootChanged()) {
        // A new root is drawn from scratch.
        host.tree.takeDirty();
        this.sync(true);
      } else {
        if (structural) {
          // A structural batch may name only deleted views, which leave no dirty node to walk from: sync the children of every parent the batch changed, so their ArkUI children are dropped, added and ordered. Views the batch did not name keep what they were given; writing every view again cost hundreds of milliseconds a navigation on a phone. Removals go first, so a view moved to another parent has left the old one before the new one takes it.
          const synced = new Set<number>();
          const targets: VNode[] = [];
          const holders: FrameNode[] = [];
          for (let i = 0; i < parents.length; i++) {
            const id = parents[i];
            if (synced.has(id)) {
              continue;
            }
            synced.add(id);
            const v = host.tree.at(id);
            const n = this.nodes.get(id);
            if (v === undefined || n === undefined) {
              continue;
            }
            const holder = v.kind === KIND_SCROLL ? this.contents.get(id) : n;
            if (holder !== undefined) {
              this.pruneChildren(v, holder);
              targets.push(v);
              holders.push(holder);
            }
          }
          for (let i = 0; i < targets.length; i++) {
            this.syncChildren(targets[i], holders[i]);
          }
        }
        // Write the views the batch named, and no others; a view made above was written as it was made.
        const dirty = host.tree.takeDirty();
        for (let i = 0; i < dirty.length; i++) {
          const v = host.tree.at(dirty[i]);
          const n = this.nodes.get(dirty[i]);
          if (v !== undefined && n !== undefined && v.dirty) {
            v.dirty = false;
            this.update(v, n);
          }
        }
      }
      const zoomed = host.tree.takeZoomed();
      for (let i = 0; i < zoomed.length; i++) {
        const v = host.tree.at(zoomed[i]);
        if (v !== undefined) {
          this.applyZoom(v);
        }
      }
    } catch (e) {
      console.error('NSGO harmony: apply failed: ' + JSON.stringify(e));
    }
  }

  // sync walks the tree, full when a structural batch asks: the ordinary
  // path writes only views the batch named.
  sync(full: boolean = false): void {
    const host = this.host;
    if (host === undefined || this.root === null || host.tree.root < 0) {
      return;
    }
    this.syncNode(host.tree.root, this.root, full);
  }

  // forget drops what the renderer holds for a view the batch deleted,
  // so an id the engine gives a new view next starts clean.
  forget(id: number): void {
    this.nodes.delete(id);
    this.styled.delete(id);
    this.contents.delete(id);
    this.scrollers.delete(id);
    this.leaves.delete(id);
    this.webStates.get(id)?.close();
    this.webStates.delete(id);
    this.webShown.delete(id);
    this.webHooksShown.delete(id);
    this.webPropsShown.delete(id);
    this.videoShown.delete(id);
    this.headers.delete(id);
    this.headerBuilders.delete(id);
    this.controllers.delete(id);
    this.shown.delete(id);
    this.scrollStates.delete(id);
    this.cellShifts.delete(id);
    this.applied.delete(id);
  }

  // create makes the node for a view: its kind decides whether ArkUI's
  // component, a Stack, or a BuilderNode draws it.
  create(v: VNode): FrameNode | null {
    const ui = this.ui;
    if (ui === undefined) {
      return null;
    }
    if (v.kind === KIND_SCROLL) {
      const s = typeNode.createNode(ui, 'Scroll');
      const sc = new Scroller();
      s.initialize(sc);
      this.scrollers.set(v.id, sc);
      const content = typeNode.createNode(ui, 'Stack');
      const ca = typeNode.getAttribute(content, 'Stack');
      if (ca !== undefined) {
        ca.alignContent(Alignment.TopStart);
      }
      content.commonAttribute.width(v.cw > 0 ? v.cw : v.w).height(v.ch > 0 ? v.ch : v.h);
      s.appendChild(content);
      this.contents.set(v.id, content);
      return s;
    }
    if (v.kind === KIND_TEXT) {
      if (v.runs.length > 0) {
        return this.leaf(v, KIND_TEXT);
      }
      return typeNode.createNode(ui, 'Text');
    }
    if (v.kind === KIND_INPUT) {
      const t = typeNode.createNode(ui, 'TextInput');
      const c = new TextInputController();
      t.initialize({ controller: c });
      this.controllers.set(v.id, c);
      return t;
    }
    if (v.kind === KIND_IMAGE) {
      if (v.data !== '' || v.src.startsWith('asset:')) {
        return this.leaf(v, KIND_IMAGE);
      }
      return typeNode.createNode(ui, 'Image');
    }
    if (v.kind === KIND_CANVAS) {
      return this.leaf(v, KIND_CANVAS);
    }
    if (v.kind === KIND_WEB) {
      const st = new WebViewState(v.id);
      v.webState = st;
      this.webStates.set(v.id, st);
      this.webHooksShown.set(v.id, webHooksKey(v.noJS, v.webBefore, v.webAfter));
      this.webShown.set(v.id, webReloadKey(v.webSrc, v.webHTML, v.webBaseURL, v.webHeaders));
      this.webPropsShown.set(v.id, webPropsKey(v));
      return this.leaf(v, KIND_WEB);
    }
    if (v.kind === KIND_VIDEO) {
      const c = new VideoController();
      v.videoController = c;
      v.videoPausedShown = v.videoPaused;
      return this.leaf(v, KIND_VIDEO);
    }
    if (v.kind === KIND_NATIVE) {
      return this.leaf(v, KIND_NATIVE);
    }
    const s = typeNode.createNode(ui, 'Stack');
    const a = typeNode.getAttribute(s, 'Stack');
    if (a !== undefined) {
      a.alignContent(Alignment.TopStart);
    }
    return s;
  }

  // leaf builds the view of a leaf kind in a Stack of its own, or in box, which holds the view the leaf replaces.
  leaf(v: VNode, kind: number, container?: FrameNode): FrameNode | null {
    const ui = this.ui;
    if (ui === undefined) {
      return null;
    }
    const b = new BuilderNode<[VNode]>(ui);
    if (kind === KIND_TEXT) {
      b.build(wrapBuilder(spanBuilder), v);
    } else if (kind === KIND_IMAGE) {
      b.build(wrapBuilder(imageBuilder), v);
    } else if (kind === KIND_WEB) {
      b.build(wrapBuilder(webBuilder), v);
    } else if (kind === KIND_VIDEO) {
      b.build(wrapBuilder(videoBuilder), v);
    } else if (kind === KIND_NATIVE) {
      b.build(wrapBuilder(tabBarBuilder), v);
    } else {
      b.build(wrapBuilder(canvasBuilder), v);
    }
    this.leaves.set(v.id, b);
    const drawn = b.getFrameNode();
    if (drawn === null) {
      return null;
    }
    // A BuilderNode's frame node ignores the position a batch sets on it, so
    // it keeps the frame of the parent's top left corner and the view lands
    // there whatever the engine laid out. A Stack of its own holds the frame
    // and the view fills it.
    const box = container ?? typeNode.createNode(ui, 'Stack');
    const ba = typeNode.getAttribute(box, 'Stack');
    if (ba !== undefined) {
      ba.alignContent(Alignment.TopStart);
    }
    drawn.commonAttribute.position({ x: 0, y: 0 }).width('100%').height('100%');
    box.appendChild(drawn);
    return box;
  }

  syncNode(id: number, parent: FrameNode, full: boolean = false): void {
    const host = this.host;
    if (host === undefined) {
      return;
    }
    const v = host.tree.at(id);
    if (v === undefined) {
      return;
    }
    const existing = this.nodes.get(id);
    if (existing === undefined) {
      const made = this.create(v);
      if (made === null) {
        return;
      }
      this.nodes.set(id, made);
      if (v.kind !== KIND_SCROLL && v.kind !== KIND_INPUT && v.kind !== KIND_IMAGE) {
        this.styled.set(id, Styled.fresh(v, made));
      }
      parent.appendChild(made);
      // The view is written below; a dirty flag left set would write it, and every view under it, again on the next batch that reaches its parent.
      v.dirty = false;
      const mf = v.matchFrame;
      if (mf !== undefined) {
        // A shared element starts at the frame of the view it replaces
        // and moves to its own over the commit's layout animation. The
        // frame is the page's, so subtract where the new parent puts it.
        made.commonAttribute.position({ x: v.x + mf.x - v.ax, y: v.y + mf.y - v.ay }).width(mf.w).height(mf.h);
        const ui = this.ui;
        if (ui !== undefined) {
          ui.animateTo({ duration: host.tree.layoutMs, curve: layoutCurve(host.tree.layoutCurve) }, () => {
            this.update(v, made, full);
          });
        } else {
          this.update(v, made, full);
        }
        v.matchFrame = undefined;
      } else {
        this.update(v, made, full);
      }
      return;
    }
    if (v.dirty || full) {
      v.dirty = false;
      this.update(v, existing, full);
    }
  }

  update(v: VNode, node: FrameNode, full: boolean = false): void {
    this.style(v, node);
    this.applyShift(v.id);
    if (v.kind === KIND_TEXT) {
      if (v.runs.length > 0) {
        const b = this.leaves.get(v.id);
        if (b !== undefined) {
          b.update(v);
        }
      } else {
        this.text(v, node);
      }
    } else if (v.kind === KIND_INPUT) {
      this.input(v, node);
      const ui = this.ui;
      if (v.needFocus && ui !== undefined) {
        v.needFocus = false;
        ui.getFocusController().requestFocus('nsgo-' + v.id);
      }
      if (v.needBlur && ui !== undefined) {
        v.needBlur = false;
        ui.getFocusController().clearFocus();
      }
    } else if (v.kind === KIND_IMAGE) {
      if (v.data !== '' || v.src.startsWith('asset:')) {
        const b = this.leaves.get(v.id);
        if (b !== undefined) {
          b.update(v);
        }
      } else {
        this.image(v, node);
      }
    } else if (v.kind === KIND_CANVAS) {
      const b = this.leaves.get(v.id);
      if (b !== undefined) {
        b.update(v);
      }
    } else if (v.kind === KIND_NATIVE) {
      const b = this.leaves.get(v.id);
      if (b !== undefined) {
        b.update(v);
      }
    } else if (v.kind === KIND_WEB) {
      const wantHooks = webHooksKey(v.noJS, v.webBefore, v.webAfter);
      const wantReload = webReloadKey(v.webSrc, v.webHTML, v.webBaseURL, v.webHeaders);
      const wantProps = webPropsKey(v);
      if (this.webHooksShown.get(v.id) !== wantHooks) {
        // ArkWeb takes its script and JavaScript hooks only as a Web is made, and a new source can start loading before changed modifiers apply, so a new Web loads the source with them, starting a new history.
        this.webStates.get(v.id)?.close();
        const old = this.leaves.get(v.id);
        const drawn = old?.getFrameNode();
        if (drawn !== undefined && drawn !== null) {
          node.removeChild(drawn);
        }
        old?.dispose();
        const st = new WebViewState(v.id);
        v.webState = st;
        this.webStates.set(v.id, st);
        this.webHooksShown.set(v.id, wantHooks);
        this.webShown.set(v.id, wantReload);
        this.webPropsShown.set(v.id, wantProps);
        this.leaf(v, KIND_WEB, node);
      } else {
        const b = this.leaves.get(v.id);
        const st = this.webStates.get(v.id);
        if (b !== undefined && st !== undefined) {
          if (this.webPropsShown.get(v.id) !== wantProps) {
            this.webPropsShown.set(v.id, wantProps);
            b.update(v);
          }
          st.applyLiveSettings(v);
          if (this.webShown.get(v.id) !== wantReload) {
            this.webShown.set(v.id, wantReload);
            if (st.attached) {
              st.loadSource(v, v.webSrc, v.webHTML, v.webBaseURL);
            }
          }
          st.drainCommands(v, () => {
            this.webPropsShown.set(v.id, webPropsKey(v));
            b.update(v);
          });
        }
      }
    } else if (v.kind === KIND_VIDEO) {
      const b = this.leaves.get(v.id);
      const c = v.videoController;
      if (b !== undefined && c !== undefined) {
        if (this.videoShown.get(v.id) !== v.videoSrc) {
          this.videoShown.set(v.id, v.videoSrc);
          b.update(v);
        }
        if (v.videoSeek >= 0) {
          c.setCurrentTime(v.videoSeek);
          v.videoSeek = -1;
        }
        if (v.videoPaused !== v.videoPausedShown) {
          v.videoPausedShown = v.videoPaused;
          if (v.videoPaused) {
            c.pause();
          } else {
            c.start();
          }
        }
      }
    } else if (v.kind === KIND_SCROLL) {
      this.scroll(v, node);
      const content = this.contents.get(v.id);
      if (content !== undefined) {
        content.commonAttribute.width(v.cw > 0 ? v.cw : v.w).height(v.ch > 0 ? v.ch : v.h);
        this.syncChildren(v, content, full);
      }
    } else {
      this.screen(v, node);
      this.syncChildren(v, node, full);
    }
  }

  // screen draws a screen's system header over its content, and takes it
  // away when the screen hides it.
  screen(v: VNode, node: FrameNode): void {
    if (!v.isScreen) {
      return;
    }
    if (v.scrHidden) {
      const h = this.headers.get(v.id);
      if (h !== undefined) {
        node.removeChild(h);
        this.headers.delete(v.id);
        this.headerBuilders.delete(v.id);
      }
      return;
    }
    let h = this.headers.get(v.id);
    const ui = this.ui;
    if (h === undefined) {
      if (ui === undefined) {
        return;
      }
      const b = new BuilderNode<[VNode]>(ui);
      b.build(wrapBuilder(headerBuilder), v);
      this.headerBuilders.set(v.id, b);
      const f = b.getFrameNode();
      if (f === null) {
        return;
      }
      h = f;
      this.headers.set(v.id, h);
      node.appendChild(h);
    }
    const b = this.headerBuilders.get(v.id);
    if (b !== undefined) {
      b.update(v);
    }
    h.commonAttribute.width(v.w).height(v.scrHeight > 0 ? v.scrHeight : 44).position({ x: 0, y: 0 });
  }

  // style writes the view's frame and look, each attribute only when it differs from what the node was last given: every attribute write crosses into ArkUI and marks the node dirty, and writing all of them for each of a navigation's hundreds of views took most of its time on a phone.
  style(v: VNode, node: FrameNode): void {
    const c = node.commonAttribute;
    let a = this.styled.get(v.id);
    if (a === undefined || a.v !== v || a.node !== node) {
      a = new Styled();
      a.v = v;
      a.node = node;
      this.styled.set(v.id, a);
    }
    if (a.w !== v.w || a.h !== v.h) {
      c.width(v.w).height(v.h);
      a.w = v.w;
      a.h = v.h;
    }
    if (a.x !== v.x || a.y !== v.y) {
      c.position({ x: v.x, y: v.y });
      a.x = v.x;
      a.y = v.y;
    }
    if (a.bg !== v.bg) {
      c.backgroundColor(v.bg === '' ? Color.Transparent : v.bg);
      a.bg = v.bg;
    }
    if (a.radius !== v.radius) {
      c.borderRadius(v.radius);
      a.radius = v.radius;
    }
    if (a.bw !== v.bw || a.bc !== v.bc) {
      c.border({ width: v.bw, color: v.bc === '' ? Color.Transparent : v.bc });
      a.bw = v.bw;
      a.bc = v.bc;
    }
    if (a.op !== v.op) {
      c.opacity(v.op);
      a.op = v.op;
    }
    if (a.tx !== v.tx || a.ty !== v.ty) {
      c.translate({ x: v.tx, y: v.ty });
      a.tx = v.tx;
      a.ty = v.ty;
    }
    if (a.scx !== v.scx || a.scy !== v.scy) {
      c.scale({ x: v.scx, y: v.scy });
      a.scx = v.scx;
      a.scy = v.scy;
    }
    if (a.rot !== v.rot) {
      c.rotate({ angle: v.rot });
      a.rot = v.rot;
    }
    if (a.hidden !== v.hidden) {
      c.visibility(v.hidden ? Visibility.Hidden : Visibility.Visible);
      a.hidden = v.hidden;
    }
    if (a.clip !== v.clip) {
      c.clip(v.clip);
      a.clip = v.clip;
    }
    if (a.acc !== v.label) {
      c.accessibilityText(v.label);
      a.acc = v.label;
    }
    if (a.accDesc !== v.accDesc) {
      c.accessibilityDescription(v.accDesc);
      a.accDesc = v.accDesc;
    }
    if (a.accLevel !== v.accLevel) {
      c.accessibilityLevel(v.accLevel);
      a.accLevel = v.accLevel;
    }
    if (a.accGroup !== v.accGroup) {
      c.accessibilityGroup(v.accGroup);
      a.accGroup = v.accGroup;
    }
    const enabled = !(v.disabled || v.propDisabled);
    if (a.enabled !== enabled) {
      c.enabled(enabled);
      a.enabled = enabled;
    }
    const key = v.testID !== '' ? v.testID : 'nsgo-' + v.id;
    if (a.key !== key) {
      c.id(key);
      a.key = key;
    }
    if (a.pointer !== v.pointer) {
      if (v.pointer === 1) {
        c.hitTestBehavior(HitTestMode.None);
      } else if (v.pointer === 2) {
        c.hitTestBehavior(HitTestMode.Transparent);
      } else if (v.pointer === 3) {
        c.hitTestBehavior(HitTestMode.BLOCK_DESCENDANTS);
      }
      a.pointer = v.pointer;
    }
    // The handler reads the view when it fires, so it is set once per view.
    if (v.press && !a.press) {
      c.onClick((e: ClickEvent) => {
        const host = this.host;
        if (host !== undefined) {
          host.send('tap', v.hit, v.ax + e.x, v.ay + e.y, '');
        }
      });
      a.press = true;
    }
  }

  text(v: VNode, node: FrameNode): void {
    const t = node as typeNode.Text;
    if (this.shown.get(v.id) !== v.text) {
      t.initialize(v.text);
      this.shown.set(v.id, v.text);
    }
    const font = v.fs + '|' + v.fw + '|' + v.color + '|' + v.align + '|' + v.maxLines;
    const st = this.styled.get(v.id);
    if (st !== undefined && st.node === node && st.font === font) {
      return;
    }
    const a = typeNode.getAttribute(node, 'Text');
    if (a === undefined) {
      return;
    }
    if (st !== undefined && st.node === node) {
      st.font = font;
    }
    a.fontSize(v.fs).fontWeight(v.fw).fontColor(v.color)
      .textAlign(v.align === 1 ? TextAlign.Center :
        (v.align === 2 ? TextAlign.End : TextAlign.Start))
      .maxLines(v.maxLines > 0 ? v.maxLines : 100)
      .textOverflow({ overflow: TextOverflow.Ellipsis });
  }

  input(v: VNode, node: FrameNode): void {
    // The node's id is its focus key, which requestFocus names.
    node.commonAttribute.id('nsgo-' + v.id);
    const i = node as typeNode.TextInput;
    const control = this.controllers.get(v.id);
    if (this.shown.get(v.id) !== v.value) {
      if (control !== undefined) {
        i.initialize({ text: v.value, placeholder: v.ph, controller: control });
      } else {
        i.initialize({ text: v.value, placeholder: v.ph });
      }
      this.shown.set(v.id, v.value);
    }
    const a = typeNode.getAttribute(node, 'TextInput');
    if (a === undefined) {
      return;
    }
    // The text goes in the content box the engine laid out, as on the other targets: ArkUI's padding, inside the border, is what lies around it in the frame. ArkUI's default padding would otherwise shift the text, and leave only the top half of each glyph visible in a short field.
    const cw = v.inW >= 0 ? v.inW : v.w;
    const ch = v.inH >= 0 ? v.inH : v.h;
    const pad: Padding = {
      top: Math.max(v.inY - v.bw, 0), left: Math.max(v.inX - v.bw, 0),
      right: Math.max(v.w - v.inX - cw - v.bw, 0), bottom: Math.max(v.h - v.inY - ch - v.bw, 0)
    };
    // ArkUI's TextInput draws its theme's grey capsule unless its own background, corners and border are set: the common attributes style() writes do not reach them. The engine's are written here again, transparent where it gives none.
    a.backgroundColor(v.bg === '' ? Color.Transparent : v.bg)
      .borderRadius(v.radius)
      .border({ width: v.bw, color: v.bc === '' ? Color.Transparent : v.bc });
    if (v.multiline) {
      // A field as tall as a page is not a line as tall as a page: using its whole height as lineHeight drops the glyphs into the middle. A single line is centered in the content box.
      node.commonAttribute.align(Alignment.TopStart);
    }
    a.fontSize(v.fs).fontWeight(v.fw).fontColor(v.color)
      .fontFamily(v.family)
      .fontStyle(v.italic ? FontStyle.Italic : FontStyle.Normal)
      .textAlign(v.align === 1 ? TextAlign.Center :
        (v.align === 2 ? TextAlign.End : TextAlign.Start))
      .type(v.masked ? InputType.Password : InputType.Normal)
      .enabled(!(v.disabled || v.propDisabled))
      // A read-only field still focuses, selects and copies; it only refuses what the user would insert or delete.
      .onWillInsert(() => !v.readonly)
      .onWillDelete(() => !v.readonly)
      .padding(pad)
      .placeholderFont({ size: v.fs, weight: v.fw })
      .onFocus(() => {
        const host = this.host;
        if (host !== undefined) {
          host.send('focus', v.id, 0, 0, '');
        }
      })
      .onBlur(() => {
        const host = this.host;
        if (host !== undefined) {
          host.send('blur', v.id, 0, 0, '');
        }
      })
      .onChange((t: string) => {
        // Text the field already shows is not an edit: it is the value the engine or a test's typeInput just set, which must not reach the engine a second time.
        if (this.shown.get(v.id) === t) {
          return;
        }
        this.shown.set(v.id, t);
        const host = this.host;
        if (host !== undefined) {
          const n = host.tree.at(v.id);
          if (n !== undefined) {
            n.value = t;
          }
          host.send('input', v.id, 0, 0, t);
        }
      })
      .onSubmit(() => {
        const host = this.host;
        if (host !== undefined) {
          host.send('confirm', v.id, 0, 0, '');
        }
      })
      .onTextSelectionChange((start: number, end: number) => {
        const host = this.host;
        if (host !== undefined) {
          host.send('selection', v.id, start, end, '');
        }
      });
    // A zero line height follows the platform's font metrics. Multiline fields still use a readable default when the style leaves it unset.
    const lineHeight = v.lh > 0 ? v.lh : (v.multiline ? Math.max(v.fs * 1.4, 1) : 0);
    if (lineHeight > 0) {
      a.lineHeight(lineHeight);
    }
    // An unset placeholder color is the platform's, as on the other targets.
    if (v.phColor !== '') {
      a.placeholderColor(v.phColor);
    }
    if (v.selStart >= 0) {
      const control = this.controllers.get(v.id);
      const start = v.selStart;
      const end = v.selEnd;
      v.selStart = -1;
      v.selEnd = -1;
      // A selection needs the field's focus, which the controller has from
      // the next turn.
      const ui = this.ui;
      if (ui !== undefined) {
        ui.getFocusController().requestFocus('nsgo-' + v.id);
      }
      if (control !== undefined) {
        setTimeout(() => {
          control.setTextSelection(start, end);
        }, 0);
      }
    }
  }

  image(v: VNode, node: FrameNode): void {
    // A view showing pixels has no data and no asset, so this node draws it: from the copy its last 'ip' op sent, which pv counts, or from its URI once the pixels stop.
    const copy = viewPixels.get(v.id)?.cur;
    const shown = copy !== undefined ? '\u0000px:' + v.pv : v.src;
    if (this.shown.get(v.id) !== shown) {
      const im = node as typeNode.Image;
      if (copy !== undefined) {
        im.initialize(copy);
      } else {
        im.initialize(v.src);
      }
      this.shown.set(v.id, shown);
    }
    const a = typeNode.getAttribute(node, 'Image');
    if (a !== undefined) {
      a.objectFit(fitOf(v.fit)).interpolation(interpolationOf(v.nearest));
    }
  }

  scroll(v: VNode, node: FrameNode): void {
    const a = typeNode.getAttribute(node, 'Scroll');
    if (a !== undefined) {
      a.scrollable(v.horizontal ? ScrollDirection.Horizontal : ScrollDirection.Vertical)
        .scrollBar(v.hideBar ? BarState.Off : BarState.Auto)
        .onDidScroll((dx: number, dy: number, state: ScrollState) => {
          const sc = this.scrollers.get(v.id);
          const host = this.host;
          if (sc !== undefined && host !== undefined) {
            const o = sc.currentOffset();
            host.scrolled(v.id, o.xOffset, o.yOffset);
          }
          this.scrollPhase(v.id, state);
          this.pinSticky(v.id);
        });
    }
    const sc = this.scrollers.get(v.id);
    if (sc !== undefined && v.skey !== '' && this.applied.get(v.id) !== v.skey) {
      this.applied.set(v.id, v.skey);
      sc.scrollTo({ xOffset: v.sx, yOffset: v.sy });
    }
    this.applyZoom(v);
    this.pinSticky(v.id);
  }

  // applyZoom scales a zoomable scroll view's content about its top left,
  // grows its box so ArkUI scrolls the zoomed range, and keeps the
  // requested center where it was. The page reports the scale and offset
  // to the engine, which clamps and reports it to the app.
  applyZoom(v: VNode): void {
    const content = this.contents.get(v.id);
    if (content === undefined) {
      return;
    }
    const z = Math.min(Math.max(v.zoom, v.zmin), v.zmax);
    v.zoom = z;
    const prev = this.zoomShown.get(v.id);
    if (prev === z) {
      return;
    }
    const cw = v.cw > 0 ? v.cw : v.w;
    const ch = v.ch > 0 ? v.ch : v.h;
    content.commonAttribute.width(cw * z).height(ch * z).scale({ x: z, y: z, centerX: 0, centerY: 0 });
    let ox = 0;
    let oy = 0;
    const sc = this.scrollers.get(v.id);
    if (sc !== undefined) {
      const cur = sc.currentOffset();
      const k = prev === undefined || prev === 0 ? 1 : z / prev;
      ox = Math.max(0, (cur.xOffset + v.zcx) * k - v.zcx);
      oy = Math.max(0, (cur.yOffset + v.zcy) * k - v.zcy);
      sc.scrollTo({ xOffset: ox, yOffset: oy });
    }
    this.zoomShown.set(v.id, z);
    const host = this.host;
    if (host !== undefined) {
      host.zoomed(v.id, z, ox, oy);
    }
  }

  // zoomTo sets a scroll view's scale from a pinch at the page's point at.
  zoomTo(v: VNode, z: number, at: number, ay: number): void {
    v.zcx = at;
    v.zcy = ay;
    v.zoom = z;
    v.zset = true;
    this.applyZoom(v);
  }

  // pull refreshes the scroll view a finger pulls down from its top, past
  // the distance it asks for. ArkUI's Scroll reports no offset for a pull,
  // its content springing where the finger goes, so the page judges the
  // distance from the finger itself.
  pull(e: TouchEvent, phase: number): void {
    const t = e.changedTouches.length > 0 ? e.changedTouches[0] : undefined;
    if (phase === 1) {
      this.pullId = 0;
      const host = this.host;
      if (t === undefined || host === undefined) {
        return;
      }
      const id = host.tree.scrollAt(host.tree.root, t.x, t.y, 0);
      const sc = this.scrollers.get(id);
      if (id === 0 || sc === undefined || sc.currentOffset().yOffset > 1) {
        return;
      }
      this.pullId = id;
      this.pullFrom = t.y;
      return;
    }
    if (this.pullId === 0) {
      return;
    }
    const host = this.host;
    const v = host === undefined ? undefined : host.tree.at(this.pullId);
    if (phase === 2) {
      if (host === undefined || v === undefined || t === undefined || v.refreshing) {
        return;
      }
      if (t.y - this.pullFrom >= v.refreshAt) {
        host.send('refresh', this.pullId, t.y - this.pullFrom, 0, '');
        this.pullId = 0;
      }
      return;
    }
    this.pullId = 0;
  }

  // pinSticky moves a scroll view's sticky cells for its offset. A cell travels between 0 and End - Start, as driver.StickyShift does: forward for a start-pinned cell, back toward the content's start for an end-pinned one, whose End is below its Start. Android moves the cell with setTranslationY; ArkUI's Scroll clips a translated child at its laid-out frame, so here the cell's position follows the scroll instead.
  pinSticky(id: number): void {
    const host = this.host;
    const sc = this.scrollers.get(id);
    if (host === undefined || sc === undefined) {
      return;
    }
    const cells = host.tree.sticky.get(id);
    const v = host.tree.at(id);
    if (cells === undefined || v === undefined) {
      return;
    }
    const o = sc.currentOffset();
    const at = v.horizontal ? o.xOffset : o.yOffset;
    for (let i = 0; i < cells.length; i++) {
      const c = cells[i];
      const travel = c.e - c.s;
      const d = Math.min(Math.max(at - c.s, Math.min(travel, 0)), Math.max(travel, 0));
      let sh = this.cellShifts.get(c.c);
      if (sh === undefined) {
        sh = new StickyShift();
        this.cellShifts.set(c.c, sh);
      }
      sh.d = d;
      sh.across = v.horizontal;
      this.applyShift(c.c);
    }
  }

  // applyShift writes a sticky cell's position over its style's, which a
  // fresh batch would otherwise have set back to the plain frame.
  applyShift(cellID: number): void {
    const sh = this.cellShifts.get(cellID);
    if (sh === undefined) {
      return;
    }
    const n = this.nodes.get(cellID);
    const v = this.host === undefined ? undefined : this.host.tree.at(cellID);
    if (n === undefined || v === undefined) {
      return;
    }
    // Move it in the content's coordinates: ArkUI's Scroll clips a
    // translated child at its laid-out frame, but a moved position takes
    // part in the clip. The tree is drawn below the status bar, so a
    // pinned cell starts that far down; before it pins, d is 0 and it
    // stays at its frame. Later siblings paint above it, so lift it too.
    const off = sh.d > 0 && !sh.across ? this.safeTop : 0;
    n.commonAttribute.zIndex(1).position({ x: v.x + (sh.across ? sh.d : 0),
      y: v.y + (sh.across ? 0 : sh.d) + off });
  }

  // scrollPhase tells the engine what a scroll view's state became: a
  // drag beginning or ending, the glide a lift set going, and the rest
  // that ends either.
  scrollPhase(id: number, state: number): void {
    const host = this.host;
    if (host === undefined) {
      return;
    }
    const last = this.scrollStates.get(id);
    if (last === state) {
      return;
    }
    this.scrollStates.set(id, state);
    const dragged = last === 1;
    const glided = last === 2;
    if (state === 1) {
      if (glided) {
        host.send('momentumend', id, 0, 0, '');
      }
      host.send('dragstart', id, 0, 0, '');
      return;
    }
    if (dragged) {
      host.send('dragend', id, 0, 0, '');
    }
    if (state === 2) {
      host.send('momentumstart', id, 0, 0, '');
      return;
    }
    if (state === 0) {
      if (glided) {
        host.send('momentumend', id, 0, 0, '');
      }
      if (dragged || glided) {
        host.send('scrollend', id, 0, 0, '');
      }
    }
  }

  // syncChildren makes the FrameNode's children match the view's, in order.
  syncChildren(v: VNode, parent: FrameNode, full: boolean = false): void {
    this.pruneChildren(v, parent);
    for (let k = 0; k < v.children.length; k++) {
      this.syncNode(v.children[k], parent, full);
      const c = this.nodes.get(v.children[k]);
      if (c === undefined) {
        continue;
      }
      if (parent.getChild(k) === c) {
        continue;
      }
      if (this.hasChild(parent, c)) {
        parent.removeChild(c);
      }
      if (k === 0) {
        parent.insertChildAfter(c, null);
      } else {
        parent.insertChildAfter(c, parent.getChild(k - 1));
      }
    }
  }

  // pruneChildren removes the FrameNode's children the view no longer has. The wanted ones are collected once: asking for each child scanned every child, quadratic in a long list.
  pruneChildren(v: VNode, parent: FrameNode): void {
    const keep = new Set<FrameNode>();
    const h = this.headers.get(v.id);
    if (h !== undefined) {
      keep.add(h);
    }
    for (let i = 0; i < v.children.length; i++) {
      const c = this.nodes.get(v.children[i]);
      if (c !== undefined) {
        keep.add(c);
      }
    }
    const count = parent.getChildrenCount();
    for (let i = count - 1; i >= 0; i--) {
      const child = parent.getChild(i);
      if (child !== null && !keep.has(child)) {
        parent.removeChild(child);
      }
    }
  }

  wanted(v: VNode, child: FrameNode): boolean {
    if (this.headers.get(v.id) === child) {
      return true;
    }
    for (let i = 0; i < v.children.length; i++) {
      if (this.nodes.get(v.children[i]) === child) {
        return true;
      }
    }
    return false;
  }

  hasChild(parent: FrameNode, child: FrameNode): boolean {
    const n = parent.getChildrenCount();
    for (let i = 0; i < n; i++) {
      if (parent.getChild(i) === child) {
        return true;
      }
    }
    return false;
  }
}

// Styled is what style last wrote to a view's node. NaN and undefined differ from every value, so the first write of each attribute happens.
class Styled {
  v: VNode | undefined = undefined;
  node: FrameNode | undefined = undefined;
  w: number = NaN;
  h: number = NaN;
  x: number = NaN;
  y: number = NaN;
  bg: string | undefined = undefined;
  radius: number = NaN;
  bw: number = NaN;
  bc: string | undefined = undefined;
  op: number = NaN;
  tx: number = NaN;
  ty: number = NaN;
  scx: number = NaN;
  scy: number = NaN;
  rot: number = NaN;
  hidden: boolean | undefined = undefined;
  clip: boolean | undefined = undefined;
  acc: string = '';
  accDesc: string = '';
  accLevel: string = 'auto';
  accGroup: boolean = false;
  enabled: boolean = true;
  key: string | undefined = undefined;
  pointer: number = NaN;
  press: boolean = false;
  // font is the text attributes last written, as one key.
  font: string = '';

  // fresh is a node just made as a Stack or a Text, which holds ArkUI's defaults: no background, corners or border, opaque, untransformed and visible. Writing those again for every view a page creates was a third of its cost.
  static fresh(v: VNode, node: FrameNode): Styled {
    const a = new Styled();
    a.v = v;
    a.node = node;
    a.bg = '';
    a.radius = 0;
    a.bw = 0;
    a.bc = '';
    a.op = 1;
    a.tx = 0;
    a.ty = 0;
    a.scx = 1;
    a.scy = 1;
    a.rot = 0;
    a.hidden = false;
    a.clip = false;
    a.pointer = 0;
    return a;
  }
}

// RenderController gives the page a NodeContainer root; the Renderer mounts
// the engine's tree under it.
class RenderController extends NodeController {
  renderer: Renderer | undefined = undefined;

  makeNode(uiContext: UIContext): FrameNode | null {
    const root = typeNode.createNode(uiContext, 'Stack');
    const a = typeNode.getAttribute(root, 'Stack');
    if (a !== undefined) {
      a.alignContent(Alignment.TopStart);
    }
    root.commonAttribute.width('100%').height('100%');
    const r = this.renderer;
    // Every finger is the engine's to route: it hit-tests them against its
    // frames, so a drag on a slider, a sheet or a carousel follows them.
    root.commonAttribute.onTouch((e: TouchEvent) => {
      if (r !== undefined && r.host !== undefined) {
        r.host.touch(e);
      }
    });
    if (r !== undefined) {
      r.attach(uiContext, root);
    }
    return root;
  }
}

