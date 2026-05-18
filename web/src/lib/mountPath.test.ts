import { describe, expect, it } from "vitest";
import { extractMountPath } from "./mountPath";

describe("extractMountPath", () => {
  it("returns an empty mount path outside the plugin proxy", () => {
    expect(extractMountPath("/admin")).toBe("");
  });

  it("supports numeric and slug plugin installation ids", () => {
    expect(extractMountPath("/api/v1/plugins/12/admin/targets")).toBe("/api/v1/plugins/12");
    expect(extractMountPath("/api/v1/plugins/notifications/admin")).toBe("/api/v1/plugins/notifications");
  });
});
