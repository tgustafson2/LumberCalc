import { describe, expect, it, vi } from "vitest";
import type {
  ClerkUserId,
  Design,
  DesignDocument,
  DesignId,
  DesignSummary,
  PatternId,
  Publication,
  UserId,
} from "../api";
import { stubClient } from "../test-client";
import {
  designActions,
  designSummaryText,
  formatUpdatedAt,
  loadDesignDetail,
  loadDesignList,
  nextDetailActivity,
  nextListActivity,
  type DetailActivity,
  type DetailEvent,
  type ListActivity,
  type ListEvent,
} from "./model";

const designId = "00000000-0000-4000-8000-000000000001" as DesignId;
const otherId = "00000000-0000-4000-8000-000000000002" as DesignId;
const patternId = "00000000-0000-4000-8000-000000000003" as PatternId;

const document = {
  schemaVersion: 1,
  units: "in",
  pieces: [
    {
      id: "piece-1",
      name: "rail",
      materialId: "mat-1",
      roughCut: false,
      lengthIn: 48,
      continuity: { x: true, y: false, z: false },
      transform: { positionIn: [0, 0, 0], rotationDeg: [0, 0, 0] },
    },
    {
      id: "piece-2",
      name: "stile",
      materialId: "mat-1",
      roughCut: false,
      lengthIn: 36,
      continuity: { x: false, y: true, z: false },
      transform: { positionIn: [1, 0, 0], rotationDeg: [0, 90, 0] },
    },
  ],
  connections: [
    {
      id: "conn-1",
      pieceAId: "piece-1",
      pieceBId: "piece-2",
      joinType: "butt",
      faceA: "end",
      faceB: "face",
      wasteIn: 0,
    },
  ],
} satisfies DesignDocument;

const design: Design = {
  id: designId,
  name: "Shelf",
  description: "A wall shelf",
  version: 4,
  updatedAt: "2026-03-15T18:04:00.000Z",
  document,
  publication: { kind: "unpublished" },
};

const { publication: _publication, ...editable } = design;

const summary: DesignSummary = {
  id: designId,
  name: "Shelf",
  description: "A wall shelf",
  version: 4,
  updatedAt: "2026-03-15T18:04:00.000Z",
};

describe("loadDesignList", () => {
  it("returns the me failure when the list also fails", async () => {
    const signal = new AbortController().signal;
    const me = vi.fn(async () => ({ kind: "failed" as const, status: 500, message: "me failed" }));
    const listDesigns = vi.fn(async () => ({
      kind: "failed" as const,
      status: 502,
      message: "list failed",
    }));

    await expect(loadDesignList(stubClient({ me, listDesigns }), signal)).resolves.toEqual({
      kind: "failed",
      status: 500,
      message: "me failed",
    });
    expect(me.mock.calls).toEqual([[{ signal }]]);
    expect(listDesigns.mock.calls).toEqual([[{ signal }]]);
  });

  it("returns the display name and the list in server order", async () => {
    const signal = new AbortController().signal;
    const second: DesignSummary = { ...summary, id: otherId, name: "Bench" };
    const me = vi.fn(async () => ({
      kind: "ok" as const,
      value: {
        userId: "user-1" as UserId,
        clerkUserId: "clerk-1" as ClerkUserId,
        displayName: "Ada",
      },
    }));
    const listDesigns = vi.fn(async () => ({ kind: "ok" as const, value: [summary, second] }));

    await expect(loadDesignList(stubClient({ me, listDesigns }), signal)).resolves.toEqual({
      kind: "ok",
      displayName: "Ada",
      designs: [summary, second],
    });
  });
});

describe("loadDesignDetail", () => {
  it("returns absent for a blank param and does not call getDesign", async () => {
    const signal = new AbortController().signal;
    const getDesign = vi.fn();

    await expect(loadDesignDetail(stubClient({ getDesign }), "   ", signal)).resolves.toEqual({
      kind: "absent",
    });
    expect(getDesign.mock.calls).toEqual([]);
  });

  it("returns absent when getDesign responds 404", async () => {
    const signal = new AbortController().signal;
    const getDesign = vi.fn(async () => ({
      kind: "failed" as const,
      status: 404,
      message: "not found",
    }));

    await expect(loadDesignDetail(stubClient({ getDesign }), designId, signal)).resolves.toEqual({
      kind: "absent",
    });
    expect(getDesign.mock.calls).toEqual([[designId, { signal }]]);
  });

  it("returns the loaded design without its publication", async () => {
    const signal = new AbortController().signal;
    const getDesign = vi.fn(async () => ({ kind: "ok" as const, value: design }));

    const model = await loadDesignDetail(stubClient({ getDesign }), ` ${designId} `, signal);

    expect(model.kind === "ok" ? model.design : model).toStrictEqual(editable);
    expect(model.kind === "ok" && "publication" in model.design).toBe(false);
    expect(getDesign.mock.calls).toEqual([[designId, { signal }]]);
  });

  const publish = { command: "publish", label: "Publish" };
  const republish = { command: "publish", label: "Republish" };
  const unpublish = { command: "unpublish", label: "Unpublish" };

  it.each<[Publication, readonly object[], string]>([
    [{ kind: "unpublished" }, [publish], "Not published."],
    [{ kind: "public", patternId }, [unpublish, republish], "Public in the patterns gallery."],
    [{ kind: "private", patternId }, [republish], "Private. Only you can see the pattern."],
  ])("shows the controls and sentence for %j", async (publication, controls, text) => {
    const getDesign = vi.fn(async () => ({ kind: "ok" as const, value: { ...design, publication } }));

    const model = await loadDesignDetail(stubClient({ getDesign }), designId, new AbortController().signal);

    expect(model).toEqual({ kind: "ok", design: editable, controls, publicationText: text });
  });
});

describe("setPublication", () => {
  it("returns changed and drops the pattern id", async () => {
    const signal = new AbortController().signal;
    const publishDesign = vi.fn(async () => ({ kind: "ok" as const, value: patternId }));
    const unpublishDesign = vi.fn(async () => ({ kind: "ok" as const, value: patternId }));
    const actions = designActions(stubClient({ publishDesign, unpublishDesign }));

    await expect(actions.setPublication(designId, "publish", signal)).resolves.toEqual({ kind: "changed" });
    await expect(actions.setPublication(designId, "unpublish", signal)).resolves.toEqual({ kind: "changed" });
    expect(publishDesign.mock.calls).toEqual([[designId, { signal }]]);
    expect(unpublishDesign.mock.calls).toEqual([[designId, { signal }]]);
  });

  it.each([
    [400, "empty_design", { kind: "rejected", notice: "Add at least one piece before you publish." }],
    [400, "custom_material", { kind: "rejected", notice: "Use only system materials before you publish." }],
    [404, "not found", { kind: "absent" }],
    [400, "bad request", { kind: "failed", status: 400, message: "bad request" }],
  ])("maps a %i %s response", async (status, message, expected) => {
    const publishDesign = vi.fn(async () => ({ kind: "failed" as const, status, message }));

    await expect(
      designActions(stubClient({ publishDesign })).setPublication(designId, "publish"),
    ).resolves.toEqual(expected);
  });
});

describe("designActions", () => {
  it("does not create a design when the name is blank", async () => {
    const createDesign = vi.fn();
    const actions = designActions(stubClient({ createDesign }));

    await expect(actions.create("   ")).resolves.toEqual({ kind: "blank-name" });
    expect(createDesign.mock.calls).toEqual([]);
  });

  it("creates a design with the trimmed name only", async () => {
    const signal = new AbortController().signal;
    const createDesign = vi.fn(async () => ({ kind: "ok" as const, value: design }));
    const actions = designActions(stubClient({ createDesign }));

    await expect(actions.create("  Shelf  ", signal)).resolves.toEqual({
      kind: "created",
      design,
    });
    expect(createDesign.mock.calls).toEqual([[{ name: "Shelf" }, { signal }]]);
  });

  it("does not replace a design when the name is blank or already saved", async () => {
    const replaceDesign = vi.fn();
    const actions = designActions(stubClient({ replaceDesign }));

    await expect(actions.rename(design, "  ")).resolves.toEqual({ kind: "blank-name" });
    await expect(actions.rename(design, "  Shelf  ")).resolves.toEqual({ kind: "same-name" });
    expect(replaceDesign.mock.calls).toEqual([]);
  });

  it("replaces a design with the loaded description, document, and version", async () => {
    const signal = new AbortController().signal;
    const replaceDesign = vi.fn(async () => ({ kind: "ok" as const, value: design }));
    const actions = designActions(stubClient({ replaceDesign }));

    await expect(actions.rename(design, "  Bench  ", signal)).resolves.toEqual({ kind: "renamed" });
    expect(replaceDesign.mock.calls).toEqual([
      [
        designId,
        {
          name: "Bench",
          description: "A wall shelf",
          document,
          version: 4,
        },
        { signal },
      ],
    ]);
  });

  it("returns stale when replaceDesign reports a stale version", async () => {
    const replaceDesign = vi.fn(async () => ({
      kind: "failed" as const,
      status: 409,
      message: "stale version",
    }));
    const actions = designActions(stubClient({ replaceDesign }));

    await expect(actions.rename(design, "Bench")).resolves.toEqual({ kind: "stale" });
  });

  it("returns absent when replaceDesign responds 404", async () => {
    const replaceDesign = vi.fn(async () => ({
      kind: "failed" as const,
      status: 404,
      message: "not found",
    }));
    const actions = designActions(stubClient({ replaceDesign }));

    await expect(actions.rename(design, "Bench")).resolves.toEqual({ kind: "absent" });
  });

  it("keeps a 409 conflict as a failed result", async () => {
    const replaceDesign = vi.fn(async () => ({
      kind: "failed" as const,
      status: 409,
      message: "conflict",
    }));
    const actions = designActions(stubClient({ replaceDesign }));

    await expect(actions.rename(design, "Bench")).resolves.toEqual({
      kind: "failed",
      status: 409,
      message: "conflict",
    });
  });

  it("returns gone when deleteDesign returns the 204 result or a 404", async () => {
    const signal = new AbortController().signal;
    const deleted = vi.fn(async () => ({ kind: "ok" as const, value: null }));
    const missing = vi.fn(async () => ({
      kind: "failed" as const,
      status: 404,
      message: "not found",
    }));

    await expect(designActions(stubClient({ deleteDesign: deleted })).delete(designId, signal)).resolves.toEqual({
      kind: "gone",
    });
    await expect(designActions(stubClient({ deleteDesign: missing })).delete(designId, signal)).resolves.toEqual({
      kind: "gone",
    });
    expect(deleted.mock.calls).toEqual([[designId, { signal }]]);
    expect(missing.mock.calls).toEqual([[designId, { signal }]]);
  });

  it("copies a design and drops the response body", async () => {
    const signal = new AbortController().signal;
    const copyDesign = vi.fn(async () => ({ kind: "ok" as const, value: design }));
    const actions = designActions(stubClient({ copyDesign }));

    await expect(actions.copy(designId, signal)).resolves.toEqual({ kind: "copied" });
    expect(copyDesign.mock.calls).toEqual([[designId, { signal }]]);
  });
});

const mediumShortTime = new Intl.DateTimeFormat("en-US", {
  dateStyle: "medium",
  timeStyle: "short",
});

describe("formatUpdatedAt", () => {
  it("returns an unparsable value unchanged", () => {
    expect(formatUpdatedAt("not-a-date")).toBe("not-a-date");
  });

  it("formats a parsed value in en-US with a medium date and a short time", () => {
    const instant = "2026-03-15T18:04:00.000Z";
    expect(formatUpdatedAt(instant)).toBe(mediumShortTime.format(new Date(instant)));
  });
});

describe("designSummaryText", () => {
  it("counts pieces and connections and uses the formatted time", () => {
    const time = mediumShortTime.format(new Date(design.updatedAt));
    expect(designSummaryText(design)).toBe(
      `Version 4. Updated ${time}. Piece count 2. Connection count 1.`,
    );
  });
});

describe("nextListActivity", () => {
  const idle: ListActivity = { kind: "idle" };
  const confirm: ListActivity = { kind: "confirm-delete", id: designId };
  const working: ListActivity = { kind: "working" };

  it.each<[ListActivity, ListEvent, ListActivity]>([
    [idle, { type: "start-work" }, working],
    [confirm, { type: "start-work" }, working],
    [working, { type: "start-work" }, working],
    [idle, { type: "ask-delete", id: designId }, confirm],
    [working, { type: "ask-delete", id: designId }, working],
    [confirm, { type: "ask-delete", id: otherId }, confirm],
    [confirm, { type: "cancel-delete" }, idle],
    [idle, { type: "cancel-delete" }, idle],
    [working, { type: "stop-work" }, idle],
    [confirm, { type: "stop-work" }, confirm],
    [idle, { type: "stop-work" }, idle],
  ])("moves %j on %j to %j", (state, event, expected) => {
    expect(nextListActivity(state, event)).toEqual(expected);
  });
});

describe("nextDetailActivity", () => {
  const idle: DetailActivity = { kind: "idle" };
  const working: DetailActivity = { kind: "working" };
  const stale: DetailActivity = { kind: "stale" };

  it.each<[DetailActivity, DetailEvent, DetailActivity]>([
    [idle, { type: "start-work" }, working],
    [working, { type: "start-work" }, working],
    [stale, { type: "start-work" }, stale],
    [working, { type: "mark-stale" }, stale],
    [idle, { type: "mark-stale" }, idle],
    [stale, { type: "mark-stale" }, stale],
    [working, { type: "stop-work" }, idle],
    [stale, { type: "stop-work" }, stale],
    [idle, { type: "stop-work" }, idle],
    [stale, { type: "load-latest" }, idle],
    [idle, { type: "load-latest" }, idle],
    [working, { type: "load-latest" }, working],
  ])("moves %j on %j to %j", (state, event, expected) => {
    expect(nextDetailActivity(state, event)).toEqual(expected);
  });
});
