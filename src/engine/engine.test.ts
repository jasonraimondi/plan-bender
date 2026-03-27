import { describe, it, expect } from "vitest";
import { render, TemplateError } from "./index.js";

describe("template engine", () => {
  describe("variable interpolation", () => {
    it("resolves simple variable", () => {
      expect(render("Hello ${name}", { name: "world" })).toBe("Hello world");
    });

    it("resolves dot-notation variable", () => {
      expect(render("${config.backend}", { config: { backend: "linear" } })).toBe(
        "linear",
      );
    });

    it("errors on unresolved variable", () => {
      expect(() => render("${missing}", {})).toThrow(/missing/);
    });

    it("throws TemplateError on null value", () => {
      expect(() => render("${x}", { x: null })).toThrow(TemplateError);
      expect(() => render("${x}", { x: null })).toThrow(/null/i);
    });

    it("includes line number when null value on later line", () => {
      try {
        render("line one\n${x}", { x: null });
        expect.unreachable("should have thrown");
      } catch (e) {
        expect(e).toBeInstanceOf(TemplateError);
        expect((e as TemplateError).line).toBe(2);
      }
    });

    it("renders multiple variables", () => {
      expect(render("${a} and ${b}", { a: "x", b: "y" })).toBe("x and y");
    });
  });

  describe("pipes", () => {
    it("upper", () => {
      expect(render("${name | upper}", { name: "hello" })).toBe("HELLO");
    });

    it("lower", () => {
      expect(render("${name | lower}", { name: "HELLO" })).toBe("hello");
    });

    it("kebab", () => {
      expect(render("${name | kebab}", { name: "hello world" })).toBe(
        "hello-world",
      );
    });

    it("join", () => {
      expect(render("${items | join(, )}", { items: ["a", "b", "c"] })).toBe(
        "a, b, c",
      );
    });

    it("indent", () => {
      expect(render("${text | indent(4)}", { text: "line1\nline2" })).toBe(
        "    line1\n    line2",
      );
    });

    it("chained pipes", () => {
      expect(render("${name | kebab | upper}", { name: "hello world" })).toBe(
        "HELLO-WORLD",
      );
    });
  });

  describe("@if blocks", () => {
    it("renders block when truthy", () => {
      expect(render("@if show\nvisible\n@end", { show: true })).toBe("visible");
    });

    it("omits block when falsy", () => {
      expect(render("@if show\nhidden\n@end", { show: false })).toBe("");
    });

    it("supports negation with !", () => {
      expect(render("@if !hide\nvisible\n@end", { hide: false })).toBe(
        "visible",
      );
    });

    it("treats empty array as falsy", () => {
      expect(render("@if items\nhas items\n@end", { items: [] })).toBe("");
    });

    it("treats non-empty array as truthy", () => {
      expect(render("@if items\nhas items\n@end", { items: [1] })).toBe(
        "has items",
      );
    });

    it("supports dot-notation condition", () => {
      expect(
        render("@if config.debug\ndebug on\n@end", {
          config: { debug: true },
        }),
      ).toBe("debug on");
    });

    it("treats !!flag as truthy check (double negation)", () => {
      expect(render("@if !!flag\nyes\n@end", { flag: true })).toBe("yes");
      expect(render("@if !!flag\nyes\n@end", { flag: false })).toBe("");
    });
  });

  describe("@each blocks", () => {
    it("iterates over array", () => {
      expect(
        render("@each items as item\n- ${item}\n@end", {
          items: ["a", "b"],
        }),
      ).toBe("- a\n- b");
    });

    it("renders empty for empty array", () => {
      expect(
        render("@each items as item\n- ${item}\n@end", { items: [] }),
      ).toBe("");
    });

    it("exposes item properties via dot notation", () => {
      expect(
        render("@each users as user\n${user.name}\n@end", {
          users: [{ name: "Alice" }, { name: "Bob" }],
        }),
      ).toBe("Alice\nBob");
    });
  });

  describe("nested blocks", () => {
    it("@if inside @each", () => {
      const tpl = [
        "@each items as item",
        "@if item.show",
        "- ${item.name}",
        "@end",
        "@end",
      ].join("\n");
      expect(
        render(tpl, {
          items: [
            { name: "a", show: true },
            { name: "b", show: false },
            { name: "c", show: true },
          ],
        }),
      ).toBe("- a\n- c");
    });

    it("@each inside @if", () => {
      const tpl = ["@if show", "@each items as i", "${i}", "@end", "@end"].join(
        "\n",
      );
      expect(render(tpl, { show: true, items: ["x", "y"] })).toBe("x\ny");
    });
  });

  describe("error reporting", () => {
    it("reports line number for unclosed block", () => {
      expect(() => render("@if x\nhello", { x: true })).toThrow(/unclosed/i);
    });

    it("reports unexpected @end", () => {
      expect(() => render("@end", {})).toThrow(/@end/);
    });

    it("reports unknown pipe", () => {
      expect(() => render("${x | bogus}", { x: "hi" })).toThrow(/bogus/);
    });

    it("throws on indent with non-numeric arg", () => {
      expect(() => render("${x | indent(abc)}", { x: "text" })).toThrow(
        TemplateError,
      );
      expect(() => render("${x | indent(abc)}", { x: "text" })).toThrow(
        /numeric/i,
      );
    });

    it("throws when join receives a non-array", () => {
      expect(() => render("${x | join(, )}", { x: "not-array" })).toThrow(
        TemplateError,
      );
      expect(() => render("${x | join(, )}", { x: "not-array" })).toThrow(
        /array/i,
      );
    });

    it("includes line number in pipe validation errors", () => {
      try {
        render("line one\n${x | indent(abc)}", { x: "text" });
        expect.unreachable("should have thrown");
      } catch (e) {
        expect(e).toBeInstanceOf(TemplateError);
        expect((e as TemplateError).line).toBe(2);
      }
    });
  });

  describe("plain text passthrough", () => {
    it("returns plain text unchanged", () => {
      expect(render("no variables here", {})).toBe("no variables here");
    });

    it("preserves blank lines", () => {
      expect(render("a\n\nb", {})).toBe("a\n\nb");
    });
  });
});
