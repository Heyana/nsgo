import { expect, open, test } from "nsgo/test";

test("stubs the network", async () => {
  const app = await open("Services.tsx", { network: { "POST https://api.example.com/todos*": { status: 201, body: [1, 2] } } });
  await app.press("id:fetch");
  await app.waitFor("201 2");
  expect(app.requests()).toEqual([{ method: "POST", url: "https://api.example.com/todos?page=1",
    headers: { "content-type": "text/plain;charset=UTF-8" }, body: "new" }]);
  await app.press("id:offline");
  await app.waitFor("offline");
});

test("answers dialogs and picks", async () => {
  const app = await open("Services.tsx");
  app.onDialog(d => { expect(d.title).toBe("Delete?"); return "Delete"; });
  await app.press("id:ask");
  await app.waitFor("chose Delete");
  app.onPick(() => [{ name: "cat.png", data: new Uint8Array([1, 2, 3]) }]);
  await app.press("id:pick");
  await app.waitFor("cat.png:image/png:3");
  expect(app.picks()).toEqual([{ multiple: true, limit: 3, maxDimension: null, quality: null, format: "compatible" }]);
});

test("records URLs, the clipboard and menus", async () => {
  const app = await open("Services.tsx", { clipboard: "before", platform: "macos" });
  expect(app.clipboard()).toBe("before");
  await app.press("id:copy");
  expect(app.clipboard()).toBe("copied");
  await app.press("id:open");
  expect(app.openedUrls()).toEqual(["https://example.com/docs"]);
  await app.press("id:menu");
  expect(app.menu()).toEqual([{ label: "Rename", disabled: false }, { label: "Share > Mail", disabled: false, checked: true }]);
  await app.chooseMenuItem("Share > Mail");
  await app.waitFor("mailed");
  expect(app.menu()).toBeNull();
  expect(app.spanStyle("text:mailed").fontWeight).toBe(400);
});

test("answers requests with the test's functions", async () => {
  const app = await open("Services.tsx", { network: {
    "POST https://api.example.com/todos*": (req) => ({ status: 202, body: [req.body, req.method] }),
    "GET https://elsewhere.example.com/": async () => ({ error: "unreachable" }),
  } });
  await app.press("id:fetch");
  await app.waitFor("202 2");
  await app.press("id:offline");
  await app.waitFor("offline");
});

test("opens with more contrast", async () => {
  expect((await open("Services.tsx")).shows("standard contrast")).toBe(true);
  expect((await open("Services.tsx", { increaseContrast: true })).shows("more contrast")).toBe(true);
});

test("reports missing camera without camera option", async () => {
  const app = await open("Services.tsx");
  await app.press("id:can-take-photo");
  await app.waitFor("camera:false");
  await app.press("id:photo");
  await app.waitFor("photo:error:core: no camera to take a photo with");
  expect(app.picks()).toEqual([]);
});

test("answers camera photos through onPick and records them in picks", async () => {
  const app = await open("Services.tsx", { camera: true });
  await app.press("id:can-take-photo");
  await app.waitFor("camera:true");

  // Without onPick the user cancels, and the capture is recorded.
  await app.press("id:photo");
  await app.waitFor("photo:cancelled");

  // A single PickFile object answers takePhoto, with size and bytes preserved.
  const seen: boolean[] = [];
  app.onPick(p => {
    seen.push(Boolean(p.camera));
    if (p.camera) {
      return { name: "shot.jpg", data: new Uint8Array([9, 8, 7]), size: { width: 640, height: 480 } };
    }
    return [{ name: "cat.png", data: new Uint8Array([1, 2, 3]) }];
  });
  await app.press("id:photo");
  await app.waitFor("photo:shot.jpg:image/jpeg:3:640x480:9-8-7");

  // Interleaved pickImages keeps order and distinguishes camera vs library picks.
  await app.press("id:pick");
  await app.waitFor("cat.png:image/png:3");

  // A PickFile[] array with number[] data also answers takePhoto using its first file.
  app.onPick(() => [{ name: "array.jpg", type: "image/jpeg", data: [4, 5] }]);
  await app.press("id:photo");
  await app.waitFor("photo:array.jpg:image/jpeg:2:0x0:4-5");

  // Calling takePhoto with no options records null maxDimension/quality and accepts ArrayBuffer data.
  app.onPick(() => ({ name: "buf.jpg", data: new Uint8Array([6, 7, 8, 9]).buffer }));
  await app.press("id:photo-default");
  await app.waitFor("default:buf.jpg:4");

  // Returning null or an empty array cancels takePhoto.
  app.onPick(() => null);
  await app.press("id:photo");
  await app.waitFor("photo:cancelled");
  app.onPick(() => []);
  await app.press("id:photo");
  await app.waitFor("photo:cancelled");

  // Throwing from onPick rejects Picker.takePhoto with the error message.
  app.onPick(() => { throw new Error("camera permission denied"); });
  await app.press("id:photo");
  await app.waitFor("photo:error:camera permission denied");

  // Clearing onPick returns to cancelling, and setColorScheme preserves camera availability.
  app.onPick(null);
  await app.setColorScheme("dark");
  await app.press("id:can-take-photo");
  await app.waitFor("camera:true");
  await app.press("id:photo");
  await app.waitFor("photo:cancelled");

  expect(seen).toEqual([true, false]);
  const photoPick = { multiple: false, limit: 1, maxDimension: 1024, quality: 0.75, format: "compatible", camera: true };
  const defaultPhotoPick = { multiple: false, limit: 1, maxDimension: null, quality: null, format: "compatible", camera: true };
  const imagePick = { multiple: true, limit: 3, maxDimension: null, quality: null, format: "compatible" };
  expect(app.picks()).toEqual([photoPick, photoPick, imagePick, photoPick, defaultPhotoPick, photoPick, photoPick, photoPick, photoPick]);
});
