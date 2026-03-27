import { parse } from "./parser.js";
import { renderAST } from "./renderer.js";

export { TemplateError } from "./errors.js";
export { parse } from "./parser.js";
export { renderAST } from "./renderer.js";

export function render(
  template: string,
  context: Record<string, unknown>,
): string {
  const ast = parse(template);
  return renderAST(ast, context);
}
