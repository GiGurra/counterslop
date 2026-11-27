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
import rack from "./rack.js";
import awning from "./awning.js";
import lamp from "./lamp.js";
// the hardware superstore (gs_ maps)
import storetile from "./storetile.js";
import shelf from "./shelf.js";
import carton from "./carton.js";
import checkout from "./checkout.js";
import screen from "./screen.js";
import pctower from "./pctower.js";
import escalator from "./escalator.js";
import glass from "./glass.js";
import handrail from "./handrail.js";
import neon from "./neon.js";
import lancarpet from "./lancarpet.js";
import car from "./car.js";
// surf_scope_creep
import gantt from "./gantt.js";
import deprecated from "./deprecated.js";
// lab_harbor
import water from "./water.js";
// de_rackmount
import pcb from "./pcb.js";
import ram from "./ram.js";
import fan from "./fan.js";
import cable from "./cable.js";
// he_merge_hell
import diff from "./diff.js";
import lgtm from "./lgtm.js";
// Star Slop (lab_moon)
import regolith from "./regolith.js";
// Slop n Roll Racing (rc_ci_pipeline)
import tarmac from "./tarmac.js";
import curb from "./curb.js";
import datasea from "./datasea.js";
import racegate from "./racegate.js";
import chevron from "./chevron.js";
import chevronr from "./chevronr.js";
// rc_merge_train
import conflict from "./conflict.js";
import conflictsign from "./conflictsign.js";
// rc_hotfix_highway
import highway from "./highway.js";
import nightcity from "./nightcity.js";
import exitsign from "./exitsign.js";
// de_serverless
import lambda from "./lambda.js";
import jumppad from "./jumppad.js";
import accelpad from "./accelpad.js";
import accelpadr from "./accelpadr.js";
// cq_moonshot: signs (content.Sign), departure boards, posters
import led from "./led.js";
import ink from "./ink.js";
import flap from "./flap.js";
import poster from "./poster.js";
// cq_moonshot's monument and Earth's banners
import bronze from "./bronze.js";
import crest from "./crest.js";

export const MATERIALS = [sand, brick, crate, hazard, concrete, carpet, plaster, metal, desk, container, stucco, rack, awning, lamp,
  storetile, shelf, carton, checkout, screen, pctower, escalator, glass, handrail, neon, lancarpet, car,
  gantt, deprecated, water, pcb, ram, fan, cable, diff, lgtm, regolith,
  tarmac, curb, datasea, racegate, chevron, chevronr, led, ink, flap, poster,
  conflict, conflictsign, highway, nightcity, exitsign, lambda, jumppad, accelpad, accelpadr, bronze, crest];
