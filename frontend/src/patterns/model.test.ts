import { describe, expect, it, vi } from "vitest";
import type { Design, DesignId, Pattern, PatternId } from "../api";
import { stubClient } from "../test-client";
import { addPattern, loadPatternDetail } from "./model";

const patternId = "00000000-0000-4000-8000-000000000003" as PatternId;

const piece = {
  id: "piece-1",
  name: "rail",
  materialId: null,
  roughCut: false,
  lengthIn: 48,
  continuity: { x: true, y: false, z: false },
  transform: { positionIn: [0, 0, 0], rotationDeg: [0, 0, 0] },
};

const pattern: Pattern = {
  id: patternId,
  name: "Shelf",
  description: "",
  publisherName: null,
  publishedAt: "2026-03-15T18:04:00.000Z",
  isPublic: false,
  document: {
    schemaVersion: 1,
    units: "in",
    pieces: [piece, { ...piece, id: "piece-2" }],
    connections: [],
  },
};

describe("loadPatternDetail", () => {
  it("returns the view of a private pattern from an unnamed publisher", async () => {
    const signal = new AbortController().signal;
    const getPattern = vi.fn(async () => ({ kind: "ok" as const, value: pattern }));

    await expect(loadPatternDetail(stubClient({ getPattern }), ` ${patternId} `, signal)).resolves.toEqual({
      kind: "ok",
      pattern: {
        id: patternId,
        name: "Shelf",
        description: "",
        publisherLabel: "By an unnamed publisher",
        pieceCount: 2,
        canAdd: false,
      },
    });
    expect(getPattern.mock.calls).toEqual([[patternId, { signal }]]);
  });

  it("labels a named publisher and allows add for a public pattern", async () => {
    const getPattern = vi.fn(async () => ({
      kind: "ok" as const,
      value: { ...pattern, publisherName: "Ada", isPublic: true },
    }));

    const model = await loadPatternDetail(stubClient({ getPattern }), patternId, new AbortController().signal);

    expect(model.kind === "ok" ? [model.pattern.publisherLabel, model.pattern.canAdd] : model).toEqual([
      "By Ada",
      true,
    ]);
  });

  it("returns absent for a blank param and for a 404", async () => {
    const signal = new AbortController().signal;
    const getPattern = vi.fn(async () => ({ kind: "failed" as const, status: 404, message: "not found" }));

    await expect(loadPatternDetail(stubClient({ getPattern }), "  ", signal)).resolves.toEqual({ kind: "absent" });
    await expect(loadPatternDetail(stubClient({ getPattern }), patternId, signal)).resolves.toEqual({
      kind: "absent",
    });
    expect(getPattern.mock.calls).toEqual([[patternId, { signal }]]);
  });
});

describe("addPattern", () => {
  const designId = "00000000-0000-4000-8000-000000000004" as DesignId;

  it("returns the new design id", async () => {
    const add = vi.fn(async () => ({ kind: "ok" as const, value: { id: designId } as Design }));
    const run = addPattern(stubClient({ addPattern: add }));

    await expect(run(patternId)).resolves.toEqual({ kind: "added", designId });
  });

  it("returns absent for a missing pattern and a notice when a material is unavailable", async () => {
    const missing = vi.fn(async () => ({ kind: "failed" as const, status: 404, message: "not found" }));
    const unavailable = vi.fn(async () => ({
      kind: "failed" as const,
      status: 409,
      message: "material not available",
    }));

    await expect(addPattern(stubClient({ addPattern: missing }))(patternId)).resolves.toEqual({ kind: "absent" });
    await expect(addPattern(stubClient({ addPattern: unavailable }))(patternId)).resolves.toEqual({
      kind: "rejected",
      notice: "This pattern uses a material that is not available.",
    });
  });
});
