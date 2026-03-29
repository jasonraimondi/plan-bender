#!/usr/bin/env node

/**
 * publish.mjs — Download GoReleaser binaries from a GitHub Release and publish
 * all npm packages with the matching version.
 *
 * Usage:
 *   node npm/scripts/publish.mjs                  # reads version from GITHUB_REF_NAME
 *   node npm/scripts/publish.mjs --version 0.0.2  # explicit version
 *   node npm/scripts/publish.mjs --dry-run         # skip npm publish
 */

import { execFileSync } from "node:child_process";
import { createWriteStream, mkdirSync, readFileSync, writeFileSync, chmodSync } from "node:fs";
import { pipeline } from "node:stream/promises";
import { join, dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = dirname(fileURLToPath(import.meta.url));
const NPM_DIR = resolve(__dirname, "..");
const REPO = "jasonraimondi/plan-bender";

/**
 * @typedef {{
 *   npmPkg: string,
 *   goOs: string,
 *   goArch: string,
 *   archiveExt: string,
 *   binaryName: string,
 * }} PlatformEntry
 */

/** @type {PlatformEntry[]} */
const PLATFORMS = [
  { npmPkg: "plan-bender-darwin-arm64", goOs: "darwin", goArch: "arm64", archiveExt: "tar.gz", binaryName: "plan-bender" },
  { npmPkg: "plan-bender-darwin-x64", goOs: "darwin", goArch: "amd64", archiveExt: "tar.gz", binaryName: "plan-bender" },
  { npmPkg: "plan-bender-linux-arm64", goOs: "linux", goArch: "arm64", archiveExt: "tar.gz", binaryName: "plan-bender" },
  { npmPkg: "plan-bender-linux-x64", goOs: "linux", goArch: "amd64", archiveExt: "tar.gz", binaryName: "plan-bender" },
  { npmPkg: "plan-bender-win32-x64", goOs: "windows", goArch: "amd64", archiveExt: "zip", binaryName: "plan-bender.exe" },
];

// --- Argument parsing ---

/**
 * Parse version from --version flag or GITHUB_REF_NAME env var.
 * @returns {string}
 */
function getVersion() {
  const args = process.argv.slice(2);
  const versionIdx = args.indexOf("--version");
  if (versionIdx !== -1 && args[versionIdx + 1]) {
    return args[versionIdx + 1].replace(/^v/, "");
  }
  const ref = process.env.GITHUB_REF_NAME;
  if (ref) {
    return ref.replace(/^v/, "");
  }
  console.error("No version provided. Use --version <version> or set GITHUB_REF_NAME.");
  process.exit(1);
}

/** @returns {boolean} */
function isDryRun() {
  return process.argv.includes("--dry-run");
}

// --- Download & extract ---

/**
 * Build the GitHub Release asset download URL.
 * @param {string} version
 * @param {PlatformEntry} platform
 * @returns {string}
 */
function buildDownloadUrl(version, platform) {
  const archiveName = `plan-bender_${version}_${platform.goOs}_${platform.goArch}.${platform.archiveExt}`;
  return `https://github.com/${REPO}/releases/download/v${version}/${archiveName}`;
}

/**
 * Download a file from url to destPath.
 * @param {string} url
 * @param {string} destPath
 */
async function downloadFile(url, destPath) {
  const headers = {};
  if (process.env.GITHUB_TOKEN) {
    headers["Authorization"] = `token ${process.env.GITHUB_TOKEN}`;
  }
  const res = await fetch(url, { headers, redirect: "follow" });
  if (!res.ok) {
    throw new Error(`Failed to download ${url}: ${res.status} ${res.statusText}`);
  }
  mkdirSync(dirname(destPath), { recursive: true });
  const fileStream = createWriteStream(destPath);
  await pipeline(/** @type {any} */ (res.body), fileStream);
}

/**
 * Extract the binary from a tar.gz archive.
 * @param {string} archivePath
 * @param {string} binaryName
 * @param {string} destDir
 */
function extractTarGz(archivePath, binaryName, destDir) {
  mkdirSync(destDir, { recursive: true });
  // Use --wildcards + --strip-components=1 for GNU tar (Linux runners)
  // The archive structure is: <name>_<ver>_<os>_<arch>/<binary>
  execFileSync("tar", [
    "xzf", archivePath,
    "-C", destDir,
    "--strip-components=1",
    "--wildcards",
    `*/${binaryName}`,
  ]);
}

/**
 * Extract the binary from a zip archive.
 * @param {string} archivePath
 * @param {string} binaryName
 * @param {string} destDir
 */
function extractZip(archivePath, binaryName, destDir) {
  mkdirSync(destDir, { recursive: true });
  execFileSync("unzip", ["-o", "-j", archivePath, binaryName, "-d", destDir]);
}

// --- Version management ---

/**
 * Update version in a package.json file.
 * @param {string} pkgJsonPath
 * @param {string} version
 */
function updatePackageVersion(pkgJsonPath, version) {
  const pkg = JSON.parse(readFileSync(pkgJsonPath, "utf8"));
  pkg.version = version;
  if (pkg.optionalDependencies) {
    for (const dep of Object.keys(pkg.optionalDependencies)) {
      pkg.optionalDependencies[dep] = version;
    }
  }
  writeFileSync(pkgJsonPath, JSON.stringify(pkg, null, 2) + "\n");
}

// --- npm publish ---

/**
 * Publish a package. Skips if already published (409/403).
 * @param {string} pkgDir
 * @param {boolean} dryRun
 */
function publishPackage(pkgDir, dryRun) {
  const pkg = JSON.parse(readFileSync(join(pkgDir, "package.json"), "utf8"));
  console.log(`Publishing ${pkg.name}@${pkg.version}...`);

  if (dryRun) {
    console.log(`  [dry-run] Would publish ${pkg.name}@${pkg.version}`);
    return;
  }

  try {
    execFileSync("npm", ["publish", "--access", "public", "--provenance"], {
      cwd: pkgDir,
      stdio: "inherit",
    });
  } catch (e) {
    const stderr = e.stderr?.toString() ?? "";
    // npm returns 403 or "You cannot publish over the previously published versions"
    if (stderr.includes("previously published") || stderr.includes("403")) {
      console.log(`  Skipping ${pkg.name}@${pkg.version} — already published`);
      return;
    }
    throw e;
  }
}

// --- Validation ---

/**
 * Verify the root package's JS can be loaded.
 * @param {string} rootPkgDir
 */
function validateRootPackage(rootPkgDir) {
  console.log("Validating root package loads...");
  execFileSync("node", ["-e", `import("${join(rootPkgDir, "src/index.mjs")}")`], {
    stdio: "inherit",
  });
  console.log("  Root package loaded successfully.");
}

// --- Main ---

async function main() {
  const version = getVersion();
  const dryRun = isDryRun();
  const tmpDir = join(NPM_DIR, ".tmp");

  console.log(`Version: ${version}${dryRun ? " (dry-run)" : ""}`);

  // 1. Download all archives
  console.log("\n--- Downloading archives ---");
  for (const plat of PLATFORMS) {
    const url = buildDownloadUrl(version, plat);
    const archivePath = join(tmpDir, `${plat.npmPkg}.${plat.archiveExt}`);
    console.log(`  ${plat.npmPkg}: ${url}`);
    await downloadFile(url, archivePath);
  }

  // 2. Extract binaries
  console.log("\n--- Extracting binaries ---");
  for (const plat of PLATFORMS) {
    const archivePath = join(tmpDir, `${plat.npmPkg}.${plat.archiveExt}`);
    const destDir = join(NPM_DIR, plat.npmPkg, "bin");
    if (plat.archiveExt === "zip") {
      extractZip(archivePath, plat.binaryName, destDir);
    } else {
      extractTarGz(archivePath, plat.binaryName, destDir);
    }
    // Ensure binary is executable on unix
    if (plat.goOs !== "windows") {
      chmodSync(join(destDir, plat.binaryName), 0o755);
    }
    console.log(`  ${plat.npmPkg}/bin/${plat.binaryName}`);
  }

  // 3. Update versions in all package.json files
  console.log("\n--- Updating versions ---");
  for (const plat of PLATFORMS) {
    const pkgJsonPath = join(NPM_DIR, plat.npmPkg, "package.json");
    updatePackageVersion(pkgJsonPath, version);
    console.log(`  ${plat.npmPkg} → ${version}`);
  }
  const rootPkgJson = join(NPM_DIR, "plan-bender", "package.json");
  updatePackageVersion(rootPkgJson, version);
  console.log(`  plan-bender (root) → ${version}`);

  // 4. Validate root package
  validateRootPackage(join(NPM_DIR, "plan-bender"));

  // 5. Publish platform packages first, then root
  console.log("\n--- Publishing packages ---");
  for (const plat of PLATFORMS) {
    publishPackage(join(NPM_DIR, plat.npmPkg), dryRun);
  }
  publishPackage(join(NPM_DIR, "plan-bender"), dryRun);

  console.log(`\nDone! Published ${PLATFORMS.length + 1} packages at v${version}.`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
