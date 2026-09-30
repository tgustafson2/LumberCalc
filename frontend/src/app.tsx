import { useEffect, useMemo, useState } from "react";
import { SignIn, SignOutButton, useAuth, useClerk } from "@clerk/clerk-react";
import {
  Link,
  Outlet,
  RouterProvider,
  createRootRouteWithContext,
  createRoute,
  createRouter,
  redirect,
  useRouter,
} from "@tanstack/react-router";
import { createV1Client } from "./api";
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
    context.api.me({ signal: abortController.signal }),
  component: DesignsPage,
});

const routeTree = rootRoute.addChildren([
  signedOutBranch.addChildren([signInRoute]),
  signedInBranch.addChildren([designsRoute]),
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

function DesignsPage() {
  const result = designsRoute.useLoaderData();
  const router = useRouter();
  switch (result.kind) {
    case "ok":
      return (
        <main>
          <h1>Designs for {result.value.displayName}</h1>
          <SignOutButton />
        </main>
      );
    case "no-token":
      return <MissingToken onRetry={() => router.invalidate()} />;
    case "unauthorized":
      return <SessionRejected />;
    case "unreachable":
      return (
        <Retry
          message="The API is not reachable."
          onRetry={() => router.invalidate()}
        />
      );
    case "invalid":
      return (
        <Retry
          message="The API sent an unexpected response."
          onRetry={() => router.invalidate()}
        />
      );
    case "failed":
      return (
        <Retry message={result.message} onRetry={() => router.invalidate()} />
      );
    default: {
      const _exhaustive: never = result;
      return _exhaustive;
    }
  }
}

function MissingToken({ onRetry }: { readonly onRetry: () => void }) {
  return (
    <main>
      <h1>Clerk did not provide a session token.</h1>
      <button type="button" onClick={onRetry}>
        Try again
      </button>
      <SignOutButton />
    </main>
  );
}

function SessionRejected() {
  return (
    <main>
      <h1>The API rejected this session.</h1>
      <p>Open http://localhost:5173, not 127.0.0.1.</p>
      <p>CLERK_AUTHORIZED_PARTIES must be http://localhost:5173.</p>
      <p>
        The publishable key and CLERK_SECRET_KEY must come from the same Clerk
        instance.
      </p>
      <SignOutButton />
    </main>
  );
}

function Retry({
  message,
  onRetry,
}: {
  readonly message: string;
  readonly onRetry: () => void;
}) {
  return (
    <main>
      <p>{message}</p>
      <button type="button" onClick={onRetry}>
        Try again
      </button>
    </main>
  );
}

function NotFound() {
  return (
    <main>
      <p>Page not found.</p>
      <Link to="/">Home</Link>
    </main>
  );
}
