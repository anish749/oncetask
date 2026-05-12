// LinkDef is one declarative link rule loaded from links.yaml.
// `url` is a Mustache-style template; `{path.to.field}` is resolved against the task
// document with auto URL encoding via encodeURIComponent.
// `requires` is a list of task-field paths that must resolve to non-empty values
// for the link to render. This is the only conditioning mechanism - no DSL.
export interface LinkDef {
  label: string;
  url: string;
  requires?: string[];
  primary?: boolean;
  icon?: string;
}

export interface TypeLinksConfig {
  label?: string;
  links: LinkDef[];
}

export interface LinksConfig {
  global: LinkDef[];
  types: Record<string, TypeLinksConfig>;
}

export interface RenderedLink {
  label: string;
  url: string;
  primary: boolean;
  icon?: string;
}

export const EMPTY_LINKS_CONFIG: LinksConfig = {
  global: [],
  types: {},
};
