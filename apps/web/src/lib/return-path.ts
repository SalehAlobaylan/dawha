/**
 * Keeps post-authentication navigation inside the current app while allowing
 * the caller to return to the page that requested sign-in.
 */
export function getSafeReturnPath(candidate: string | null, currentOrigin: string): string {
  if (!candidate || !candidate.startsWith("/") || candidate.startsWith("//") || candidate.includes("\\")) {
    return "/";
  }

  try {
    const origin = new URL(currentOrigin).origin;
    const target = new URL(candidate, origin);
    return target.origin === origin ? `${target.pathname}${target.search}${target.hash}` : "/";
  } catch {
    return "/";
  }
}
