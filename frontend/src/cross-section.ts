import type { Material } from "./api";

export type CrossSection = {
  readonly thicknessIn: number;
  readonly widthIn: number;
};

type PhysicalSizes = Pick<
  Material,
  "actualThicknessIn" | "actualWidthIn" | "roughThicknessIn" | "roughWidthIn"
>;

// crossSection selects the dressed actual sizes when roughCut is missing or false.
// It selects the rough sizes when roughCut is true.
// Nominal sizes and piece length are not inputs.
export function crossSection(
  material: PhysicalSizes,
  roughCut: boolean | undefined,
): CrossSection {
  if (roughCut) {
    return {
      thicknessIn: material.roughThicknessIn,
      widthIn: material.roughWidthIn,
    };
  }
  return {
    thicknessIn: material.actualThicknessIn,
    widthIn: material.actualWidthIn,
  };
}
