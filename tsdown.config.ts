import { defineConfig } from "tsdown";

export default defineConfig({
  entry: { cli: "src/cli/index.ts" },
  format: "esm",
  target: "node22",
  outDir: "dist",
  clean: true,
  banner: "#!/usr/bin/env node",
});
