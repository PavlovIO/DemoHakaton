#!/usr/bin/env python3
"""Создаёт метки, вехи и issues из scripts/issues.yaml в GitHub-репозитории.

Использование:
    python3 scripts/create_issues.py --dry-run                 # проверить YAML и показать план
    python3 scripts/create_issues.py --repo OWNER/NAME         # создать (нужен авторизованный `gh`)

Скрипт идемпотентен:
- метки и вехи создаются, только если их нет;
- issue ищется по префиксу заголовка «[KEY]», существующие не дублируются;
- тело созданного в этом запуске issue пересобирается: {{KEY}} → #номер, внизу — обратные ссылки
  на сценарии; существующие issues не трогаются, чтобы не затереть ручные правки в GitHub.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
import time
from pathlib import Path

import yaml

ROOT = Path(__file__).resolve().parent
REF_RE = re.compile(r"\{\{([A-Z]+-\d+)\}\}")
KEY_TITLE_RE = re.compile(r"^\[([A-Z]+-\d+)\]")


def load(path: Path) -> dict:
    data = yaml.safe_load(path.read_text(encoding="utf-8"))
    keys = [i["key"] for i in data["issues"]]
    errors: list[str] = []
    dupes = {k for k in keys if keys.count(k) > 1}
    if dupes:
        errors.append(f"повторяющиеся ключи: {sorted(dupes)}")
    label_names = {l["name"] for l in data["labels"]}
    ms_keys = {m["key"] for m in data["milestones"]}
    for issue in data["issues"]:
        for ref in REF_RE.findall(issue["body"]):
            if ref not in keys:
                errors.append(f"{issue['key']}: ссылка на несуществующий {ref}")
        for label in issue["labels"]:
            if label not in label_names:
                errors.append(f"{issue['key']}: неизвестная метка {label}")
        if issue.get("milestone") and issue["milestone"] not in ms_keys:
            errors.append(f"{issue['key']}: неизвестная веха {issue['milestone']}")
    if errors:
        sys.exit("Ошибки в issues.yaml:\n  " + "\n  ".join(errors))
    return data


def backlinks(data: dict) -> dict[str, list[str]]:
    """Для каждой задачи — список сценариев (US-*), которые на неё ссылаются."""
    result: dict[str, list[str]] = {}
    for issue in data["issues"]:
        if not issue["key"].startswith("US-"):
            continue
        for ref in REF_RE.findall(issue["body"]):
            result.setdefault(ref, []).append(issue["key"])
    return result


def gh(method: str, path: str, payload: dict | None = None, paginate: bool = False) -> list | dict:
    cmd = ["gh", "api", "--method", method, path]
    if paginate:
        cmd += ["--paginate", "--jq", ".[]"]
    if payload is not None:
        cmd += ["--input", "-"]
    # Обрыв на TLS-рукопожатии — запрос ещё не ушёл, повтор безопасен и для POST.
    for attempt in range(5):
        proc = subprocess.run(cmd, input=json.dumps(payload) if payload is not None else None,
                              capture_output=True, text=True, encoding="utf-8")  # на Windows иначе cp1251
        if proc.returncode == 0 or "TLS handshake timeout" not in proc.stderr:
            break
        time.sleep(2 * (attempt + 1))
    if proc.returncode != 0:
        raise RuntimeError(f"gh api {method} {path} failed: {proc.stderr.strip() or proc.stdout.strip()}")
    if paginate:
        return [json.loads(line) for line in proc.stdout.splitlines() if line.strip()]
    return json.loads(proc.stdout) if proc.stdout.strip() else {}


def render_body(issue: dict, numbers: dict[str, int], links: dict[str, list[str]]) -> str:
    def ref(m: re.Match) -> str:
        key = m.group(1)
        return f"#{numbers[key]}" if key in numbers else f"`{key}`"

    body = REF_RE.sub(ref, issue["body"]).rstrip()
    parents = links.get(issue["key"])
    if parents:
        refs = ", ".join(f"#{numbers[p]}" if p in numbers else p for p in parents)
        body += f"\n\n---\n**Сценарии:** {refs}"
    return body + f"\n\n<sub>Ключ: `{issue['key']}` · создано из `scripts/issues.yaml`</sub>"


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--repo", default="MaxPastukhov15/Liquidity_platform")
    parser.add_argument("--file", default=str(ROOT / "issues.yaml"))
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args()

    data = load(Path(args.file))
    links = backlinks(data)

    if args.dry_run:
        print(f"OK: {len(data['issues'])} issues, {len(data['labels'])} меток, {len(data['milestones'])} вех")
        by_ms: dict[str, list[str]] = {}
        for i in data["issues"]:
            by_ms.setdefault(i.get("milestone", "—"), []).append(f"[{i['key']}] {i['title']}")
        for ms in sorted(by_ms):
            print(f"\n{ms}:")
            for line in by_ms[ms]:
                print("  ", line)
        orphans = [i["key"] for i in data["issues"]
                   if not i["key"].startswith("US-") and i["key"] not in links]
        print(f"\nЗадачи без сценария (инфраструктура / сквозные): {', '.join(orphans) or '—'}")
        return

    repo = f"repos/{args.repo}"

    # 1. Метки
    existing_labels = {l["name"] for l in gh("GET", f"{repo}/labels", paginate=True)}
    for label in data["labels"]:
        if label["name"] not in existing_labels:
            gh("POST", f"{repo}/labels", label)
            print("label  +", label["name"])

    # 2. Вехи
    milestones = {m["title"]: m["number"] for m in gh("GET", f"{repo}/milestones?state=all", paginate=True)}
    ms_numbers: dict[str, int] = {}
    for ms in data["milestones"]:
        if ms["title"] not in milestones:
            created = gh("POST", f"{repo}/milestones", {
                "title": ms["title"],
                "description": ms["description"],
                "due_on": f"{ms['due']}T20:59:59Z",  # 23:59 по Москве
            })
            milestones[ms["title"]] = created["number"]
            print("milestone +", ms["title"])
        ms_numbers[ms["key"]] = milestones[ms["title"]]

    # 3. Существующие issues по ключу
    numbers: dict[str, int] = {}
    for item in gh("GET", f"{repo}/issues?state=all", paginate=True):
        if "pull_request" in item:
            continue
        m = KEY_TITLE_RE.match(item["title"])
        if m:
            numbers[m.group(1)] = item["number"]

    # 4. Создаём недостающие (тело — временное, ссылки проставим во втором проходе)
    created_issues: list[dict] = []
    for issue in data["issues"]:
        if issue["key"] in numbers:
            print(f"skip   #{numbers[issue['key']]} [{issue['key']}]")
            continue
        payload = {
            "title": f"[{issue['key']}] {issue['title']}",
            "body": render_body(issue, numbers, links),
            "labels": issue["labels"],
        }
        if issue.get("milestone"):
            payload["milestone"] = ms_numbers[issue["milestone"]]
        created = gh("POST", f"{repo}/issues", payload)
        numbers[issue["key"]] = created["number"]
        created_issues.append(issue)
        print(f"issue  + #{created['number']} [{issue['key']}] {issue['title']}")
        time.sleep(1)  # бережём secondary rate limit

    # 5. Второй проход: проставляем ссылки #N в телах созданных issues
    for issue in created_issues:
        gh("PATCH", f"{repo}/issues/{numbers[issue['key']]}", {"body": render_body(issue, numbers, links)})
    print(f"\nГотово: создано {len(created_issues)}, всего {len(numbers)} issues; ссылки в новых проставлены.")


if __name__ == "__main__":
    main()
