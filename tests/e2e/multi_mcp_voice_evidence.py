#!/usr/bin/env python3
"""Verify retained two-MCP live captures. Never invokes AWS."""
import argparse
import json
import sqlite3
from pathlib import Path


def check(condition, message):
    if not condition:
        raise ValueError(message)


def load(path):
    return [json.loads(line) for line in Path(path).read_text().splitlines() if line]


def verify(directory):
    root = Path(directory)
    events = load(root / "query-2-events.jsonl")
    private = load(root / "private-tools.jsonl")
    session = next(e["sessionId"] for e in events if e.get("sessionId"))
    calls = [(i, e["tool"]) for i, e in enumerate(events) if e["type"] == "tool.result"]
    check(len(calls) == 3, "expected one Notes and two Agenda read calls")
    counts = {}
    for _, tool in calls:
        name, identifier = tool["name"], tool["operationId"]
        counts[name] = counts.get(name, 0) + 1
        result = tool["result"]
        check(not result.get("isError"), "tool failed")
        check(any(p["session_id"] == session and p["operation_id"] == identifier
                  and p["result"] == result for p in private), "private MCP evidence mismatch")
        check(any(e["type"] == "tool.started" and e["tool"]["operationId"] == identifier
                  and e["tool"]["name"] == name.replace(".", "_") for e in events),
              "Nova toolUse/host result correlation missing")
        value = json.loads(result["content"][0]["text"])
        check(value["status"] == "ok", "not a completed query")
        if name == "memo.notes.list":
            check(any(n["title"] == "Aurora" and "9274" in n["content"] for n in value["notes"]),
                  "stored code not retrieved")
        elif name == "local.agenda.list_events":
            event = next(e for e in value["events"] if e["id"] == "fixture-2030")
            check(event["start"] == "2030-05-16T13:00:00Z"
                  and event["start_local"] == "2030-05-16T10:00:00-03:00"
                  and event["end_local"] == "2030-05-16T11:00:00-03:00", "wrong UTC/local time")
        else:
            raise ValueError("unexpected mutation/tool")
    check(counts == {"memo.notes.list": 1, "local.agenda.list_events": 2}, "wrong tool selection")
    transcripts = [(i, e["text"]) for i, e in enumerate(events)
                   if e["type"] == "transcript" and e.get("role") == "ASSISTANT"]
    spoken = " ".join(text for _, text in transcripts).lower()
    check("9274" in spoken and "dez horas às onze horas" in spoken, "spoken response mismatch")
    check(min(i for i, _ in transcripts) > max(i for i, _ in calls), "answer preceded tool results")
    with sqlite3.connect(f"file:{(root / 'agenda.sqlite').resolve()}?mode=ro", uri=True) as db:
        check(db.execute("SELECT count(*) FROM operations").fetchone()[0] == 0, "agenda mutated")
        check(db.execute("SELECT start,end FROM agenda_events WHERE id='fixture-2030'").fetchone()
              == (1905166800, 1905170400), "SQLite fixture changed")
    with sqlite3.connect(f"file:{(root / 'audit.sqlite').resolve()}?mode=ro", uri=True) as db:
        check(db.execute("SELECT count(*) FROM tool_operations WHERE session_id=? AND state='completed'",
                         (session,)).fetchone()[0] == 3, "audit missing completed calls")
    return {"real_nova_two_mcp_selection": True, "calls": counts,
            "spoken_code": "9274", "local_event": "2030-05-16 10:00–11:00 America/Sao_Paulo",
            "private_correlation": True, "agenda_mutations": 0}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory")
    print(json.dumps(verify(parser.parse_args().directory), indent=2, ensure_ascii=False))
