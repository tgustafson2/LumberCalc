import { describe, expect, it } from "vitest";
import {
  dollarsText,
  formatCents,
  formatInches,
  formatLength,
  formatSize,
  parseDollars,
  parseInches,
} from "./units";

describe("parseDollars", () => {
  it.each([
    ["0", 0],
    ["4.1", 410],
    ["4.10", 410],
    ["$1,250.00", 125000],
    ["21474836.47", 2147483647],
  ])("parses %s as %i cents", (raw, cents) => {
    expect(parseDollars(raw)).toEqual({ kind: "ok", value: cents });
  });

  it.each(["4.999", "-1", "", "4."])("rejects %s", (raw) => {
    expect(parseDollars(raw)).toEqual({
      kind: "invalid",
      problem: "Enter dollars, like 4.98.",
    });
  });

  it("rejects a price above 2147483647 cents", () => {
    expect(parseDollars("21474836.48")).toEqual({
      kind: "invalid",
      problem: "That price is too large.",
    });
  });
});

describe("parseInches", () => {
  it.each([
    ["1 1/2", 1.5],
    ["1-1/2", 1.5],
    ["3/4", 0.75],
    ['1.5"', 1.5],
    ["3/4in", 0.75],
  ])("parses %s as %s inches", (raw, inches) => {
    expect(parseInches(raw)).toEqual({ kind: "ok", value: inches });
  });

  it.each(["0", "1/0"])("rejects %s", (raw) => {
    expect(parseInches(raw)).toEqual({
      kind: "invalid",
      problem: "Enter inches greater than 0.",
    });
  });
});

describe("formatLength", () => {
  it.each([
    [96, "8 ft"],
    [100, "8 ft 4 in"],
    [8, "8 in"],
  ])("formats %i inches as %s", (lengthIn, label) => {
    expect(formatLength(lengthIn)).toBe(label);
  });
});

describe("format round trips", () => {
  it("formats inches as the shortest text that parses back", () => {
    expect(formatInches(1.5)).toBe("1.5");
    expect(formatInches(4)).toBe("4");
    expect(parseInches(formatInches(1.5))).toEqual({ kind: "ok", value: 1.5 });
    expect(parseInches(formatInches(4))).toEqual({ kind: "ok", value: 4 });
    expect(formatSize(1.5, 3.5)).toBe("1.5 x 3.5");
  });

  it("formats cents as dollar text that parses back", () => {
    expect(dollarsText(498)).toBe("4.98");
    expect(formatCents(498)).toBe("$4.98");
    for (const cents of [0, 410, 498, 125000, 2147483647]) {
      expect(parseDollars(dollarsText(cents))).toEqual({ kind: "ok", value: cents });
    }
  });
});
