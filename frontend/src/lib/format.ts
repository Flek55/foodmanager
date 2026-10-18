/** Russian plural form: pluralize(5, "участник", "участника", "участников"). */
export function pluralize(n: number, one: string, few: string, many: string) {
  const mod10 = n % 10;
  const mod100 = n % 100;
  if (mod10 === 1 && mod100 !== 11) return one;
  if (mod10 >= 2 && mod10 <= 4 && (mod100 < 12 || mod100 > 14)) return few;
  return many;
}

const dateFmt = new Intl.DateTimeFormat("ru-RU", { day: "numeric", month: "long" });

export function formatDate(iso: string) {
  return dateFmt.format(new Date(iso));
}

/** "сегодня", "вчера", "3 дня назад", or a date for anything older than a week. */
export function formatAgo(iso: string) {
  const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  const days = Math.round((startOfDay(new Date()) - startOfDay(new Date(iso))) / 86_400_000);
  if (days <= 0) return "сегодня";
  if (days === 1) return "вчера";
  if (days < 7) return `${days} ${pluralize(days, "день", "дня", "дней")} назад`;
  return formatDate(iso);
}
