import { Link, useRouter } from "@tanstack/react-router";
import { FailedResult } from "../failed-result";
import { SignedInHeader } from "../signed-in-header";
import type { PatternGalleryModel } from "./model";

export function PatternGalleryPage(props: { readonly model: PatternGalleryModel }) {
  const router = useRouter();
  if (props.model.kind !== "ok") {
    return <FailedResult result={props.model} onRetry={() => router.invalidate()} />;
  }
  return (
    <main>
      <SignedInHeader title="Patterns" />
      {props.model.patterns.length === 0 ? (
        <p>No patterns are published yet. Publish a design from its page to share it here.</p>
      ) : (
        <ul>
          {props.model.patterns.map((pattern) => (
            <li key={pattern.id}>
              <Link to="/patterns/$patternId" params={{ patternId: pattern.id }}>
                {pattern.name}
              </Link>
              <p>{pattern.publisherLabel}</p>
            </li>
          ))}
        </ul>
      )}
    </main>
  );
}
