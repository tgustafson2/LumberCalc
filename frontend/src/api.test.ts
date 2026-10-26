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

describe("createV1Client getDesign publication", () => {
  const designId = "00000000-0000-4000-8000-000000000001" as DesignId;
  const patternId = "00000000-0000-4000-8000-000000000002";

  function respondWith(publication: unknown) {
    const body = {
      id: designId,
      name: "Shelf",
      description: "",
      version: 1,
      updatedAt: "2026-03-15T18:04:00.000Z",
      document: { schemaVersion: 1, units: "in", pieces: [], connections: [] },
      publication,
    };
    vi.stubGlobal(
      "fetch",
      vi.fn(
        async () =>
          new Response(JSON.stringify(body), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          }),
      ),
    );
    return createV1Client({ getToken: async () => "session-token", baseUrl: "http://localhost" });
  }

  it.each([
    [null, { kind: "unpublished" }],
    [{ patternId, isPublic: true }, { kind: "public", patternId }],
    [{ patternId, isPublic: false }, { kind: "private", patternId }],
  ])("parses %j", async (publication, expected) => {
    const result = await respondWith(publication).getDesign(designId);

    expect(result.kind === "ok" ? result.value.publication : result).toEqual(expected);
  });

  it.each([
    [{ patternId: "  ", isPublic: true }],
    [{ patternId }],
    [undefined],
  ])("returns invalid for %j", async (publication) => {
    await expect(respondWith(publication).getDesign(designId)).resolves.toEqual({ kind: "invalid" });
  });
});
