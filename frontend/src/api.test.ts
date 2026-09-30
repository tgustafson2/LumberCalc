import { afterEach, describe, expect, it, vi } from "vitest";
import { createV1Client } from "./api";

afterEach(() => {
  vi.unstubAllGlobals();
});

describe("createV1Client me", () => {
  it("returns no-token and does not call fetch when Clerk has no token", async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal("fetch", fetchMock);
    const client = createV1Client({ getToken: async () => null });

    await expect(client.me()).resolves.toEqual({ kind: "no-token" });
    expect(fetchMock.mock.calls).toEqual([]);
  });

  it("returns unauthorized when the API responds 401", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => new Response(JSON.stringify({ error: "unauthorized" }), { status: 401 })),
    );
    const client = createV1Client({ getToken: async () => "session-token" });

    await expect(client.me()).resolves.toEqual({ kind: "unauthorized" });
  });
});
