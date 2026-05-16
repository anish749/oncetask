import "server-only";
import { readFileSync, existsSync } from "fs";
import { resolve } from "path";
import yaml from "js-yaml";
import { EMPTY_LINKS_CONFIG, LinksConfig } from "./types";

let cached: LinksConfig | null = null;

// loadLinksConfig reads the YAML link config at startup.
// Path is taken from ONCETASK_LINKS_CONFIG (env var); if unset or file missing,
// returns an empty config and the UI shows no links beyond actions.
export function loadLinksConfig(): LinksConfig {
  if (cached) return cached;

  const path = process.env.ONCETASK_LINKS_CONFIG;
  if (!path) {
    cached = EMPTY_LINKS_CONFIG;
    return cached;
  }

  const abs = resolve(path);
  if (!existsSync(abs)) {
    console.warn(
      `[oncetask-admin] links config not found at ${abs}, no links will render`,
    );
    cached = EMPTY_LINKS_CONFIG;
    return cached;
  }

  const raw = readFileSync(abs, "utf-8");
  const parsed = yaml.load(raw) as Partial<LinksConfig> | undefined;

  cached = {
    global: parsed?.global ?? [],
    types: parsed?.types ?? {},
  };
  return cached;
}
