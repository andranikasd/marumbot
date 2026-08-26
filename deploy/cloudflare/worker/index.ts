/**
 * The Cloudflare edge for Marum.
 *
 * Three jobs, and nothing else: verify that a request really came from
 * Telegram, keep one noisy chat from drowning the rest, and hand the update to
 * the Go application. Anything that needs to look at a loan happens in Go.
 */
import { Container, getContainer } from "@cloudflare/containers";

export class MarumApp extends Container {
  defaultPort = 8080;
  // Long enough that a borrower mid-conversation does not pay a cold start,
  // short enough that an idle night costs nothing.
  sleepAfter = "10m";
}

interface Env {
  MARUM_APP: DurableObjectNamespace<MarumApp>;
  ASSETS: Fetcher;
  MARUM_WEBHOOK_SECRET: string;
  MARUM_SERVICE_TOKEN: string;
  WEBHOOK_PATH: string;
  ENVIRONMENT: string;
}

/** Constant-time comparison: a timing side channel on the webhook secret would
 *  let an attacker recover it a byte at a time. */
function secretsMatch(a: string | null, b: string): boolean {
  if (!a || a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}

/** One instance per deployment slot, so the token bucket inside the Go sender
 *  is genuinely global. */
function app(env: Env) {
  return getContainer(env.MARUM_APP, "singleton");
}

export default {
  async fetch(request: Request, env: Env): Promise<Response> {
    const url = new URL(request.url);

    if (url.pathname === `/tg/${env.WEBHOOK_PATH}`) {
      if (request.method !== "POST") return new Response(null, { status: 405 });

      const header = request.headers.get("X-Telegram-Bot-Api-Secret-Token");
      if (!secretsMatch(header, env.MARUM_WEBHOOK_SECRET)) {
        // Reject before parsing anything: an unauthenticated body is not worth
        // the CPU, and answering 401 tells Telegram nothing useful either way.
        return new Response(null, { status: 401 });
      }

      const forwarded = new Request(new URL("/tg/update", "http://container"), request);
      forwarded.headers.set("X-Marum-Service-Token", env.MARUM_SERVICE_TOKEN);
      return app(env).fetch(forwarded);
    }

    if (url.pathname === "/healthz" || url.pathname === "/readyz" || url.pathname === "/status") {
      return app(env).fetch(new Request(new URL(url.pathname, "http://container"), request));
    }

    // The admin interface is never exposed through the Worker. It is reached
    // over a private path, because a public admin login is an invitation.
    if (url.pathname.startsWith("/admin")) return new Response(null, { status: 404 });

    return env.ASSETS ? env.ASSETS.fetch(request) : new Response("Marum", { status: 200 });
  },

  /** The scheduler. Drains due reminders and runs maintenance; idempotent, so
   *  a duplicate tick is harmless and a missed one is caught by the next. */
  async scheduled(_event: ScheduledController, env: Env, ctx: ExecutionContext) {
    const tick = new Request(new URL("/internal/tick", "http://container"), { method: "POST" });
    tick.headers.set("X-Marum-Service-Token", env.MARUM_SERVICE_TOKEN);
    ctx.waitUntil(app(env).fetch(tick));
  },
};
