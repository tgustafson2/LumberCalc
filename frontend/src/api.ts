import createClient from "openapi-fetch";
import type { components, paths } from "./api.gen";

declare const brand: unique symbol;
type Brand<T, B extends string> = T & { readonly [brand]: B };

export type UserId = Brand<string, "UserId">;

export type ClerkUserId = Brand<string, "ClerkUserId">;

export type Me = {
  readonly userId: UserId;
  readonly clerkUserId: ClerkUserId;
  readonly displayName: string;
};

export type DesignDocument = components["schemas"]["DesignDocument"];

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
  if (result.data !== undefined) {
    return toValue(result.data);
  }
  return {
    kind: "failed",
    status: result.response.status,
    message: errorMessage(result.error),
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
