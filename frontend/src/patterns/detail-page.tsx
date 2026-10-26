import { useRef, useState } from "react";
import { Link, useRouter } from "@tanstack/react-router";
import type { PatternId, V1Failure } from "../api";
import { FailedResult } from "../failed-result";
import { beginAction, mutationView, settle } from "../mutation";
import { SignedInHeader } from "../signed-in-header";
import type { AddPatternResult, PatternDetailModel } from "./model";

export function PatternDetailPage(props: {
  readonly model: PatternDetailModel;
  readonly add: (id: PatternId) => Promise<AddPatternResult>;
}) {
  const router = useRouter();
  const gate = useRef<"open" | "busy" | "held">("open");
  const [working, setWorking] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  const [failure, setFailure] = useState<V1Failure | null>(null);

  if (failure !== null) {
    return (
      <FailedResult
        result={failure}
        onRetry={() => {
          setFailure(null);
          setNotice(null);
          void router.invalidate();
        }}
      />
    );
  }
  if (props.model.kind === "absent") {
    return (
      <main>
        <SignedInHeader title="Pattern not found" />
        <Link to="/patterns">All patterns</Link>
      </main>
    );
  }
  if (props.model.kind !== "ok") {
    return <FailedResult result={props.model} onRetry={() => router.invalidate()} />;
  }

  const pattern = props.model.pattern;

  function onAdd() {
    const job = beginAction(gate, async () => {
      const settled = await settle(props.add(pattern.id));
      if (settled.kind === "aborted") {
        return "done";
      }
      if (settled.kind === "added") {
        setNotice(null);
        await router.invalidate();
        await router.navigate({ to: "/designs/$designId", params: { designId: settled.designId } });
        return "done";
      }
      if (settled.kind === "absent") {
        setNotice(null);
        await router.invalidate();
        return "done";
      }
      if (settled.kind === "rejected") {
        setNotice(settled.notice);
        return "done";
      }
      const view = mutationView(settled);
      if (view.kind === "session") {
        setNotice(null);
        setFailure(view.failure);
        return "done";
      }
      setNotice(view.message);
      return "done";
    });
    if (job === null) {
      return;
    }
    setWorking(true);
    void job.finally(() => setWorking(false));
  }

  return (
    <main>
      <SignedInHeader title={pattern.name} />
      <p>{pattern.publisherLabel}</p>
      {pattern.description.length > 0 ? <p>{pattern.description}</p> : null}
      <p>{`Piece count ${pattern.pieceCount}.`}</p>
      {pattern.canAdd ? (
        <button type="button" disabled={working} onClick={onAdd}>
          Add to my designs
        </button>
      ) : (
        <p>This pattern is private. Only you can see it. Republish its design to add it.</p>
      )}
      {notice !== null ? <p>{notice}</p> : null}
      <Link to="/patterns">All patterns</Link>
    </main>
  );
}
