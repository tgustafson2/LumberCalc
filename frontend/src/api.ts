import createClient from "openapi-fetch";
import type { components, paths } from "./api.gen";

declare const brand: unique symbol;
type Brand<T, B extends string> = T & { readonly [brand]: B };

export type UserId = Brand<string, "UserId">;

export type ClerkUserId = Brand<string, "ClerkUserId">;

export type DesignId = Brand<string, "DesignId">;

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

export type Design = DesignSummary & {
  readonly document: DesignDocument;
};

export type V1Result<T> =
  | { readonly kind: "ok"; readonly value: T }
  | { readonly kind: "no-token" }
  | { readonly kind: "unauthorized" }
  | { readonly kind: "unreachable" }
  | { readonly kind: "invalid" }
  | { readonly kind: "failed"; readonly status: number; readonly message: string };

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
    body: { name: string; description?: string; document: DesignDocument },
    options?: RequestOptions,
  ) => Promise<V1Result<Design>>;
  readonly deleteDesign: (id: DesignId, options?: RequestOptions) => Promise<V1Result<null>>;
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

function toDesign(wire: Wire["Design"]): Parsed<Design> {
  const summary = toSummary(wire);
  if (summary.kind !== "ok" || wire.document === undefined) {
    return { kind: "invalid" };
  }
  return { kind: "ok", value: { ...summary.value, document: wire.document } };
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
