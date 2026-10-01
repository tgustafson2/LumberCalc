import react from "@vitejs/plugin-react";
import { defineConfig } from "vitest/config";

export default defineConfig({
  plugins: [react()],
  test: {
    passWithNoTests: true,
  },
  server: {
    // Clerk rejects the session unless azp equals CLERK_AUTHORIZED_PARTIES.
    host: "localhost",
    port: 5173,
    strictPort: true,
    proxy: {
      "/v1": { target: "http://127.0.0.1:8080" },
    },
  },
});
