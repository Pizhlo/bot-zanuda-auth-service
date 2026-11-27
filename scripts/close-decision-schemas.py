"""Apply the strict decision-request contract after swag has generated all three formats."""

import json
import sys
from pathlib import Path

docs = Path(sys.argv[1])
swagger = json.loads((docs / "swagger.json").read_text())
definitions = swagger["definitions"]
request = swagger["paths"]["/api/v0/decisions/check"]["post"]["parameters"]
root = next(p["schema"]["$ref"] for p in request if p["in"] == "body")
pending = [root.rsplit("/", 1)[1]]
closed = set()
while pending:
    name = pending.pop()
    schema = definitions[name]
    if name in closed or schema.get("type") != "object" or "properties" not in schema:
        continue
    closed.add(name)
    for field in schema["properties"].values():
        if "$ref" in field:
            pending.append(field["$ref"].rsplit("/", 1)[1])

# Only fixed request objects are closed. Map schemas and response DTOs keep their own contract.
for filename in ("swagger.json", "swagger.yaml", "docs.go"):
    path = docs / filename
    data = path.read_text()
    for name in sorted(closed):
        if filename.endswith(".yaml"):
            header = f"  {name}:\n"
            restriction = "    additionalProperties: false\n"
        else:
            header = f'        "{name}": {{\n'
            restriction = '            "additionalProperties": false,\n'
        if data.count(header) != 1:
            raise SystemExit(f"{path}: expected one definition for {name}")
        if header + restriction not in data:
            data = data.replace(header, header + restriction)
    path.write_text(data)
