import { useState } from "react";
import { useRouter } from "@tanstack/react-router";
import type { MaterialId, Price } from "../api";
import { FailedResult } from "../failed-result";
import { useMutationRunner } from "../mutation";
import { SignedInHeader } from "../signed-in-header";
import { dollarsText, formatCents, type StandardLengthIn } from "./units";
import {
  buildPriceGrid,
  cellAt,
  type CatalogCommand,
  type CatalogModel,
  type CatalogOutcome,
  type MaterialProblems,
  type PriceCell,
  type PriceGrid,
} from "./model";

type PriceSlot =
  | { readonly kind: "new"; readonly materialId: MaterialId; readonly lengthIn: StandardLengthIn }
  | { readonly kind: "existing"; readonly price: Price };

type PriceEntry = {
  readonly kind: "entering";
  readonly slot: PriceSlot;
  readonly dollars: string;
  readonly problem: string | null;
};

type PriceEditor =
  | { readonly kind: "closed" }
  | PriceEntry
  | { readonly kind: "confirm-delete"; readonly price: Price };

export function PricesPage(props: {
  readonly model: CatalogModel;
  readonly run: (command: CatalogCommand) => Promise<CatalogOutcome>;
}) {
  const router = useRouter();
  const runner = useMutationRunner();
  const [editor, setEditor] = useState<PriceEditor>({ kind: "closed" });

  if (runner.sessionFailure !== null) {
    return <FailedResult result={runner.sessionFailure} onRetry={runner.retry} />;
  }
  if (props.model.kind !== "ok") {
    return <FailedResult result={props.model} onRetry={() => router.invalidate()} />;
  }

  const model = props.model;
  const grid = buildPriceGrid(model);
  const live = livePriceEditor(editor, grid);

  async function onSave(entry: PriceEntry) {
    const command: CatalogCommand =
      entry.slot.kind === "new"
        ? {
            type: "create-price",
            materialId: entry.slot.materialId,
            lengthIn: entry.slot.lengthIn,
            dollars: entry.dollars,
          }
        : { type: "update-price", price: entry.slot.price, dollars: entry.dollars };
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
      setEditor({ ...entry, problem: outcome.notice });
    }
  }

  async function onDelete(price: Price) {
    const status = await runner.run(() => props.run({ type: "delete-price", price }));
    if (status !== "skipped") {
      setEditor({ kind: "closed" });
    }
  }

  return (
    <main>
      <SignedInHeader title={`Prices for ${model.displayName}`} />
      {runner.notice !== null ? <p>{runner.notice}</p> : null}
      {grid.rows.length === 0 ? (
        <p>There are no materials to price.</p>
      ) : (
        <table>
          <thead>
            <tr>
              <th scope="col">Material</th>
              {grid.columns.map((column) => (
                <th key={column.lengthIn} scope="col">
                  {column.label}
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {grid.rows.map((row) => (
              <tr key={row.materialId}>
                <th scope="row">{row.label}</th>
                {grid.columns.map((column) => (
                  <td key={column.lengthIn} className="numeric">
                    <PriceCellView
                      cell={cellAt(row, column)}
                      editor={live}
                      busy={runner.busy}
                      onAdd={(cell) =>
                        setEditor({
                          kind: "entering",
                          slot: {
                            kind: "new",
                            materialId: cell.materialId,
                            lengthIn: cell.lengthIn,
                          },
                          dollars: "",
                          problem: null,
                        })
                      }
                      onEdit={(price) =>
                        setEditor({
                          kind: "entering",
                          slot: { kind: "existing", price },
                          dollars: dollarsText(price.priceCents),
                          problem: null,
                        })
                      }
                      onAskDelete={(price) => setEditor({ kind: "confirm-delete", price })}
                      onChange={(dollars) => {
                        if (live.kind === "entering") {
                          setEditor({ ...live, dollars, problem: null });
                        }
                      }}
                      onSave={() => {
                        if (live.kind === "entering") {
                          void onSave(live);
                        }
                      }}
                      onDelete={(price) => void onDelete(price)}
                      onCancel={() => setEditor({ kind: "closed" })}
                    />
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </main>
  );
}

function PriceCellView(props: {
  readonly cell: PriceCell;
  readonly editor: PriceEditor;
  readonly busy: boolean;
  readonly onAdd: (cell: Extract<PriceCell, { readonly kind: "open" }>) => void;
  readonly onEdit: (price: Price) => void;
  readonly onAskDelete: (price: Price) => void;
  readonly onChange: (dollars: string) => void;
  readonly onSave: () => void;
  readonly onDelete: (price: Price) => void;
  readonly onCancel: () => void;
}) {
  const active = editorForCell(props.editor, props.cell);
  if (active?.kind === "entering") {
    return (
      <form
        onSubmit={(event) => {
          event.preventDefault();
          props.onSave();
        }}
      >
        <label>
          Price in dollars
          <input value={active.dollars} onChange={(event) => props.onChange(event.target.value)} />
        </label>
        {active.problem !== null ? <p>{active.problem}</p> : null}
        <button type="submit" disabled={props.busy}>
          Save
        </button>
        <button type="button" disabled={props.busy} onClick={props.onCancel}>
          Cancel
        </button>
      </form>
    );
  }
  if (active?.kind === "confirm-delete") {
    return (
      <>
        <p>Delete this price?</p>
        <button type="button" disabled={props.busy} onClick={() => props.onDelete(active.price)}>
          Delete
        </button>
        <button type="button" disabled={props.busy} onClick={props.onCancel}>
          Cancel
        </button>
      </>
    );
  }
  if (props.cell.kind === "priced") {
    const price = props.cell.price;
    return (
      <>
        {formatCents(price.priceCents)}
        <button type="button" disabled={props.busy} onClick={() => props.onEdit(price)}>
          Edit
        </button>
        <button type="button" disabled={props.busy} onClick={() => props.onAskDelete(price)}>
          Delete
        </button>
      </>
    );
  }
  if (props.cell.kind === "open") {
    const cell = props.cell;
    return (
      <button type="button" disabled={props.busy} onClick={() => props.onAdd(cell)}>
        Add
      </button>
    );
  }
  return null;
}

function editorForCell(editor: PriceEditor, cell: PriceCell): PriceEditor | null {
  if (editor.kind === "closed") {
    return null;
  }
  if (editor.kind === "confirm-delete") {
    return cell.kind === "priced" && cell.price.id === editor.price.id ? editor : null;
  }
  if (editor.slot.kind === "new") {
    return cell.kind === "open" &&
      cell.materialId === editor.slot.materialId &&
      cell.lengthIn === editor.slot.lengthIn
      ? editor
      : null;
  }
  return cell.kind === "priced" && cell.price.id === editor.slot.price.id ? editor : null;
}

function livePriceEditor(editor: PriceEditor, grid: PriceGrid): PriceEditor {
  if (editor.kind === "closed") {
    return editor;
  }
  if (editor.kind === "confirm-delete") {
    return findPrice(grid, editor.price.id) === undefined ? { kind: "closed" } : editor;
  }
  const slot = editor.slot;
  if (slot.kind === "existing") {
    return findPrice(grid, slot.price.id) === undefined ? { kind: "closed" } : editor;
  }
  const row = grid.rows.find((item) => item.materialId === slot.materialId);
  if (row === undefined || !row.accepting || row.priced.has(slot.lengthIn)) {
    return { kind: "closed" };
  }
  return editor;
}

function findPrice(grid: PriceGrid, id: Price["id"]): Price | undefined {
  for (const row of grid.rows) {
    for (const price of row.priced.values()) {
      if (price.id === id) {
        return price;
      }
    }
  }
  return undefined;
}

function isFieldInvalid(
  outcome: CatalogOutcome,
): outcome is { readonly kind: "invalid"; readonly notice: string; readonly problems: MaterialProblems } {
  return outcome.kind === "invalid" && "problems" in outcome;
}
