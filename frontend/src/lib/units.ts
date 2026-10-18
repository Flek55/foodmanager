import type { Schemas } from "../api/client";

export type Unit = Schemas["Unit"];

export const UNITS: { code: Unit; label: string }[] = [
  { code: "pcs", label: "шт" },
  { code: "kg", label: "кг" },
  { code: "g", label: "г" },
  { code: "l", label: "л" },
  { code: "ml", label: "мл" },
  { code: "pack", label: "уп" },
];

const LABELS = Object.fromEntries(UNITS.map((u) => [u.code, u.label])) as Record<Unit, string>;

const numberFmt = new Intl.NumberFormat("ru-RU", { maximumFractionDigits: 3 });

/** "2 л", "1,5 кг", "3 шт"; null when there is no quantity. */
export function formatQuantity(quantity?: number, unit?: Unit): string | null {
  if (quantity === undefined) return null;
  return `${numberFmt.format(quantity)} ${LABELS[unit ?? "pcs"]}`;
}

// Words people type after a number, mapped to unit codes.
const UNIT_WORDS: Record<string, Unit> = {
  шт: "pcs", штук: "pcs", штука: "pcs", штуки: "pcs",
  кг: "kg", кило: "kg", килограмм: "kg", килограмма: "kg",
  г: "g", гр: "g", грамм: "g", грамма: "g",
  л: "l", литр: "l", литра: "l", литров: "l",
  мл: "ml",
  уп: "pack", упак: "pack", упаковка: "pack", упаковки: "pack", упаковок: "pack",
  пач: "pack", пачка: "pack", пачки: "pack", пачек: "pack",
};

// Longest first, so "литра" wins over "л".
const UNIT_RE = Object.keys(UNIT_WORDS)
  .sort((a, b) => b.length - a.length)
  .join("|");
const NUM = String.raw`(\d+(?:[.,]\d+)?)`;
const TRAILING = new RegExp(String.raw`^(.+?)[\s,]+(?:[xх×]\s*)?${NUM}\s*(${UNIT_RE})?\.?$`, "iu");
const LEADING = new RegExp(String.raw`^${NUM}\s*(${UNIT_RE})?\.?\s+(.+)$`, "iu");

/**
 * Splits quick-add input into name and amount:
 *   "Молоко 2л"      → Молоко, 2 l
 *   "яйца 10"        → яйца, 10 pcs
 *   "500 г сыра"     → сыра, 500 g
 *   "Молоко 3,2%"    → Молоко 3,2% (the % keeps it a name)
 */
export function parseQuickAdd(input: string): { name: string; quantity?: number; unit?: Unit } {
  const text = input.trim().replace(/\s+/g, " ");
  const toNum = (s: string) => Number(s.replace(",", "."));
  const toUnit = (s?: string) => (s ? UNIT_WORDS[s.toLowerCase()] : "pcs");

  const t = TRAILING.exec(text);
  if (t?.[1] && t[2]) {
    return { name: t[1].trim(), quantity: toNum(t[2]), unit: toUnit(t[3]) };
  }
  const l = LEADING.exec(text);
  if (l?.[1] && l[3]) {
    return { name: l[3].trim(), quantity: toNum(l[1]), unit: toUnit(l[2]) };
  }
  return { name: text };
}

/** Case- and ё-insensitive form for searching product names. */
export function searchable(s: string) {
  return s.toLowerCase().replaceAll("ё", "е");
}
