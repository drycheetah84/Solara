import { NextRequest, NextResponse } from "next/server";
import { randomBytes } from "node:crypto";
import { discordConfig, publicUrl, stateHash } from "@/lib/discord";

const DISCORD_STATE_COOKIE = "halcyon_dstate";

// GET /auth/discord -> Discord's authorize page (scopes: identify + reading this user's membership/roles in our server).
export async function GET(req: NextRequest) {
  const cfg = discordConfig();
  if (!cfg) return NextResponse.redirect(publicUrl("/login?err=discord-not-configured"));
  const state = randomBytes(24).toString("hex");
  const url = new URL("https://discord.com/oauth2/authorize");
  url.searchParams.set("client_id", cfg.clientId);
  url.searchParams.set("response_type", "code");
  url.searchParams.set("redirect_uri", cfg.redirectUri);
  url.searchParams.set("scope", "identify guilds.members.read");
  url.searchParams.set("state", state);
  url.searchParams.set("prompt", "none");
  const res = NextResponse.redirect(url);
  const proto = req.headers.get("x-forwarded-proto") ?? req.nextUrl.protocol.replace(":", "");
  res.cookies.set(DISCORD_STATE_COOKIE, stateHash(state), {
    httpOnly: true, sameSite: "lax", secure: proto.split(",")[0].trim() === "https", path: "/auth/discord", maxAge: 600,
  });
  return res;
}

export const dynamic = "force-dynamic";
