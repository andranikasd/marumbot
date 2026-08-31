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
  // 8080 is the public surface, 8081 the admin interface. Both must be up
  // before the container counts as ready, otherwise the first admin request
  // races the listener.
  requiredPorts = [8080, 8081];
  // Long enough that a borrower mid-conversation does not pay a cold start,
  // short enough that an idle night costs nothing.
  sleepAfter = "10m";

  /**
   * The container is a plain Go binary that reads its configuration from the
   * environment, exactly as it does under `make up`. Nothing is baked into the
   * image, so the same image runs in dev and production.
   *
   * The database URL is a secret, NOT env.HYPERDRIVE.connectionString.
   *
   * Hyperdrive hands back a hostname of the form <id>.hyperdrive.local, and
   * that name only resolves inside the Workers runtime. A container is a
   * separate network sandbox, so the lookup fails immediately and the Go
   * process exits before it can bind a port -- which surfaces as the thoroughly
   * misleading "Container crashed while checking for ports". Cloudflare confirm
   * the limitation in cloudflare/containers#97: "hyperdrive from inside a
   * container isn't supported currently - its on the roadmap".
   *
   * Losing Hyperdrive here costs less than it looks. Hyperdrive exists because
   * a Worker holds no persistent connections and can start anywhere, so every
   * isolate would otherwise handshake afresh. The container is the opposite: a
   * long-lived process with its own pgxpool. It needs the direct, unpooled
   * connection string precisely so it can hold that pool itself.
   */
  constructor(ctx: DurableObjectState<Env>, env: Env) {
    super(ctx, env);
    this.envVars = {
      MARUM_ENV: env.ENVIRONMENT,
      // Without this every deployed container reports as "local-1", the config
      // default, so telemetry from Cloudflare is indistinguishable from a
      // laptop's. The Durable Object id is stable for the lifetime of the
      // instance and unique across them, which is exactly what a
      // service.instance.id should be.
      MARUM_INSTANCE_ID: `${env.ENVIRONMENT}-${ctx.id.toString().slice(0, 12)}`,
      // The edge already owns the transport. Long polling here would mean two
      // readers of the same bot and a race for every update.
      MARUM_MODE: "webhook",
      // The deployed version, for telemetry and for the Mini App cache stamp:
      // every URL the bot hands out carries it, so a deploy is a new URL.
      MARUM_VERSION: env.VERSION ?? "dev",
      MARUM_DATABASE_URL: env.MARUM_DATABASE_URL,
      MARUM_BOT_TOKEN: env.MARUM_BOT_TOKEN,
      MARUM_WEBHOOK_SECRET: env.MARUM_WEBHOOK_SECRET,
      // Proves to the container that a request arrived through this Worker.
      MARUM_SERVICE_TOKEN: env.MARUM_SERVICE_TOKEN,
      // Absolute URL of the loan form. Without it /add has no button to offer
      // and answers with an error, which is what shipped: the secret existed,
      // the container never saw it, and the failure looked like a bug in the
      // command rather than a missing variable.
      MARUM_MINIAPP_URL: env.MARUM_MINIAPP_URL ?? "",
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
      // Grafana Cloud's Application Observability derives its `job` label as
      // `service.namespace/service.name`, so both must be set and neither may
      // contain a slash. deployment.environment is emitted under both the old
      // and the current OpenTelemetry semconv names: the old one is what
      // Grafana still documents, the new one is what newer tooling reads, and
      // baselines do not work without it.
      OTEL_RESOURCE_ATTRIBUTES: [
        `deployment.environment=${env.ENVIRONMENT}`,
        `deployment.environment.name=${env.ENVIRONMENT}`,
        "service.namespace=marum",
      ].join(","),
      // Continuous profiling. The Go side has been wired for this since the
      // observability work, but nothing was passing the address, so Grafana had
      // traces, metrics and logs and a permanently empty profiles tab.
      PYROSCOPE_SERVER_ADDRESS: env.PYROSCOPE_SERVER_ADDRESS ?? "",
      PYROSCOPE_BASIC_AUTH_USER: env.PYROSCOPE_BASIC_AUTH_USER ?? "",
      PYROSCOPE_BASIC_AUTH_PASSWORD: env.PYROSCOPE_BASIC_AUTH_PASSWORD ?? "",
    };
  }
}

interface Env {
  MARUM_APP: DurableObjectNamespace<MarumApp>;
  ASSETS: Fetcher;
  // Bound and provisioned, but unused until Cloudflare ships Hyperdrive support
  // for containers. Kept so that becomes a one-line change rather than a
  // Terraform migration.
  HYPERDRIVE: Hyperdrive;
  MARUM_DATABASE_URL: string;
  MARUM_WEBHOOK_SECRET: string;
  MARUM_SERVICE_TOKEN: string;
  MARUM_BOT_TOKEN: string;
  MARUM_IDENTITY_KEY: string;
  MARUM_ADMIN_USER?: string;
  MARUM_ADMIN_PASSWORD_HASH?: string;
  MARUM_MINIAPP_URL?: string;
  OTEL_EXPORTER_OTLP_ENDPOINT?: string;
  OTEL_EXPORTER_OTLP_HEADERS?: string;
  PYROSCOPE_SERVER_ADDRESS?: string;
  PYROSCOPE_BASIC_AUTH_USER?: string;
  PYROSCOPE_BASIC_AUTH_PASSWORD?: string;
  WEBHOOK_PATH: string;
  VERSION?: string;
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

    // The Mini App and its API. Forwarded whole -- path, query and body -- so
    // the Go handler sees the request the browser actually made.
    //
    // No service token here, deliberately. Every call the form makes carries
    // Telegram's signed initData, which proves far more than a shared secret
    // would: not merely that the request came through this Worker, but which
    // Telegram user made it.
    if (url.pathname === "/app" || url.pathname.startsWith("/app/")) {
      // The API is the container's; everything else under /app is a static
      // file and is served from the edge — no container hop, no cold start,
      // and the versioned prefix makes it immutable.
      if (url.pathname.startsWith("/app/api/")) {
        const res = await app(env).containerFetch(
          new Request(new URL(url.pathname + url.search, "http://container"), request));
        const headers = new Headers(res.headers);
        headers.set("Cache-Control", "no-store, no-cache, must-revalidate");
        return new Response(res.body, { status: res.status, statusText: res.statusText, headers });
      }
      if (env.ASSETS) {
        let rest = url.pathname.slice("/app".length) || "/";
        let immutable = false;
        const versioned = rest.match(/^\/a\/[^/]+(\/.*)$/);
        if (versioned) { rest = versioned[1]; immutable = true; }
        if (rest === "/") rest = "/index.html";
        const asset = await env.ASSETS.fetch(new Request(new URL(rest, request.url), request));
        const headers = new Headers(asset.headers);
        if (rest === "/index.html") {
          // The shell names its assets under the versioned prefix, exactly
          // as the container's own handler does.
          const v = encodeURIComponent(env.VERSION ?? "dev");
          const body = (await asset.text()).replace(
            /(href|src)="((?:js\/[^"]+|styles\.css))"/g, `$1="a/${v}/$2"`);
          headers.set("Cache-Control", "no-store");
          headers.set("Content-Type", "text/html; charset=utf-8");
          return new Response(body, { status: asset.status, headers });
        }
        headers.set("Cache-Control", immutable ? "public, max-age=31536000, immutable" : "no-store");
        return new Response(asset.body, { status: asset.status, headers });
      }
      // Self-hosted or assets not bound: the container serves the app.
      const res = await app(env).containerFetch(
        new Request(new URL(url.pathname + url.search, "http://container"), request));
      return res;
    }

    if (url.pathname === "/healthz" || url.pathname === "/readyz" || url.pathname === "/status") {
      return app(env).fetch(new Request(new URL(url.pathname, "http://container"), request));
    }

    // The admin interface listens on container port 8081 and serves from the
    // root: /login, /loans, /users. It gets its own hostname rather than a
    // /admin prefix, because stripping a prefix here would leave every link and
    // form action in the templates pointing at the public host.
    //
    // The hostname is one level deep (admin-dev.marum.loan, not
    // admin.dev.marum.loan) because Cloudflare's universal certificate covers
    // *.marum.loan and no deeper.
    //
    // Outside production this is reachable, because an admin interface nobody
    // can open is not an admin interface. It still sits behind the app's own
    // PBKDF2 login, and the listener stays down entirely when
    // MARUM_ADMIN_PASSWORD_HASH is unset -- so exposing the hostname cannot
    // produce an unauthenticated console.
    //
    // Production returns 404 here. A public login page there is a standing
    // invitation to credential stuffing, and the answer is Cloudflare Access in
    // front of the hostname rather than trusting one password.
    if (url.hostname.startsWith("admin-")) {
      if (env.ENVIRONMENT === "production") return new Response(null, { status: 404 });
      return app(env).containerFetch(request, 8081);
    }

    // No admin surface on the public hostname, in any environment.
    if (url.pathname === "/admin" || url.pathname.startsWith("/admin/")) {
      return new Response(null, { status: 404 });
    }

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
