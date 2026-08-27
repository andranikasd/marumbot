#!/usr/bin/env python3
"""Push the dashboards in this directory to a Grafana instance.

The files under dashboards/ are the source of truth for both places Marum's
dashboards appear: docker compose provisions them from disk, and this pushes the
same files to Grafana Cloud.

Two properties matter and are checked rather than assumed:

  uid          A dashboard is identified by its uid. Push one without a uid and
               Grafana creates a NEW dashboard every run instead of updating the
               existing one, which is how a folder ends up with nine copies of
               three dashboards.

  datasources  Panels must bind through the ${ds_*} variables. A hard-coded uid
               belongs to exactly one Grafana, so a dashboard naming a local uid
               renders as "datasource not found" in Cloud while its data arrives
               perfectly.

Reads GRAFANA_URL and GRAFANA_TOKEN from the environment. The token is a Grafana
service account token (glsa_), not a Cloud access policy token.
"""
import json
import os
import pathlib
import sys
import urllib.error
import urllib.request

FOLDER_UID = "marum"
FOLDER_TITLE = "Marum"
LOCAL_UIDS = {"marum-prometheus", "marum-loki", "marum-tempo", "marum-pyroscope"}

here = pathlib.Path(__file__).parent / "grafana" / "dashboards"


def api(base, token, path, payload=None, method="GET"):
    body = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request(
        base.rstrip("/") + path,
        data=body,
        method=method,
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"},
    )
    try:
        return json.load(urllib.request.urlopen(req, timeout=30)), None
    except urllib.error.HTTPError as e:
        return None, f"{e.code} {e.read().decode()[:300]}"


def check(path, d):
    problems = []
    if not d.get("uid"):
        problems.append("no uid: every push would create a new dashboard rather than update")
    if not d.get("title"):
        problems.append("no title")
    hard = sorted(LOCAL_UIDS.intersection(json.dumps(d).split('"')))
    if hard:
        problems.append(f"hard-coded local datasource uid {hard}; use the ${{ds_*}} variables")
    return problems


def main():
    base, token = os.environ.get("GRAFANA_URL"), os.environ.get("GRAFANA_TOKEN")
    if not base or not token:
        print("GRAFANA_URL and GRAFANA_TOKEN are required", file=sys.stderr)
        return 2

    files = sorted(here.glob("*.json"))
    if not files:
        print(f"no dashboards found in {here}", file=sys.stderr)
        return 2

    loaded, failed = [], False
    for f in files:
        try:
            d = json.loads(f.read_text())
        except json.JSONDecodeError as e:
            print(f"::error file={f}::invalid JSON: {e}")
            failed = True
            continue
        for p in check(f, d):
            print(f"::error file={f}::{p}")
            failed = True
        loaded.append((f, d))
    # Validate every file before writing any, so a bad one does not leave the
    # instance holding half an update.
    if failed:
        return 1

    # Creating a folder that exists answers 409 or 412 depending on the version;
    # both mean "already there", which is the desired end state.
    _, err = api(base, token, "/api/folders", {"title": FOLDER_TITLE, "uid": FOLDER_UID}, "POST")
    if err and not err.startswith(("409", "412")):
        print(f"::warning::could not ensure folder: {err}")

    for f, d in loaded:
        d.pop("id", None)  # id is instance-local; uid identifies the dashboard
        r, err = api(base, token, "/api/dashboards/db", {
            "dashboard": d,
            "folderUid": FOLDER_UID,
            "overwrite": True,
            "message": "pushed from deploy/observability/grafana/dashboards",
        }, "POST")
        if err:
            print(f"::error file={f}::push failed: {err}")
            failed = True
        else:
            print(f"  ok  {r['uid']:16s} v{r['version']}  {r['url']}")

    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
