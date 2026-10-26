import { useEffect, useMemo, useState } from "react";
import { SignIn, useAuth, useClerk } from "@clerk/clerk-react";
import {
  Link,
  Outlet,
  RouterProvider,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
} from "@tanstack/react-router";
import { createV1Client } from "./api";
import { DesignDetailPage } from "./designs/detail-page";
import { DesignsListPage } from "./designs/list-page";
import { loadCatalog, runCatalogCommand } from "./catalog/model";
import { MaterialsPage } from "./catalog/materials-page";
import { PricesPage } from "./catalog/prices-page";
import { designActions, loadDesignDetail, loadDesignList } from "./designs/model";
import { PatternDetailPage } from "./patterns/detail-page";
import { PatternGalleryPage } from "./patterns/gallery-page";
import { addPattern, loadPatternDetail, loadPatternGallery } from "./patterns/model";
import { redirectFor, type SessionGate } from "./gate";

type ResolvedGate = Exclude<SessionGate, { kind: "loading" }>;

type RouterContext = { readonly gate: ResolvedGate };

function useSessionGate(): SessionGate {
  const { isLoaded, isSignedIn, sessionId } = useAuth();
  const clerk = useClerk();
  return useMemo((): SessionGate => {
    if (!isLoaded) {
      return { kind: "loading" };
    }
    if (!isSignedIn || sessionId === null || sessionId.length === 0) {
      return { kind: "signed-out" };
    }
    return {
      kind: "signed-in",
      api: createV1Client({
        getToken: () => clerk.session?.getToken() ?? Promise.resolve(null),
      }),
    };
  }, [isLoaded, isSignedIn, sessionId, clerk]);
}

const rootRoute = createRootRouteWithContext<RouterContext>()({
  component: Outlet,
  notFoundComponent: NotFound,
});

const signedOutBranch = createRoute({
  getParentRoute: () => rootRoute,
  id: "signed-out",
  beforeLoad: ({ context }) => {
    const destination = redirectFor(context.gate, "signed-out");
    if (destination !== null) {
      throw redirect({ to: destination });
    }
  },
});

const signedInBranch = createRoute({
  getParentRoute: () => rootRoute,
  id: "signed-in",
  beforeLoad: ({ context }) => {
    const destination = redirectFor(context.gate, "signed-in");
    if (destination !== null) {
      throw redirect({ to: destination });
    }
    if (context.gate.kind !== "signed-in") {
      throw redirect({ to: "/" });
    }
    return { api: context.gate.api };
  },
});

const signInRoute = createRoute({
  getParentRoute: () => signedOutBranch,
  path: "/",
  component: SignInPage,
});

const designsRoute = createRoute({
  getParentRoute: () => signedInBranch,
  path: "/designs",
  loader: ({ context, abortController }) =>
    loadDesignList(context.api, abortController.signal),
  component: DesignsListRoute,
});

const designDetailRoute = createRoute({
  getParentRoute: () => signedInBranch,
  path: "/designs/$designId",
  loader: ({ context, params, abortController }) =>
    loadDesignDetail(context.api, params.designId, abortController.signal),
  component: DesignDetailRoute,
});

const patternsRoute = createRoute({
  getParentRoute: () => signedInBranch,
  path: "/patterns",
  loader: ({ context, abortController }) => loadPatternGallery(context.api, abortController.signal),
  component: PatternGalleryRoute,
});

const patternDetailRoute = createRoute({
  getParentRoute: () => signedInBranch,
  path: "/patterns/$patternId",
  loader: ({ context, params, abortController }) =>
    loadPatternDetail(context.api, params.patternId, abortController.signal),
  component: PatternDetailRoute,
});

const materialsRoute = createRoute({
  getParentRoute: () => signedInBranch,
  path: "/materials",
  loader: ({ context, abortController }) => loadCatalog(context.api, abortController.signal),
  component: MaterialsRoute,
});

const pricesRoute = createRoute({
  getParentRoute: () => signedInBranch,
  path: "/prices",
  loader: ({ context, abortController }) => loadCatalog(context.api, abortController.signal),
  component: PricesRoute,
});

const routeTree = rootRoute.addChildren([
  signedOutBranch.addChildren([signInRoute]),
  signedInBranch.addChildren([
    designsRoute,
    designDetailRoute,
    patternsRoute,
    patternDetailRoute,
    materialsRoute,
    pricesRoute,
  ]),
]);

function buildRouter(gate: ResolvedGate) {
  return createRouter({ routeTree, context: { gate } });
}

declare module "@tanstack/router-core" {
  interface Register {
    router: ReturnType<typeof buildRouter>;
  }
}

export function App() {
  const gate = useSessionGate();
  if (gate.kind === "loading") {
    return null;
  }
  return <SessionRouter gate={gate} />;
}

function SessionRouter({ gate }: { readonly gate: ResolvedGate }) {
  const [router] = useState(() => buildRouter(gate));
  useEffect(() => {
    void router.invalidate();
  }, [router, gate]);
  return <RouterProvider router={router} context={{ gate }} />;
}

function SignInPage() {
  return (
    <main>
      <SignIn routing="hash" />
    </main>
  );
}

function DesignsListRoute() {
  const model = designsRoute.useLoaderData();
  const { api } = designsRoute.useRouteContext();
  const { create, copy, delete: remove } = designActions(api);
  return <DesignsListPage model={model} actions={{ create, copy, delete: remove }} />;
}

function DesignDetailRoute() {
  const model = designDetailRoute.useLoaderData();
  const { api } = designDetailRoute.useRouteContext();
  const { rename, setPublication } = designActions(api);
  return <DesignDetailPage model={model} rename={rename} setPublication={setPublication} />;
}

function PatternGalleryRoute() {
  return <PatternGalleryPage model={patternsRoute.useLoaderData()} />;
}

function PatternDetailRoute() {
  const model = patternDetailRoute.useLoaderData();
  const { api } = patternDetailRoute.useRouteContext();
  return <PatternDetailPage model={model} add={addPattern(api)} />;
}

function MaterialsRoute() {
  const model = materialsRoute.useLoaderData();
  const { api } = materialsRoute.useRouteContext();
  return <MaterialsPage model={model} run={(command) => runCatalogCommand(api, command)} />;
}

function PricesRoute() {
  const model = pricesRoute.useLoaderData();
  const { api } = pricesRoute.useRouteContext();
  return <PricesPage model={model} run={(command) => runCatalogCommand(api, command)} />;
}

function NotFound() {
  return (
    <main>
      <p>Page not found.</p>
      <Link to="/">Home</Link>
    </main>
  );
}
