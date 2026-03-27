import { TemplateError } from "./errors.js";
import { type ParsedPipe, parsePipeExpr } from "./pipes.js";

export type ASTNode = TextNode | VarNode | IfNode | EachNode;

export interface TextNode {
  type: "text";
  value: string;
  line: number;
}

export interface VarNode {
  type: "var";
  name: string;
  pipes: ParsedPipe[];
  line: number;
}

export interface IfNode {
  type: "if";
  condition: string;
  negated: boolean;
  body: ASTNode[];
  line: number;
}

export interface EachNode {
  type: "each";
  collection: string;
  itemName: string;
  body: ASTNode[];
  line: number;
}

const VAR_RE = /\$\{([^}]+)\}/g;

/** Split on | but not inside parentheses */
function splitPipes(expr: string): string[] {
  const parts: string[] = [];
  let current = "";
  let depth = 0;
  for (const ch of expr) {
    if (ch === "(") depth++;
    else if (ch === ")") depth--;
    if (ch === "|" && depth === 0) {
      parts.push(current.trim());
      current = "";
    } else {
      current += ch;
    }
  }
  parts.push(current.trim());
  return parts;
}

function parseLine(text: string, lineNum: number): ASTNode[] {
  const nodes: ASTNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  VAR_RE.lastIndex = 0;
  while ((match = VAR_RE.exec(text)) !== null) {
    if (match.index > lastIndex) {
      nodes.push({
        type: "text",
        value: text.slice(lastIndex, match.index),
        line: lineNum,
      });
    }
    const inner = match[1]!.trim();
    const parts = splitPipes(inner);
    const name = parts[0]!;
    const pipes = parts.slice(1).map(parsePipeExpr);
    nodes.push({ type: "var", name, pipes, line: lineNum });
    lastIndex = VAR_RE.lastIndex;
  }

  if (lastIndex < text.length) {
    nodes.push({ type: "text", value: text.slice(lastIndex), line: lineNum });
  }

  return nodes;
}

export function parse(template: string): ASTNode[] {
  const lines = template.split("\n");
  const root: ASTNode[] = [];
  const stack: { body: ASTNode[]; type: "if" | "each"; line: number }[] = [];

  function current(): ASTNode[] {
    return stack.length > 0 ? stack[stack.length - 1]!.body : root;
  }

  for (let i = 0; i < lines.length; i++) {
    const lineNum = i + 1;
    const line = lines[i]!;
    const trimmed = line.trim();

    if (trimmed.startsWith("@if ")) {
      let condition = trimmed.slice(4).trim();
      let negated = false;
      while (condition.startsWith("!")) {
        negated = !negated;
        condition = condition.slice(1).trim();
      }
      const node: IfNode = {
        type: "if",
        condition,
        negated,
        body: [],
        line: lineNum,
      };
      current().push(node);
      stack.push({ body: node.body, type: "if", line: lineNum });
      continue;
    }

    if (trimmed.startsWith("@each ")) {
      const m = trimmed.match(/^@each\s+(\S+)\s+as\s+(\S+)$/);
      if (!m) throw new TemplateError(`Invalid @each syntax: "${trimmed}"`, lineNum);
      const node: EachNode = {
        type: "each",
        collection: m[1]!,
        itemName: m[2]!,
        body: [],
        line: lineNum,
      };
      current().push(node);
      stack.push({ body: node.body, type: "each", line: lineNum });
      continue;
    }

    if (trimmed === "@end") {
      if (stack.length === 0) {
        throw new TemplateError("Unexpected @end without matching block", lineNum);
      }
      stack.pop();
      continue;
    }

    current().push(...parseLine(line, lineNum));
    // Add newline between lines (except trailing)
    if (i < lines.length - 1) {
      current().push({ type: "text", value: "\n", line: lineNum });
    }
  }

  if (stack.length > 0) {
    const unclosed = stack[stack.length - 1]!;
    throw new TemplateError(
      `Unclosed @${unclosed.type} block`,
      unclosed.line,
    );
  }

  return root;
}
