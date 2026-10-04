import { SignOutButton } from "@clerk/clerk-react";
import type { V1Failure } from "./designs/model";

export function FailedResult(props: {
  readonly result: V1Failure;
  readonly onRetry: () => void;
}) {
  switch (props.result.kind) {
    case "no-token":
      return <MissingToken onRetry={props.onRetry} />;
    case "unauthorized":
      return <SessionRejected />;
    case "unreachable":
      return <Retry message="The API is not reachable." onRetry={props.onRetry} />;
    case "invalid":
      return <Retry message="The API sent an unexpected response." onRetry={props.onRetry} />;
    case "failed":
      return <Retry message={props.result.message} onRetry={props.onRetry} />;
    default: {
      const _exhaustive: never = props.result;
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
