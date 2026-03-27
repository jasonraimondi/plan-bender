import type { ASTNode } from "./parser.js";
import { TemplateError } from "./errors.js";
import { applyPipes } from "./pipes.js";

type Context = Record<string, unknown>;

function resolve(name: string, ctx: Context, line?: number): unknown {
  const parts = name.split(".");
  let current: unknown = ctx;
  for (const part of parts) {
    if (current == null || typeof current !== "object") {
      throw new TemplateError(`Cannot resolve "${name}"`, line);
    }
    current = (current as Record<string, unknown>)[part];
  }
  if (current === undefined) {
    throw new TemplateError(`Unresolved variable "${name}"`, line);
  }
  if (current === null) {
    throw new TemplateError(`Null value for variable "${name}"`, line);
  }
  return current;
}

function isTruthy(val: unknown): boolean {
  if (Array.isArray(val)) return val.length > 0;
  return Boolean(val);
}

function stripTrailingNewline(s: string): string {
  return s.endsWith("\n") ? s.slice(0, -1) : s;
}

function renderNodes(nodes: ASTNode[], ctx: Context): string {
  const parts: string[] = [];

  for (const node of nodes) {
    switch (node.type) {
      case "text":
        parts.push(node.value);
        break;

      case "var": {
        const value = resolve(node.name, ctx, node.line);
        parts.push(applyPipes(value, node.pipes, node.line));
        break;
      }

      case "if": {
        const val = resolve(node.condition, ctx, node.line);
        const show = node.negated ? !isTruthy(val) : isTruthy(val);
        if (show) {
          parts.push(stripTrailingNewline(renderNodes(node.body, ctx)));
        }
        break;
      }

      case "each": {
        const collection = resolve(node.collection, ctx, node.line);
        if (!Array.isArray(collection)) {
          throw new TemplateError(
            `"${node.collection}" is not an array`,
            node.line,
          );
        }
        const rendered: string[] = [];
        for (const item of collection) {
          const childCtx = { ...ctx, [node.itemName]: item };
          rendered.push(stripTrailingNewline(renderNodes(node.body, childCtx)));
        }
        parts.push(rendered.filter(Boolean).join("\n"));
        break;
      }
    }
  }

  return parts.join("");
}

export function renderAST(nodes: ASTNode[], ctx: Context): string {
  return renderNodes(nodes, ctx);
}
