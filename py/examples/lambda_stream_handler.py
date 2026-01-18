from __future__ import annotations

from dataclasses import dataclass

from theorydb_py import ModelDefinition, theorydb_field, unmarshal_stream_record


@dataclass(frozen=True)
class Note:
    pk: str = theorydb_field(roles=["pk"])
    sk: str = theorydb_field(roles=["sk"])
    value: int = theorydb_field()


MODEL = ModelDefinition.from_dataclass(Note)


def handler(event, context):  # noqa: ANN001, ARG001
    records = event.get("Records", [])
    for record in records:
        note = unmarshal_stream_record(MODEL, record, image="NewImage")
        if note is None:
            continue
        print("note:", note)
