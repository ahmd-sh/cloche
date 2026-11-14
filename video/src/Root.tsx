import React from "react";
import { Composition } from "remotion";
import { Landscape } from "./Landscape";
import { Portrait } from "./Portrait";

export const Root: React.FC = () => (
  <>
    <Composition id="Portrait" component={Portrait} width={1080} height={1920} fps={30} durationInFrames={450} />
    <Composition id="Landscape" component={Landscape} width={1920} height={1080} fps={30} durationInFrames={450} />
  </>
);
