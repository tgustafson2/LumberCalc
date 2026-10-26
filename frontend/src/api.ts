import createClient from "openapi-fetch";
import type { components, paths } from "./api.gen";

declare const brand: unique symbol;
export type Brand<T, B extends string> = T & { readonly [brand]: B };

export type UserId = Brand<string, "UserId">;

export type ClerkUserId = Brand<string, "ClerkUserId">;

export type DesignId = Brand<string, "DesignId">;

export type MaterialId = Brand<string, "MaterialId">;

export type PriceId = Brand<string, "PriceId">;

export type PatternId = Brand<string, "PatternId">;

export type Me = {
  readonly userId: UserId;
  readonly clerkUserId: ClerkUserId;
  readonly displayName: string;
};

export type DesignDocument = components["schemas"]["DesignDocument"];

export type DesignSummary = {
  readonly id: DesignId;
  readonly name: string;
  readonly description: string;
  readonly version: number;
  readonly updatedAt: string;
};

export type Publication =
  | { readonly kind: "unpublished" }
  | { readonly kind: "public"; readonly patternId: PatternId }
  | { readonly kind: "private"; readonly patternId: PatternId };

export type Design = DesignSummary & {
  readonly document: DesignDocument;
  readonly publication: Publication;
};

export type PatternSummary = {
  readonly id: PatternId;
  readonly name: string;
  readonly description: string;
  readonly publisherName: string | null;
  readonly publishedAt: string;
};

export type Pattern = PatternSummary & {
  readonly isPublic: boolean;
  readonly document: components["schemas"]["PatternDocument"];
};

export type MaterialWrite = {
  readonly name: string;
  readonly species?: string | null;
  readonly grade?: string | null;
  readonly nominalThicknessIn: number;
  readonly nominalWidthIn: number;
  readonly actualThicknessIn: number;
  readonly actualWidthIn: number;
  readonly roughThicknessIn: number;
  readonly roughWidthIn: number;
};

export type Material = {
  readonly id: MaterialId;
  readonly name: string;
  readonly species: string | null;
  readonly grade: string | null;
  readonly system: boolean;
  readonly nominalThicknessIn: number;
  readonly nominalWidthIn: number;
  readonly actualThicknessIn: number;
  readonly actualWidthIn: number;
  readonly roughThicknessIn: number;
  readonly roughWidthIn: number;
  readonly updatedAt: string;
};

export type PriceCreate = {
  readonly materialId: MaterialId;
  readonly lengthIn: number;
  readonly priceCents: number;
};

export type Price = {
  readonly id: PriceId;
  readonly materialId: MaterialId;
  readonly lengthIn: number;
  readonly priceCents: number;
  readonly updatedAt: string;
};

export type V1Result<T> =
  | { readonly kind: "ok"; readonly value: T }
  | { readonly kind: "no-token" }
  | { readonly kind: "unauthorized" }
  | { readonly kind: "unreachable" }
  | { readonly kind: "invalid" }
  | { readonly kind: "failed"; readonly status: number; readonly message: string };

export type V1Failure = Exclude<V1Result<unknown>, { readonly kind: "ok" }>;

export type MeResult = V1Result<Me>;

type RequestOptions = { readonly signal?: AbortSignal };

export type V1Client = {
  readonly me: (options?: RequestOptions) => Promise<MeResult>;
  readonly createDesign: (
    body: { name: string; description?: string; document?: DesignDocument },
    options?: RequestOptions,
  ) => Promise<V1Result<Design>>;
  readonly listDesigns: (options?: RequestOptions) => Promise<V1Result<readonly DesignSummary[]>>;
  readonly getDesign: (id: DesignId, options?: RequestOptions) => Promise<V1Result<Design>>;
  readonly replaceDesign: (
    id: DesignId,
    body: { name: string; description?: string; document: DesignDocument; version: number },
    options?: RequestOptions,
  ) => Promise<V1Result<Design>>;
  readonly deleteDesign: (id: DesignId, options?: RequestOptions) => Promise<V1Result<null>>;
  readonly copyDesign: (id: DesignId, options?: RequestOptions) => Promise<V1Result<Design>>;
  readonly publishDesign: (id: DesignId, options?: RequestOptions) => Promise<V1Result<PatternId>>;
  readonly unpublishDesign: (id: DesignId, options?: RequestOptions) => Promise<V1Result<PatternId>>;
  readonly listPatterns: (options?: RequestOptions) => Promise<V1Result<readonly PatternSummary[]>>;
  readonly getPattern: (id: PatternId, options?: RequestOptions) => Promise<V1Result<Pattern>>;
  readonly addPattern: (id: PatternId, options?: RequestOptions) => Promise<V1Result<Design>>;
  readonly createMaterial: (body: MaterialWrite, options?: RequestOptions) => Promise<V1Result<Material>>;
  readonly listMaterials: (options?: RequestOptions) => Promise<V1Result<readonly Material[]>>;
  readonly replaceMaterial: (
    id: MaterialId,
    body: MaterialWrite,
    options?: RequestOptions,
  ) => Promise<V1Result<Material>>;
  readonly deleteMaterial: (id: MaterialId, options?: RequestOptions) => Promise<V1Result<null>>;
  readonly createPrice: (body: PriceCreate, options?: RequestOptions) => Promise<V1Result<Price>>;
  readonly listPrices: (options?: RequestOptions) => Promise<V1Result<readonly Price[]>>;
  readonly updatePrice: (
    id: PriceId,
    body: { readonly priceCents: number },
    options?: RequestOptions,
  ) => Promise<V1Result<Price>>;
  readonly deletePrice: (id: PriceId, options?: RequestOptions) => Promise<V1Result<null>>;
};

type TokenSource = () => Promise<string | null>;

type Wire = components["schemas"];

type Parsed<T> = { readonly kind: "ok"; readonly value: T } | { readonly kind: "invalid" };

type ClientResult<W> = {
  readonly data?: W;
  readonly error?: Wire["Error"];
  readonly response: Response;
};

export function createV1Client(options: {
  readonly getToken: TokenSource;
  readonly baseUrl?: string;
}): V1Client {
  const http = createClient<paths>({ baseUrl: options.baseUrl });
  return {
    me: ({ signal } = {}) =>
      send(options.getToken, (headers) => http.GET("/v1/me", { signal, headers }), toMe),
    createDesign: (body, { signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.POST("/v1/designs", { body, signal, headers }),
        toDesign,
      ),
    listDesigns: ({ signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.GET("/v1/designs", { signal, headers }),
        toDesignList,
      ),
    getDesign: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.GET("/v1/designs/{id}", { params: { path: { id } }, signal, headers }),
        toDesign,
      ),
    replaceDesign: (id, body, { signal } = {}) =>
      send(
        options.getToken,
        (headers) =>
          http.PUT("/v1/designs/{id}", { params: { path: { id } }, body, signal, headers }),
        toDesign,
      ),
    deleteDesign: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.DELETE("/v1/designs/{id}", { params: { path: { id } }, signal, headers }),
        () => ({ kind: "invalid" }),
      ),
    copyDesign: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) =>
          http.POST("/v1/designs/{id}/copy", { params: { path: { id } }, signal, headers }),
        toDesign,
      ),
    publishDesign: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) =>
          http.POST("/v1/designs/{id}/publish", { params: { path: { id } }, signal, headers }),
        toPublishedPattern,
      ),
    unpublishDesign: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) =>
          http.POST("/v1/designs/{id}/unpublish", { params: { path: { id } }, signal, headers }),
        toPublishedPattern,
      ),
    listPatterns: ({ signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.GET("/v1/patterns", { signal, headers }),
        toPatternList,
      ),
    getPattern: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.GET("/v1/patterns/{id}", { params: { path: { id } }, signal, headers }),
        toPattern,
      ),
    addPattern: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) =>
          http.POST("/v1/patterns/{id}/add", { params: { path: { id } }, signal, headers }),
        toDesign,
      ),
    createMaterial: (body, { signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.POST("/v1/materials", { body, signal, headers }),
        toMaterial,
      ),
    listMaterials: ({ signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.GET("/v1/materials", { signal, headers }),
        toMaterialList,
      ),
    replaceMaterial: (id, body, { signal } = {}) =>
      send(
        options.getToken,
        (headers) =>
          http.PUT("/v1/materials/{id}", { params: { path: { id } }, body, signal, headers }),
        toMaterial,
      ),
    deleteMaterial: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) =>
          http.DELETE("/v1/materials/{id}", { params: { path: { id } }, signal, headers }),
        () => ({ kind: "invalid" }),
      ),
    createPrice: (body, { signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.POST("/v1/prices", { body, signal, headers }),
        toPrice,
      ),
    listPrices: ({ signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.GET("/v1/prices", { signal, headers }),
        toPriceList,
      ),
    updatePrice: (id, body, { signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.PUT("/v1/prices/{id}", { params: { path: { id } }, body, signal, headers }),
        toPrice,
      ),
    deletePrice: (id, { signal } = {}) =>
      send(
        options.getToken,
        (headers) => http.DELETE("/v1/prices/{id}", { params: { path: { id } }, signal, headers }),
        () => ({ kind: "invalid" }),
      ),
  };
}

async function send<W, T>(
  getToken: TokenSource,
  request: (headers: { Authorization: string }) => Promise<ClientResult<W>>,
  toValue: (wire: W) => Parsed<T>,
): Promise<V1Result<T>> {
  const token = await getToken();
  if (token === null) {
    return { kind: "no-token" };
  }

  let result: ClientResult<W>;
  try {
    result = await request({ Authorization: `Bearer ${token}` });
  } catch (error: unknown) {
    if (isAbortError(error)) {
      throw error;
    }
    if (error instanceof SyntaxError) {
      return { kind: "invalid" };
    }
    return { kind: "unreachable" };
  }

  if (result.response.status === 401) {
    return { kind: "unauthorized" };
  }
  if (result.response.status === 204) {
    return { kind: "ok", value: null as T };
  }
  if (result.data !== undefined) {
    return toValue(result.data);
  }
  return {
    kind: "failed",
    status: result.response.status,
    message: errorMessage(result.error),
  };
}

export function isNotFound(result: V1Failure): boolean {
  return result.kind === "failed" && result.status === 404;
}

function toDesign(wire: Wire["Design"]): Parsed<Design> {
  const summary = toSummary(wire);
  const publication = toPublication(wire.publication);
  if (summary.kind !== "ok" || wire.document === undefined || publication.kind !== "ok") {
    return { kind: "invalid" };
  }
  return {
    kind: "ok",
    value: { ...summary.value, document: wire.document, publication: publication.value },
  };
}

function toPublication(wire: Wire["Design"]["publication"] | undefined): Parsed<Publication> {
  if (wire === null) {
    return { kind: "ok", value: { kind: "unpublished" } };
  }
  const patternId = text(wire?.patternId);
  if (patternId === null || typeof wire?.isPublic !== "boolean") {
    return { kind: "invalid" };
  }
  return {
    kind: "ok",
    value: { kind: wire.isPublic ? "public" : "private", patternId: patternId as PatternId },
  };
}

function toPublishedPattern(wire: Wire["PublishedPattern"]): Parsed<PatternId> {
  const id = text(wire.id);
  if (id === null) {
    return { kind: "invalid" };
  }
  return { kind: "ok", value: id as PatternId };
}

function toDesignList(wire: readonly Wire["DesignSummary"][]): Parsed<readonly DesignSummary[]> {
  const values: DesignSummary[] = [];
  for (const item of wire) {
    const summary = toSummary(item);
    if (summary.kind !== "ok") {
      return summary;
    }
    values.push(summary.value);
  }
  return { kind: "ok", value: values };
}

function toSummary(wire: Wire["DesignSummary"]): Parsed<DesignSummary> {
  const id = text(wire.id);
  const name = text(wire.name);
  const updatedAt = text(wire.updatedAt);
  if (
    id === null ||
    name === null ||
    updatedAt === null ||
    typeof wire.description !== "string" ||
    typeof wire.version !== "number"
  ) {
    return { kind: "invalid" };
  }
  return {
    kind: "ok",
    value: {
      id: id as DesignId,
      name,
      description: wire.description,
      version: wire.version,
      updatedAt,
    },
  };
}

function toPattern(wire: Wire["Pattern"]): Parsed<Pattern> {
  const summary = toPatternSummary(wire);
  if (summary.kind !== "ok" || typeof wire.isPublic !== "boolean" || wire.document === undefined) {
    return { kind: "invalid" };
  }
  return {
    kind: "ok",
    value: { ...summary.value, isPublic: wire.isPublic, document: wire.document },
  };
}

function toPatternList(wire: readonly Wire["PatternSummary"][]): Parsed<readonly PatternSummary[]> {
  const values: PatternSummary[] = [];
  for (const item of wire) {
    const summary = toPatternSummary(item);
    if (summary.kind !== "ok") {
      return summary;
    }
    values.push(summary.value);
  }
  return { kind: "ok", value: values };
}

function toPatternSummary(wire: Wire["PatternSummary"]): Parsed<PatternSummary> {
  const id = text(wire.id);
  const name = text(wire.name);
  const publishedAt = text(wire.publishedAt);
  const publisherName = wire.publisherName === null ? null : text(wire.publisherName);
  if (
    id === null ||
    name === null ||
    publishedAt === null ||
    (publisherName === null && wire.publisherName !== null) ||
    typeof wire.description !== "string"
  ) {
    return { kind: "invalid" };
  }
  return {
    kind: "ok",
    value: {
      id: id as PatternId,
      name,
      description: wire.description,
      publisherName,
      publishedAt,
    },
  };
}

function toMaterial(wire: Wire["Material"]): Parsed<Material> {
  const id = text(wire.id);
  const name = text(wire.name);
  const updatedAt = text(wire.updatedAt);
  const species = nullableText(wire.species);
  const grade = nullableText(wire.grade);
  if (
    id === null ||
    name === null ||
    updatedAt === null ||
    species === undefined ||
    grade === undefined ||
    typeof wire.system !== "boolean" ||
    !positiveNumber(wire.nominalThicknessIn) ||
    !positiveNumber(wire.nominalWidthIn) ||
    !positiveNumber(wire.actualThicknessIn) ||
    !positiveNumber(wire.actualWidthIn) ||
    !positiveNumber(wire.roughThicknessIn) ||
    !positiveNumber(wire.roughWidthIn)
  ) {
    return { kind: "invalid" };
  }
  return {
    kind: "ok",
    value: {
      id: id as MaterialId,
      name,
      species,
      grade,
      system: wire.system,
      nominalThicknessIn: wire.nominalThicknessIn,
      nominalWidthIn: wire.nominalWidthIn,
      actualThicknessIn: wire.actualThicknessIn,
      actualWidthIn: wire.actualWidthIn,
      roughThicknessIn: wire.roughThicknessIn,
      roughWidthIn: wire.roughWidthIn,
      updatedAt,
    },
  };
}

function toPrice(wire: Wire["Price"]): Parsed<Price> {
  const id = text(wire.id);
  const materialId = text(wire.materialId);
  const updatedAt = text(wire.updatedAt);
  if (
    id === null ||
    materialId === null ||
    updatedAt === null ||
    !positiveNumber(wire.lengthIn) ||
    !nonNegativeInteger(wire.priceCents)
  ) {
    return { kind: "invalid" };
  }
  return {
    kind: "ok",
    value: {
      id: id as PriceId,
      materialId: materialId as MaterialId,
      lengthIn: wire.lengthIn,
      priceCents: wire.priceCents,
      updatedAt,
    },
  };
}

function toPriceList(wire: readonly Wire["Price"][]): Parsed<readonly Price[]> {
  const values: Price[] = [];
  for (const item of wire) {
    const price = toPrice(item);
    if (price.kind !== "ok") {
      return price;
    }
    values.push(price.value);
  }
  return { kind: "ok", value: values };
}

function nonNegativeInteger(value: unknown): value is number {
  return typeof value === "number" && Number.isInteger(value) && value >= 0;
}

function toMaterialList(wire: readonly Wire["Material"][]): Parsed<readonly Material[]> {
  const values: Material[] = [];
  for (const item of wire) {
    const material = toMaterial(item);
    if (material.kind !== "ok") {
      return material;
    }
    values.push(material.value);
  }
  return { kind: "ok", value: values };
}

function nullableText(value: unknown): string | null | undefined {
  if (value === null) {
    return null;
  }
  if (typeof value !== "string") {
    return undefined;
  }
  return value;
}

function positiveNumber(value: unknown): value is number {
  return typeof value === "number" && Number.isFinite(value) && value > 0;
}

function toMe(wire: Wire["Me"]): Parsed<Me> {
  const userId = text(wire.user_id);
  const clerkUserId = text(wire.clerk_user_id);
  const displayName = text(wire.display_name);
  if (userId === null || clerkUserId === null || displayName === null) {
    return { kind: "invalid" };
  }
  return {
    kind: "ok",
    value: {
      userId: userId as UserId,
      clerkUserId: clerkUserId as ClerkUserId,
      displayName,
    },
  };
}

function text(value: unknown): string | null {
  if (typeof value !== "string" || value.trim().length === 0) {
    return null;
  }
  return value;
}

function errorMessage(error: Wire["Error"] | undefined): string {
  if (error === undefined || typeof error.error !== "string" || error.error.trim().length === 0) {
    return "The API request failed.";
  }
  return error.error;
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}
