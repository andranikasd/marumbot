// Boot. The screens are plug-ins: importing one registers it, and the order
// of imports is the order of the tabs. Adding a screen is one file under
// screens/ and one import here.
"use strict";
import "./screens/loans.js";
import "./screens/add.js";
import "./screens/budget.js";
import "./screens/plan.js";
import { buildTabs, go } from "./nav.js";
import { prefetch } from "./api.js";

buildTabs();

// The build badge: the one honest answer to "which version am I looking
// at". It reads the stamp off this module's own URL, so a cached copy
// names itself.
const stamp = (new URL(import.meta.url).pathname.match(/\/a\/([^/]+)\//) || [, "unstamped"])[1];
document.getElementById("build").textContent = "Marum " + stamp;

// Staleness. Telegram keeps a minimised Mini App alive across deploys and
// reopens the same instance, so a deploy is invisible until the page asks
// what is deployed and reloads itself. Asked on boot and each time the app
// comes back to the foreground; the reload goes to a URL that names the new
// build, so a webview that ignored no-store still fetches afresh. One reload
// per target version, so a server that disagrees forever cannot loop.
async function checkBuild() {
  try {
    const res = await fetch("version", { cache: "no-store" });
    if (!res.ok) return;
    const { version } = await res.json();
    if (!version || version === decodeURIComponent(stamp)) return;
    const key = "marum.reloaded-to";
    if (sessionStorage.getItem(key) === version) return;
    sessionStorage.setItem(key, version);
    const next = new URL(location.href);
    next.searchParams.set("v", version);
    location.replace(next.toString());
  } catch { /* offline, or an older server without the endpoint */ }
}
checkBuild();
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") checkBuild();
});
window.Telegram?.WebApp?.onEvent?.("activated", checkBuild);

// Warm every screen's data in parallel while the first one renders, so a
// tab switch lands on a ready screen instead of a spinner.
prefetch(["api/loans", "api/budget", "api/plan"]);

const requested = new URLSearchParams(location.search).get("screen");
go(
  requested === "budget" ? "budget"
    : requested === "plan" ? "plan"
      : requested === "loan" || requested === "add" ? "add"
        : "loans",
);
