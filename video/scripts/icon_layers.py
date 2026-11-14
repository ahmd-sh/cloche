import os, sys
sys.path.insert(0, os.path.join(os.path.dirname(__file__), "..", "..", "scripts"))
import icon

def layer(indices, prefix):
    keep = icon.shapes
    defs, body = [], []
    for i in indices:
        s = keep[i]
        if "fill" in s:
            kind, g, stops = s["fill"]
            sts = "".join(f'<stop offset="{o}" stop-color="{icon.color(c)[0]}" stop-opacity="{icon.color(c)[1]:.3f}"/>' for o, c in stops)
            if kind == "linear":
                defs.append(f'<linearGradient id="{prefix}{i}" gradientUnits="userSpaceOnUse" x1="{g[0]:.2f}" y1="{g[1]:.2f}" x2="{g[2]:.2f}" y2="{g[3]:.2f}">{sts}</linearGradient>')
            else:
                defs.append(f'<radialGradient id="{prefix}{i}" gradientUnits="userSpaceOnUse" cx="{g[0]:.2f}" cy="{g[1]:.2f}" r="{g[2]:.2f}">{sts}</radialGradient>')
            body.append(f'<path d="{s["d"]}" fill="url(#{prefix}{i})"/>')
        else:
            c, a = icon.color(s["stroke"])
            body.append(f'<path d="{s["d"]}" fill="none" stroke="{c}" stroke-opacity="{a:.3f}" stroke-width="{s["width"]}" stroke-linecap="round"/>')
    k, (px, py) = icon.SCALE, icon.PIVOT
    t = f"translate({px*(1-k):.3f},{py*(1-k)+icon.LIFT:.3f}) scale({k})"
    return f'<svg xmlns="http://www.w3.org/2000/svg" width="432" height="432" viewBox="0 0 108 108"><defs>{"".join(defs)}</defs><g transform="{t}">{"".join(body)}</g></svg>'

out = os.path.join(os.path.dirname(__file__), "..", "public")
lid = list(icon.LID)
open(os.path.join(out, "cloche_base.svg"), "w").write(layer([i for i in range(len(icon.shapes)) if i not in lid], "b"))
open(os.path.join(out, "cloche_lid.svg"), "w").write(layer(lid, "l"))
print("wrote cloche_base.svg, cloche_lid.svg")
