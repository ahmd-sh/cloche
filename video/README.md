# Cloche videos

Two 15 second demos, rendered with [Remotion](https://www.remotion.dev): sound effects only, no music.

- `out/cloche-portrait.mp4` (1080×1920): "Order up". An agent pushes a build, it runs up the tailnet, lands as a notification, one tap lifts the lid and stamps it installed, then three beats on what Cloche does and the end card.
- `out/cloche-landscape.mp4` (1920×1080): "The drawing". A blueprint sheet plots itself: the computer, the phone, the tailnet line. A build runs it, the camera goes in for the tap and the stamp, the sheet turns red while Tailscale's off, and the title block names it.

```sh
npm install
npm run render     # both videos to out/
npm run studio     # to scrub through them in a browser
```

What's in it:

- `src/Portrait.tsx`, `src/Landscape.tsx`: the two videos, frame by frame (30 fps, 450 frames).
- `src/parts.tsx`: the shared pieces, in the app's blueprint ink and IBM Plex Mono.
- `public/shots/`: screenshots of the real app, taken on an emulator.
- `public/cloche_base.svg`, `public/cloche_lid.svg`: the icon in two layers, so its lid can lift. `python3 scripts/icon_layers.py` writes them from `../scripts/icon.py`.
- `public/sfx/`: every sound, synthesized by `node scripts/sfx.mjs` (keys, whooshes, a desk bell, the stamp, glints).
