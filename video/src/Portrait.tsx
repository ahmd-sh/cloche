import React from "react";
import {
  AbsoluteFill, Caret, Flash, Cloche, Corners, Glint, Grain, Grid, Img, Keys, Phone, Plot, Rise, Sfx, Shot, Stamp, T, Tap, Vignette,
  clamp, ease, easeIn, fade, ink, interpolate, pop, prog, staticFile, typed, useCurrentFrame, useVideoConfig,
} from "./parts";

export const Portrait: React.FC = () => {
  const frame = useCurrentFrame();
  return (
    <AbsoluteFill style={{ backgroundColor: ink.paper, overflow: "hidden" }}>
      <Grid y={-frame * 0.8} />
      <Vignette />
      {frame < 66 && <Push />}
      {frame >= 58 && frame < 110 && <Travel />}
      {frame >= 100 && frame < 276 && <Land />}
      {frame >= 268 && frame < 384 && <Features />}
      {frame >= 372 && <End />}
      <Flash at={206} strength={0.16} color={ink.live} />
      <Flash at={382} strength={0.22} />
      <Grain />
      <Sound />
    </AbsoluteFill>
  );
};

const CMD = 'cloche push app.apk -m "fresh"';
const TYPE_AT = 14;
const EVERY = 1.25;

function Sound() {
  return (
    <>
      <Keys at={TYPE_AT} count={CMD.length} every={EVERY} volume={0.32} />
      <Sfx at={56} src="enter" volume={0.55} />
      <Sfx at={60} src="whoosh" volume={0.45} />
      <Sfx at={66} src="zip" volume={0.5} />
      <Sfx at={74} src="tick" volume={0.35} />
      <Sfx at={82} src="tick" volume={0.35} />
      <Sfx at={90} src="tick" volume={0.35} />
      <Sfx at={100} src="swipe" volume={0.4} />
      <Sfx at={114} src="pop" volume={0.45} />
      <Sfx at={116} src="bell" volume={0.6} />
      <Sfx at={150} src="tap" volume={0.6} />
      <Sfx at={156} src="swipe" volume={0.35} />
      <Sfx at={166} src="lift" volume={0.6} />
      <Sfx at={170} src="sparkle" volume={0.35} />
      <Sfx at={206} src="stamp" volume={0.95} />
      <Sfx at={268} src="whoosh" volume={0.4} />
      <Sfx at={280} src="tick" volume={0.45} />
      <Sfx at={280} src="pen_short" volume={0.3} />
      <Sfx at={310} src="swipe" volume={0.3} />
      <Sfx at={312} src="tick" volume={0.45} />
      <Sfx at={312} src="pen_short" volume={0.3} />
      <Sfx at={342} src="swipe" volume={0.3} />
      <Sfx at={344} src="tick" volume={0.45} />
      <Sfx at={344} src="pen_short" volume={0.3} />
      <Sfx at={360} src="riser" volume={0.45} />
      <Sfx at={382} src="hit" volume={0.85} />
      <Sfx at={398} src="lift" volume={0.5} />
      <Sfx at={402} src="sparkle" volume={0.4} />
      <Sfx at={404} src="bell_low" volume={0.55} />
    </>
  );
}

function Push() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const box = pop(frame, fps, 4, { damping: 18, stiffness: 160 });
  const out = prog(frame, 58, 10, easeIn);
  const done = frame >= 56;
  return (
    <AbsoluteFill style={{ transform: `translateY(${out * 900}px)`, opacity: 1 - out * 0.6 }}>
      <div style={{ position: "absolute", top: 470, left: 60, right: 60 }}>
        <T size={26} color={ink.faint} track={0.22} style={{ textAlign: "center", opacity: prog(frame, 2, 8) }}>
          SHEET 01 · ORDER IN
        </T>
        <div style={{ height: 34 }} />
        <Rise text="Your agent" at={4} size={92} />
        <Rise text="ships a build." at={8} size={92} accent={["build."]} />
      </div>
      <div style={{ position: "absolute", left: 80, top: 900, width: 920, height: 300, transform: `scale(${0.9 + box * 0.1})`, opacity: box }}>
        <div style={{ position: "absolute", inset: 0, border: `2px solid ${ink.line}`, background: "rgba(8,42,77,0.55)" }} />
        <Corners w={920} h={300} m={22} t={box} />
        <div style={{ position: "absolute", left: 32, top: 26, display: "flex", gap: 12 }}>
          {[0, 1, 2].map((i) => (
            <div key={i} style={{ width: 16, height: 16, borderRadius: 8, border: `2px solid ${ink.faint}` }} />
          ))}
        </div>
        <T size={22} color={ink.faint} track={0.12} style={{ position: "absolute", right: 32, top: 22 }}>
          ~/dev/satchel
        </T>
        <div style={{ position: "absolute", left: 32, top: 96, right: 32, whiteSpace: "pre-wrap" }}>
          <span style={{ fontFamily: "Plex", fontSize: 40, fontWeight: 600, color: ink.live }}>$ </span>
          <span style={{ fontFamily: "Plex", fontSize: 40, fontWeight: 500, color: ink.text }}>{typed(frame, CMD, TYPE_AT, EVERY)}</span>
          {!done && <Caret size={40} solid={frame < TYPE_AT + CMD.length * EVERY} />}
        </div>
        <T size={28} color={ink.dim} style={{ position: "absolute", left: 32, top: 210, opacity: prog(frame, 57, 4) }}>
          pushed Satchel 0.3.0 · 4.6 MB
        </T>
      </div>
    </AbsoluteFill>
  );
}

function Travel() {
  const frame = useCurrentFrame();
  const enter = prog(frame, 58, 10);
  const leave = prog(frame, 100, 10, easeIn);
  const p = prog(frame, 64, 34, ease);
  const y = interpolate(p, [0, 1], [1700, 260]);
  const marks = [
    { y: 1300, at: 74, text: "SIGNED", side: -1 },
    { y: 1000, at: 82, text: "SHA-256 OK", side: 1 },
    { y: 700, at: 90, text: "OVER YOUR TAILNET", side: -1 },
  ];
  return (
    <AbsoluteFill style={{ opacity: enter * (1 - leave), transform: `translateY(${(1 - enter) * -300 + leave * -600}px)` }}>
      <svg width={1080} height={1920} style={{ position: "absolute", inset: 0 }}>
        <line x1={540} y1={1920} x2={540} y2={0} stroke={ink.line} strokeWidth={3} strokeDasharray="14 12" strokeDashoffset={frame * 6} opacity={0.75} />
        {Array.from({ length: 33 }, (_, i) => (
          <line key={i} x1={540 - (i % 5 === 0 ? 26 : 12)} y1={i * 60} x2={540 + (i % 5 === 0 ? 26 : 12)} y2={i * 60} stroke={ink.line} strokeWidth={1.6} opacity={0.45} />
        ))}
        <line x1={540} y1={y + 60} x2={540} y2={Math.min(1920, y + 60 + 520 * Math.sin(Math.PI * p))} stroke={ink.live} strokeWidth={10} strokeLinecap="round" opacity={0.55} />
      </svg>
      {marks.map((m) => {
        const s = prog(frame, m.at, 8);
        const x = m.side < 0 ? 20 : 600;
        return (
          <div key={m.text} style={{ position: "absolute", top: m.y - 26, left: x, width: 460, textAlign: m.side < 0 ? "right" : "left", opacity: s, transform: `translateX(${(1 - s) * 40 * m.side}px)` }}>
            <T size={32} weight={600} color={ink.text} track={0.1} style={{ display: "inline-block" }}>
              {m.side > 0 ? "✓ " : ""}{m.text}{m.side < 0 ? " ✓" : ""}
            </T>
          </div>
        );
      })}
      <div style={{ position: "absolute", left: 540 - 70, top: y - 70, width: 140, height: 140, borderRadius: 28, background: ink.paper, border: `4px solid ${ink.live}`, boxShadow: `0 0 60px rgba(159,240,213,0.7)`, display: "flex", alignItems: "center", justifyContent: "center" }}>
        <T size={36} weight={600} color={ink.live} track={0.06}>APK</T>
      </div>
    </AbsoluteFill>
  );
}

const PW = 660;
const PX = (1080 - PW) / 2;
const PY = 440;
const K = (PW - PW * 0.032 * 2) / 1280;
const toComp = (sx: number, sy: number) => [PX + PW * 0.032 + sx * K, PY + PW * 0.032 + sy * K];

function Land() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const rise = pop(frame, fps, 100, { damping: 17, stiffness: 140 });
  const leave = prog(frame, 266, 10, easeIn);
  const drop = pop(frame, fps, 112, { damping: 13, stiffness: 190 });
  const [nx, ny] = toComp(640, 372);
  const zoom = interpolate(frame, [114, 148, 158, 176], [1, 1.18, 1.18, 1], { ...clamp, easing: ease });
  const shake = frame >= 206 && frame < 218 ? Math.sin(frame * 2.7) * 14 * (1 - (frame - 206) / 12) : 0;
  const splashUp = prog(frame, 178, 14, ease);
  const lid = prog(frame, 164, 14, ease);
  const installing = frame >= 192 && frame < 206;
  const installed = frame >= 205;
  const [stx, sty] = toComp(640, 1200);
  return (
    <AbsoluteFill style={{ opacity: 1 - leave, transform: `translateX(${leave * -500}px)` }}>
      <div style={{ position: "absolute", top: 130, left: 60, right: 60, height: 300 }}>
        {frame < 202 && (
          <div style={{ opacity: fade(frame, 102, 202, 4, 6) }}>
            <Rise text="It lands on" at={104} size={88} />
            <Rise text="your phone." at={108} size={88} accent={["phone."]} />
          </div>
        )}
        {frame >= 204 && (
          <>
            <Rise text="One tap." at={207} size={88} />
            <Rise text="Installed." at={211} size={88} accent={["Installed."]} />
          </>
        )}
      </div>
      <AbsoluteFill style={{ transform: `translate(${shake}px, ${shake * 0.6}px) scale(${zoom})`, transformOrigin: `${nx}px ${ny}px` }}>
        <Phone w={PW} style={{ left: PX, top: PY + (1 - rise) * 1200 }} glow={installed ? prog(frame, 206, 6) * (1 - prog(frame, 222, 30)) : 0}>
          <Shot src={installed ? "s8b_detail_installed" : "s8a_detail_ready"} />
          {installing && <Installing />}
          {frame < 194 && (
            <div style={{ position: "absolute", inset: 0, transform: `translateY(${-splashUp * 2856}px)`, opacity: frame >= 158 ? 1 : 0 }}>
              <div style={{ position: "absolute", inset: 0, background: ink.paper }} />
              <div style={{ position: "absolute", left: 640 - 300, top: 1428 - 300 }}>
                <Cloche size={600} lift={lid} tilt={Math.sin(frame * 1.3) * 3 * (1 - lid) * (frame < 166 ? 1 : 0)} />
              </div>
              <Glint x={900} y={1200} r={60} t={(frame - 168) / 14} />
              <Glint x={980} y={1320} r={34} t={(frame - 172) / 12} />
              <div style={{ position: "absolute", left: 0, right: 0, bottom: -80, height: 80, background: "linear-gradient(rgba(0,0,0,0.35), rgba(0,0,0,0))" }} />
            </div>
          )}
          {frame < 158 && (
            <>
              <Wallpaper />
              <Img
                src={staticFile("shots/notif.png")}
                style={{ position: "absolute", left: 46, top: 178 + (1 - drop) * -520, width: 1188, opacity: interpolate(drop, [0, 0.2], [0, 1], clamp), boxShadow: "0 30px 80px rgba(0,0,0,0.35)", borderRadius: 84 }}
              />
              <Tap at={150} x={46 + 215} y={178 + 296} size={230} />
            </>
          )}
        </Phone>
        {installed && (
          <div style={{ position: "absolute", left: stx, top: sty, transform: "translate(-50%, -50%)" }}>
            <Stamp at={206} size={70} rotate={-9} />
          </div>
        )}
        {installed && (
          <>
            <Glint x={stx + 250} y={sty - 70} r={34} t={(frame - 209) / 14} color={ink.live} />
            <Glint x={stx - 270} y={sty + 60} r={24} t={(frame - 212) / 12} color={ink.live} />
          </>
        )}
      </AbsoluteFill>
    </AbsoluteFill>
  );
}

function Wallpaper() {
  return (
    <div style={{ position: "absolute", inset: 0, width: 1280, height: 2856, overflow: "hidden" }}>
      <div style={{ position: "absolute", inset: 0, background: "linear-gradient(165deg, #1E5C9C 0%, #0F3D6E 38%, #071E38 100%)" }} />
      <div style={{ position: "absolute", left: -300, top: 1500, width: 1500, height: 1500, borderRadius: "50%", background: "radial-gradient(circle, rgba(159,240,213,0.28), rgba(159,240,213,0) 65%)" }} />
      <div style={{ position: "absolute", left: 500, top: -400, width: 1300, height: 1300, borderRadius: "50%", background: "radial-gradient(circle, rgba(220,235,255,0.22), rgba(220,235,255,0) 60%)" }} />
      <div style={{ position: "absolute", left: 0, right: 0, top: 900, textAlign: "center", fontFamily: "system-ui, sans-serif", fontWeight: 300, fontSize: 300, color: "rgba(242,247,255,0.92)", letterSpacing: "-0.02em" }}>9:41</div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 1240, textAlign: "center", fontFamily: "system-ui, sans-serif", fontSize: 64, color: "rgba(242,247,255,0.8)" }}>Saturday, Nov 7</div>
      <div style={{ position: "absolute", left: 74, top: 38, fontFamily: "system-ui, sans-serif", fontSize: 48, color: "#fff" }}>9:41</div>
      <svg width={160} height={60} style={{ position: "absolute", right: 60, top: 40 }}>
        <path d="M18,44L40,16Q29,6 18,6Q7,6 -4,16Z" transform="translate(30,0)" fill="#fff" />
        <rect x={104} y={10} width={24} height={40} rx={5} fill="#fff" />
      </svg>
      <div style={{ position: "absolute", left: 540, bottom: 40, width: 200, height: 10, borderRadius: 5, background: "rgba(255,255,255,0.8)" }} />
    </div>
  );
}

function Installing() {
  const frame = useCurrentFrame();
  const x = 60 + ((frame * 38) % 1000);
  return (
    <div style={{ position: "absolute", left: 30, top: 2450, width: 1220, height: 200, background: ink.paper }}>
      <svg width={1220} height={200} style={{ position: "absolute", inset: 0 }}>
        <g stroke={ink.line} strokeWidth={3}>
          <line x1={30} y1={70} x2={30} y2={130} />
          <line x1={1190} y1={70} x2={1190} y2={130} />
          <line x1={30} y1={100} x2={450} y2={100} />
          <line x1={770} y1={100} x2={1190} y2={100} />
        </g>
        <line x1={x} y1={100} x2={Math.min(1190, x + 170)} y2={100} stroke={ink.live} strokeWidth={9} strokeLinecap="round" />
      </svg>
      <T size={42} color={ink.dim} track={0.04} style={{ position: "absolute", left: 0, right: 0, top: 74, textAlign: "center" }}>
        installing
      </T>
    </div>
  );
}

function Features() {
  const frame = useCurrentFrame();
  const beats = [
    { at: 280, n: "01", a: "Over your", b: "tailnet.", sub: "home Wi-Fi, a café, mobile data", icon: "tailnet" },
    { at: 312, n: "02", a: "Signed by", b: "your computer.", sub: "the phone pins its key", icon: "key" },
    { at: 344, n: "03", a: "Checked on", b: "your phone.", sub: "hash, signer, version", icon: "check" },
  ];
  return (
    <AbsoluteFill>
      <T size={26} color={ink.faint} track={0.22} style={{ position: "absolute", top: 380, left: 0, right: 0, textAlign: "center", opacity: fade(frame, 272, 380, 8, 8) }}>
        SPECIFICATION
      </T>
      {beats.map((r, i) => {
        const end = i < beats.length - 1 ? beats[i + 1].at : 380;
        if (frame < r.at - 2 || frame >= end + 6) return null;
        const inn = prog(frame, r.at - 2, 8);
        const out = prog(frame, end - 2, 8, easeIn);
        return (
          <AbsoluteFill key={r.n} style={{ opacity: inn * (1 - out), transform: `translateX(${(1 - inn) * 120 - out * 160}px)` }}>
            <svg width={1080} height={1920} style={{ position: "absolute", inset: 0, overflow: "visible" }}>
              <g transform="translate(540,690) scale(2.6) translate(-80,-96)">
                <Icon kind={r.icon} at={r.at} />
              </g>
              <Plot d="M190,1330H890" at={r.at + 6} dur={14} length={700} width={2} color={ink.faint} />
            </svg>
            <T size={30} weight={500} color={ink.faint} track={0.16} style={{ position: "absolute", top: 930, left: 0, right: 0, textAlign: "center" }}>
              {r.n} / 03
            </T>
            <div style={{ position: "absolute", top: 1000, left: 40, right: 40 }}>
              <Rise text={r.a} at={r.at} size={100} stagger={2} />
              <Rise text={r.b} at={r.at + 3} size={100} stagger={2} accent={[r.b.split(" ").pop() ?? ""]} />
            </div>
            <T size={36} weight={400} color={ink.dim} style={{ position: "absolute", top: 1370, left: 0, right: 0, textAlign: "center", opacity: prog(frame, r.at + 8, 8) }}>
              {r.sub}
            </T>
          </AbsoluteFill>
        );
      })}
    </AbsoluteFill>
  );
}

function Icon({ kind, at }: { kind: string; at: number }) {
  if (kind === "tailnet") {
    return (
      <g>
        <Plot d="M10,60h34v64h-34z" at={at} dur={10} length={200} width={3} />
        <Plot d="M96,70h40v40h-40zM90,118h52" at={at + 4} dur={10} length={240} width={3} />
        <Plot d="M48,92H90" at={at + 8} dur={8} length={60} width={3} dash="6 6" color={ink.live} />
      </g>
    );
  }
  if (kind === "key") {
    return (
      <g>
        <Plot d="M30,92a26,26 0 1,0 52,0a26,26 0 1,0 -52,0" at={at} dur={12} length={180} width={3} />
        <Plot d="M82,92H140M120,92v20M136,92v14" at={at + 6} dur={10} length={140} width={3} color={ink.live} />
      </g>
    );
  }
  return (
    <g>
      <Plot d="M24,48h96v96h-96z" at={at} dur={12} length={400} width={3} />
      <Plot d="M46,96l20,20l36,-42" at={at + 8} dur={8} length={100} width={5} color={ink.live} />
    </g>
  );
}

function End() {
  const frame = useCurrentFrame();
  const { fps } = useVideoConfig();
  const enter = prog(frame, 374, 8);
  const land = pop(frame, fps, 382, { damping: 11, stiffness: 170 });
  const lid = prog(frame, 396, 16, ease);
  const settle = prog(frame, 418, 20, ease);
  const liftNow = lid * (1 - settle * 0.75);
  const cmd = prog(frame, 414, 10);
  return (
    <AbsoluteFill style={{ opacity: enter }}>
      <AbsoluteFill style={{ background: `radial-gradient(circle at 50% 40%, rgba(159,240,213,${0.16 * lid}) 0%, rgba(0,0,0,0) 45%)` }} />
      <div style={{ position: "absolute", left: 540 - 300, top: 420, transform: `scale(${0.4 + land * 0.6}) translateY(${(1 - land) * 200}px)`, opacity: interpolate(land, [0, 0.2], [0, 1], clamp) }}>
        <Cloche size={600} lift={liftNow} />
        <Glint x={470} y={150} r={46} t={(frame - 400) / 16} />
        <Glint x={530} y={250} r={28} t={(frame - 405) / 14} />
        <Glint x={110} y={200} r={22} t={(frame - 409) / 12} />
      </div>
      <div style={{ position: "absolute", top: 1080, left: 0, right: 0 }}>
        <Rise text="CLOCHE" at={390} size={132} track={0.28} weight={600} />
      </div>
      <div style={{ position: "absolute", top: 1270, left: 80, right: 80 }}>
        <Rise text="Your freshest builds, served." at={400} size={52} stagger={2} weight={500} color={ink.dim} accent={["freshest"]} />
      </div>
      <div style={{ position: "absolute", top: 1520, left: 0, right: 0, display: "flex", justifyContent: "center", opacity: cmd, transform: `translateY(${(1 - cmd) * 30}px)` }}>
        <div style={{ position: "relative", padding: "22px 40px", border: `2px solid ${ink.line}`, background: "rgba(8,42,77,0.6)" }}>
          <T size={36} weight={500}>
            <span style={{ color: ink.live }}>$ </span>cloche push app.apk
          </T>
        </div>
      </div>
    </AbsoluteFill>
  );
}
