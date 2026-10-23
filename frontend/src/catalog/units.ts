import type { Brand } from "../api";

/** Integer cents from 0 through 2147483647. Only parseDollars creates one. */
export type Cents = Brand<number, "Cents">;

export const standardLengthsIn = [96, 120, 144] as const;
export type StandardLengthIn = (typeof standardLengthsIn)[number];

export type FieldParse<T> =
  | { readonly kind: "ok"; readonly value: T }
  | { readonly kind: "invalid"; readonly problem: string };

const maxCents = 2147483647;
const dollarProblem = "Enter dollars, like 4.98.";
const dollarTooLarge = "That price is too large.";
const inchProblem = "Enter inches greater than 0.";

const usd = new Intl.NumberFormat("en-US", {
  style: "currency",
  currency: "USD",
});

/**
 * Accepts "1.5", "1 1/2", "1-1/2", and "3/4", with an optional trailing `"` or "in".
 * The result is finite and greater than 0.
 */
export function parseInches(raw: string): FieldParse<number> {
  const text = stripInchSuffix(raw.trim());
  const mixedSpace = /^(\d+)\s+(\d+)\/(\d+)$/.exec(text);
  const mixedHyphen = /^(\d+)-(\d+)\/(\d+)$/.exec(text);
  const fraction = /^(\d+)\/(\d+)$/.exec(text);
  const decimal = /^(\d+)(?:\.(\d+))?$/.exec(text);
  const value = mixedSpace
    ? fractionValue(mixedSpace[1], mixedSpace[2], mixedSpace[3])
    : mixedHyphen
      ? fractionValue(mixedHyphen[1], mixedHyphen[2], mixedHyphen[3])
      : fraction
        ? fractionValue("0", fraction[1], fraction[2])
        : decimal
          ? decimalValue(decimal[1], decimal[2])
          : null;
  if (value === null || !(value > 0)) {
    return { kind: "invalid", problem: inchProblem };
  }
  return { kind: "ok", value };
}

/** Shortest decimal text. parseInches(formatInches(n)) returns n. */
export function formatInches(inches: number): string {
  if (!Number.isFinite(inches)) {
    return "Invalid";
  }
  return String(inches);
}

export function formatSize(thicknessIn: number, widthIn: number): string {
  return `${formatInches(thicknessIn)} x ${formatInches(widthIn)}`;
}

/** 96 is "8 ft". 100 is "8 ft 4 in". 8 is "8 in". Storage stays in inches. */
export function formatLength(lengthIn: number): string {
  if (!Number.isFinite(lengthIn)) {
    return "Invalid length";
  }
  const negative = lengthIn < 0;
  const abs = Math.abs(lengthIn);
  const feet = Math.floor(abs / 12);
  const inches = Number.isInteger(lengthIn) ? Math.abs(lengthIn) % 12 : abs - feet * 12;
  let text: string;
  if (feet === 0) {
    text = `${formatInches(inches)} in`;
  } else if (inches === 0) {
    text = `${feet} ft`;
  } else {
    text = `${feet} ft ${formatInches(inches)} in`;
  }
  return negative ? `-${text}` : text;
}

/**
 * Accepts "4.98", "$4.98", "4", "1,250.00", and "0". At most two decimals.
 * Uses the digit text, so "4.10" is 410 cents.
 */
export function parseDollars(raw: string): FieldParse<Cents> {
  const text = raw.trim().replace(/^\$/, "").replaceAll(",", "");
  const match = /^(\d+)(?:\.(\d{1,2}))?$/.exec(text);
  if (match === null) {
    return { kind: "invalid", problem: dollarProblem };
  }
  const wholeText = match[1];
  if (wholeText === undefined) {
    return { kind: "invalid", problem: dollarProblem };
  }
  const whole = Number(wholeText);
  const fraction = Number((match[2] ?? "").padEnd(2, "0"));
  if (!Number.isSafeInteger(whole) || !Number.isSafeInteger(fraction)) {
    return { kind: "invalid", problem: dollarTooLarge };
  }
  const cents = whole * 100 + fraction;
  if (!Number.isSafeInteger(cents) || cents > maxCents) {
    return { kind: "invalid", problem: dollarTooLarge };
  }
  return { kind: "ok", value: cents as Cents };
}

export function formatCents(cents: number): string {
  return usd.format(signedDollars(cents));
}

/** Prefill for the edit input. parseDollars(dollarsText(cents)) returns cents. */
export function dollarsText(cents: number): string {
  const negative = cents < 0;
  const abs = Math.abs(Math.trunc(cents));
  const whole = Math.trunc(abs / 100);
  const fraction = abs % 100;
  const text = `${whole}.${String(fraction).padStart(2, "0")}`;
  return negative ? `-${text}` : text;
}

function stripInchSuffix(text: string): string {
  if (text.endsWith('"')) {
    return text.slice(0, -1).trim();
  }
  if (text.endsWith("in")) {
    return text.slice(0, -2).trim();
  }
  return text;
}

function fractionValue(
  wholeText: string | undefined,
  numeratorText: string | undefined,
  denominatorText: string | undefined,
): number | null {
  if (wholeText === undefined || numeratorText === undefined || denominatorText === undefined) {
    return null;
  }
  const denominator = Number(denominatorText);
  if (denominator === 0) {
    return null;
  }
  const value = Number(wholeText) + Number(numeratorText) / denominator;
  if (!Number.isFinite(value)) {
    return null;
  }
  return value;
}

function decimalValue(wholeText: string | undefined, fractionText: string | undefined): number | null {
  if (wholeText === undefined) {
    return null;
  }
  const text = fractionText === undefined ? wholeText : `${wholeText}.${fractionText}`;
  const value = Number(text);
  if (!Number.isFinite(value)) {
    return null;
  }
  return value;
}

function signedDollars(cents: number): number {
  const negative = cents < 0;
  const abs = Math.abs(Math.trunc(cents));
  const whole = Math.trunc(abs / 100);
  const fraction = abs % 100;
  const amount = Number(`${whole}.${String(fraction).padStart(2, "0")}`);
  return negative ? -amount : amount;
}
