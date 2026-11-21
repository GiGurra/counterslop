// Material registry. A map box names its material; the renderer compiles every
// material here into one shader and picks by index. Add a material: write
// <name>.js next to this file (a GLSL function vec3 mat_<name>(Surf s)) and list it below.
import sand from "./sand.js";
import brick from "./brick.js";
import crate from "./crate.js";
import hazard from "./hazard.js";
import concrete from "./concrete.js";
import carpet from "./carpet.js";
import plaster from "./plaster.js";
import metal from "./metal.js";
import desk from "./desk.js";
import container from "./container.js";
import stucco from "./stucco.js";

export const MATERIALS = [sand, brick, crate, hazard, concrete, carpet, plaster, metal, desk, container, stucco];
