import { expect, open, test } from "nsgo/test";

test("checks for updates and handles available, up-to-date, restart and error results", async () => {
  const app = await open("Updater.tsx", {
    platform: "linux",
    updater: { managed: "self", automaticChecks: true },
  });
  expect(app.value("id:cap")).toBe("self:none");
  expect(app.value("id:status")).toBe("true:false:none:none:idle");

  // Update available: Update opens URL, Later does not.
  app.onUpdateCheck(() => ({
    app: "Demo",
    current: "1.0",
    version: "1.1",
    url: "https://example.com/releases/1.1",
    notes: "Bug fixes",
  }));
  await app.press("id:check");
  expect(app.dialogs()).toHaveLength(1);
  expect(app.dialogs()[0].title).toBe("A new version of Demo is available");
  expect(app.dialogs()[0].message).toContain("Bug fixes");
  app.answer(0);
  expect(app.openedUrls()).toEqual(["https://example.com/releases/1.1"]);

  // Prepared restart update: confirming triggers restart.
  app.onUpdateCheck(() => ({ app: "Demo", current: "1.0", restart: true }));
  await app.press("id:check");
  expect(app.dialogs()[0].message).toContain("Restart to use it.");
  app.answer(0);
  expect(app.restarts()).toBe(1);

  // Up to date and error results.
  app.onUpdateCheck(() => ({ app: "Demo", current: "1.0" }));
  await app.press("id:check");
  expect(app.dialogs()[0].title).toBe("You’re up to date");
  app.answer(0);

  app.onUpdateCheck(() => ({ error: "offline" }));
  await app.press("id:check");
  expect(app.dialogs()[0].title).toBe("Can’t check for updates");
  app.answer(0);

  // Background check stays quiet on error.
  await app.press("id:bg");
  expect(app.dialogs()).toHaveLength(0);
  expect(app.updateChecks()).toEqual([
    { background: false },
    { background: false },
    { background: false },
    { background: false },
    { background: true },
  ]);
});

test("tracks updater preferences and daily schedule", async () => {
  const app = await open("Updater.tsx", {
    platform: "macos",
    updater: { managed: "self", nativePreferences: true, automaticChecks: true },
  });
  await app.press("id:auto-off");
  expect(app.value("id:status")).toBe("false:false:none:none:idle");
  await app.press("id:daily");
  expect(app.updateChecks()).toEqual([]);

  await app.press("id:dl-on");
  await app.press("id:auto-on");
  expect(app.value("id:status")).toBe("true:true:none:none:idle");

  app.onUpdateCheck(() => ({ managed: "self", version: "2.0", notes: "New UI" }));
  await app.press("id:daily");
  expect(app.updateChecks()).toEqual([{ background: true }]);
  expect(app.value("id:status")).toBe("true:true:2.0:New UI:idle");
});
