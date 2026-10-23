import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { crossSection } from "./cross-section";

type FixtureCase = {
  name: string;
  material: {
    actualThicknessIn: number;
    actualWidthIn: number;
    roughThicknessIn: number;
    roughWidthIn: number;
  };
  roughCut?: boolean;
  want: { thicknessIn: number; widthIn: number };
};

const fixturePath = join(
  dirname(fileURLToPath(import.meta.url)),
  "../../backend/internal/store/testdata/cross_sections.json",
);

const cases = JSON.parse(readFileSync(fixturePath, "utf8")) as FixtureCase[];

describe("crossSection", () => {
  it("reads every shared fixture case", () => {
    expect(cases.length).toBeGreaterThan(0);
    for (const c of cases) {
      expect(crossSection(c.material, c.roughCut), c.name).toEqual(c.want);
    }
  });
});
