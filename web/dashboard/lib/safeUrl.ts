// External-URL allowlisting for user-controlled links/images.
//
// Sinks: fleet badge_url / avatar URLs land in <img src>, Discord invites in
// <a href>. React escapes attribute values (no breakout), but without scheme
// validation a compromised/malicious backend row could point users at
// javascript:/data: payloads or credential-bearing URLs. Only absolute
// http(s) survives; everything else renders as unlinked text/tiles.

const MAX_URL_LEN = 2048;
// Control chars, whitespace, and quote/angle chars have no business in a URL
// we embed. Browsers would percent-encode some of these; reject instead.
const UNSAFE_CHARS = /[\s<>"'`\u0000-\u001f\u007f]/;

export function safeHttpUrl(raw: unknown): string | null {
  if (typeof raw !== "string") return null;
  const s = raw.trim();
  if (!s || s.length > MAX_URL_LEN || UNSAFE_CHARS.test(s)) return null;
  let u: URL;
  try {
    // Absolute-only: no base URL, so protocol-relative ("//evil") and
    // path-relative values throw instead of resolving somewhere surprising.
    u = new URL(s);
  } catch {
    return null;
  }
  if (u.protocol !== "http:" && u.protocol !== "https:") return null;
  if (u.username || u.password) return null;
  return u.toString();
}

/** Discord invites are stored bare ("discord.gg/…", sometimes with scheme). */
export function safeInviteUrl(raw: unknown): string | null {
  if (typeof raw !== "string") return null;
  const bare = raw.trim().replace(/^https?:\/\//i, "").replace(/^\/+/, "");
  if (!bare) return null;
  return safeHttpUrl(`https://${bare}`);
}
