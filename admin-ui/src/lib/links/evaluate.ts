import get from "lodash/get";
import { OnceTask } from "@/lib/types/oncetask";
import { LinkDef, LinksConfig, RenderedLink } from "./types";

// evaluateLinks renders all applicable links for a task given the loaded config.
// Returns an array of RenderedLink, in this order: global links first, then per-type links.
//
// A link renders only when:
//   1. Every entry in `requires` resolves to a non-empty value on the task.
//   2. Every `{placeholder}` in `url` resolves to a non-empty value.
//
// Placeholders are auto URL-encoded via encodeURIComponent. Operators must pre-encode
// the surrounding URL structure (this matches how URLs are typically copy-pasted from
// browser bars).
export function evaluateLinks(
  task: OnceTask,
  config: LinksConfig,
): RenderedLink[] {
  const candidates: LinkDef[] = [
    ...config.global,
    ...(config.types[task.type]?.links ?? []),
  ];

  const rendered: RenderedLink[] = [];
  for (const def of candidates) {
    if (!meetsRequires(task, def.requires)) continue;
    const url = interpolate(def.url, task);
    if (url === null) continue;
    rendered.push({
      label: def.label,
      url,
      primary: !!def.primary,
      icon: def.icon,
    });
  }
  return rendered;
}

function meetsRequires(
  task: OnceTask,
  requires: string[] | undefined,
): boolean {
  if (!requires || requires.length === 0) return true;
  return requires.every((path) => {
    const value = get(task, path);
    return value !== undefined && value !== null && value !== "";
  });
}

// interpolate replaces every {path.to.field} with the URL-encoded value from the task.
// Returns null if any placeholder resolves to empty - this is how "auto-hide if data missing"
// is implemented.
function interpolate(template: string, task: OnceTask): string | null {
  let missing = false;
  const result = template.replace(/\{([^}]+)\}/g, (_, path) => {
    const value = get(task, path.trim());
    if (value === undefined || value === null || value === "") {
      missing = true;
      return "";
    }
    return encodeURIComponent(String(value));
  });
  return missing ? null : result;
}
