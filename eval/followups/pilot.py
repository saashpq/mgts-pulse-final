"""Aggregate a snapshot from GET /api/v1/follow-ups without exporting people or comments."""
import argparse
import json
from datetime import date, datetime, timezone, timedelta
from pathlib import Path

MOSCOW = timezone(timedelta(hours=3))
OUTCOMES = ('done', 'partial', 'no_change', 'unknown')


def moscow_day(timestamp, snapshot_day):
    recorded = datetime.fromisoformat(timestamp.replace('Z', '+00:00'))
    if recorded.tzinfo is None:
        raise ValueError('Timestamp must include its time zone')
    day = recorded.astimezone(MOSCOW).date()
    if day > snapshot_day:
        raise ValueError('Timestamp is later than the snapshot')
    return day


def summarize(queue):
    today = date.fromisoformat(queue['today'])
    counts = {outcome: 0 for outcome in OUTCOMES}
    delivery_done = contacts = deadlines = on_time = supported = overdue_open = late_moves = 0
    for item in queue['items']:
        for move in item.get('reschedules', []):
            if date.fromisoformat(move['due_date']) < moscow_day(move['at'], today):
                deadlines += 1
                late_moves += 1
        for check in item['checks']:
            due = date.fromisoformat(check['due_date'])
            recorded_day = moscow_day(check['at'], today)
            outcome = check['outcome']
            counts[outcome] += 1
            contacts += 1
            delivery_done += check['delivery'] == 'done'
            if due <= today:
                deadlines += 1
                if recorded_day <= due:
                    on_time += 1
                    supported += outcome != 'unknown'
        if item['status'] == 'open':
            due = date.fromisoformat(item['due_date'])
            if due <= today:
                deadlines += 1
            if due < today:
                overdue_open += 1
    return {
        'as_of_moscow': today.isoformat(), 'arrangements':len(queue['items']),
        'due_contacts':deadlines, 'on_time_records':on_time, 'on_time_known_outcomes':supported,
        'open_overdue':overdue_open, 'moved_after_deadline':late_moves, 'recorded_contacts':contacts,
        'action_completed_at_contact':delivery_done, 'reported_outcomes':counts,
        'known_outcomes':contacts-counts['unknown']
    }


def fraction(a,b):
    return f'{a}/{b} ({100*a/b:.0f}%)' if b else 'нет наблюдений (0/0)'


def render(result):
    outcomes=result['reported_outcomes']
    return '\n'.join([
        f'Контроль договорённостей на {result["as_of_moscow"]} по Москве',
        f'Договорённостей: {result["arrangements"]}. Открытых с истёкшим сроком: {result["open_overdue"]}.',
        f'Сроков, перенесённых после истечения: {result["moved_after_deadline"]}.',
        'Контакт отмечен вовремя среди наступивших сроков: '+fraction(result['on_time_records'],result['due_contacts']),
        'Исход известен вовремя среди наступивших сроков: '+fraction(result['on_time_known_outcomes'],result['due_contacts']),
        'Действие выполнено среди записанных контактов: '+fraction(result['action_completed_at_contact'],result['recorded_contacts']),
        'Участник подтвердил пользу среди известных исходов: '+fraction(outcomes['done'],result['known_outcomes']),
        f'Все исходы: помогло {outcomes["done"]}, частично {outcomes["partial"]}, без улучшения {outcomes["no_change"]}, нет ответа о результате {outcomes["unknown"]}.'
    ])


if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument('snapshot',type=Path)
    p.add_argument('--json',action='store_true')
    args=p.parse_args()
    result=summarize(json.loads(args.snapshot.read_text('utf-8-sig')))
    print(json.dumps(result,ensure_ascii=False,indent=2) if args.json else render(result))
