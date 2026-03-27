import { TemplateError } from "./errors.js";

type PipeFn = (value: unknown, arg?: string) => string;

const PIPES: Record<string, PipeFn> = {
  upper: (v) => String(v).toUpperCase(),
  lower: (v) => String(v).toLowerCase(),
  kebab: (v) =>
    String(v)
      .replace(/[\s_]+/g, "-")
      .toLowerCase(),
  join: (v, arg) => {
    if (!Array.isArray(v)) return String(v);
    return v.join(arg ?? ", ");
  },
  indent: (v, arg) => {
    const n = parseInt(arg ?? "2", 10);
    const pad = " ".repeat(n);
    return String(v)
      .split("\n")
      .map((line) => `${pad}${line}`)
      .join("\n");
  },
};

export interface ParsedPipe {
  name: string;
  arg?: string;
}

export function parsePipeExpr(raw: string): ParsedPipe {
  const match = raw.match(/^(\w+)(?:\(([^)]*)\))?$/);
  if (!match) throw new TemplateError(`Invalid pipe: "${raw}"`);
  return { name: match[1], arg: match[2] };
}

export function applyPipes(
  value: unknown,
  pipes: ParsedPipe[],
  line?: number,
): string {
  let result: unknown = value;
  for (const pipe of pipes) {
    const fn = PIPES[pipe.name];
    if (!fn) throw new TemplateError(`Unknown pipe "${pipe.name}"`, line);
    result = fn(result, pipe.arg);
  }
  return String(result);
}
