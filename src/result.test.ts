import { describe, it, expect } from "vitest";
import { ok, err, DomainError } from "./result.js";
import type { Result } from "./result.js";

describe("Result type", () => {
  it("ok() wraps data with ok: true", () => {
    const result = ok(42);
    expect(result).toEqual({ ok: true, data: 42 });
  });

  it("err() wraps error with ok: false", () => {
    const error = new DomainError("boom");
    const result = err(error);
    expect(result).toEqual({ ok: false, error });
  });

  it("narrows via ok discriminant", () => {
    const result: Result<number, DomainError> = ok(42);
    if (result.ok) {
      expect(result.data).toBe(42);
    } else {
      expect.unreachable("should be ok");
    }
  });

  it("DomainError is instanceof Error", () => {
    const error = new DomainError("test");
    expect(error).toBeInstanceOf(Error);
    expect(error.name).toBe("DomainError");
    expect(error.message).toBe("test");
  });

  it("DomainError supports cause via ErrorOptions", () => {
    const cause = new Error("root");
    const error = new DomainError("wrapped", { cause });
    expect(error.cause).toBe(cause);
  });
});
