"use client";

import { useState } from "react";

/* Right-side character art, like the robot on the reference dashboard.
   Save the robot render as public/mascot.png (exact name) and it appears
   automatically — no code changes, no restart in dev (rebuild for prod).
   `screen` blend melts a black background into the dark page; if you get a
   clean transparent PNG export, use that instead. */
export function Mascot() {
  const [src, setSrc] = useState("/mascot.png");
  const isFallback = src !== "/mascot.png";
  return (
    <div aria-hidden className="hidden lg:flex justify-center items-start pointer-events-none select-none sticky top-24">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={src}
        alt=""
        width={440}
        onError={() => {
          if (!isFallback) setSrc("/logo.webp");
        }}
        style={
          isFallback
            ? {
                maxWidth: "100%",
                borderRadius: 16,
                opacity: 0.95,
                filter: "drop-shadow(0 0 60px rgba(124,199,255,0.18))",
              }
            : {
                maxWidth: "100%",
                mixBlendMode: "screen",
                filter: "drop-shadow(0 0 50px rgba(124,199,255,0.15))",
              }
        }
      />
    </div>
  );
}
