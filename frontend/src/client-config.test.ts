import { describe, expect, it } from "vitest";
import { readClientConfig } from "./client-config";

function env(key?: string): { readonly VITE_CLERK_PUBLISHABLE_KEY?: string } {
  return { VITE_CLERK_PUBLISHABLE_KEY: key };
}

function clerkKey(prefix: "pk_test_" | "pk_live_", host: string): string {
  return `${prefix}${btoa(`${host}$`)}`;
}

describe("readClientConfig", () => {
  it("rejects a missing key", () => {
    expect(readClientConfig(env())).toEqual({
      kind: "invalid",
      reason: "Set VITE_CLERK_PUBLISHABLE_KEY in frontend/.env",
    });
    expect(readClientConfig(env("  "))).toEqual({
      kind: "invalid",
      reason: "Set VITE_CLERK_PUBLISHABLE_KEY in frontend/.env",
    });
  });

  it("rejects a secret key", () => {
    expect(readClientConfig(env("sk_test_abc"))).toEqual({
      kind: "invalid",
      reason: "A secret key is not allowed in the frontend",
    });
  });

  it("rejects the example placeholder before Clerk can throw", () => {
    expect(readClientConfig(env("pk_test_replace_me"))).toEqual({
      kind: "invalid",
      reason: "VITE_CLERK_PUBLISHABLE_KEY must be a Clerk publishable key",
    });
  });

  it("accepts a Clerk publishable key", () => {
    const key = clerkKey("pk_test_", "clerk.example.com");
    expect(readClientConfig(env(`  ${key}  `))).toEqual({
      kind: "ready",
      publishableKey: key,
    });
    expect(readClientConfig(env(clerkKey("pk_live_", "clerk.example.com")))).toEqual({
      kind: "ready",
      publishableKey: clerkKey("pk_live_", "clerk.example.com"),
    });
  });
});
