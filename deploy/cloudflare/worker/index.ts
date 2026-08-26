/**
 * The Cloudflare edge for Marum.
 *
 * Three jobs, and nothing else: verify that a request really came from
 * Telegram, keep one noisy chat from drowning the rest, and hand the update to
 * the Go application. Anything that needs to look at a loan happens in Go.
 */
import { Container, getContainer } from "@cloudflare/containers";

export class MarumApp extends Container<Env> {
  defaultPort = 8080;
  // Long enough that a borrower mid-conversation does not pay a cold start,
  // short enough that an idle night costs nothing.
  sleepAfter = "10m";

  /**
   * The container is a plain Go binary that reads its configuration from the
   * environment, exactly as it does under `make up`. Nothing is baked into the
   * image, so the same image runs in dev and production.
   *
   * The database URL comes from the Hyperdrive binding rather than a secret:
   * Hyperdrive hands back a local connection string that points at the pool,
   * not at Neon, so the origin credentials never enter the container at all.
   */
  constructor(ctx: DurableObjectState<Env>, env: Env) {
    super(ctx, env);
    this.envVars = {
      MARUM_ENV: env.ENVIRONMENT,
      // The edge already owns the transport. Long polling here would mean two
      // readers of the same bot and a race for every update.
      MARUM_MODE: "webhook",
      MARUM_DATABASE_URL: env.HYPERDRIVE.connectionString,
      MARUM_BOT_TOKEN: env.MARUM_BOT_TOKEN,
      MARUM_WEBHOOK_SECRET: env.MARUM_WEBHOOK_SECRET,
      MARUM_IDENTITY_KEY: env.MARUM_IDENTITY_KEY,
      // Absent leaves the admin listener down rather than unauthenticated,
      // which is the behaviour we want if the secret was never set.
      MARUM_ADMIN_USER: env.MARUM_ADMIN_USER ?? "admin",
      MARUM_ADMIN_PASSWORD_HASH: env.MARUM_ADMIN_PASSWORD_HASH ?? "",
      // Empty disables telemetry outright, so a missing endpoint degrades to a
      // quiet app rather than a crash loop.
      OTEL_EXPORTER_OTLP_ENDPOINT: env.OTEL_EXPORTER_OTLP_ENDPOINT ?? "",
      OTEL_EXPORTER_OTLP_HEADERS: env.OTEL_EXPORTER_OTLP_HEADERS ?? "",
      OTEL_EXPORTER_OTLP_PROTOCOL: "http/protobuf",
      OTEL_SERVICE_NAME: "marum",
      OTEL_RESOURCE_ATTRIBUTES: `deployment.environment=${env.ENVIRONMENT},service.namespace=marum`,
    };
  }
}

interface Env {
  MARUM_APP: DurableObjectNamespace<MarumApp>;
  ASSETS: Fetcher;
  HYPERDRIVE: Hyperdrive;
  MARUM_WEBHOOK_SECRET: string;
  MARUM_SERVICE_TOKEN: string;
  MARUM_BOT_TOKEN: string;
  MARUM_IDENTITY_KEY: string;
  MARUM_ADMIN_USER?: string;
  MARUM_ADMIN_PASSWORD_HASH?: string;
  OTEL_EXPORTER_OTLP_ENDPOINT?: string;
  OTEL_EXPORTER_OTLP_HEADERS?: string;
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
