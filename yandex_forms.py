"""Create two Yandex Forms for Business and import their responses into Pulse."""
import argparse
import csv
import io
import json
import os
import re
import sys
import time
import uuid
from pathlib import Path
from urllib.error import HTTPError
from urllib.parse import urljoin, urlsplit
from urllib.request import Request, build_opener, HTTPRedirectHandler

YANDEX = "https://api.forms.yandex.net"
ROLES = {"intern": "Стажёр", "mentor": "Наставник"}


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def request(url, headers, body=None):
    if isinstance(body, dict):
        body = json.dumps(body, ensure_ascii=False).encode()
        headers = {**headers, "Content-Type": "application/json"}
    req = Request(url, data=body, headers=headers)
    with build_opener(NoRedirect()).open(req, timeout=60) as response:
        data = response.read()
        return json.loads(data) if data else {}


def schema(survey, role):
    required = [{"type": "required"}]
    def number(label, maximum):
        return {"type": "integer", "label": label, "validators": required + [
            {"type": "min", "value": 1}, {"type": "max", "value": maximum}]}
    fields = {}
    if role == "mentor":
        fields["mentor_id"] = {"type": "string", "label": "ID наставника", "validators": required}
    fields["intern_id"] = {"type": "string", "label": "ID стажера", "validators": required}
    fields["track"] = {"type": "string", "label": "Подразделение / направление стажировки", "validators": required}
    fields["period"] = number("Контрольный период", survey["periods"])
    for q in survey["questions"]:
        if q["role"] == role:
            fields[q["code"]] = (number(q["text"], int(q["max"])) if q["kind"] == "scale" else
                                 {"type": "string", "label": q["text"], "multiline": True,
                                  "validators": [{"type": "max", "value": 2000}]})
    return fields


def create(survey, forms, api, save):
    for role, label in ROLES.items():
        if role not in forms:
            form = api("/v1/surveys/", {"name": survey["title"] + " · " + label})
            forms[role] = {"id": form["id"], "questions": {}, "published": False}
            save(forms)
        form = forms[role]
        path = form_path(form)
        for code, question in schema(survey, role).items():
            if code not in form["questions"]:
                result = api(path + "/questions/", question)
                form["questions"][code] = str(result["id"])
                save(forms)
        if not form["published"]:
            api(path + "/publish/", {})
            form["published"] = True
            save(forms)


def form_path(form):
    if not re.fullmatch("[a-fA-F0-9]{24}", str(form["id"])):
        raise ValueError("Неверный идентификатор формы.")
    return "/v1/surveys/" + form["id"]


def responses(form, role, survey, api):
    columns = schema(survey, role)
    if set(form["questions"]) != set(columns):
        raise ValueError("Состав вопросов формы отличается от анкеты дашборда.")
    path = form_path(form) + "/answers"
    url = path + "?ordering=asc&page_size=50"
    seen = set()
    latest = {}
    known = {p[role]:p for p in survey.get("pairs") or []}
    while url:
        parts = urlsplit(urljoin(YANDEX, url))
        if (parts.scheme != "https" or parts.netloc != "api.forms.yandex.net" or
                parts.path.rstrip("/") != path or parts.geturl() in seen):
            raise ValueError("Некорректная ссылка следующей страницы Яндекс Форм.")
        seen.add(parts.geturl())
        page = api(parts.path + "?" + parts.query)
        ids = [str(c["id"]) for c in page["columns"]]
        if not set(form["questions"].values()) <= set(ids):
            raise ValueError("В ответе Яндекса отсутствуют настроенные столбцы.")
        for answer in page["answers"]:
            values = dict(zip(ids, [item.get("value") if item else None for item in answer["data"]], strict=True))
            row = {code: values[qid] for code, qid in form["questions"].items()}
            person = row[role + "_id"]
            if person not in known:
                raise ValueError(f"Ответ {answer['id']}: неизвестный код участника для этой роли.")
            pair = known[person]
            if row["intern_id"] != pair["intern"] or str(row["track"]).strip().casefold() != pair["track"].casefold():
                raise ValueError(f"Ответ {answer['id']}: проверьте ID стажёра и подразделение по справочнику.")
            for code, field in columns.items():
                value = row[code]
                if field["type"] == "integer":
                    if type(value) is not int or not 1 <= value <= field["validators"][-1]["value"]:
                        raise ValueError(f"Ответ {answer['id']}: значение {code} вне шкалы.")
                elif value is not None and (not isinstance(value, str) or len(value) > 2000):
                    raise ValueError(f"Ответ {answer['id']}: неверное текстовое значение.")
            key = (person, row["period"])
            order = (answer["created"], int(answer["id"]))
            if key not in latest or order > latest[key][0]:
                latest[key] = (order, row)
        url = (page.get("next") or {}).get("next_url")
    return [value[1] for value in latest.values()]


def upload(role, rows, pulse, headers):
    if not rows:
        return 0
    text = io.StringIO(newline="")
    writer = csv.DictWriter(text, fieldnames=list(rows[0]))
    writer.writeheader()
    writer.writerows(rows)
    boundary = uuid.uuid4().hex
    start = f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="yandex-{role}.csv"\r\nContent-Type: text/csv\r\n\r\n'
    end = f'\r\n--{boundary}\r\nContent-Disposition: form-data; name="source"\r\n\r\nyandex-forms\r\n--{boundary}--\r\n'
    body = start.encode() + text.getvalue().encode("utf-8-sig") + end.encode()
    result = request(pulse + "/api/v1/imports", {**headers, "Content-Type": "multipart/form-data; boundary="+boundary}, body)
    if result["report"]["accepted"] != len(rows) or result["report"]["rejected"]:
        raise ValueError("Часть строк отклонена. Проверьте справочник пар и состав анкеты.")
    return result["changed"]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("create", "sync"))
    parser.add_argument("--config", type=Path, default=Path("yandex-forms.json"))
    parser.add_argument("--pulse", default="http://127.0.0.1:8260")
    parser.add_argument("--interval", type=int, metavar="SECONDS", help="повторять sync с заданным интервалом")
    args = parser.parse_args()
    if args.interval is not None and (args.command != "sync" or args.interval <= 0):
        parser.error("--interval применяется только к sync и должен быть положительным")
    pulse = args.pulse.rstrip("/")
    target = urlsplit(pulse)
    if target.scheme != "https" and not (target.scheme == "http" and target.hostname in ("127.0.0.1","localhost","::1")):
        raise ValueError("Для удалённого дашборда нужен HTTPS.")
    headers = {"Authorization":"OAuth " + os.environ["FORMS_TOKEN"], "X-Org-Id":os.environ["ORG_ID"]}
    auth = {"Authorization":"Bearer " + os.environ["HR_TOKEN"]} if os.environ.get("HR_TOKEN") else {}
    api = lambda path, body=None: request(YANDEX + path, headers, body)
    forms = json.loads(args.config.read_text(encoding="utf-8-sig")) if args.config.exists() else {}
    if args.command == "create":
        survey = request(pulse + "/api/v1/survey/export", auth)
        def save(data):
            temp = args.config.with_suffix(".tmp")
            temp.write_text(json.dumps(data, indent=2), encoding="utf-8")
            temp.replace(args.config)
        create(survey, forms, api, save)
        for role, form in forms.items():
            print(ROLES[role], "https://forms.yandex.ru/cloud/u/" + form["id"] + "/")
    else:
        while True:
            try:
                survey = request(pulse + "/api/v1/survey/export", auth)
                batches = {role: responses(forms[role], role, survey, api) for role in ROLES}
                for role, rows in batches.items():
                    print(ROLES[role], "анкет:", len(rows), "изменено:", upload(role, rows, pulse, auth), flush=True)
            except OSError as error:
                if not args.interval or isinstance(error, HTTPError) and error.code < 500 and error.code != 429:
                    raise
                print("Сервис временно недоступен, повтор после интервала.", file=sys.stderr, flush=True)
            if not args.interval:
                break
            time.sleep(args.interval)


if __name__ == "__main__":
    try:
        main()
    except (OSError, KeyError, ValueError) as error:
        sys.exit(f"Ошибка синхронизации: {error}")
