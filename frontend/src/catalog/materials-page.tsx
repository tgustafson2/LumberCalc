import { useState } from "react";
import { useRouter } from "@tanstack/react-router";
import { FailedResult } from "../failed-result";
import { useMutationRunner } from "../mutation";
import { SignedInHeader } from "../signed-in-header";
import { formatSize } from "./units";
import {
  blankMaterialFields,
  fieldsFromMaterial,
  sizeKinds,
  type CatalogCommand,
  type CatalogMaterial,
  type CatalogModel,
  type CatalogOutcome,
  type CustomMaterial,
  type MaterialFieldKey,
  type MaterialFields,
  type MaterialProblems,
} from "./model";

type OpenMaterialEditor = {
  readonly kind: "open";
  readonly target: "new" | CustomMaterial;
  readonly fields: MaterialFields;
  readonly problems: MaterialProblems;
};

type MaterialEditor =
  | { readonly kind: "closed" }
  | OpenMaterialEditor
  | { readonly kind: "confirm-delete"; readonly target: CustomMaterial };

export function MaterialsPage(props: {
  readonly model: CatalogModel;
  readonly run: (command: CatalogCommand) => Promise<CatalogOutcome>;
}) {
  const router = useRouter();
  const runner = useMutationRunner();
  const [editor, setEditor] = useState<MaterialEditor>({ kind: "closed" });

  if (runner.sessionFailure !== null) {
    return <FailedResult result={runner.sessionFailure} onRetry={runner.retry} />;
  }
  if (props.model.kind !== "ok") {
    return <FailedResult result={props.model} onRetry={() => router.invalidate()} />;
  }

  const model = props.model;
  const live = liveMaterialEditor(editor, model.materials);

  async function onSave(current: OpenMaterialEditor) {
    const command: CatalogCommand =
      current.target === "new"
        ? { type: "create-material", fields: current.fields }
        : { type: "replace-material", target: current.target, fields: current.fields };
    let outcome: CatalogOutcome | undefined;
    const status = await runner.run(async () => {
      outcome = await props.run(command);
      return outcome;
    });
    if (status === "saved") {
      setEditor({ kind: "closed" });
      return;
    }
    if (outcome !== undefined && isFieldInvalid(outcome)) {
      setEditor({ ...current, problems: outcome.problems });
    }
  }

  async function onDelete(target: CustomMaterial) {
    const status = await runner.run(() => props.run({ type: "delete-material", target }));
    if (status !== "skipped") {
      setEditor({ kind: "closed" });
    }
  }

  function materialActions(material: CatalogMaterial) {
    if (material.system) {
      return null;
    }
    if (live.kind === "confirm-delete" && live.target.id === material.id) {
      const target = live.target;
      return (
        <>
          <p>Delete this material?</p>
          <button type="button" disabled={runner.busy} onClick={() => void onDelete(target)}>
            Delete
          </button>
          <button type="button" disabled={runner.busy} onClick={() => setEditor({ kind: "closed" })}>
            Cancel
          </button>
        </>
      );
    }
    return (
      <>
        <button
          type="button"
          disabled={runner.busy}
          onClick={() =>
            setEditor({
              kind: "open",
              target: material,
              fields: fieldsFromMaterial(material),
              problems: {},
            })
          }
        >
          Edit
        </button>
        <button
          type="button"
          disabled={runner.busy}
          onClick={() => setEditor({ kind: "confirm-delete", target: material })}
        >
          Delete
        </button>
      </>
    );
  }

  return (
    <main>
      <SignedInHeader title={`Materials for ${model.displayName}`} />
      {runner.notice !== null ? <p>{runner.notice}</p> : null}
      {live.kind === "open" && live.target === "new" ? (
        <MaterialForm
          fields={live.fields}
          problems={live.problems}
          busy={runner.busy}
          submitLabel="Save"
          onChange={(fields) => setEditor({ ...live, fields, problems: {} })}
          onSubmit={() => void onSave(live)}
          onCancel={() => setEditor({ kind: "closed" })}
        />
      ) : (
        <button
          type="button"
          disabled={runner.busy || live.kind !== "closed"}
          onClick={() =>
            setEditor({
              kind: "open",
              target: "new",
              fields: blankMaterialFields,
              problems: {},
            })
          }
        >
          New material
        </button>
      )}
      <table>
        <thead>
          <tr>
            <th scope="col">Name</th>
            {sizeKinds.map((kind) => (
              <th key={kind.key} scope="col">
                {kind.label}
              </th>
            ))}
            <th scope="col">Source</th>
            <th scope="col">Actions</th>
          </tr>
        </thead>
        <tbody>
          {model.materials.map((material) => (
            <tr key={material.id}>
              {live.kind === "open" && live.target !== "new" && live.target.id === material.id ? (
                <td colSpan={sizeKinds.length + 3}>
                  <MaterialForm
                    fields={live.fields}
                    problems={live.problems}
                    busy={runner.busy}
                    submitLabel="Save"
                    onChange={(fields) => setEditor({ ...live, fields, problems: {} })}
                    onSubmit={() => void onSave(live)}
                    onCancel={() => setEditor({ kind: "closed" })}
                  />
                </td>
              ) : (
                <>
                  <td>{material.name}</td>
                  {sizeKinds.map((kind) => (
                    <td key={kind.key} className="numeric">
                      {formatSize(material[kind.thickness], material[kind.width])}
                    </td>
                  ))}
                  <td>{material.system ? "System" : "Custom"}</td>
                  <td>{materialActions(material)}</td>
                </>
              )}
            </tr>
          ))}
        </tbody>
      </table>
    </main>
  );
}

function MaterialForm(props: {
  readonly fields: MaterialFields;
  readonly problems: MaterialProblems;
  readonly busy: boolean;
  readonly submitLabel: string;
  readonly onChange: (fields: MaterialFields) => void;
  readonly onSubmit: () => void;
  readonly onCancel: () => void;
}) {
  function setField(key: MaterialFieldKey, value: string) {
    props.onChange({ ...props.fields, [key]: value });
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        props.onSubmit();
      }}
    >
      <label>
        Name
        <input value={props.fields.name} onChange={(event) => setField("name", event.target.value)} />
      </label>
      {props.problems.name !== undefined ? <p>{props.problems.name}</p> : null}
      <label>
        Species
        <input
          value={props.fields.species}
          onChange={(event) => setField("species", event.target.value)}
        />
      </label>
      <label>
        Grade
        <input value={props.fields.grade} onChange={(event) => setField("grade", event.target.value)} />
      </label>
      {sizeKinds.map((kind) => (
        <fieldset key={kind.key}>
          <legend>{kind.label}</legend>
          <label>
            Thickness
            <input
              value={props.fields[kind.thickness]}
              onChange={(event) => setField(kind.thickness, event.target.value)}
            />
          </label>
          {props.problems[kind.thickness] !== undefined ? <p>{props.problems[kind.thickness]}</p> : null}
          <label>
            Width
            <input
              value={props.fields[kind.width]}
              onChange={(event) => setField(kind.width, event.target.value)}
            />
          </label>
          {props.problems[kind.width] !== undefined ? <p>{props.problems[kind.width]}</p> : null}
        </fieldset>
      ))}
      <button type="submit" disabled={props.busy}>
        {props.submitLabel}
      </button>
      <button type="button" disabled={props.busy} onClick={props.onCancel}>
        Cancel
      </button>
    </form>
  );
}

function liveMaterialEditor(
  editor: MaterialEditor,
  materials: readonly CatalogMaterial[],
): MaterialEditor {
  if (editor.kind === "closed") {
    return editor;
  }
  if (editor.kind === "confirm-delete") {
    const found = findCustom(materials, editor.target.id);
    return found === undefined ? { kind: "closed" } : { kind: "confirm-delete", target: found };
  }
  if (editor.target === "new") {
    return editor;
  }
  const found = findCustom(materials, editor.target.id);
  if (found === undefined) {
    return { kind: "closed" };
  }
  return { ...editor, target: found };
}

function findCustom(
  materials: readonly CatalogMaterial[],
  id: CatalogMaterial["id"],
): CustomMaterial | undefined {
  const found = materials.find((material) => material.id === id);
  if (found === undefined || found.system) {
    return undefined;
  }
  return found;
}

function isFieldInvalid(
  outcome: CatalogOutcome,
): outcome is { readonly kind: "invalid"; readonly notice: string; readonly problems: MaterialProblems } {
  return outcome.kind === "invalid" && "problems" in outcome;
}
