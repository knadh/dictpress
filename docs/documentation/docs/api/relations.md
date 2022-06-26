
### POST /api/entries/:fromID/relations/:toID
Adds a relation from an entry to another entry, making `fromID` entry the main entry and `toID` entry its definition.

#### Request
```bash
curl -u username:password 'http://localhost:9000/api/entries/1/relations/3' -X POST \
    -H 'Content-Type: application/json; charset=utf-8' \
    --data-binary @- << EOF
    {
        "types": ["noun"],
        "tags": ["my-tag"],
        "notes": "Optional notes",
        "weight": 2,
        "status": "enabled"
    }
EOF

```

**Response**
```json
{
    "data": true
}
```


#### Params
| Param     | Type   |                                                                                                                                     |
|-----------|------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `types`      | `[]string`   | One or more parts-of-speech types that describe the definition's (toID) relationship with the main entry. Example `noun\|verb`. |
| `tags`      | `[]string`   | Optional tags describing the relationship (definition). |
| `notes`      | `string`   | Optional notes describing the relationship (definition). |
| `weight`      | `int`   | Optional numerical weight to order the definition. If left empty, the definition is added to the end of any existing definitions. |
| `status`      | `string`   | `enabled` = Visible in public search and APIs.<br />`pending` = Pending moderation in the admin UI.<br />`disabled` = Hidden from public search and APIs. |




### PUT /api/entries/:id/relations/:relationID
Updates the properties of a relation between a main entry and a definition entry.
`:relationID` is the ID of the relation row in the `relations` table.
This is available in the `GET /entries/:id` API for all relations of an entry.

#### Request
```bash
curl -u username:password 'http://localhost:9000/api/entries/1/relations/:relationID' -X PUT \
    -H 'Content-Type: application/json; charset=utf-8' \
    --data-binary @- << EOF
    {
        "types": ["noun"],
        "tags": ["my-tag"],
        "notes": "Optional notes",
        "weight": 2,
        "status": "enabled"
    }
EOF

```

**Response**
```json
{
    "data": true
}
```


#### Params
| Param     | Type   |                                                                                                                                     |
|-----------|------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `types`      | `[]string`   | One or more parts-of-speech types that describe the definition's (toID) relationship with the main entry. Example `noun\|verb`. |
| `tags`      | `[]string`   | Optional tags describing the relationship (definition). |
| `notes`      | `string`   | Optional notes describing the relationship (definition). |
| `weight`      | `int`   | Optional numerical weight to order the definition. If left empty, the definition is added to the end of any existing definitions. |
| `status`      | `string`   | `enabled` = Visible in public search and APIs.<br />`pending` = Pending moderation in the admin UI.<br />`disabled` = Hidden from public search and APIs. |



### PUT /api/entries/:id/relations/weghts
Re-order the relations (definition entries) of a main entry

#### Request
```bash
curl -u username:password 'http://localhost:9000/api/entries/1/relations/weights' -X PUT \
    -H 'Content-Type: application/json; charset=utf-8' \
    --data-raw '[3, 4, 5]'

```

**Response**
```json
{
    "data": true
}
```


#### Params
Raw list of relation IDs in the desired order.



### DELETE /api/entries/:fromID/relations/:toID
Delete a relation between two entries. This removes the `:toID` as a definition from the `:fromID` main entry.

#### Request
```bash
curl -u username:password 'http://localhost:9000/api/entries/1/relations/3' -X DELETE
```

**Response**
```json
{
    "data": true
}
```

