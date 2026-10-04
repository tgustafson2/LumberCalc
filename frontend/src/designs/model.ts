import type { Design, DesignId, DesignSummary, V1Client, V1Result } from "../api";

export type V1Failure = Exclude<V1Result<unknown>, { readonly kind: "ok" }>;

export type DesignsListModel =
  | {
      readonly kind: "ok";
      readonly displayName: string;
      readonly designs: readonly DesignSummary[];
    }
  | V1Failure;

export type DesignDetailModel =
  | { readonly kind: "ok"; readonly design: Design }
  | { readonly kind: "absent" }
  | V1Failure;

export type CreateDesignResult =
  | { readonly kind: "blank-name" }
  | { readonly kind: "created"; readonly design: Design }
  | V1Failure;

export type CopyDesignResult = { readonly kind: "copied" } | V1Failure;

export type DeleteDesignResult = { readonly kind: "gone" } | V1Failure;

export type RenameDesignResult =
  | { readonly kind: "blank-name" }
  | { readonly kind: "same-name" }
  | { readonly kind: "renamed" }
  | { readonly kind: "stale" }
  | { readonly kind: "absent" }
  | V1Failure;

export type DesignActions = {
  readonly create: (rawName: string, signal?: AbortSignal) => Promise<CreateDesignResult>;
  readonly copy: (id: DesignId, signal?: AbortSignal) => Promise<CopyDesignResult>;
  readonly delete: (id: DesignId, signal?: AbortSignal) => Promise<DeleteDesignResult>;
  readonly rename: (design: Design, rawName: string, signal?: AbortSignal) => Promise<RenameDesignResult>;
};

export type ActionGate = { current: "open" | "busy" | "held" };

export type ListActivity =
  | { readonly kind: "idle" }
  | { readonly kind: "confirm-delete"; readonly id: DesignId }
  | { readonly kind: "working" };

export type ListEvent =
  | { readonly type: "ask-delete"; readonly id: DesignId }
  | { readonly type: "cancel-delete" }
  | { readonly type: "start-work" }
  | { readonly type: "stop-work" };

export type DetailActivity = { readonly kind: "idle" } | { readonly kind: "working" } | { readonly kind: "stale" };

export type DetailEvent =
  | { readonly type: "start-work" }
  | { readonly type: "stop-work" }
  | { readonly type: "mark-stale" }
  | { readonly type: "load-latest" };

const staleVersionMessage = "stale version";

const updatedAtFormat = new Intl.DateTimeFormat("en-US", {
  dateStyle: "medium",
  timeStyle: "short",
});

export async function loadDesignList(api: V1Client, signal: AbortSignal): Promise<DesignsListModel> {
  const [me, list] = await Promise.all([api.me({ signal }), api.listDesigns({ signal })]);
  if (me.kind !== "ok") {
    return me;
  }
  if (list.kind !== "ok") {
    return list;
  }
  return { kind: "ok", displayName: me.value.displayName, designs: list.value };
}

export async function loadDesignDetail(
  api: V1Client,
  designIdParam: string,
  signal: AbortSignal,
): Promise<DesignDetailModel> {
  const id = designIdFromParam(designIdParam);
  if (id === null) {
    return { kind: "absent" };
  }
  const result = await api.getDesign(id, { signal });
  if (result.kind === "ok") {
    return { kind: "ok", design: result.value };
  }
  if (isNotFound(result)) {
    return { kind: "absent" };
  }
  return result;
}

export function designActions(api: V1Client): DesignActions {
  return {
    create: async (rawName, signal) => {
      const name = trimmedName(rawName);
      if (name === null) {
        return { kind: "blank-name" };
      }
      const result = await api.createDesign({ name }, { signal });
      if (result.kind === "ok") {
        return { kind: "created", design: result.value };
      }
      return result;
    },
    copy: async (id, signal) => {
      const result = await api.copyDesign(id, { signal });
      if (result.kind === "ok") {
        return { kind: "copied" };
      }
      return result;
    },
    delete: async (id, signal) => {
      const result = await api.deleteDesign(id, { signal });
      if (result.kind === "ok" || isNotFound(result)) {
        return { kind: "gone" };
      }
      return result;
    },
    rename: async (design, rawName, signal) => {
      const name = trimmedName(rawName);
      if (name === null) {
        return { kind: "blank-name" };
      }
      if (name === design.name) {
        return { kind: "same-name" };
      }
      const result = await api.replaceDesign(
        design.id,
        {
          name,
          description: design.description,
          document: design.document,
          version: design.version,
        },
        { signal },
      );
      if (result.kind === "ok") {
        return { kind: "renamed" };
      }
      if (isNotFound(result)) {
        return { kind: "absent" };
      }
      if (isStaleVersion(result)) {
        return { kind: "stale" };
      }
      return result;
    },
  };
}

export function nextListActivity(state: ListActivity, event: ListEvent): ListActivity {
  switch (event.type) {
    case "start-work":
      if (state.kind === "idle" || state.kind === "confirm-delete") {
        return { kind: "working" };
      }
      return state;
    case "ask-delete":
      if (state.kind === "idle") {
        return { kind: "confirm-delete", id: event.id };
      }
      return state;
    case "cancel-delete":
      if (state.kind === "confirm-delete") {
        return { kind: "idle" };
      }
      return state;
    case "stop-work":
      if (state.kind === "working") {
        return { kind: "idle" };
      }
      return state;
    default: {
      const _exhaustive: never = event;
      return _exhaustive;
    }
  }
}

export function nextDetailActivity(state: DetailActivity, event: DetailEvent): DetailActivity {
  switch (event.type) {
    case "start-work":
      if (state.kind === "idle") {
        return { kind: "working" };
      }
      return state;
    case "mark-stale":
      if (state.kind === "working") {
        return { kind: "stale" };
      }
      return state;
    case "stop-work":
      if (state.kind === "working") {
        return { kind: "idle" };
      }
      return state;
    case "load-latest":
      if (state.kind === "stale") {
        return { kind: "idle" };
      }
      return state;
    default: {
      const _exhaustive: never = event;
      return _exhaustive;
    }
  }
}

export function formatUpdatedAt(updatedAt: string): string {
  const parsed = new Date(updatedAt);
  if (Number.isNaN(parsed.getTime())) {
    return updatedAt;
  }
  return updatedAtFormat.format(parsed);
}

export function designSummaryText(design: Design): string {
  return `Version ${design.version}. Updated ${formatUpdatedAt(design.updatedAt)}. Piece count ${design.document.pieces.length}. Connection count ${design.document.connections.length}.`;
}

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

function trimmedName(rawName: string): string | null {
  const name = rawName.trim();
  if (name.length === 0) {
    return null;
  }
  return name;
}

function designIdFromParam(param: string): DesignId | null {
  const trimmed = param.trim();
  if (trimmed.length === 0) {
    return null;
  }
  return trimmed as DesignId;
}

function isNotFound(result: V1Failure): boolean {
  return result.kind === "failed" && result.status === 404;
}

function isStaleVersion(result: V1Failure): boolean {
  return result.kind === "failed" && result.status === 409 && result.message === staleVersionMessage;
}
