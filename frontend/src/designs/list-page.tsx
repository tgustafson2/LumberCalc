import { useRef, useState, type FormEvent } from "react";
import { SignOutButton } from "@clerk/clerk-react";
import { Link, useRouter } from "@tanstack/react-router";
import type { DesignId } from "../api";
import { FailedResult } from "../failed-result";
import {
  beginAction,
  formatUpdatedAt,
  mutationView,
  nextListActivity,
  settle,
  type DesignActions,
  type DesignsListModel,
  type ListActivity,
  type V1Failure,
} from "./model";

export function DesignsListPage(props: {
  readonly model: DesignsListModel;
  readonly actions: Pick<DesignActions, "create" | "copy" | "delete">;
}) {
  const router = useRouter();
  const gate = useRef<"open" | "busy" | "held">("open");
  const [activity, setActivity] = useState<ListActivity>({ kind: "idle" });
  const [draftName, setDraftName] = useState("");
  const [notice, setNotice] = useState<string | null>(null);
  const [failure, setFailure] = useState<V1Failure | null>(null);

  function showMutation(result: V1Failure) {
    const view = mutationView(result);
    if (view.kind === "session") {
      setNotice(null);
      setFailure(view.failure);
      return;
    }
    setNotice(view.message);
  }

  async function run(work: () => Promise<void>) {
    const job = beginAction(gate, async () => {
      await work();
      return "done";
    });
    if (job === null) {
      return;
    }
    setActivity((state) => nextListActivity(state, { type: "start-work" }));
    try {
      await job;
    } finally {
      setActivity((state) => nextListActivity(state, { type: "stop-work" }));
    }
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
  if (props.model.kind !== "ok") {
    return <FailedResult result={props.model} onRetry={() => router.invalidate()} />;
  }

  function onCreate(event: FormEvent) {
    event.preventDefault();
    void run(async () => {
      const settled = await settle(props.actions.create(draftName));
      if (settled.kind === "blank-name") {
        setNotice("Enter a name.");
        return;
      }
      if (settled.kind === "aborted") {
        return;
      }
      if (settled.kind === "created") {
        setNotice(null);
        await router.invalidate();
        await router.navigate({
          to: "/designs/$designId",
          params: { designId: settled.design.id },
        });
        return;
      }
      showMutation(settled);
    });
  }

  function onCopy(id: DesignId) {
    void run(async () => {
      const settled = await settle(props.actions.copy(id));
      if (settled.kind === "aborted") {
        return;
      }
      if (settled.kind === "copied") {
        setNotice(null);
        await router.invalidate();
        return;
      }
      showMutation(settled);
    });
  }

  function onDelete(id: DesignId) {
    void run(async () => {
      const settled = await settle(props.actions.delete(id));
      if (settled.kind === "aborted") {
        return;
      }
      if (settled.kind === "gone") {
        setNotice(null);
        await router.invalidate();
        return;
      }
      showMutation(settled);
    });
  }

  return (
    <main>
      <h1>Designs for {props.model.displayName}</h1>
      <SignOutButton />
      <form onSubmit={onCreate}>
        <label>
          Name
          <input value={draftName} onChange={(event) => setDraftName(event.target.value)} />
        </label>
        <button type="submit" disabled={activity.kind === "working"}>
          Create
        </button>
      </form>
      {notice !== null ? <p>{notice}</p> : null}
      {props.model.designs.length === 0 ? <p>You have no designs.</p> : null}
      <ul>
        {props.model.designs.map((design) => (
          <li key={design.id}>
            <Link to="/designs/$designId" params={{ designId: design.id }}>
              {design.name}
            </Link>
            <p>{formatUpdatedAt(design.updatedAt)}</p>
            {activity.kind === "confirm-delete" && activity.id === design.id ? (
              <>
                <p>Delete this design?</p>
                <button type="button" onClick={() => void onDelete(design.id)}>
                  Delete
                </button>
                <button
                  type="button"
                  onClick={() =>
                    setActivity((state) => nextListActivity(state, { type: "cancel-delete" }))
                  }
                >
                  Cancel
                </button>
              </>
            ) : (
              <>
                <button
                  type="button"
                  disabled={activity.kind === "working"}
                  onClick={() => void onCopy(design.id)}
                >
                  Copy
                </button>
                <button
                  type="button"
                  disabled={activity.kind === "working"}
                  onClick={() =>
                    setActivity((state) =>
                      nextListActivity(state, { type: "ask-delete", id: design.id }),
                    )
                  }
                >
                  Delete
                </button>
              </>
            )}
          </li>
        ))}
      </ul>
    </main>
  );
}
