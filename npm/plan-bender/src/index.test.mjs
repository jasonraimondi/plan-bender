import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { platform, arch } from "node:os";

describe("index.mjs", () => {
  describe("PLATFORM_PACKAGE_MAP", () => {
    it("maps all 5 supported platform+arch combos to package names", async () => {
      const { PLATFORM_PACKAGE_MAP } = await import("./index.mjs");
      assert.equal(PLATFORM_PACKAGE_MAP["darwin-arm64"], "@jasonraimondi/plan-bender-darwin-arm64");
      assert.equal(PLATFORM_PACKAGE_MAP["darwin-x64"], "@jasonraimondi/plan-bender-darwin-x64");
      assert.equal(PLATFORM_PACKAGE_MAP["linux-arm64"], "@jasonraimondi/plan-bender-linux-arm64");
      assert.equal(PLATFORM_PACKAGE_MAP["linux-x64"], "@jasonraimondi/plan-bender-linux-x64");
      assert.equal(PLATFORM_PACKAGE_MAP["win32-x64"], "@jasonraimondi/plan-bender-win32-x64");
      assert.equal(Object.keys(PLATFORM_PACKAGE_MAP).length, 5);
    });
  });

  describe("getPackageNameForPlatform", () => {
    it("returns the correct package name for a supported platform", async () => {
      const { getPackageNameForPlatform } = await import("./index.mjs");
      assert.equal(
        getPackageNameForPlatform("linux", "x64"),
        "@jasonraimondi/plan-bender-linux-x64"
      );
    });

    it("returns null for unsupported platform", async () => {
      const { getPackageNameForPlatform } = await import("./index.mjs");
      assert.equal(getPackageNameForPlatform("freebsd", "x64"), null);
    });

    it("returns null for unsupported arch on supported platform", async () => {
      const { getPackageNameForPlatform } = await import("./index.mjs");
      assert.equal(getPackageNameForPlatform("win32", "arm64"), null);
    });
  });

  describe("getBinaryName", () => {
    it("returns plan-bender.exe for win32", async () => {
      const { getBinaryName } = await import("./index.mjs");
      assert.equal(getBinaryName("win32"), "plan-bender.exe");
    });

    it("returns plan-bender for non-windows", async () => {
      const { getBinaryName } = await import("./index.mjs");
      assert.equal(getBinaryName("linux"), "plan-bender");
      assert.equal(getBinaryName("darwin"), "plan-bender");
    });
  });

  describe("execPath", () => {
    it("is a string when binary is available, null otherwise", async () => {
      const { execPath } = await import("./index.mjs");
      // In dev, neither platform package nor PATH may have the binary
      assert.ok(
        execPath === null || typeof execPath === "string",
        `execPath should be string or null, got ${typeof execPath}`
      );
    });
  });

  describe("exec", () => {
    it("returns {stdout, stderr, exitCode} even when binary is missing", async () => {
      const { exec } = await import("./index.mjs");
      const result = await exec(["--version"]);
      assert.equal(typeof result.stdout, "string");
      assert.equal(typeof result.stderr, "string");
      assert.equal(typeof result.exitCode, "number");
    });

    it("returns an object with stdout, stderr, exitCode when binary exists", async () => {
      const { exec, execPath } = await import("./index.mjs");
      if (!execPath) return; // skip when binary not available
      const result = await exec(["--version"]);
      assert.equal(result.exitCode, 0);
      assert.ok(result.stdout.length > 0);
    });

    it("returns non-zero exitCode for invalid command when binary exists", async () => {
      const { exec, execPath } = await import("./index.mjs");
      if (!execPath) return;
      const result = await exec(["--nonexistent-flag-xyz"]);
      assert.notEqual(result.exitCode, 0);
    });
  });
});
