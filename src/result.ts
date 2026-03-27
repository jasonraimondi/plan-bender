export type Result<T, E extends Error = Error> =
  | { ok: true; data: T }
  | { ok: false; error: E };

export function ok<T>(data: T): Result<T, never> {
  return { ok: true, data };
}

export function err<E extends Error>(error: E): Result<never, E> {
  return { ok: false, error };
}

export class DomainError extends Error {
  constructor(message: string, options?: ErrorOptions) {
    super(message, options);
    this.name = "DomainError";
  }
}
