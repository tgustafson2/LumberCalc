import type { V1Client } from "./api";

export function stubClient(methods: Partial<V1Client>): V1Client {
  return {
    me: async () => ({ kind: "invalid" }),
    createDesign: async () => ({ kind: "invalid" }),
    listDesigns: async () => ({ kind: "invalid" }),
    getDesign: async () => ({ kind: "invalid" }),
    replaceDesign: async () => ({ kind: "invalid" }),
    deleteDesign: async () => ({ kind: "invalid" }),
    copyDesign: async () => ({ kind: "invalid" }),
    publishDesign: async () => ({ kind: "invalid" }),
    unpublishDesign: async () => ({ kind: "invalid" }),
    listPatterns: async () => ({ kind: "invalid" }),
    getPattern: async () => ({ kind: "invalid" }),
    addPattern: async () => ({ kind: "invalid" }),
    createMaterial: async () => ({ kind: "invalid" }),
    listMaterials: async () => ({ kind: "invalid" }),
    replaceMaterial: async () => ({ kind: "invalid" }),
    deleteMaterial: async () => ({ kind: "invalid" }),
    createPrice: async () => ({ kind: "invalid" }),
    listPrices: async () => ({ kind: "invalid" }),
    updatePrice: async () => ({ kind: "invalid" }),
    deletePrice: async () => ({ kind: "invalid" }),
    ...methods,
  };
}
