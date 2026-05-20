import { describe, expect, it } from "vitest";
import { fieldSection, enhanceProviderField, providerHelpText } from "./lib/providerPresentation";

describe("notification provider field presentation", () => {
  it("makes the generic webhook URL an obvious destination field", () => {
    const field = enhanceProviderField({ key: "url", label: "URL", required: true }, "webhook");

    expect(fieldSection(field, "webhook")).toBe("destination");
    expect(field.label).toBe("Webhook URL");
    expect(field.control).toBe("url");
    expect(field.placeholder).toContain("https://");
    expect(field.help).toContain("POST notification JSON");
  });

  it("groups secret credentials under authentication", () => {
    const field = enhanceProviderField({ key: "authorization", label: "Authorization", secret: true }, "webhook");

    expect(fieldSection(field, "webhook")).toBe("auth");
    expect(field.label).toBe("Authorization header");
    expect(field.placeholder).toBe("Bearer ...");
  });

  it("keeps secret webhook URLs in destination instead of authentication", () => {
    const field = enhanceProviderField({ key: "webhook_url", label: "Webhook URL", secret: true }, "discord");

    expect(fieldSection(field, "discord")).toBe("destination");
  });

  it("treats provider display usernames as options, not credentials", () => {
    expect(fieldSection({ key: "username", label: "Username" }, "discord")).toBe("options");
    expect(fieldSection({ key: "username", label: "Username" }, "smtp")).toBe("auth");
  });

  it("describes bridge-backed providers differently from native providers", () => {
    expect(providerHelpText({ id: "smtp", name: "SMTP", fields: [] })).toContain("directly");
    expect(providerHelpText({ id: "signal_api", name: "Signal", fields: [], delivery_kind: "bridge" })).toContain("bridge");
  });
});
