import { useRef, useState, type FormEvent } from "react";
import { Link, useRouter } from "@tanstack/react-router";
import type { V1Failure } from "../api";
import { FailedResult } from "../failed-result";
import { beginAction, mutationView, settle } from "../mutation";
import { SignedInHeader } from "../signed-in-header";
import {
  designSummaryText,
  nextDetailActivity,
  type DesignActions,
  type DesignDetailModel,
  type DetailActivity,
  type PublicationCommand,
} from "./model";

export function DesignDetailPage(props: {
  readonly model: DesignDetailModel;
  readonly rename: DesignActions["rename"];
  readonly setPublication: DesignActions["setPublication"];
}) {
  const router = useRouter();
  const gate = useRef<"open" | "busy" | "held">("open");
  const saved = props.model.kind === "ok" ? props.model.design : null;
  const savedId = saved?.id;
  const savedName = saved?.name;
  const savedVersion = saved?.version;
  const [draftName, setDraftName] = useState(savedName ?? "");
  const [seenDesign, setSeenDesign] = useState({
    id: savedId,
    name: savedName,
    version: savedVersion,
  });
  const [activity, setActivity] = useState<DetailActivity>({ kind: "idle" });
  const [notice, setNotice] = useState<string | null>(null);
  const [failure, setFailure] = useState<V1Failure | null>(null);
  if (seenDesign.id !== savedId || seenDesign.name !== savedName || seenDesign.version !== savedVersion) {
    setSeenDesign({ id: savedId, name: savedName, version: savedVersion });
    setDraftName(savedName ?? "");
  }

  function showMutation(result: V1Failure) {
    const view = mutationView(result);
    if (view.kind === "session") {
      setNotice(null);
      setFailure(view.failure);
      return;
    }
    setNotice(view.message);
  }

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
        <SignedInHeader title="Design not found" />
        <Link to="/designs">All designs</Link>
      </main>
    );
  }
  if (props.model.kind !== "ok") {
    return <FailedResult result={props.model} onRetry={() => router.invalidate()} />;
  }

  const design = props.model.design;

  function onPublication(command: PublicationCommand) {
    const job = beginAction(gate, async () => {
      const settled = await settle(props.setPublication(design.id, command));
      if (settled.kind === "aborted") {
        return "done";
      }
      if (settled.kind === "changed" || settled.kind === "absent") {
        setNotice(null);
        await router.invalidate();
        return "done";
      }
      if (settled.kind === "rejected") {
        setNotice(settled.notice);
        return "done";
      }
      showMutation(settled);
      return "done";
    });
    if (job === null) {
      return;
    }
    setActivity((state) => nextDetailActivity(state, { type: "start-work" }));
    void job.finally(() => {
      setActivity((state) => nextDetailActivity(state, { type: "stop-work" }));
    });
  }

  function onSave(event: FormEvent) {
    event.preventDefault();
    const job = beginAction(gate, async () => {
      const settled = await settle(props.rename(design, draftName));
      if (settled.kind === "blank-name") {
        setNotice("Enter a name.");
        return "done";
      }
      if (settled.kind === "aborted") {
        return "done";
      }
      if (settled.kind === "same-name") {
        setNotice("The name is already saved.");
        return "done";
      }
      if (settled.kind === "renamed" || settled.kind === "absent") {
        setNotice(null);
        await router.invalidate();
        return "done";
      }
      if (settled.kind === "stale") {
        setNotice("This design changed. Load the latest version.");
        return "hold";
      }
      showMutation(settled);
      return "done";
    });
    if (job === null) {
      return;
    }
    setActivity((state) => nextDetailActivity(state, { type: "start-work" }));
    void job.finally(() => {
      setActivity((state) =>
        nextDetailActivity(state, {
          type: gate.current === "held" ? "mark-stale" : "stop-work",
        }),
      );
    });
  }

  return (
    <main>
      <SignedInHeader title={design.name} />
      <p>{designSummaryText(design)}</p>
      {design.description.length > 0 ? <p>{design.description}</p> : null}
      <form onSubmit={onSave}>
        <label>
          Name
          <input value={draftName} onChange={(event) => setDraftName(event.target.value)} />
        </label>
        {activity.kind === "stale" ? (
          <button
            type="button"
            onClick={() => {
              gate.current = "open";
              setActivity((state) => nextDetailActivity(state, { type: "load-latest" }));
              setNotice(null);
              setDraftName(design.name);
              void router.invalidate();
            }}
          >
            Load latest
          </button>
        ) : (
          <button type="submit" disabled={activity.kind === "working"}>
            Save name
          </button>
        )}
      </form>
      <section>
        <p>{props.model.publicationText}</p>
        {props.model.controls.map((control) => (
          <button
            key={control.label}
            type="button"
            disabled={activity.kind !== "idle"}
            onClick={() => onPublication(control.command)}
          >
            {control.label}
          </button>
        ))}
      </section>
      {notice !== null ? <p>{notice}</p> : null}
      <pre>{JSON.stringify(design.document, null, 2)}</pre>
      <Link to="/designs">All designs</Link>
    </main>
  );
}
