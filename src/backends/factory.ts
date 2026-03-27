// Auto-register all backends by importing them (they call registerBackend)
import "./yaml-fs.js";
import "./linear.js";

export { registerBackend, createBackend } from "./registry.js";
