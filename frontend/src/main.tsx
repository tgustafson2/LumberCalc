import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { ClerkProvider } from "@clerk/clerk-react";
import { App } from "./app";
import { readClientConfig } from "./client-config";
import "./index.css";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("The page is missing #root.");
}

const config = readClientConfig(import.meta.env);

createRoot(rootElement).render(
  <StrictMode>
    {config.kind === "invalid" ? (
      <p>{config.reason}</p>
    ) : (
      <ClerkProvider
        publishableKey={config.publishableKey}
        afterSignOutUrl="/"
      >
        <App />
      </ClerkProvider>
    )}
  </StrictMode>,
);
