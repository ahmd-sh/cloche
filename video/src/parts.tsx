import React from "react";
import {
  AbsoluteFill,
  Audio,
  Easing,
  Img,
  Sequence,
  continueRender,
  delayRender,
  interpolate,
  spring,
  staticFile,
  useCurrentFrame,
  useVideoConfig,
} from "remotion";

export const ink = {
  paper: "#0F3D6E",
  deep: "#082A4D",
  line: "#DCEBFF",
  text: "#F2F7FF",
  dim: "#ADC6E5",
  faint: "#7598C2",
  live: "#9FF0D5",
  redline: "#FF8F7E",
};

export const mono = "Plex, ui-monospace, Menlo, monospace";

const fonts = delayRender("fonts");
Promise.all(
  [
    ["400", "plex_mono_regular.ttf"],
    ["500", "plex_mono_medium.ttf"],
    ["600", "plex_mono_semibold.ttf"],
  ].map(([weight, file]) =>
    new FontFace("Plex", `url(${staticFile(`fonts/${file}`)})`, { weight }).load().then((f) => document.fonts.add(f)),
  ),
).then(() => continueRender(fonts));

export const clamp = { extrapolateLeft: "clamp", extrapolateRight: "clamp" } as const;
export const ease = Easing.bezier(0.2, 0, 0, 1);
export const easeIn = Easing.bezier(0.5, 0, 0.9, 0.4);

export function prog(frame: number, from: number, dur: number, e = ease) {
  return interpolate(frame, [from, from + dur], [0, 1], { ...clamp, easing: e });
}

export function pop(frame: number, fps: number, from: number, cfg: { damping?: number; stiffness?: number; mass?: number } = {}) {
  return spring({ frame: frame - from, fps, config: { damping: 14, stiffness: 180, mass: 0.9, ...cfg } });
}

export const Sfx: React.FC<{ at: number; src: string; volume?: number; rate?: number }> = ({ at, src, volume = 1, rate = 1 }) => (
  <Sequence from={Math.round(at)} layout="none">
    <Audio src={staticFile(`sfx/${src}.wav`)} volume={volume} playbackRate={rate} />
  </Sequence>
);

export const Keys: React.FC<{ at: number; count: number; every: number; volume?: number }> = ({ at, count, every, volume = 0.5 }) => (
  <>
    {Array.from({ length: count }, (_, i) => (
      <Sfx key={i} at={at + i * every} src={`key${(i * 7) % 5 + 1}`} volume={volume * (0.75 + ((i * 13) % 5) / 16)} />
    ))}
  </>
);

export const Grid: React.FC<{ cell?: number; x?: number; y?: number; opacity?: number; paper?: string }> = ({ cell = 30, x = 0, y = 0, opacity = 1, paper = ink.paper }) => {
  const major = cell * 5;
  const bg = [
    `linear-gradient(rgba(255,255,255,0.14) 1.5px, transparent 1.5px)`,
    `linear-gradient(90deg, rgba(255,255,255,0.14) 1.5px, transparent 1.5px)`,
    `linear-gradient(rgba(255,255,255,0.06) 1px, transparent 1px)`,
    `linear-gradient(90deg, rgba(255,255,255,0.06) 1px, transparent 1px)`,
  ].join(",");
  return (
    <AbsoluteFill
      style={{
        backgroundColor: paper,
        backgroundImage: bg,
        backgroundSize: `${major}px ${major}px, ${major}px ${major}px, ${cell}px ${cell}px, ${cell}px ${cell}px`,
        backgroundPosition: `${x}px ${y}px`,
        opacity,
      }}
    />
  );
};

export const Vignette: React.FC<{ strength?: number }> = ({ strength = 0.55 }) => (
  <AbsoluteFill style={{ background: `radial-gradient(ellipse at 50% 45%, rgba(0,0,0,0) 45%, rgba(3,18,38,${strength}) 100%)` }} />
);

export const Grain: React.FC = () => {
  const frame = useCurrentFrame();
  return (
    <AbsoluteFill style={{ mixBlendMode: "overlay", opacity: 0.16, pointerEvents: "none" }}>
      <svg width="100%" height="100%">
        <filter id="grain">
          <feTurbulence type="fractalNoise" baseFrequency="0.9" numOctaves="2" seed={frame % 60} stitchTiles="stitch" />
          <feColorMatrix type="saturate" values="0" />
        </filter>
        <rect width="100%" height="100%" filter="url(#grain)" />
      </svg>
    </AbsoluteFill>
  );
};

export const T: React.FC<{
  children: React.ReactNode;
  size: number;
  weight?: number;
  color?: string;
  track?: number;
  style?: React.CSSProperties;
}> = ({ children, size, weight = 500, color = ink.text, track = 0, style }) => (
  <div style={{ fontFamily: mono, fontSize: size, fontWeight: weight, color, letterSpacing: `${track}em`, lineHeight: 1.12, whiteSpace: "pre", ...style }}>
    {children}
  </div>
);

export const Rise: React.FC<{
  text: string;
  at: number;
  size: number;
  stagger?: number;
  weight?: number;
  color?: string;
  accent?: string[];
  track?: number;
  align?: "left" | "center";
}> = ({ text, at, size, stagger = 3, weight = 600, color = ink.text, accent = [], track = -0.02, align = "center" }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const words = text.split(" ");
  return (
    <div style={{ display: "flex", flexWrap: "wrap", justifyContent: align === "center" ? "center" : "flex-start", columnGap: size * 0.42 }}>
      {words.map((w, i) => {
        const s = pop(frame, fps, at + i * stagger, { damping: 16, stiffness: 210 });
        return (
          <div key={i} style={{ overflow: "hidden", paddingBottom: size * 0.08 }}>
            <div
              style={{
                fontFamily: mono,
                fontSize: size,
                fontWeight: weight,
                letterSpacing: `${track}em`,
                color: accent.includes(w) || accent.includes(w.replace(/[.,]/g, "")) ? ink.live : color,
                transform: `translateY(${(1 - s) * 110}%) rotate(${(1 - s) * 6}deg)`,
                opacity: interpolate(s, [0, 0.3], [0, 1], clamp),
                lineHeight: 1.05,
              }}
            >
              {w}
            </div>
          </div>
        );
      })}
    </div>
  );
};

export const Phone: React.FC<{ w: number; children?: React.ReactNode; style?: React.CSSProperties; glow?: number }> = ({ w, children, style, glow = 0 }) => {
  const bez = w * 0.032;
  const sw = w - bez * 2;
  const sh = sw * (2856 / 1280);
  const k = sw / 1280;
  return (
    <div
      style={{
        position: "absolute",
        width: w,
        height: sh + bez * 2,
        borderRadius: w * 0.13,
        background: "linear-gradient(145deg, #2a3442, #0a0e14 40%, #161d27)",
        boxShadow: `0 ${w * 0.08}px ${w * 0.22}px rgba(2,10,24,0.55), 0 0 0 ${w * 0.004}px rgba(220,235,255,0.35), 0 0 ${w * 0.25 * glow}px rgba(159,240,213,${0.45 * glow})`,
        ...style,
      }}
    >
      <div style={{ position: "absolute", left: bez, top: bez, width: sw, height: sh, borderRadius: w * 0.1, overflow: "hidden", background: ink.paper }}>
        <div style={{ position: "absolute", left: 0, top: 0, width: 1280, height: 2856, transform: `scale(${k})`, transformOrigin: "0 0" }}>{children}</div>
      </div>
    </div>
  );
};

export const Shot: React.FC<{ src: string; opacity?: number; style?: React.CSSProperties }> = ({ src, opacity = 1, style }) => (
  <Img src={staticFile(`shots/${src}.png`)} style={{ position: "absolute", left: 0, top: 0, width: 1280, height: 2856, opacity, ...style }} />
);

export const Tap: React.FC<{ at: number; x: number; y: number; size?: number }> = ({ at, x, y, size = 160 }) => {
  const frame = useCurrentFrame();
  const t = (frame - at) / 14;
  if (t < -0.6 || t > 1) return null;
  const press = interpolate(t, [-0.6, 0, 0.25, 1], [0.6, 1, 0.85, 0.85], clamp);
  const ring = interpolate(t, [0, 1], [0.3, 1.6], clamp);
  return (
    <>
      <div
        style={{
          position: "absolute", left: x - size / 2, top: y - size / 2, width: size, height: size, borderRadius: "50%",
          background: "rgba(255,255,255,0.55)", transform: `scale(${press})`,
          opacity: interpolate(t, [-0.6, -0.3, 0.4, 1], [0, 0.85, 0.7, 0], clamp),
        }}
      />
      <div
        style={{
          position: "absolute", left: x - size / 2, top: y - size / 2, width: size, height: size, borderRadius: "50%",
          border: `${size * 0.04}px solid rgba(255,255,255,0.9)`, transform: `scale(${ring})`,
          opacity: interpolate(t, [0, 1], [0.9, 0], clamp),
        }}
      />
    </>
  );
};

export const Cloche: React.FC<{ size: number; lift?: number; tilt?: number; style?: React.CSSProperties }> = ({ size, lift = 0, tilt = 0, style }) => (
  <div style={{ position: "relative", width: size, height: size, ...style }}>
    <Img src={staticFile("cloche_base.svg")} style={{ position: "absolute", inset: 0, width: size, height: size }} />
    <Img
      src={staticFile("cloche_lid.svg")}
      style={{
        position: "absolute", inset: 0, width: size, height: size,
        transform: `translate(${lift * size * 0.06}px, ${-lift * size * 0.32}px) rotate(${tilt + lift * 14}deg)`,
        transformOrigin: "50% 57%",
      }}
    />
  </div>
);

export const Glint: React.FC<{ x: number; y: number; r: number; t: number; color?: string }> = ({ x, y, r, t, color = "#fff" }) => {
  if (t <= 0 || t >= 1) return null;
  const s = Math.sin(Math.PI * t);
  const d = `M0,${-r}Q0,0 ${r},0Q0,0 0,${r}Q0,0 ${-r},0Q0,0 0,${-r}z`;
  return (
    <svg style={{ position: "absolute", left: x - r, top: y - r, overflow: "visible" }} width={r * 2} height={r * 2}>
      <path d={d} fill={color} transform={`translate(${r},${r}) scale(${s}) rotate(${t * 90})`} />
    </svg>
  );
};

export const Stamp: React.FC<{ at: number; size: number; text?: string; rotate?: number; color?: string }> = ({ at, size, text = "INSTALLED", rotate = -7, color = ink.live }) => {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  if (frame < at - 4) return null;
  const down = interpolate(frame, [at - 4, at], [2.4, 1], { ...clamp, easing: easeIn });
  const settle = spring({ frame: frame - at, fps, config: { damping: 9, stiffness: 260 } });
  const k = frame < at ? down : 1 + (1 - settle) * -0.06;
  return (
    <div
      style={{
        transform: `scale(${k}) rotate(${rotate}deg)`,
        opacity: interpolate(frame, [at - 4, at - 1], [0, 1], clamp),
        border: `${size * 0.09}px solid ${color}`,
        padding: `${size * 0.12}px ${size * 0.4}px`,
        borderRadius: size * 0.1,
        boxShadow: `inset 0 0 0 ${size * 0.05}px ${ink.paper}, inset 0 0 0 ${size * 0.1}px ${color}`,
        background: "rgba(15,61,110,0.35)",
      }}
    >
      <T size={size} weight={600} color={color} track={0.16}>
        {text}
      </T>
    </div>
  );
};

export const Dim: React.FC<{ x1: number; x2: number; y: number; at: number; dur?: number; label?: string; size?: number; color?: string; dash?: boolean; flow?: number }> = ({
  x1, x2, y, at, dur = 18, label, size = 22, color = ink.line, dash = false, flow = 0,
}) => {
  const frame = useCurrentFrame();
  const t = prog(frame, at, dur);
  const len = x2 - x1;
  const mid = (x1 + x2) / 2;
  const gap = label ? label.length * size * 0.66 + size : 0;
  const a = 12;
  return (
    <>
      <svg style={{ position: "absolute", left: 0, top: 0, overflow: "visible" }} width={1} height={1}>
        <g stroke={color} strokeWidth={2.2} fill="none" opacity={t > 0 ? 1 : 0}>
          <line x1={x1} y1={y - 22} x2={x1} y2={y - 22 + 44 * t} />
          <line x1={x2} y1={y - 22} x2={x2} y2={y - 22 + 44 * t} />
          <line x1={mid - (len / 2) * t} y1={y} x2={mid - gap / 2} y2={y} strokeDasharray={dash ? "10 9" : undefined} strokeDashoffset={-flow} opacity={t * len / 2 > gap / 2 ? 1 : 0} />
          <line x1={mid + gap / 2} y1={y} x2={mid + (len / 2) * t} y2={y} strokeDasharray={dash ? "10 9" : undefined} strokeDashoffset={flow} opacity={t * len / 2 > gap / 2 ? 1 : 0} />
          {t > 0.98 && (
            <>
              <path d={`M${x1 + a},${y - a * 0.45}L${x1},${y}L${x1 + a},${y + a * 0.45}`} />
              <path d={`M${x2 - a},${y - a * 0.45}L${x2},${y}L${x2 - a},${y + a * 0.45}`} />
            </>
          )}
        </g>
      </svg>
      {label && (
        <div style={{ position: "absolute", left: mid - gap / 2, top: y - size * 0.62, width: gap, textAlign: "center", opacity: prog(frame, at + dur * 0.6, 8) }}>
          <T size={size} weight={500} color={color} track={0.14} style={{ display: "inline-block" }}>
            {label}
          </T>
        </div>
      )}
    </>
  );
};

export const Plot: React.FC<{ d: string; at: number; dur: number; width?: number; color?: string; length?: number; fill?: string; dash?: string }> = ({
  d, at, dur, width = 2.4, color = ink.line, length = 4000, fill = "none", dash,
}) => {
  const frame = useCurrentFrame();
  const t = prog(frame, at, dur, Easing.bezier(0.45, 0, 0.2, 1));
  if (t <= 0) return null;
  return (
    <path
      d={d}
      fill={fill}
      stroke={color}
      strokeWidth={width}
      strokeLinecap="round"
      strokeLinejoin="round"
      strokeDasharray={dash ?? `${length}`}
      strokeDashoffset={dash ? undefined : length * (1 - t)}
      opacity={dash ? t : 1}
    />
  );
};

export const Corners: React.FC<{ w: number; h: number; m?: number; color?: string; width?: number; t?: number }> = ({ w, h, m = 18, color = ink.line, width = 2.4, t = 1 }) => {
  const k = m * t;
  return (
    <svg style={{ position: "absolute", left: 0, top: 0, overflow: "visible" }} width={w} height={h}>
      <g stroke={color} strokeWidth={width} fill="none" opacity={t > 0 ? 1 : 0}>
        <path d={`M${-k},0H${k}M0,${-k}V${k}`} />
        <path d={`M${w - k},0H${w + k}M${w},${-k}V${k}`} />
        <path d={`M${-k},${h}H${k}M0,${h - k}V${h + k}`} />
        <path d={`M${w - k},${h}H${w + k}M${w},${h - k}V${h + k}`} />
      </g>
    </svg>
  );
};

export function typed(frame: number, text: string, at: number, every: number) {
  const n = Math.max(0, Math.min(text.length, Math.floor((frame - at) / every) + 1));
  return frame < at ? "" : text.slice(0, n);
}

export const Caret: React.FC<{ size: number; color?: string; solid?: boolean }> = ({ size, color = ink.live, solid }) => {
  const frame = useCurrentFrame();
  const on = solid || Math.floor(frame / 8) % 2 === 0;
  return <span style={{ display: "inline-block", width: size * 0.58, height: size * 1.05, background: color, opacity: on ? 1 : 0, verticalAlign: "-0.16em", marginLeft: 2 }} />;
};

export const Laptop: React.FC<{ w: number; at: number; color?: string }> = ({ w, at, color = ink.line }) => {
  const h = w * 0.62;
  const sx = w * 0.12, sw = w * 0.76, sh = h * 0.78;
  return (
    <svg width={w} height={h} style={{ overflow: "visible" }}>
      <Plot d={`M${sx},${sh}V${h * 0.04}H${sx + sw}V${sh}`} at={at} dur={16} color={color} length={w * 2.4} width={3} />
      <Plot d={`M${sx + w * 0.03},${sh - w * 0.03}V${h * 0.09}H${sx + sw - w * 0.03}V${sh - w * 0.03}Z`} at={at + 8} dur={14} color={color} length={w * 2.2} width={1.6} />
      <Plot d={`M0,${h * 0.9}L${sx},${sh}H${sx + sw}L${w},${h * 0.9}Z`} at={at + 12} dur={14} color={color} length={w * 2.6} width={3} />
    </svg>
  );
};

export const PhoneLine: React.FC<{ w: number; at: number; color?: string }> = ({ w, at, color = ink.line }) => {
  const h = w * 1.95;
  const r = w * 0.16;
  return (
    <svg width={w} height={h} style={{ overflow: "visible" }}>
      <Plot d={`M${r},0H${w - r}Q${w},0 ${w},${r}V${h - r}Q${w},${h} ${w - r},${h}H${r}Q0,${h} 0,${h - r}V${r}Q0,0 ${r},0Z`} at={at} dur={20} color={color} length={w * 6.5} width={3} />
      <Plot d={`M${w * 0.38},${w * 0.1}H${w * 0.62}`} at={at + 16} dur={6} color={color} length={w} width={3} />
    </svg>
  );
};

export const Flash: React.FC<{ at: number; strength?: number; color?: string }> = ({ at, strength = 0.22, color = "#ffffff" }) => {
  const frame = useCurrentFrame();
  const t = frame - at;
  if (t < 0 || t > 8) return null;
  return <AbsoluteFill style={{ background: color, opacity: strength * Math.exp(-t / 2.2), pointerEvents: "none" }} />;
};

export function fade(frame: number, from: number, to: number, inDur = 6, outDur = 6) {
  return interpolate(frame, [from, from + inDur, to - outDur, to], [0, 1, 1, 0], clamp);
}

export { AbsoluteFill, Img, Sequence, interpolate, spring, staticFile, useCurrentFrame, useVideoConfig, Easing };
