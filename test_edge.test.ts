import { describe, it, expect } from "vitest";
import { render, parse } from "./src/engine/index.js";

describe("edge cases", () => {
  it("empty variable reference", () => {
    expect(() => render("${}", {})).toThrow();
  });

  it("unclosed brace", () => {
    const result = render("${name", { name: "test" });
    expect(result).toBe("${name");
  });

  it("null value", () => {
    expect(() => render("${val}", { val: null })).toThrow();
  });

  it("undefined in path", () => {
    expect(() => render("${a.b.c}", { a: { b: {} } })).toThrow();
  });

  it("pipe char in argument", () => {
    const result = render("${items | join(|)}", { items: ["a", "b"] });
    expect(result).toContain("a");
  });
});
