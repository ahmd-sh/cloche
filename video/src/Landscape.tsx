import React from "react";
import {
  AbsoluteFill, Caret, Flash, Cloche, Corners, Dim, Glint, Grain, Grid, Keys, Laptop, Phone, PhoneLine, Plot, Rise, Sfx, Shot, Stamp, T, Tap, Vignette,
  clamp, ease, easeIn, ink, interpolate, pop, prog, typed, useCurrentFrame, useVideoConfig,
} from "./parts";

export const Landscape: React.FC = () => {
  const frame = useCurrentFrame();
  const red = interpolate(frame, [232, 244, 276, 290], [0, 1, 1, 0], { ...clamp, easing: ease });
  const paper = mix(ink.paper, "#7E2F2B", red);
  const end = prog(frame, 300, 16, easeIn);
  return (
    <AbsoluteFill style={{ backgroundColor: paper, overflow: "hidden" }}>
      <Grid cell={32} paper={paper} />
      <Vignette strength={0.5} />
      {frame < 318 && (
        <AbsoluteFill style={{ opacity: 1 - end, transform: `scale(${1 - end * 0.06})`, filter: end > 0 ? `blur(${end * 6}px)` : undefined }}>
          <Drawing red={red} />
        </AbsoluteFill>
      )}
      {frame >= 300 && <TitleBlock />}
      <Flash at={182} strength={0.16} color={ink.live} />
      <Flash at={316} strength={0.2} />
      <Grain />
      <Sound />
    </AbsoluteFill>
  );
};

function mix(a: string, b: string, t: number) {
  const pa = [1, 3, 5].map((i) => parseInt(a.slice(i, i + 2), 16));
  const pb = [1, 3, 5].map((i) => parseInt(b.slice(i, i + 2), 16));
  return `#${pa.map((v, i) => Math.round(v + (pb[i] - v) * t).toString(16).padStart(2, "0")).join("")}`;
}

const CMD = "cloche push app.apk";
const TYPE_AT = 30;
const EVERY = 1.4;

function Sound() {
  return (
    <>
      <Sfx at={0} src="pen" volume={0.3} />
      <Sfx at={12} src="pen" volume={0.25} />
      <Keys at={TYPE_AT} count={CMD.length} every={EVERY} volume={0.3} />
      <Sfx at={60} src="enter" volume={0.5} />
      <Sfx at={50} src="tick" volume={0.35} />
      <Sfx at={62} src="pen_short" volume={0.3} />
      <Sfx at={72} src="zip" volume={0.5} />
      <Sfx at={86} src="tick" volume={0.35} />
      <Sfx at={104} src="knock" volume={0.5} />
      <Sfx at={106} src="bell" volume={0.55} />
      <Sfx at={114} src="whoosh" volume={0.4} />
      <Sfx at={146} src="tap" volume={0.6} />
      <Sfx at={182} src="stamp" volume={0.95} />
      <Sfx at={196} src="whoosh" volume={0.35} />
      <Sfx at={210} src="tick" volume={0.35} />
      <Sfx at={232} src="swipe" volume={0.35} />
      <Sfx at={236} src="tick" volume={0.4} />
      <Sfx at={276} src="pop" volume={0.4} />
      <Sfx at={282} src="tick" volume={0.4} />
      <Sfx at={288} src="riser" volume={0.4} />
      <Sfx at={316} src="hit" volume={0.85} />
      <Sfx at={332} src="lift" volume={0.5} />
      <Sfx at={336} src="sparkle" volume={0.4} />
      <Sfx at={338} src="bell_low" volume={0.55} />
      <Sfx at={346} src="tick" volume={0.3} />
      <Sfx at={354} src="tick" volume={0.3} />
      <Sfx at={362} src="tick" volume={0.3} />
      <Sfx at={370} src="tick" volume={0.3} />
    </>
  );
}

const PW = 360;
const PX = 1430;
const PY = 120;
const K = (PW - PW * 0.032 * 2) / 1280;
const toSheet = (sx: number, sy: number) => [PX + PW * 0.032 + sx * K, PY + PW * 0.032 + sy * K];
const LX = 200;
const LY = 330;
const LW = 480;

function Drawing({ red }: { red: number }) {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const zin = interpolate(frame, [112, 136, 190, 212], [0, 1, 1, 0], { ...clamp, easing: ease });
  const [ox, oy] = toSheet(640, 1500);
  const drift = interpolate(frame, [0, 110], [1.04, 1], { ...clamp, easing: ease });
  const cam = `translate(${zin * (960 - ox)}px, ${zin * (560 - oy)}px) scale(${(1 + zin * 0.62) * drift})`;
  const shake = frame >= 182 && frame < 192 ? Math.sin(frame * 2.9) * 10 * (1 - (frame - 182) / 10) : 0;
  const phoneIn = pop(frame, fps, 34, { damping: 18, stiffness: 120 });
  const p = prog(frame, 72, 32, ease);
  const px = interpolate(p, [0, 1], [LX + LW + 30, PX - 30]);
  const screen = frame < 104 ? "s1_home_wallflower" : frame < 152 ? "s3_home_ready" : frame < 180 ? "s4_home_installing" : "s5_home_installed";
  const [bx, by] = toSheet(640, 1630);
  const [sx, sy] = toSheet(640, 1180);
  const flow = frame * 2.2;
  const down = red > 0.5;
  return (
    <AbsoluteFill style={{ transform: `${cam} translate(${shake}px, ${shake * 0.5}px)`, transformOrigin: `${ox}px ${oy}px` }}>
      <svg width={1920} height={1080} style={{ position: "absolute", inset: 0 }}>
        <Plot d="M40,40H1880V1040H40Z" at={0} dur={22} length={5700} width={2} color={ink.faint} />
        <Plot d="M60,60H1860V1020H60Z" at={4} dur={24} length={5600} width={3} />
      </svg>
      <T size={20} color={ink.dim} track={0.24} style={{ position: "absolute", left: 90, top: 84, opacity: prog(frame, 14, 10) }}>
        SHEET 01 · DELIVERY
      </T>
      <T size={20} color={ink.dim} track={0.24} style={{ position: "absolute", right: 90, top: 84, opacity: prog(frame, 18, 10) }}>
        SCALE 1:1 · CLOCHE
      </T>

      <div style={{ position: "absolute", left: LX, top: LY }}>
        <Laptop w={LW} at={8} />
        <div style={{ position: "absolute", left: LW * 0.12 + 26, top: 44, width: LW * 0.76 - 52 }}>
          <span style={{ fontFamily: "Plex", fontSize: 21, fontWeight: 600, color: ink.live, opacity: frame >= 26 ? 1 : 0 }}>$ </span>
          <span style={{ fontFamily: "Plex", fontSize: 21, fontWeight: 500, color: ink.text }}>{typed(frame, CMD, TYPE_AT, EVERY)}</span>
          {frame >= 26 && frame < 62 && <Caret size={21} />}
          <T size={17} color={ink.dim} style={{ marginTop: 14, opacity: prog(frame, 61, 4) }}>pushed Satchel 0.3.0</T>
          <T size={17} color={ink.dim} style={{ marginTop: 6, opacity: prog(frame, 66, 4) }}>pixel-10-pro has it</T>
        </div>
        <T size={18} color={ink.dim} track={0.2} style={{ position: "absolute", left: 0, width: LW, top: LW * 0.62 + 18, textAlign: "center", opacity: prog(frame, 30, 10) }}>
          YOUR COMPUTER
        </T>
      </div>

      <div style={{ position: "absolute", left: PX, top: PY, opacity: 1 - prog(frame, 40, 10) }}>
        <PhoneLine w={PW} at={12} />
      </div>
      <Phone w={PW} style={{ left: PX, top: PY, opacity: phoneIn, transform: `scale(${0.94 + phoneIn * 0.06})` }} glow={frame >= 182 ? prog(frame, 182, 6) * (1 - prog(frame, 196, 24)) : 0}>
        <Shot src={screen} />
        {red > 0 && <Shot src={screen} opacity={red} style={{ filter: "hue-rotate(152deg) saturate(1.25) brightness(0.92)" }} />}
        <Tap at={146} x={640} y={1630} size={260} />
      </Phone>
      <T size={18} color={ink.dim} track={0.2} style={{ position: "absolute", left: PX, width: PW, top: PY + 800, textAlign: "center", opacity: prog(frame, 40, 10) }}>
        YOUR PHONE
      </T>

      <Dim x1={LX + LW + 30} x2={PX - 30} y={500} at={60} dur={16} label={down ? "TAILSCALE OFF" : "YOUR TAILNET"} dash flow={down ? 0 : flow} color={down ? "#FFD2C9" : ink.line} />
      {down && (
        <svg width={1920} height={1080} style={{ position: "absolute", inset: 0 }}>
          <Plot d={`M${(LX + LW + PX) / 2 - 40},460L${(LX + LW + PX) / 2 + 40},540`} at={238} dur={6} length={120} width={6} color="#3A0909" />
        </svg>
      )}
      {frame >= 72 && frame < 108 && (
        <>
          <div style={{ position: "absolute", left: Math.max(LX + LW + 30, px - 260 * Math.sin(Math.PI * p)), top: 496, width: Math.min(260 * Math.sin(Math.PI * p), px - LX - LW - 30), height: 8, borderRadius: 4, background: `linear-gradient(90deg, rgba(159,240,213,0), ${ink.live})`, opacity: 0.8 }} />
          <div style={{ position: "absolute", left: px - 40, top: 460, width: 80, height: 80, borderRadius: 18, background: ink.paper, border: `3px solid ${ink.live}`, boxShadow: "0 0 40px rgba(159,240,213,0.7)", display: "flex", alignItems: "center", justifyContent: "center", opacity: 1 - prog(frame, 102, 5) }}>
            <T size={22} weight={600} color={ink.live}>APK</T>
          </div>
        </>
      )}

      <Note n="01" at={48} x={LX + 20} y={LY + LW * 0.62 + 80} title="Your agent runs cloche push" sub="from any session on your computer" />
      <Note n="02" at={84} x={790} y={300} title="Signed, then sent" sub="your computer's key, pinned on the phone" />
      <Note n="03" at={208} x={790} y={620} title="Checked, then installed" sub="hash, signer and version, in one tap" />
      {red > 0 && (
        <div style={{ position: "absolute", left: 790, top: 800, opacity: red }}>
          <Note n="04" at={236} x={0} y={0} title="Tailscale off? The sheet goes red" sub="and it says why, until the link's back" color="#FFD2C9" />
        </div>
      )}

      {frame >= 178 && (
        <div style={{ position: "absolute", left: sx, top: sy, transform: "translate(-50%, -50%)" }}>
          <Stamp at={182} size={34} rotate={-9} />
        </div>
      )}
      <Glint x={sx + 130} y={sy - 40} r={18} t={(frame - 185) / 14} color={ink.live} />
      <Glint x={bx - 150} y={by + 20} r={12} t={(frame - 188) / 12} color={ink.live} />
    </AbsoluteFill>
  );
}

function Note({ n, at, x, y, title, sub, color = ink.text }: { n: string; at: number; x: number; y: number; title: string; sub: string; color?: string }) {
  const frame = useCurrentFrame();
  const s = prog(frame, at, 10);
  if (frame < at) return null;
  return (
    <div style={{ position: "absolute", left: x, top: y, opacity: s, transform: `translateY(${(1 - s) * 16}px)` }}>
      <div style={{ display: "flex", alignItems: "center", gap: 16 }}>
        <div style={{ width: 44, height: 44, borderRadius: 22, border: `2px solid ${color}`, display: "flex", alignItems: "center", justifyContent: "center" }}>
          <T size={18} weight={600} color={color}>{n}</T>
        </div>
        <Rise text={title} at={at + 1} size={30} stagger={1} weight={600} color={color} align="left" track={-0.01} />
      </div>
      <T size={19} color={ink.dim} style={{ marginLeft: 60, marginTop: 4, opacity: prog(frame, at + 6, 8) }}>
        {sub}
      </T>
    </div>
  );
}

function TitleBlock() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const land = pop(frame, fps, 316, { damping: 12, stiffness: 160 });
  const lid = prog(frame, 330, 16, ease);
  const settle = prog(frame, 352, 20, ease);
  const box = prog(frame, 306, 16);
  const cells = [
    ["DRAWING", "Cloche"],
    ["SERVES", "your freshest builds"],
    ["OVER", "your tailnet"],
    ["RUN", "cloche push app.apk"],
  ];
  return (
    <AbsoluteFill>
      <AbsoluteFill style={{ background: `radial-gradient(circle at 34% 46%, rgba(159,240,213,${0.14 * lid}) 0%, rgba(0,0,0,0) 40%)` }} />
      <div style={{ position: "absolute", left: 300, top: 230, transform: `scale(${0.5 + land * 0.5}) translateY(${(1 - land) * 120}px)`, opacity: interpolate(land, [0, 0.2], [0, 1], clamp) }}>
        <Cloche size={460} lift={lid * (1 - settle * 0.75)} />
        <Glint x={360} y={110} r={36} t={(frame - 334) / 16} />
        <Glint x={410} y={190} r={22} t={(frame - 339) / 14} />
      </div>
      <div style={{ position: "absolute", left: 820, top: 330 }}>
        <Rise text="CLOCHE" at={322} size={150} track={0.26} align="left" />
        <div style={{ height: 16 }} />
        <Rise text="Your freshest builds, served." at={332} size={46} stagger={2} weight={500} color={ink.dim} align="left" accent={["freshest"]} />
      </div>
      <div style={{ position: "absolute", left: 180, right: 180, top: 800, height: 120, opacity: box, transform: `translateY(${(1 - box) * 40}px)` }}>
        <svg width={1560} height={120} style={{ position: "absolute", inset: 0, overflow: "visible" }}>
          <Plot d="M0,0H1560V120H0Z" at={308} dur={18} length={3400} width={2.4} />
          {[1, 2, 3].map((i) => (
            <Plot key={i} d={`M${i * 390},0V120`} at={316 + i * 3} dur={8} length={140} width={1.6} color={ink.faint} />
          ))}
        </svg>
        <Corners w={1560} h={120} m={16} t={box} />
        {cells.map(([k, v], i) => (
          <div key={k} style={{ position: "absolute", left: i * 390 + 24, top: 22, opacity: prog(frame, 346 + i * 8, 8) }}>
            <T size={16} color={ink.faint} track={0.2}>{k}</T>
            <T size={i === 3 ? 26 : 30} weight={i === 0 ? 600 : 500} color={i === 3 ? ink.live : ink.text} style={{ marginTop: 10 }}>{v}</T>
          </div>
        ))}
      </div>
    </AbsoluteFill>
  );
}
