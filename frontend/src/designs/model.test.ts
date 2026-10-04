import { describe, expect, it, vi } from "vitest";
import type {
  ClerkUserId,
  Design,
  DesignDocument,
  DesignId,
  DesignSummary,
  UserId,
  V1Client,
} from "../api";
import {
  beginAction,
  designActions,
  designSummaryText,
  formatUpdatedAt,
  loadDesignDetail,
  loadDesignList,
  mutationView,
  nextDetailActivity,
  nextListActivity,
  type ActionGate,
  type DetailActivity,
  type DetailEvent,
  type ListActivity,
  type ListEvent,
} from "./model";

const designId = "00000000-0000-4000-8000-000000000001" as DesignId;
const otherId = "00000000-0000-4000-8000-000000000002" as DesignId;

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
};

const summary: DesignSummary = {
  id: designId,
  name: "Shelf",
  description: "A wall shelf",
  version: 4,
  updatedAt: "2026-03-15T18:04:00.000Z",
};

function client(methods: Partial<V1Client>): V1Client {
  return {
    me: async () => ({ kind: "invalid" }),
    createDesign: async () => ({ kind: "invalid" }),
    listDesigns: async () => ({ kind: "invalid" }),
    getDesign: async () => ({ kind: "invalid" }),
    replaceDesign: async () => ({ kind: "invalid" }),
    deleteDesign: async () => ({ kind: "invalid" }),
    copyDesign: async () => ({ kind: "invalid" }),
    ...methods,
  };
}

describe("loadDesignList", () => {
  it("returns the me failure when the list also fails", async () => {
    const signal = new AbortController().signal;
    const me = vi.fn(async () => ({ kind: "failed" as const, status: 500, message: "me failed" }));
    const listDesigns = vi.fn(async () => ({
      kind: "failed" as const,
      status: 502,
      message: "list failed",
    }));

    await expect(loadDesignList(client({ me, listDesigns }), signal)).resolves.toEqual({
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

    await expect(loadDesignList(client({ me, listDesigns }), signal)).resolves.toEqual({
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

    await expect(loadDesignDetail(client({ getDesign }), "   ", signal)).resolves.toEqual({
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

    await expect(loadDesignDetail(client({ getDesign }), designId, signal)).resolves.toEqual({
      kind: "absent",
    });
    expect(getDesign.mock.calls).toEqual([[designId, { signal }]]);
  });

  it("returns the loaded design", async () => {
    const signal = new AbortController().signal;
    const getDesign = vi.fn(async () => ({ kind: "ok" as const, value: design }));

    await expect(loadDesignDetail(client({ getDesign }), ` ${designId} `, signal)).resolves.toEqual({
      kind: "ok",
      design,
    });
    expect(getDesign.mock.calls).toEqual([[designId, { signal }]]);
  });
});

describe("designActions", () => {
  it("does not create a design when the name is blank", async () => {
    const createDesign = vi.fn();
    const actions = designActions(client({ createDesign }));

    await expect(actions.create("   ")).resolves.toEqual({ kind: "blank-name" });
    expect(createDesign.mock.calls).toEqual([]);
  });

  it("creates a design with the trimmed name only", async () => {
    const signal = new AbortController().signal;
    const createDesign = vi.fn(async () => ({ kind: "ok" as const, value: design }));
    const actions = designActions(client({ createDesign }));

    await expect(actions.create("  Shelf  ", signal)).resolves.toEqual({
      kind: "created",
      design,
    });
    expect(createDesign.mock.calls).toEqual([[{ name: "Shelf" }, { signal }]]);
  });

  it("does not replace a design when the name is blank or already saved", async () => {
    const replaceDesign = vi.fn();
    const actions = designActions(client({ replaceDesign }));

    await expect(actions.rename(design, "  ")).resolves.toEqual({ kind: "blank-name" });
    await expect(actions.rename(design, "  Shelf  ")).resolves.toEqual({ kind: "same-name" });
    expect(replaceDesign.mock.calls).toEqual([]);
  });

  it("replaces a design with the loaded description, document, and version", async () => {
    const signal = new AbortController().signal;
    const replaceDesign = vi.fn(async () => ({ kind: "ok" as const, value: design }));
    const actions = designActions(client({ replaceDesign }));

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
    const actions = designActions(client({ replaceDesign }));

    await expect(actions.rename(design, "Bench")).resolves.toEqual({ kind: "stale" });
  });

  it("returns absent when replaceDesign responds 404", async () => {
    const replaceDesign = vi.fn(async () => ({
      kind: "failed" as const,
      status: 404,
      message: "not found",
    }));
    const actions = designActions(client({ replaceDesign }));

    await expect(actions.rename(design, "Bench")).resolves.toEqual({ kind: "absent" });
  });

  it("keeps a 409 conflict as a failed result", async () => {
    const replaceDesign = vi.fn(async () => ({
      kind: "failed" as const,
      status: 409,
      message: "conflict",
    }));
    const actions = designActions(client({ replaceDesign }));

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

    await expect(designActions(client({ deleteDesign: deleted })).delete(designId, signal)).resolves.toEqual({
      kind: "gone",
    });
    await expect(designActions(client({ deleteDesign: missing })).delete(designId, signal)).resolves.toEqual({
      kind: "gone",
    });
    expect(deleted.mock.calls).toEqual([[designId, { signal }]]);
    expect(missing.mock.calls).toEqual([[designId, { signal }]]);
  });

  it("copies a design and drops the response body", async () => {
    const signal = new AbortController().signal;
    const copyDesign = vi.fn(async () => ({ kind: "ok" as const, value: design }));
    const actions = designActions(client({ copyDesign }));

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

describe("mutationView", () => {
  it("shows a failed mutation message on the current screen", () => {
    expect(mutationView({ kind: "failed", status: 409, message: "conflict" })).toEqual({
      kind: "notice",
      message: "conflict",
    });
    expect(mutationView({ kind: "unreachable" })).toEqual({
      kind: "notice",
      message: "The API is not reachable.",
    });
    expect(mutationView({ kind: "invalid" })).toEqual({
      kind: "notice",
      message: "The API sent an unexpected response.",
    });
  });

  it("replaces the screen when the session is gone", () => {
    expect(mutationView({ kind: "no-token" })).toEqual({
      kind: "session",
      failure: { kind: "no-token" },
    });
    expect(mutationView({ kind: "unauthorized" })).toEqual({
      kind: "session",
      failure: { kind: "unauthorized" },
    });
  });
});

describe("beginAction", () => {
  it("returns null while busy and does not start the second action", async () => {
    let release: (value: "done") => void = () => {};
    const gate: ActionGate = { current: "open" };
    const first = vi.fn(
      () =>
        new Promise<"done">((resolve) => {
          release = resolve;
        }),
    );
    const second = vi.fn(async () => "done" as const);

    const job = beginAction(gate, first);
    expect(gate.current).toBe("busy");
    expect(beginAction(gate, second)).toBe(null);
    expect(second.mock.calls).toEqual([]);

    release("done");
    await job;
    expect(gate.current).toBe("open");
  });

  it("returns null while held and does not start work", () => {
    const gate: ActionGate = { current: "held" };
    const work = vi.fn(async () => "done" as const);

    expect(beginAction(gate, work)).toBe(null);
    expect(work.mock.calls).toEqual([]);
    expect(gate.current).toBe("held");
  });

  it("leaves the gate held when the action returns hold", async () => {
    const gate: ActionGate = { current: "open" };
    const work = vi.fn(async () => "hold" as const);

    const job = beginAction(gate, work);
    expect(gate.current).toBe("busy");
    await job;
    expect(gate.current).toBe("held");
    expect(work.mock.calls).toEqual([[]]);
  });
});
