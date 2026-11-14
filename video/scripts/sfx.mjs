import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const SR = 48000;
const OUT = join(dirname(fileURLToPath(import.meta.url)), "..", "public", "sfx");
mkdirSync(OUT, { recursive: true });

let seed = 0x9e3779b9;
function rand() {
  seed ^= seed << 13; seed >>>= 0;
  seed ^= seed >>> 17;
  seed ^= seed << 5; seed >>>= 0;
  return seed / 4294967296;
}
const noise = () => rand() * 2 - 1;
const buf = (sec) => new Float32Array(Math.round(sec * SR));
const TAU = Math.PI * 2;

function mixInto(dst, src, at = 0, gain = 1) {
  const o = Math.round(at * SR);
  for (let i = 0; i < src.length && o + i < dst.length; i++) if (o + i >= 0) dst[o + i] += src[i] * gain;
  return dst;
}

function biquad(x, type, freq, q = 0.707) {
  const y = new Float32Array(x.length);
  let x1 = 0, x2 = 0, y1 = 0, y2 = 0;
  for (let i = 0; i < x.length; i++) {
    const t = i / SR;
    const f = Math.min(typeof freq === "function" ? freq(t) : freq, SR * 0.45);
    const Q = typeof q === "function" ? q(t) : q;
    const w = (TAU * f) / SR, cw = Math.cos(w), sw = Math.sin(w), a = sw / (2 * Q);
    let b0, b1, b2;
    if (type === "lp") { b0 = (1 - cw) / 2; b1 = 1 - cw; b2 = (1 - cw) / 2; }
    else if (type === "hp") { b0 = (1 + cw) / 2; b1 = -(1 + cw); b2 = (1 + cw) / 2; }
    else { b0 = a; b1 = 0; b2 = -a; }
    const a0 = 1 + a, a1 = -2 * cw, a2 = 1 - a;
    const v = (b0 * x[i] + b1 * x1 + b2 * x2 - a1 * y1 - a2 * y2) / a0;
    x2 = x1; x1 = x[i]; y2 = y1; y1 = v; y[i] = v;
  }
  return y;
}

function env(x, f) {
  for (let i = 0; i < x.length; i++) x[i] *= f(i / SR);
  return x;
}
const decay = (tau) => (t) => Math.exp(-t / tau);
const ad = (attack, tau) => (t) => (t < attack ? t / attack : Math.exp(-(t - attack) / tau));

function tone(sec, f, shape = Math.sin) {
  const x = buf(sec);
  let ph = 0;
  for (let i = 0; i < x.length; i++) {
    ph += (TAU * (typeof f === "function" ? f(i / SR) : f)) / SR;
    x[i] = shape(ph);
  }
  return x;
}
const white = (sec) => { const x = buf(sec); for (let i = 0; i < x.length; i++) x[i] = noise(); return x; };

function room(x, wet = 0.25, size = 1, tail = 1.2) {
  const n = x.length + Math.round(tail * SR);
  const dry = new Float32Array(n); dry.set(x);
  const out = new Float32Array(n);
  const combs = [1557, 1617, 1491, 1422].map((d) => Math.round(d * size * (SR / 44100)));
  for (const d of combs) {
    const line = new Float32Array(d); let p = 0, lp = 0;
    for (let i = 0; i < n; i++) {
      const o = line[p];
      lp = o * 0.8 + lp * 0.2;
      line[p] = dry[i] + lp * 0.82;
      p = (p + 1) % d;
      out[i] += o / combs.length;
    }
  }
  for (const d of [225, 556].map((d) => Math.round(d * (SR / 44100)))) {
    const line = new Float32Array(d); let p = 0;
    for (let i = 0; i < n; i++) {
      const b = line[p], v = out[i];
      line[p] = v + b * 0.5; out[i] = b - v * 0.5; p = (p + 1) % d;
    }
  }
  for (let i = 0; i < n; i++) out[i] = dry[i] + out[i] * wet;
  return out;
}

function fadeOut(x, sec = 0.01) {
  const n = Math.min(x.length, Math.round(sec * SR));
  for (let i = 0; i < n; i++) x[x.length - 1 - i] *= i / n;
  return x;
}

function write(name, x, peak = 0.89) {
  let m = 0; for (const v of x) m = Math.max(m, Math.abs(v));
  const g = m > 0 ? peak / m : 1;
  fadeOut(x, 0.005);
  const data = Buffer.alloc(44 + x.length * 2);
  data.write("RIFF", 0); data.writeUInt32LE(36 + x.length * 2, 4); data.write("WAVE", 8);
  data.write("fmt ", 12); data.writeUInt32LE(16, 16); data.writeUInt16LE(1, 20); data.writeUInt16LE(1, 22);
  data.writeUInt32LE(SR, 24); data.writeUInt32LE(SR * 2, 28); data.writeUInt16LE(2, 32); data.writeUInt16LE(16, 34);
  data.write("data", 36); data.writeUInt32LE(x.length * 2, 40);
  for (let i = 0; i < x.length; i++) data.writeInt16LE(Math.max(-32767, Math.min(32767, Math.round(x[i] * g * 32767))), 44 + i * 2);
  writeFileSync(join(OUT, `${name}.wav`), data);
  console.log(`${name}.wav  ${(x.length / SR).toFixed(2)}s`);
}

function key(pitch = 1, weight = 1) {
  const x = buf(0.12);
  const click = env(biquad(white(0.03), "bp", 4200 * pitch, 1.4), ad(0.0006, 0.004));
  const tick2 = env(biquad(white(0.02), "bp", 2600 * pitch, 2), ad(0.0005, 0.003));
  const thock = env(tone(0.08, (t) => (230 * pitch) * (1 - t * 2.2)), ad(0.001, 0.016 * weight));
  const body = env(biquad(white(0.06), "lp", 900 * pitch), ad(0.001, 0.012 * weight));
  mixInto(x, click, 0, 0.7);
  mixInto(x, thock, 0.002, 0.9 * weight);
  mixInto(x, body, 0.002, 0.5 * weight);
  mixInto(x, tick2, 0.011 + rand() * 0.006, 0.25);
  return x;
}
for (let i = 0; i < 5; i++) write(`key${i + 1}`, key(0.9 + rand() * 0.22), 0.7);
write("enter", room(key(0.78, 1.7), 0.12, 0.6, 0.3), 0.85);

function whoosh(sec, from, peakF, to, at = 0.45, q = 1.1) {
  const n = white(sec);
  const f = (t) => {
    const u = t / sec;
    return u < at ? from * Math.pow(peakF / from, u / at) : peakF * Math.pow(to / peakF, (u - at) / (1 - at));
  };
  const band = biquad(biquad(n, "bp", f, q), "bp", f, q * 0.8);
  const rumble = biquad(white(sec), "lp", 260);
  const shape = (t) => { const u = t / sec; return Math.pow(Math.sin(Math.PI * Math.min(1, u < at ? (u / at) * 0.5 : 0.5 + ((u - at) / (1 - at)) * 0.5)), 2); };
  const x = buf(sec);
  mixInto(x, env(band, shape), 0, 1);
  mixInto(x, env(rumble, shape), 0, 0.35);
  return x;
}
write("whoosh", room(whoosh(0.6, 300, 3200, 700, 0.42), 0.18, 0.8, 0.4));
write("swipe", whoosh(0.28, 900, 5200, 1800, 0.35, 0.9), 0.75);

function zip() {
  const sec = 0.5;
  const x = buf(sec + 0.3);
  const sweep = (t) => 520 * Math.pow(6, Math.min(1, t / sec));
  const fm = tone(sec, (t) => sweep(t) * (1 + 0.04 * Math.sin(TAU * 34 * t)));
  mixInto(x, env(fm, (t) => Math.sin(Math.PI * Math.min(1, t / sec)) * 0.6), 0, 0.5);
  mixInto(x, env(biquad(white(sec), "bp", (t) => sweep(t) * 2.2, 2.5), (t) => Math.pow(t / sec, 1.5)), 0, 0.8);
  mixInto(x, env(tone(0.25, 3950), decay(0.05)), sec - 0.03, 0.3);
  return room(x, 0.2, 0.7, 0.4);
}
write("zip", zip(), 0.8);

function bell(f0 = 1318, len = 2.6) {
  const x = buf(len);
  const parts = [
    [1.0, 1.0, 1.9], [2.32, 0.55, 1.0], [3.91, 0.34, 0.62], [5.43, 0.2, 0.4], [7.05, 0.11, 0.26], [0.5, 0.12, 1.2],
  ];
  for (const [r, a, tau] of parts) {
    for (const det of [-1.3, 1.3]) {
      const p = env(tone(len, f0 * r + det * r), ad(0.0015, tau));
      mixInto(x, p, 0, a * 0.5);
    }
  }
  mixInto(x, env(biquad(white(0.02), "hp", 3000), decay(0.002)), 0, 0.4);
  return room(x, 0.32, 1.1, 1.4);
}
write("bell", bell(), 0.85);
write("bell_low", bell(988, 3.0), 0.85);

function stamp() {
  const x = buf(0.6);
  mixInto(x, env(tone(0.4, (t) => 46 + 90 * Math.exp(-t / 0.03)), ad(0.001, 0.09)), 0, 1.0);
  mixInto(x, env(biquad(white(0.2), "lp", 1400), ad(0.0008, 0.025)), 0, 0.9);
  mixInto(x, env(biquad(white(0.05), "bp", 2500, 1.5), ad(0.0005, 0.006)), 0.004, 0.35);
  for (let i = 0; i < x.length; i++) x[i] = Math.tanh(x[i] * 1.6);
  return room(x, 0.14, 0.6, 0.35);
}
write("stamp", stamp(), 0.95);

function tap() {
  const x = buf(0.08);
  mixInto(x, env(tone(0.05, 1650), ad(0.0005, 0.006)), 0, 0.5);
  mixInto(x, env(biquad(white(0.04), "bp", 5200, 1.2), ad(0.0003, 0.003)), 0, 0.6);
  mixInto(x, env(biquad(white(0.05), "lp", 700), ad(0.0008, 0.01)), 0, 0.4);
  return x;
}
write("tap", tap(), 0.7);

function pop() {
  const x = buf(0.35);
  const f = (t) => 520 + 520 * Math.min(1, t / 0.05);
  mixInto(x, env(tone(0.3, f), ad(0.002, 0.07)), 0, 0.8);
  mixInto(x, env(tone(0.3, (t) => f(t) * 2), ad(0.002, 0.04)), 0, 0.25);
  return room(x, 0.2, 0.6, 0.3);
}
write("pop", pop(), 0.8);

function sparkle(count = 11, sec = 0.7) {
  const x = buf(sec + 0.4);
  for (let i = 0; i < count; i++) {
    const at = (i / count) * sec * (0.8 + rand() * 0.3);
    const f = 3000 + (i / count) * 4200 + rand() * 1500;
    mixInto(x, env(tone(0.3, f), ad(0.001, 0.04 + rand() * 0.06)), at, 0.25 + rand() * 0.25);
  }
  return room(x, 0.4, 0.9, 0.7);
}
write("sparkle", sparkle(), 0.6);

function lift() {
  const x = buf(0.9);
  mixInto(x, whoosh(0.5, 700, 4800, 2600, 0.6, 1.4), 0, 0.8);
  for (const [r, a] of [[1, 1], [2.71, 0.5], [4.2, 0.25]]) mixInto(x, env(tone(0.6, 2350 * r), ad(0.002, 0.12)), 0.26, 0.18 * a);
  return room(x, 0.3, 0.9, 0.6);
}
write("lift", lift(), 0.8);

function riser(sec = 1.1) {
  const x = buf(sec);
  const f = (t) => 300 * Math.pow(20, t / sec);
  mixInto(x, env(biquad(white(sec), "bp", f, 1.3), (t) => Math.pow(t / sec, 2.2)), 0, 1);
  mixInto(x, env(tone(sec, (t) => 140 * Math.pow(4, t / sec)), (t) => Math.pow(t / sec, 3) * 0.5), 0, 0.5);
  return x;
}
write("riser", riser(), 0.7);

function hit() {
  const x = buf(1.6);
  mixInto(x, env(tone(1.5, (t) => 36 + 60 * Math.exp(-t / 0.06)), ad(0.002, 0.45)), 0, 1);
  mixInto(x, env(biquad(white(0.6), "lp", 420), ad(0.001, 0.08)), 0, 0.6);
  mixInto(x, env(biquad(white(0.3), "bp", 1800, 0.8), ad(0.001, 0.03)), 0, 0.25);
  for (let i = 0; i < x.length; i++) x[i] = Math.tanh(x[i] * 1.4);
  return room(x, 0.25, 1.2, 1.0);
}
write("hit", hit(), 0.95);

function pen(sec = 1.6) {
  const x = buf(sec);
  const scratch = biquad(biquad(white(sec), "bp", 2900, 1.8), "hp", 1200);
  let g = 0.5;
  for (let i = 0; i < x.length; i++) {
    if (i % 240 === 0) g = 0.35 + rand() * 0.65;
    const t = i / SR, edge = Math.min(1, t / 0.04, (sec - t) / 0.06);
    x[i] = scratch[i] * g * edge;
  }
  return x;
}
write("pen", pen(), 0.45);
write("pen_short", pen(0.55), 0.45);

function tick() {
  const x = buf(0.05);
  mixInto(x, env(tone(0.04, 2300), ad(0.0004, 0.005)), 0, 0.8);
  mixInto(x, env(biquad(white(0.02), "hp", 4000), ad(0.0003, 0.002)), 0, 0.4);
  return x;
}
write("tick", tick(), 0.6);

function knock() {
  const x = buf(0.3);
  for (const [at, g] of [[0, 1], [0.075, 0.55]]) {
    mixInto(x, env(tone(0.12, (t) => 160 + 120 * Math.exp(-t / 0.01)), ad(0.001, 0.03)), at, g);
    mixInto(x, env(biquad(white(0.05), "lp", 1800), ad(0.0005, 0.008)), at, g * 0.5);
  }
  return room(x, 0.1, 0.5, 0.2);
}
write("knock", knock(), 0.75);
