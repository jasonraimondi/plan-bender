import { platform, arch } from "node:os";
import { execFile, execFileSync } from "node:child_process";
import { createRequire } from "node:module";
import { join } from "node:path";

/** @type {Record<string, string>} */
export const PLATFORM_PACKAGE_MAP = {
  "darwin-arm64": "@jasonraimondi/plan-bender-darwin-arm64",
  "darwin-x64": "@jasonraimondi/plan-bender-darwin-x64",
  "linux-arm64": "@jasonraimondi/plan-bender-linux-arm64",
  "linux-x64": "@jasonraimondi/plan-bender-linux-x64",
  "win32-x64": "@jasonraimondi/plan-bender-win32-x64",
};

/**
 * @param {string} os
 * @param {string} cpu
 * @returns {string | null}
 */
export function getPackageNameForPlatform(os, cpu) {
  return PLATFORM_PACKAGE_MAP[`${os}-${cpu}`] ?? null;
}

/**
 * @param {string} os
 * @returns {string}
 */
export function getBinaryName(os) {
  return os === "win32" ? "plan-bender.exe" : "plan-bender";
}

/**
 * Try to resolve the binary from the platform package, then fall back to PATH.
 * @returns {string | null}
 */
function resolveBinaryPath() {
  const os = platform();
  const cpu = arch();
  const pkgName = getPackageNameForPlatform(os, cpu);
  const binaryName = getBinaryName(os);

  // (1) Try platform package
  if (pkgName) {
    try {
      const require = createRequire(import.meta.url);
      const pkgDir = join(require.resolve(`${pkgName}/package.json`), "..");
      return join(pkgDir, "bin", binaryName);
    } catch {
      // Platform package not installed, fall through to PATH
    }
  }

  // (2) PATH fallback
  try {
    const cmd = os === "win32" ? "where" : "which";
    const result = execFileSync(cmd, ["plan-bender"], { encoding: "utf8" });
    const resolved = result.trim().split("\n")[0];
    if (resolved) return resolved;
  } catch {
    // Not on PATH
  }

  // (3) Unsupported / not found
  if (!pkgName) {
    printUnsupportedPlatformError(os, cpu);
  }

  return null;
}

/**
 * @param {string} os
 * @param {string} cpu
 */
function printUnsupportedPlatformError(os, cpu) {
  console.error(
    `plan-bender does not have a prebuilt binary for ${os}/${cpu}.\n\n` +
      `Install alternatives:\n` +
      `  brew install jasonraimondi/tap/plan-bender\n` +
      `  go install github.com/jasonraimondi/plan-bender/cmd/plan-bender@latest\n` +
      `  Download from https://github.com/jasonraimondi/plan-bender/releases\n`
  );
}

/** @type {string | null} */
export const execPath = resolveBinaryPath();

/**
 * @typedef {{ stdout: string, stderr: string, exitCode: number }} ExecResult
 */

/**
 * Spawn the plan-bender binary with the given arguments.
 * @param {string[]} args
 * @returns {Promise<ExecResult>}
 */
export function exec(args) {
  if (!execPath) {
    return Promise.resolve({
      stdout: "",
      stderr: `plan-bender binary not found for ${platform()}/${arch()}`,
      exitCode: 1,
    });
  }

  return new Promise((resolve) => {
    execFile(execPath, args, { encoding: "utf8" }, (error, stdout, stderr) => {
      resolve({
        stdout: stdout ?? "",
        stderr: stderr ?? "",
        exitCode: error ? error.code ?? 1 : 0,
      });
    });
  });
}
