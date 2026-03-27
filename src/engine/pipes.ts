import { TemplateError } from "./errors.js";

type PipeFn = (value: unknown, arg: string | undefined, line: number | undefined) => string;

const PIPES: Record<string, PipeFn> = {
  upper: (v) => String(v).toUpperCase(),
  lower: (v) => String(v).toLowerCase(),
  kebab: (v) =>
    String(v)
      .replace(/[\s_]+/g, "-")
      .toLowerCase(),
  join: (v, arg, line) => {
    if (!Array.isArray(v)) {
      throw new TemplateError(`join pipe requires an array`, line);
    }
    return v.join(arg ?? ", ");
  },
  indent: (v, arg, line) => {
    const raw = arg ?? "2";
    const n = parseInt(raw, 10);
    if (isNaN(n)) {
      throw new TemplateError(
        `indent pipe requires a numeric argument, got "${raw}"`,
        line,
      );
    }
    const pad = " ".repeat(n);
    return String(v)
      .split("\n")
      .map((l) => `${pad}${l}`)
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
  return { name: match[1]!, arg: match[2] };
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
    result = fn(result, pipe.arg, line);
  }
  return String(result);
}
