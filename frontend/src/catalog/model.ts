import {
  isNotFound,
  type Material,
  type MaterialId,
  type MaterialWrite,
  type Price,
  type V1Client,
  type V1Failure,
  type V1Result,
} from "../api";
import {
  formatInches,
  formatLength,
  parseDollars,
  parseInches,
  standardLengthsIn,
  type StandardLengthIn,
} from "./units";

export type SystemMaterial = Material & { readonly system: true };
export type CustomMaterial = Material & { readonly system: false };
export type CatalogMaterial = SystemMaterial | CustomMaterial;

export type Catalog = {
  readonly materials: readonly CatalogMaterial[];
  readonly prices: readonly Price[];
};

export type CatalogModel =
  | ({ readonly kind: "ok"; readonly displayName: string } & Catalog)
  | V1Failure;

export const sizeKinds = [
  { key: "nominal", label: "Nominal", thickness: "nominalThicknessIn", width: "nominalWidthIn" },
  { key: "dressed", label: "Dressed", thickness: "actualThicknessIn", width: "actualWidthIn" },
  { key: "rough", label: "Rough", thickness: "roughThicknessIn", width: "roughWidthIn" },
] as const;

export type InchField = (typeof sizeKinds)[number]["thickness" | "width"];

type NumericWriteField = {
  [K in keyof MaterialWrite]-?: MaterialWrite[K] extends number ? K : never;
}[keyof MaterialWrite];

export type MaterialFieldKey = "name" | "species" | "grade" | InchField;
export type MaterialFields = Readonly<Record<MaterialFieldKey, string>>;
export type MaterialProblems = Readonly<Partial<Record<MaterialFieldKey, string>>>;

export type CatalogCommand =
  | { readonly type: "create-material"; readonly fields: MaterialFields }
  | { readonly type: "replace-material"; readonly target: CustomMaterial; readonly fields: MaterialFields }
  | { readonly type: "delete-material"; readonly target: CustomMaterial }
  | {
      readonly type: "create-price";
      readonly materialId: MaterialId;
      readonly lengthIn: StandardLengthIn;
      readonly dollars: string;
    }
  | { readonly type: "update-price"; readonly price: Price; readonly dollars: string }
  | { readonly type: "delete-price"; readonly price: Price };

export type CatalogOutcome =
  | { readonly kind: "saved" }
  | { readonly kind: "invalid"; readonly notice: string; readonly problems: MaterialProblems }
  | V1Failure;

export type LengthColumn = {
  readonly lengthIn: number;
  readonly label: string;
  readonly addable: StandardLengthIn | null;
};

export type PriceRow = {
  readonly materialId: MaterialId;
  readonly label: string;
  readonly accepting: boolean;
  readonly priced: ReadonlyMap<number, Price>;
};

export type PriceGrid = {
  readonly columns: readonly LengthColumn[];
  readonly rows: readonly PriceRow[];
};

export type PriceCell =
  | { readonly kind: "priced"; readonly price: Price }
  | { readonly kind: "open"; readonly materialId: MaterialId; readonly lengthIn: StandardLengthIn }
  | { readonly kind: "blank" };

const systemMaterialNotice = "System materials cannot be changed.";
const unavailableMaterialLabel = "Unavailable material";

const fieldOrder = [
  "name",
  "species",
  "grade",
  ...sizeKinds.flatMap((kind) => [kind.thickness, kind.width]),
] as const satisfies readonly MaterialFieldKey[];

export const blankMaterialFields: MaterialFields = {
  name: "",
  species: "",
  grade: "",
  ...inchRecord(() => ""),
};

export async function loadCatalog(api: V1Client, signal: AbortSignal): Promise<CatalogModel> {
  const [me, materials, prices] = await Promise.all([
    api.me({ signal }),
    api.listMaterials({ signal }),
    api.listPrices({ signal }),
  ]);
  if (me.kind !== "ok") {
    return me;
  }
  if (materials.kind !== "ok") {
    return materials;
  }
  if (prices.kind !== "ok") {
    return prices;
  }
  return {
    kind: "ok",
    displayName: me.value.displayName,
    materials: materials.value.map(toCatalogMaterial).sort(byCatalogOrder),
    prices: prices.value,
  };
}

export function materialLabel(material: CatalogMaterial): string {
  if (material.system) {
    return material.name;
  }
  return `${material.name} (custom)`;
}

export function fieldsFromMaterial(material: CustomMaterial): MaterialFields {
  return {
    name: material.name,
    species: material.species ?? "",
    grade: material.grade ?? "",
    ...inchRecord((key) => formatInches(material[key])),
  };
}

export function parseMaterialFields(
  fields: MaterialFields,
): { readonly kind: "ok"; readonly write: MaterialWrite } | { readonly kind: "invalid"; readonly problems: MaterialProblems } {
  const problems: Partial<Record<MaterialFieldKey, string>> = {};
  const name = fields.name.trim();
  if (name.length === 0) {
    problems.name = "Enter a name.";
  }
  const inches: Partial<Record<InchField, number>> = {};
  for (const kind of sizeKinds) {
    const keys = [kind.thickness, kind.width] as const;
    for (const key of keys) {
      const parsed = parseInches(fields[key]);
      if (parsed.kind === "invalid") {
        problems[key] = parsed.problem;
      } else {
        inches[key] = parsed.value;
      }
    }
  }
  if (name.length === 0 || !isCompleteInchRecord(inches)) {
    return { kind: "invalid", problems };
  }
  return {
    kind: "ok",
    write: {
      name,
      species: blankToNull(fields.species),
      grade: blankToNull(fields.grade),
      ...inches,
    },
  };
}

export async function runCatalogCommand(
  api: V1Client,
  command: CatalogCommand,
  signal?: AbortSignal,
): Promise<CatalogOutcome> {
  switch (command.type) {
    case "create-material":
      return createMaterial(api, command.fields, signal);
    case "replace-material":
      return replaceMaterial(api, command.target, command.fields, signal);
    case "delete-material":
      return deleteMaterial(api, command.target, signal);
    case "create-price":
      return createPrice(api, command.materialId, command.lengthIn, command.dollars, signal);
    case "update-price":
      return updatePrice(api, command.price, command.dollars, signal);
    case "delete-price":
      return deletePrice(api, command.price, signal);
    default: {
      const _exhaustive: never = command;
      return _exhaustive;
    }
  }
}

export function buildPriceGrid(catalog: Catalog): PriceGrid {
  const byMaterial = new Map<MaterialId, Map<number, Price>>();
  for (const price of catalog.prices) {
    const priced = byMaterial.get(price.materialId) ?? new Map<number, Price>();
    if (!priced.has(price.lengthIn)) {
      priced.set(price.lengthIn, price);
    }
    byMaterial.set(price.materialId, priced);
  }
  const lengths = new Set<number>(standardLengthsIn);
  for (const price of catalog.prices) {
    lengths.add(price.lengthIn);
  }
  const columns = [...lengths].sort((left, right) => left - right).map(lengthColumn);
  const seen = new Set<MaterialId>();
  const rows: PriceRow[] = [];
  for (const material of catalog.materials) {
    seen.add(material.id);
    rows.push({
      materialId: material.id,
      label: materialLabel(material),
      accepting: true,
      priced: byMaterial.get(material.id) ?? new Map<number, Price>(),
    });
  }
  for (const [materialId, priced] of byMaterial) {
    if (seen.has(materialId)) {
      continue;
    }
    rows.push({
      materialId,
      label: unavailableMaterialLabel,
      accepting: false,
      priced,
    });
  }
  return { columns, rows };
}

export function cellAt(row: PriceRow, column: LengthColumn): PriceCell {
  const price = row.priced.get(column.lengthIn);
  if (price !== undefined) {
    return { kind: "priced", price };
  }
  if (column.addable !== null && row.accepting) {
    return { kind: "open", materialId: row.materialId, lengthIn: column.addable };
  }
  return { kind: "blank" };
}

function toCatalogMaterial(material: Material): CatalogMaterial {
  if (material.system) {
    return { ...material, system: true };
  }
  return { ...material, system: false };
}

function byCatalogOrder(left: CatalogMaterial, right: CatalogMaterial): number {
  if (left.system !== right.system) {
    return left.system ? -1 : 1;
  }
  const byName = left.name.localeCompare(right.name, "en");
  if (byName !== 0) {
    return byName;
  }
  if (left.id < right.id) {
    return -1;
  }
  if (left.id > right.id) {
    return 1;
  }
  return 0;
}

function sizeRegistryCoversInchFields(): true {
  const covered: Exclude<NumericWriteField, InchField> extends never ? true : never = true;
  return covered;
}

function inchRecord<T>(read: (key: InchField) => T): Record<InchField, T> {
  sizeRegistryCoversInchFields();
  const record: Partial<Record<InchField, T>> = {};
  for (const kind of sizeKinds) {
    record[kind.thickness] = read(kind.thickness);
    record[kind.width] = read(kind.width);
  }
  if (!isCompleteInchRecord(record)) {
    throw new Error("sizeKinds did not fill every inch field");
  }
  return record;
}

function isCompleteInchRecord<T>(
  record: Partial<Record<InchField, T>>,
): record is Record<InchField, T> {
  return sizeKinds.every(
    (kind) => record[kind.thickness] !== undefined && record[kind.width] !== undefined,
  );
}

function blankToNull(raw: string): string | null {
  const text = raw.trim();
  if (text.length === 0) {
    return null;
  }
  return text;
}

async function createMaterial(
  api: V1Client,
  fields: MaterialFields,
  signal: AbortSignal | undefined,
): Promise<CatalogOutcome> {
  const parsed = parseMaterialFields(fields);
  if (parsed.kind === "invalid") {
    return fieldInvalid(parsed.problems);
  }
  return savedOr(await api.createMaterial(parsed.write, callOptions(signal)));
}

async function replaceMaterial(
  api: V1Client,
  target: CustomMaterial,
  fields: MaterialFields,
  signal: AbortSignal | undefined,
): Promise<CatalogOutcome> {
  const parsed = parseMaterialFields(fields);
  if (parsed.kind === "invalid") {
    return fieldInvalid(parsed.problems);
  }
  return materialChange(await api.replaceMaterial(target.id, parsed.write, callOptions(signal)));
}

async function deleteMaterial(
  api: V1Client,
  target: CustomMaterial,
  signal: AbortSignal | undefined,
): Promise<CatalogOutcome> {
  return deleted(await api.deleteMaterial(target.id, callOptions(signal)), true);
}

async function createPrice(
  api: V1Client,
  materialId: MaterialId,
  lengthIn: StandardLengthIn,
  dollars: string,
  signal: AbortSignal | undefined,
): Promise<CatalogOutcome> {
  const parsed = parseDollars(dollars);
  if (parsed.kind === "invalid") {
    return { kind: "invalid", notice: parsed.problem, problems: {} };
  }
  return savedOr(
    await api.createPrice(
      { materialId, lengthIn, priceCents: parsed.value },
      callOptions(signal),
    ),
  );
}

async function updatePrice(
  api: V1Client,
  price: Price,
  dollars: string,
  signal: AbortSignal | undefined,
): Promise<CatalogOutcome> {
  const parsed = parseDollars(dollars);
  if (parsed.kind === "invalid") {
    return { kind: "invalid", notice: parsed.problem, problems: {} };
  }
  return savedOr(await api.updatePrice(price.id, { priceCents: parsed.value }, callOptions(signal)));
}

async function deletePrice(
  api: V1Client,
  price: Price,
  signal: AbortSignal | undefined,
): Promise<CatalogOutcome> {
  return deleted(await api.deletePrice(price.id, callOptions(signal)), false);
}

function fieldInvalid(problems: MaterialProblems): CatalogOutcome {
  return { kind: "invalid", notice: materialNotice(problems), problems };
}

function materialNotice(problems: MaterialProblems): string {
  for (const key of fieldOrder) {
    const problem = problems[key];
    if (problem !== undefined) {
      return problem;
    }
  }
  return "Check the material fields.";
}

function savedOr(result: V1Result<unknown>): CatalogOutcome {
  if (result.kind === "ok") {
    return { kind: "saved" };
  }
  return result;
}

function materialChange(result: V1Result<Material>): CatalogOutcome {
  if (result.kind === "ok") {
    return { kind: "saved" };
  }
  return forbidSystem(result);
}

function deleted(result: V1Result<null>, rewriteForbidden: boolean): CatalogOutcome {
  if (result.kind === "ok" || isNotFound(result)) {
    return { kind: "saved" };
  }
  if (rewriteForbidden) {
    return forbidSystem(result);
  }
  return result;
}

function forbidSystem(result: V1Failure): V1Failure {
  if (result.kind === "failed" && result.status === 403 && result.message === "forbidden") {
    return { kind: "failed", status: 403, message: systemMaterialNotice };
  }
  return result;
}

function callOptions(signal: AbortSignal | undefined): { readonly signal?: AbortSignal } {
  return { signal };
}

function lengthColumn(lengthIn: number): LengthColumn {
  return {
    lengthIn,
    label: formatLength(lengthIn),
    addable: standardLength(lengthIn),
  };
}

function standardLength(lengthIn: number): StandardLengthIn | null {
  for (const length of standardLengthsIn) {
    if (length === lengthIn) {
      return length;
    }
  }
  return null;
}
