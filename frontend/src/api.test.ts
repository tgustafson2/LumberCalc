import { afterEach, describe, expect, it, vi } from "vitest";
import { createV1Client, type DesignId } from "./api";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createV1Client me", () => {
  it("returns no-token and does not call fetch when Clerk has no token", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const client = createV1Client({ getToken: async () => null, baseUrl: "http://localhost" });

    await expect(client.me()).resolves.toEqual({ kind: "no-token" });
    expect(fetchMock.mock.calls).toEqual([]);
  });

  it("returns unauthorized when the API responds 401", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 })),
    );
    const client = createV1Client({
      getToken: async () => "session-token",
      baseUrl: "http://localhost",
    });

    await expect(client.me()).resolves.toEqual({ kind: "unauthorized" });
  });
});

describe("createV1Client deleteDesign", () => {
  it("returns ok with a nil value when the API responds 204", async () => {
    vi.stubGlobal("fetch", vi.fn(async () => new Response(null, { status: 204 })));
    const client = createV1Client({
      getToken: async () => "session-token",
      baseUrl: "http://localhost",
    });

    await expect(
      client.deleteDesign("00000000-0000-4000-8000-000000000001" as DesignId),
    ).resolves.toEqual({ kind: "ok", value: null });
  });
});
