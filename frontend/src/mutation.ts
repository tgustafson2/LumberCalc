import { useRef, useState } from "react";
import { useRouter } from "@tanstack/react-router";
import type { V1Failure } from "./api";

export type ActionGate = { current: "open" | "busy" | "held" };

export type MutationOutcome =
  | { readonly kind: "saved" }
  | { readonly kind: "invalid"; readonly notice: string }
  | V1Failure;

export type MutationRunner = {
  readonly busy: boolean;
  readonly notice: string | null;
  readonly sessionFailure: V1Failure | null;
  readonly run: (work: () => Promise<MutationOutcome>) => Promise<"saved" | "rejected" | "skipped">;
  readonly retry: () => void;
};

export function beginAction(
  gate: ActionGate,
  work: () => Promise<"hold" | "done">,
): Promise<void> | null {
  if (gate.current !== "open") {
    return null;
  }
  gate.current = "busy";
  let pending: Promise<"hold" | "done">;
  try {
    pending = work();
  } catch (error) {
    gate.current = "open";
    throw error;
  }
  return pending.then(
    (result) => {
      gate.current = result === "hold" ? "held" : "open";
    },
    (error: unknown) => {
      gate.current = "open";
      throw error;
    },
  );
}

export function mutationView(
  result: V1Failure,
):
  | { readonly kind: "session"; readonly failure: V1Failure }
  | { readonly kind: "notice"; readonly message: string } {
  switch (result.kind) {
    case "no-token":
    case "unauthorized":
      return { kind: "session", failure: result };
    case "unreachable":
      return { kind: "notice", message: "The API is not reachable." };
    case "invalid":
      return { kind: "notice", message: "The API sent an unexpected response." };
    case "failed":
      return { kind: "notice", message: result.message };
    default: {
      const _exhaustive: never = result;
      return _exhaustive;
    }
  }
}

export async function settle<T>(work: Promise<T>): Promise<T | { readonly kind: "aborted" }> {
  try {
    return await work;
  } catch (error: unknown) {
    if (error instanceof Error && error.name === "AbortError") {
      return { kind: "aborted" };
    }
    throw error;
  }
}

export function useMutationRunner(): MutationRunner {
  const router = useRouter();
  const gate = useRef<"open" | "busy" | "held">("open");
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [sessionFailure, setSessionFailure] = useState<V1Failure | null>(null);

  async function run(work: () => Promise<MutationOutcome>): Promise<"saved" | "rejected" | "skipped"> {
    let status: "saved" | "rejected" | "skipped" = "skipped";
    const job = beginAction(gate, async () => {
      const settled = await settle(work());
      if (settled.kind === "aborted") {
        status = "skipped";
        return "done";
      }
      if (settled.kind === "saved") {
        setNotice(null);
        setSessionFailure(null);
        await router.invalidate();
        status = "saved";
        return "done";
      }
      if (isFieldInvalid(settled)) {
        setNotice(settled.notice);
        status = "rejected";
        return "done";
      }
      const view = mutationView(settled);
      if (view.kind === "session") {
        setNotice(null);
        setSessionFailure(view.failure);
        status = "rejected";
        return "done";
      }
      setNotice(view.message);
      setSessionFailure(null);
      if (settled.kind === "failed") {
        await router.invalidate();
      }
      status = "rejected";
      return "done";
    });
    if (job === null) {
      return "skipped";
    }
    setBusy(true);
    try {
      await job;
    } finally {
      setBusy(false);
    }
    return status;
  }

  function retry() {
    setSessionFailure(null);
    setNotice(null);
    void router.invalidate();
  }

  return { busy, notice, sessionFailure, run, retry };
}

function isFieldInvalid(
  result: Exclude<MutationOutcome, { readonly kind: "saved" }>,
): result is { readonly kind: "invalid"; readonly notice: string } {
  return result.kind === "invalid" && "notice" in result;
}
