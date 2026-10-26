import {
  isNotFound,
  type DesignId,
  type PatternId,
  type PatternSummary,
  type V1Client,
  type V1Failure,
} from "../api";

export type PatternCard = {
  readonly id: PatternId;
  readonly name: string;
  readonly publisherLabel: string;
};

export type PatternDetailView = {
  readonly id: PatternId;
  readonly name: string;
  readonly description: string;
  readonly publisherLabel: string;
  readonly pieceCount: number;
  readonly canAdd: boolean;
};

export type PatternGalleryModel = { readonly kind: "ok"; readonly patterns: readonly PatternCard[] } | V1Failure;

export type PatternDetailModel =
  | { readonly kind: "ok"; readonly pattern: PatternDetailView }
  | { readonly kind: "absent" }
  | V1Failure;

export type AddPatternResult =
  | { readonly kind: "added"; readonly designId: DesignId }
  | { readonly kind: "absent" }
  | { readonly kind: "rejected"; readonly notice: string }
  | V1Failure;

const materialUnavailableMessage = "material not available";

export async function loadPatternGallery(api: V1Client, signal: AbortSignal): Promise<PatternGalleryModel> {
  const result = await api.listPatterns({ signal });
  if (result.kind !== "ok") {
    return result;
  }
  return {
    kind: "ok",
    patterns: result.value.map((pattern) => ({
      id: pattern.id,
      name: pattern.name,
      publisherLabel: publisherLabel(pattern),
    })),
  };
}

export async function loadPatternDetail(
  api: V1Client,
  patternIdParam: string,
  signal: AbortSignal,
): Promise<PatternDetailModel> {
  const id = patternIdFromParam(patternIdParam);
  if (id === null) {
    return { kind: "absent" };
  }
  const result = await api.getPattern(id, { signal });
  if (result.kind === "ok") {
    const pattern = result.value;
    return {
      kind: "ok",
      pattern: {
        id: pattern.id,
        name: pattern.name,
        description: pattern.description,
        publisherLabel: publisherLabel(pattern),
        pieceCount: pattern.document.pieces.length,
        canAdd: pattern.isPublic,
      },
    };
  }
  if (isNotFound(result)) {
    return { kind: "absent" };
  }
  return result;
}

export function addPattern(api: V1Client) {
  return async (id: PatternId, signal?: AbortSignal): Promise<AddPatternResult> => {
    const result = await api.addPattern(id, { signal });
    if (result.kind === "ok") {
      return { kind: "added", designId: result.value.id };
    }
    if (isNotFound(result)) {
      return { kind: "absent" };
    }
    if (result.kind === "failed" && result.status === 409 && result.message === materialUnavailableMessage) {
      return { kind: "rejected", notice: "This pattern uses a material that is not available." };
    }
    return result;
  };
}

function publisherLabel(pattern: PatternSummary): string {
  return pattern.publisherName === null ? "By an unnamed publisher" : `By ${pattern.publisherName}`;
}

function patternIdFromParam(param: string): PatternId | null {
  const trimmed = param.trim();
  if (trimmed.length === 0) {
    return null;
  }
  return trimmed as PatternId;
}
