import { test } from "node:test";
import assert from "node:assert/strict";
import { isSoftwareGpu } from "./gpuclass.js";

test("isSoftwareGpu: CPU rasterisers yes, real GPUs no", () => {
  for (const s of [
    "ANGLE (Google, Vulkan 1.3.0 (SwiftShader Device (Subzero) (0x0000C0DE)), SwiftShader driver)",
    "Google SwiftShader", "llvmpipe (LLVM 15.0.7, 256 bits)", "Mesa softpipe",
    "ANGLE (Microsoft, Microsoft Basic Render Driver Direct3D11 vs_5_0 ps_5_0, D3D11)",
  ]) assert.ok(isSoftwareGpu(s), s);
  for (const s of [
    "ANGLE (NVIDIA, NVIDIA GeForce RTX 3070 (0x00002484) Direct3D11 vs_5_0 ps_5_0, D3D11)",
    "ANGLE (Intel, Intel(R) UHD Graphics 620 Direct3D11 vs_5_0 ps_5_0, D3D11)",
    "Apple M1", "Mali-G78", "", null,
  ]) assert.ok(!isSoftwareGpu(s), String(s));
});
