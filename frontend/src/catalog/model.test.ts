import { describe, expect, it, vi } from "vitest";
import type { ClerkUserId, Material, MaterialId, Price, PriceId, UserId, V1Client } from "../api";
import {
  blankMaterialFields,
  buildPriceGrid,
  cellAt,
  loadCatalog,
  parseMaterialFields,
  runCatalogCommand,
  type Catalog,
  type CatalogMaterial,
  type CustomMaterial,
  type MaterialFields,
  type PriceGrid,
  type PriceRow,
} from "./model";

const systemId = "00000000-0000-4000-8000-000000000010" as MaterialId;
const systemLaterId = "00000000-0000-4000-8000-000000000011" as MaterialId;
const customId = "00000000-0000-4000-8000-000000000020" as MaterialId;
const customLaterId = "00000000-0000-4000-8000-000000000021" as MaterialId;
const orphanId = "00000000-0000-4000-8000-000000000030" as MaterialId;
const priceId = "00000000-0000-4000-8000-000000000101" as PriceId;
const extraPriceId = "00000000-0000-4000-8000-000000000102" as PriceId;
const orphanPriceId = "00000000-0000-4000-8000-000000000103" as PriceId;

function material(fields: {
  readonly id: MaterialId;
  readonly name: string;
  readonly system: boolean;
  readonly species?: string | null;
  readonly grade?: string | null;
}): Material {
  return {
    id: fields.id,
    name: fields.name,
    species: fields.species ?? null,
    grade: fields.grade ?? null,
    system: fields.system,
    nominalThicknessIn: 2,
    nominalWidthIn: 4,
    actualThicknessIn: 1.5,
    actualWidthIn: 3.5,
    roughThicknessIn: 2,
    roughWidthIn: 4,
    updatedAt: "2026-03-15T18:04:00.000Z",
  };
}

function catalogMaterial(fields: {
  readonly id: MaterialId;
  readonly name: string;
  readonly system: boolean;
  readonly species?: string | null;
  readonly grade?: string | null;
}): CatalogMaterial {
  const value = material(fields);
  if (value.system) {
    return { ...value, system: true };
  }
  return { ...value, system: false };
}

function price(fields: {
  readonly id: PriceId;
  readonly materialId: MaterialId;
  readonly lengthIn: number;
  readonly priceCents: number;
}): Price {
  return {
    id: fields.id,
    materialId: fields.materialId,
    lengthIn: fields.lengthIn,
    priceCents: fields.priceCents,
    updatedAt: "2026-03-15T18:04:00.000Z",
  };
}

const systemBoard = catalogMaterial({ id: systemId, name: "2x4", system: true });
const customBoard = catalogMaterial({
  id: customId,
  name: "Pine",
  system: false,
  species: "Pine",
  grade: "No. 2",
});

function asCustom(value: CatalogMaterial): CustomMaterial {
  if (value.system) {
    throw new Error("expected a custom material");
  }
  return value;
}

const custom = asCustom(customBoard);

const pricedAt8 = price({ id: priceId, materialId: systemId, lengthIn: 96, priceCents: 498 });
const pricedAt100 = price({
  id: extraPriceId,
  materialId: systemId,
  lengthIn: 100,
  priceCents: 600,
});
const orphanPrice = price({
  id: orphanPriceId,
  materialId: orphanId,
  lengthIn: 120,
  priceCents: 100,
});

function fields(overrides: Partial<MaterialFields> = {}): MaterialFields {
  return { ...blankMaterialFields, ...overrides };
}

const readyFields = fields({
  name: "  Pine  ",
  species: "   ",
  grade: "",
  nominalThicknessIn: "2",
  nominalWidthIn: "4",
  actualThicknessIn: "1.5",
  actualWidthIn: "3.5",
  roughThicknessIn: "1 1/2",
  roughWidthIn: "3/4",
});

function client(methods: Partial<V1Client>): V1Client {
  return {
    me: async () => ({ kind: "invalid" }),
    createDesign: async () => ({ kind: "invalid" }),
    listDesigns: async () => ({ kind: "invalid" }),
    getDesign: async () => ({ kind: "invalid" }),
    replaceDesign: async () => ({ kind: "invalid" }),
    deleteDesign: async () => ({ kind: "invalid" }),
    copyDesign: async () => ({ kind: "invalid" }),
    createMaterial: async () => ({ kind: "invalid" }),
    listMaterials: async () => ({ kind: "invalid" }),
    replaceMaterial: async () => ({ kind: "invalid" }),
    deleteMaterial: async () => ({ kind: "invalid" }),
    createPrice: async () => ({ kind: "invalid" }),
    listPrices: async () => ({ kind: "invalid" }),
    updatePrice: async () => ({ kind: "invalid" }),
    deletePrice: async () => ({ kind: "invalid" }),
    ...methods,
  };
}

function rowAt(grid: PriceGrid, index: number): PriceRow {
  const row = grid.rows[index];
  if (row === undefined) {
    throw new Error(`missing row ${index}`);
  }
  return row;
}

function columnAt(grid: PriceGrid, lengthIn: number) {
  const column = grid.columns.find((item) => item.lengthIn === lengthIn);
  if (column === undefined) {
    throw new Error(`missing column ${lengthIn}`);
  }
  return column;
}

describe("loadCatalog", () => {
  it("returns the me failure when the lists also fail", async () => {
    const signal = new AbortController().signal;
    const me = vi.fn(async () => ({ kind: "failed" as const, status: 500, message: "me failed" }));
    const listMaterials = vi.fn(async () => ({
      kind: "failed" as const,
      status: 502,
      message: "materials failed",
    }));
    const listPrices = vi.fn(async () => ({
      kind: "failed" as const,
      status: 503,
      message: "prices failed",
    }));

    await expect(loadCatalog(client({ me, listMaterials, listPrices }), signal)).resolves.toEqual({
      kind: "failed",
      status: 500,
      message: "me failed",
    });
    expect(me.mock.calls).toEqual([[{ signal }]]);
    expect(listMaterials.mock.calls).toEqual([[{ signal }]]);
    expect(listPrices.mock.calls).toEqual([[{ signal }]]);
  });

  it("returns the materials failure before the prices failure", async () => {
    const signal = new AbortController().signal;
    const me = vi.fn(async () => ({
      kind: "ok" as const,
      value: {
        userId: "user-1" as UserId,
        clerkUserId: "clerk-1" as ClerkUserId,
        displayName: "Ada",
      },
    }));
    const listMaterials = vi.fn(async () => ({
      kind: "failed" as const,
      status: 502,
      message: "materials failed",
    }));
    const listPrices = vi.fn(async () => ({
      kind: "failed" as const,
      status: 503,
      message: "prices failed",
    }));

    await expect(loadCatalog(client({ me, listMaterials, listPrices }), signal)).resolves.toEqual({
      kind: "failed",
      status: 502,
      message: "materials failed",
    });
  });

  it("returns the display name and sorts system rows before custom rows", async () => {
    const signal = new AbortController().signal;
    const me = vi.fn(async () => ({
      kind: "ok" as const,
      value: {
        userId: "user-1" as UserId,
        clerkUserId: "clerk-1" as ClerkUserId,
        displayName: "Ada",
      },
    }));
    const listMaterials = vi.fn(async () => ({
      kind: "ok" as const,
      value: [
        material({ id: customLaterId, name: "Cedar", system: false }),
        material({ id: systemLaterId, name: "2x4", system: true }),
        material({ id: customId, name: "Cedar", system: false }),
        material({ id: systemId, name: "2x4", system: true }),
      ],
    }));
    const listPrices = vi.fn(async () => ({ kind: "ok" as const, value: [pricedAt8] }));

    await expect(loadCatalog(client({ me, listMaterials, listPrices }), signal)).resolves.toEqual({
      kind: "ok",
      displayName: "Ada",
      materials: [
        catalogMaterial({ id: systemId, name: "2x4", system: true }),
        catalogMaterial({ id: systemLaterId, name: "2x4", system: true }),
        catalogMaterial({ id: customId, name: "Cedar", system: false }),
        catalogMaterial({ id: customLaterId, name: "Cedar", system: false }),
      ],
      prices: [pricedAt8],
    });
  });
});

describe("parseMaterialFields", () => {
  it("trims the name, clears blank species and grade, and parses every inch field", () => {
    expect(parseMaterialFields(readyFields)).toEqual({
      kind: "ok",
      write: {
        name: "Pine",
        species: null,
        grade: null,
        nominalThicknessIn: 2,
        nominalWidthIn: 4,
        actualThicknessIn: 1.5,
        actualWidthIn: 3.5,
        roughThicknessIn: 1.5,
        roughWidthIn: 0.75,
      },
    });
  });

  it("collects every invalid field", () => {
    expect(
      parseMaterialFields(
        fields({
          name: "  ",
          species: "Fir",
          nominalThicknessIn: "0",
          nominalWidthIn: "1/0",
          actualThicknessIn: "1",
          actualWidthIn: "2",
          roughThicknessIn: "3",
          roughWidthIn: "4",
        }),
      ),
    ).toEqual({
      kind: "invalid",
      problems: {
        name: "Enter a name.",
        nominalThicknessIn: "Enter inches greater than 0.",
        nominalWidthIn: "Enter inches greater than 0.",
      },
    });
  });
});

describe("runCatalogCommand", () => {
  it("does not create a material when an inch field is invalid", async () => {
    const createMaterial = vi.fn();
    const outcome = await runCatalogCommand(client({ createMaterial }), {
      type: "create-material",
      fields: fields({
        name: "Pine",
        nominalThicknessIn: "0",
        nominalWidthIn: "4",
        actualThicknessIn: "1.5",
        actualWidthIn: "3.5",
        roughThicknessIn: "2",
        roughWidthIn: "4",
      }),
    });

    expect(outcome).toEqual({
      kind: "invalid",
      notice: "Enter inches greater than 0.",
      problems: { nominalThicknessIn: "Enter inches greater than 0." },
    });
    expect(createMaterial.mock.calls).toEqual([]);
  });

  it("does not replace a material when an inch field is invalid", async () => {
    const replaceMaterial = vi.fn();
    await runCatalogCommand(client({ replaceMaterial }), {
      type: "replace-material",
      target: custom,
      fields: fields({ name: "Pine", nominalThicknessIn: "nope" }),
    });
    expect(replaceMaterial.mock.calls).toEqual([]);
  });

  it("creates a material with the parsed write", async () => {
    const signal = new AbortController().signal;
    const createMaterial = vi.fn(async () => ({ kind: "ok" as const, value: material(custom) }));
    await expect(
      runCatalogCommand(
        client({ createMaterial }),
        { type: "create-material", fields: readyFields },
        signal,
      ),
    ).resolves.toEqual({ kind: "saved" });
    expect(createMaterial.mock.calls).toEqual([
      [
        {
          name: "Pine",
          species: null,
          grade: null,
          nominalThicknessIn: 2,
          nominalWidthIn: 4,
          actualThicknessIn: 1.5,
          actualWidthIn: 3.5,
          roughThicknessIn: 1.5,
          roughWidthIn: 0.75,
        },
        { signal },
      ],
    ]);
  });

  it("keeps the API message when a material is in use", async () => {
    const deleteMaterial = vi.fn(async () => ({
      kind: "failed" as const,
      status: 409,
      message: "material in use",
    }));
    await expect(
      runCatalogCommand(client({ deleteMaterial }), { type: "delete-material", target: custom }),
    ).resolves.toEqual({ kind: "failed", status: 409, message: "material in use" });
  });

  it("rewrites a forbidden material change", async () => {
    const replaceMaterial = vi.fn(async () => ({
      kind: "failed" as const,
      status: 403,
      message: "forbidden",
    }));
    const deleteMaterial = vi.fn(async () => ({
      kind: "failed" as const,
      status: 403,
      message: "forbidden",
    }));

    await expect(
      runCatalogCommand(client({ replaceMaterial }), {
        type: "replace-material",
        target: custom,
        fields: readyFields,
      }),
    ).resolves.toEqual({
      kind: "failed",
      status: 403,
      message: "System materials cannot be changed.",
    });
    await expect(
      runCatalogCommand(client({ deleteMaterial }), { type: "delete-material", target: custom }),
    ).resolves.toEqual({
      kind: "failed",
      status: 403,
      message: "System materials cannot be changed.",
    });
  });

  it("treats a missing material or price as saved", async () => {
    const signal = new AbortController().signal;
    const deleteMaterial = vi.fn(async () => ({
      kind: "failed" as const,
      status: 404,
      message: "not found",
    }));
    const deletePrice = vi.fn(async () => ({
      kind: "failed" as const,
      status: 404,
      message: "not found",
    }));

    await expect(
      runCatalogCommand(
        client({ deleteMaterial }),
        { type: "delete-material", target: custom },
        signal,
      ),
    ).resolves.toEqual({ kind: "saved" });
    await expect(
      runCatalogCommand(client({ deletePrice }), { type: "delete-price", price: pricedAt8 }, signal),
    ).resolves.toEqual({ kind: "saved" });
    expect(deleteMaterial.mock.calls).toEqual([[custom.id, { signal }]]);
    expect(deletePrice.mock.calls).toEqual([[pricedAt8.id, { signal }]]);
  });

  it("does not save a price when the dollars are invalid", async () => {
    const createPrice = vi.fn();
    const updatePrice = vi.fn();
    const api = client({ createPrice, updatePrice });

    await expect(
      runCatalogCommand(api, {
        type: "create-price",
        materialId: custom.id,
        lengthIn: 120,
        dollars: "4.999",
      }),
    ).resolves.toEqual({
      kind: "invalid",
      notice: "Enter dollars, like 4.98.",
      problems: {},
    });
    await expect(
      runCatalogCommand(api, { type: "update-price", price: pricedAt8, dollars: "-1" }),
    ).resolves.toEqual({
      kind: "invalid",
      notice: "Enter dollars, like 4.98.",
      problems: {},
    });
    expect(createPrice.mock.calls).toEqual([]);
    expect(updatePrice.mock.calls).toEqual([]);
  });

  it("sends integer cents for a dollar amount", async () => {
    const signal = new AbortController().signal;
    const created = price({ id: priceId, materialId: custom.id, lengthIn: 120, priceCents: 410 });
    const createPrice = vi.fn(async () => ({ kind: "ok" as const, value: created }));

    await expect(
      runCatalogCommand(
        client({ createPrice }),
        { type: "create-price", materialId: custom.id, lengthIn: 120, dollars: "4.10" },
        signal,
      ),
    ).resolves.toEqual({ kind: "saved" });
    expect(createPrice.mock.calls).toEqual([
      [{ materialId: custom.id, lengthIn: 120, priceCents: 410 }, { signal }],
    ]);
  });
});

describe("buildPriceGrid", () => {
  const catalog: Catalog = {
    materials: [systemBoard, customBoard],
    prices: [pricedAt8, pricedAt100, orphanPrice],
  };

  it("adds an extra length column and an orphan row", () => {
    const grid = buildPriceGrid(catalog);

    expect(grid.columns).toEqual([
      { lengthIn: 96, label: "8 ft", addable: 96 },
      { lengthIn: 100, label: "8 ft 4 in", addable: null },
      { lengthIn: 120, label: "10 ft", addable: 120 },
      { lengthIn: 144, label: "12 ft", addable: 144 },
    ]);
    expect(grid.rows.map((row) => ({ id: row.materialId, label: row.label, accepting: row.accepting }))).toEqual([
      { id: systemId, label: "2x4", accepting: true },
      { id: customId, label: "Pine (custom)", accepting: true },
      { id: orphanId, label: "Unavailable material", accepting: false },
    ]);
  });

  it("keeps the standard columns when there are no prices", () => {
    const grid = buildPriceGrid({ materials: [systemBoard], prices: [] });
    expect(grid.columns.map((column) => column.lengthIn)).toEqual([96, 120, 144]);
    expect(grid.rows).toHaveLength(1);
    expect(cellAt(rowAt(grid, 0), columnAt(grid, 96))).toEqual({
      kind: "open",
      materialId: systemId,
      lengthIn: 96,
    });
  });
});

describe("cellAt", () => {
  const grid = buildPriceGrid({
    materials: [systemBoard, customBoard],
    prices: [pricedAt8, pricedAt100, orphanPrice],
  });
  const systemRow = rowAt(grid, 0);
  const customRow = rowAt(grid, 1);
  const orphanRow = rowAt(grid, 2);

  it("returns priced, open, and blank cells", () => {
    expect(cellAt(systemRow, columnAt(grid, 96))).toEqual({ kind: "priced", price: pricedAt8 });
    expect(cellAt(systemRow, columnAt(grid, 100))).toEqual({ kind: "priced", price: pricedAt100 });
    expect(cellAt(systemRow, columnAt(grid, 120))).toEqual({
      kind: "open",
      materialId: systemId,
      lengthIn: 120,
    });
    expect(cellAt(customRow, columnAt(grid, 100))).toEqual({ kind: "blank" });
    expect(cellAt(orphanRow, columnAt(grid, 120))).toEqual({ kind: "priced", price: orphanPrice });
    expect(cellAt(orphanRow, columnAt(grid, 96))).toEqual({ kind: "blank" });
    expect(cellAt(orphanRow, columnAt(grid, 144))).toEqual({ kind: "blank" });
  });
});
