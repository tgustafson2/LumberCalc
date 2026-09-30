import type { V1Client } from "./api";

export type BranchId = "signed-out" | "signed-in";

export type SessionGate =
  | { readonly kind: "loading" }
  | { readonly kind: "signed-out" }
  | { readonly kind: "signed-in"; readonly api: V1Client };

export function redirectFor(
  gate: { readonly kind: BranchId },
  branch: BranchId,
): "/designs" | "/" | null {
  if (gate.kind === "signed-in" && branch === "signed-out") {
    return "/designs";
  }
  if (gate.kind === "signed-out" && branch === "signed-in") {
    return "/";
  }
  return null;
}
