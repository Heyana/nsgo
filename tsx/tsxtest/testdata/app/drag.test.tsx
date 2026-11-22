import { expect, open, test } from "nsgo/test";

test("drags to another view using the touch responder", async () => {
  const app = await open("Drag.tsx");
  await app.dragTo("id:source", "id:target");
  expect(app.texts()).toContain("landed 160");
});
