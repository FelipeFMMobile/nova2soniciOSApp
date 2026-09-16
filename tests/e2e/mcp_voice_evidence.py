#!/usr/bin/env python3
"""Verify existing live captures; this command does NOT invoke AWS."""
import argparse
import json
import sqlite3
from pathlib import Path


def load(path):
    return [json.loads(line) for line in Path(path).read_text().splitlines() if line]


def results(events, name, status):
    matches = []
    for index, event in enumerate(events):
        tool = event.get("tool", {})
        if event.get("type") != "tool.result" or tool.get("name") != name:
            continue
        result = tool.get("result", {})
        if result.get("isError"):
            continue
        for content in result.get("content", []):
            value = json.loads(content["text"])
            if value.get("status") == status:
                matches.append((index, event, value))
    return matches


def check(condition, message):
    if not condition:
        raise ValueError(message)


def verify(prefix, database):
    captures = {name: load(f"{prefix}-{name}-events.jsonl") for name in ("create", "query", "retry", "refusal", "delete", "empty")}
    created = results(captures["create"], "notes.create", "created")
    replay = results(captures["retry"], "notes.create", "created")
    check(len(created) == len(replay) == 1, "missing successful creation/replay")
    note = created[0][2]["note"]
    check(replay[0][2]["note"] == note, "retry produced a different note")
    queried = results(captures["query"], "notes.list", "ok")
    check(any(note in value["notes"] for _, _, value in queried), "new conversation did not retrieve stored data")
    check(captures["query"][0]["sessionId"] != captures["create"][0]["sessionId"], "query reused creation conversation")
    spoken = " ".join(event.get("text", "") for event in captures["query"] if event.get("type") == "transcript" and event.get("role") == "ASSISTANT")
    check("7419" in spoken or "sete quatro um nove" in spoken, "response did not incorporate the retrieved code")
    check(not results(captures["refusal"], "notes.delete", "deleted"), "refusal deleted the note")
    pending = results(captures["delete"], "notes.delete", "confirmation_required")
    deleted = results(captures["delete"], "notes.delete", "deleted")
    check(len(pending) >= 1 and len(deleted) == 1, "deletion lacked pending/confirmed phases")
    pending_index, pending_event, _ = pending[0]
    deleted_index, _, value = deleted[0]
    check(value["id"] == note["id"], "deleted another note")
    confirmations = [(i, e) for i, e in enumerate(captures["delete"]) if e.get("type") == "transcript" and e.get("role") == "USER" and e.get("stage") == "FINAL" and e.get("text", "").lower().strip(" .!?") == "confirmo excluir"]
    check(any(pending_index < i < deleted_index and e["turnId"] != pending_event["turnId"] for i, e in confirmations), "no subsequent finalized USER confirmation before deletion")
    empty = results(captures["empty"], "notes.list", "ok")
    check(any(not value["notes"] for _, _, value in empty), "post-delete query did not return empty")
    with sqlite3.connect(f"file:{Path(database).resolve()}?mode=ro", uri=True) as db:
        check(db.execute("SELECT count(*) FROM notes").fetchone()[0] == 0, "test notes remain")
        counts = dict(db.execute("SELECT name,count(*) FROM operations GROUP BY name"))
        check(counts == {"notes.create": 1, "notes.delete": 1}, "duplicate/missing durable mutations")
        check(db.execute("SELECT count(*) FROM sessions WHERE ended_at IS NOT NULL").fetchone()[0] >= 1, "session closure not audited")
        check(db.execute("SELECT count(*) FROM tool_operations WHERE name='notes.list' AND state='completed'").fetchone()[0] >= 1, "tool result not audited")
    return {"retrieval_in_new_conversation": True, "retry_same_note": True, "refusal_no_deletion": True, "subsequent_voice_confirmation": True, "post_delete_empty": True, "durable_mutations": counts, "audit": True}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prefix", default="/private/tmp/sts-mcp")
    parser.add_argument("--db", default="/private/tmp/sts-mcp-live.sqlite")
    args = parser.parse_args()
    print(json.dumps(verify(args.prefix, args.db), indent=2))
