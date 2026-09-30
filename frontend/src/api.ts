declare const brand: unique symbol;
type Brand<T, B extends string> = T & { readonly [brand]: B };

export type UserId = Brand<string, "UserId">;

export type ClerkUserId = Brand<string, "ClerkUserId">;

export type Me = {
  readonly userId: UserId;
  readonly clerkUserId: ClerkUserId;
  readonly displayName: string;
};

export type V1Result<T> =
  | { readonly kind: "ok"; readonly value: T }
  | { readonly kind: "no-token" }
  | { readonly kind: "unauthorized" }
  | { readonly kind: "unreachable" }
  | { readonly kind: "invalid" }
  | { readonly kind: "failed"; readonly status: number; readonly message: string };

export type MeResult = V1Result<Me>;

type ParseResult<T> =
  | { readonly kind: "parsed"; readonly value: T }
  | { readonly kind: "invalid" };

type RequestOptions = { readonly signal?: AbortSignal };

export type V1Client = {
  readonly me: (options?: RequestOptions) => Promise<MeResult>;
};

type TokenSource = () => Promise<string | null>;

export function createV1Client(options: {
  readonly getToken: TokenSource;
}): V1Client {
  return {
    me: ({ signal } = {}) =>
      getV1({
        path: "/v1/me",
        signal,
        getToken: options.getToken,
        parse: parseMe,
      }),
  };
}

async function getV1<T>(request: {
  readonly path: `/v1/${string}`;
  readonly signal: AbortSignal | undefined;
  readonly getToken: TokenSource;
  readonly parse: (body: unknown) => ParseResult<T>;
}): Promise<V1Result<T>> {
  const token = await request.getToken();
  if (token === null) {
    return { kind: "no-token" };
  }

  let response: Response;
  try {
    response = await fetch(request.path, {
      signal: request.signal,
      headers: { Authorization: `Bearer ${token}` },
    });
  } catch (error: unknown) {
    if (isAbortError(error)) {
      throw error;
    }
    return { kind: "unreachable" };
  }

  if (response.status === 401) {
    return { kind: "unauthorized" };
  }

  const body = await readBody(response);
  if (response.status !== 200) {
    return {
      kind: "failed",
      status: response.status,
      message: errorMessage(body),
    };
  }

  const parsed = request.parse(body);
  if (parsed.kind === "invalid") {
    return { kind: "invalid" };
  }
  return { kind: "ok", value: parsed.value };
}

function isAbortError(error: unknown): boolean {
  return error instanceof Error && error.name === "AbortError";
}

async function readBody(response: Response): Promise<unknown> {
  const text = await response.text();
  try {
    const parsed: unknown = JSON.parse(text);
    return parsed;
  } catch {
    return null;
  }
}

// A Clerk subject in this app starts with "user_".
// store.ParseClerkUserID also accepts any other non-empty subject up to 64 bytes.
function parseMe(body: unknown): ParseResult<Me> {
  if (!isRecord(body)) {
    return { kind: "invalid" };
  }
  const userId = body.user_id;
  const clerkUserId = body.clerk_user_id;
  const displayName = body.display_name;
  if (typeof userId !== "string" || userId.length === 0) {
    return { kind: "invalid" };
  }
  if (typeof clerkUserId !== "string" || !clerkUserId.startsWith("user_")) {
    return { kind: "invalid" };
  }
  if (typeof displayName !== "string") {
    return { kind: "invalid" };
  }
  return {
    kind: "parsed",
    value: {
      userId: userId as UserId,
      clerkUserId: clerkUserId as ClerkUserId,
      displayName,
    },
  };
}

function errorMessage(body: unknown): string {
  if (!isRecord(body)) {
    return "The API request failed.";
  }
  const message = body.error;
  if (typeof message !== "string") {
    return "The API request failed.";
  }
  return message;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}
