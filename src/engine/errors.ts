export class TemplateError extends Error {
  constructor(
    message: string,
    public readonly line?: number,
  ) {
    const prefix = line != null ? `Line ${line}: ` : "";
    super(`${prefix}${message}`);
    this.name = "TemplateError";
  }
}
