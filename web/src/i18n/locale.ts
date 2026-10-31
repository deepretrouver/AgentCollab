import { useSyncExternalStore } from "react";
import english from "./en.json";

export type Locale = "zh-CN" | "en";
const key = "agentcollab.locale";
let current: Locale = readLocale();
if (typeof document !== "undefined") document.documentElement.lang = current;
const listeners = new Set<() => void>();
function readLocale(): Locale {
  try { return localStorage.getItem(key) === "en" ? "en" : "zh-CN"; }
  catch { return "zh-CN"; }
}
function subscribe(listener: () => void) { listeners.add(listener); return () => { listeners.delete(listener); }; }
export function setLocale(locale: Locale) {
  current = locale;
  try { localStorage.setItem(key, locale); } catch { /* still usable without persistence */ }
  document.documentElement.lang = locale;
  listeners.forEach(listener => listener());
}
export function useLocale() {
  const locale = useSyncExternalStore(subscribe, () => current);
  const t = (text: string) => locale === "en" ? (english as Record<string, string>)[text] ?? text : text;
  return { locale, setLocale, t };
}
