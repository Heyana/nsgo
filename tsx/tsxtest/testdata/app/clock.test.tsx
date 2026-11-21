import { expect, open, test } from "nsgo/test";

test("clock.time as a Date sets what Date reads", async () => {
  const app = await open("Clock.tsx", { clock: { time: new Date("2026-01-02T03:04:05Z") } });
  await app.waitFor("id:now");
  expect(app.texts()).toContain("1767323045000 2026-01-02T03:04:05.000Z");
  expect(app.texts()).toContain("waiting");
});

test("clock.time as milliseconds sets what Date reads", async () => {
  const app = await open("Clock.tsx", { clock: { time: 1772600767890 } });
  await app.waitFor("id:now");
  expect(app.texts()).toContain("1772600767890 2026-03-04T05:06:07.890Z");
});

test("setSystemTime takes a Date or milliseconds and fires no timer", async () => {
  const app = await open("Clock.tsx", { clock: { time: 0 } });
  await app.waitFor("id:now");
  app.clock.setSystemTime(new Date("2026-04-05T06:07:08Z"));
  await app.press("id:read");
  expect(app.texts()).toContain("1775369228000 2026-04-05T06:07:08.000Z");
  app.clock.setSystemTime(1780000000123);
  await app.press("id:read");
  expect(app.texts()).toContain("1780000000123 2026-05-28T20:26:40.123Z");
  expect(app.texts()).toContain("waiting");
});

test("runFor fires a timer once it is due and not before", async () => {
  const app = await open("Clock.tsx", { clock: { time: 0 } });
  await app.waitFor("id:now");
  await app.clock.runFor(99);
  expect(app.texts()).toContain("waiting");
  await app.clock.runFor(1);
  expect(app.texts()).toContain("fired at 100");
});

test("waitFor leaves an explicit clock still", async () => {
  const app = await open("Clock.tsx", { clock: { time: 0 } });
  await app.clock.runFor(99);
  await expect(() => app.waitFor("text:fired at 100", { timeout: 20 })).rejects.toThrow("the condition did not hold");
  await app.clock.runFor(1);
  expect(app.texts()).toContain("fired at 100");
});

test("fastForward fires the timers due meanwhile", async () => {
  const app = await open("Clock.tsx", { clock: { time: 0 } });
  await app.waitFor("id:now");
  await app.clock.fastForward(99);
  expect(app.texts()).toContain("waiting");
  await app.clock.fastForward(1);
  expect(app.texts()).toContain("fired at 100");
});
