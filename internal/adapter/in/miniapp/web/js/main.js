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
