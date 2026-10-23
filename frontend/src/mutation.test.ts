import { describe, expect, it, vi } from "vitest";
import { beginAction, mutationView, type ActionGate } from "./mutation";

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
