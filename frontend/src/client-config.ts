export type ClientConfig =
  | { readonly kind: "ready"; readonly publishableKey: string }
  | { readonly kind: "invalid"; readonly reason: string };

export function readClientConfig(env: {
  readonly VITE_CLERK_PUBLISHABLE_KEY?: string;
}): ClientConfig {
  const key = env.VITE_CLERK_PUBLISHABLE_KEY?.trim();
  if (key === undefined || key.length === 0) {
    return {
      kind: "invalid",
      reason: "Set VITE_CLERK_PUBLISHABLE_KEY in frontend/.env",
    };
  }
  if (key.startsWith("sk_")) {
    return {
      kind: "invalid",
      reason: "A secret key is not allowed in the frontend",
    };
  }
  if (!isClerkPublishableKey(key)) {
    return {
      kind: "invalid",
      reason: "VITE_CLERK_PUBLISHABLE_KEY must be a Clerk publishable key",
    };
  }
  return { kind: "ready", publishableKey: key };
}

// ClerkProvider throws for a key that fails this check.
function isClerkPublishableKey(key: string): boolean {
  if (!key.startsWith("pk_test_") && !key.startsWith("pk_live_")) {
    return false;
  }
  const parts = key.split("_");
  if (parts.length !== 3) {
    return false;
  }
  const encoded = parts[2];
  if (encoded === undefined || encoded.length === 0) {
    return false;
  }
  let decoded: string;
  try {
    decoded = atob(encoded);
  } catch {
    return false;
  }
  if (!decoded.endsWith("$")) {
    return false;
  }
  const host = decoded.slice(0, -1);
  if (host.includes("$") || !host.includes(".")) {
    return false;
  }
  return true;
}
