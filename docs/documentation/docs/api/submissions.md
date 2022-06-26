# Public submissions

The submissions API is available unauthenticated, publicly, to accept new submissions from the public. These submissions go sit in the admin moderation queue for approva. Public submissions can be enabled or disabled in the config. 

### POST /api/submissions
Accept a public entry + definition submission and add to the admin moderation queue. Entries created via this have `pending` status in the entries table.


#### Request

```bash
curl -u username:password 'http://localhost:9000/api/submissions' -X POST \
    -H 'Content-Type: application/json; charset=utf-8' \
    --data-binary @- << EOF
    {
        "entry_lang": "english",
        "entry_content": "Apple",
        "entry_phones": ["aapl"],
        "entry_notes": "Optional notes",
        "relation_lang": "italian",
        "relation_content": "il pomo"
        "relation_type": "noun"
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
|-----------|------------|------------------------------------------------------------------------------------------|
| `entry_content`      | `string`   | The main entry content (word or phrase). |
| `entry_lang`      | `string`   | Language of the main entry as defined in the config. |
| `entry_phones`      | `string`   | Optional phonetic notations representing the pronunciations of the main entry. |
| `entry_notes`      | `string`   | Optional notes describing the main entry. |
| `relation_content`      | `string`   | The definition content (word or phrase). |
| `relation_lang`      | `string`   | Language of the definition entry as defined in the config. |
| `relation_notes`      | `string`   | Optional notes describing the definition entry. |




### POST /api/comments
Accept a public comment or suggestion on a relation (definition).
The comment shows up in the admin moderation queue where the admin can choose to make a change based
on the comment or discard it.


#### Request

```bash
curl -u username:password 'http://localhost:9000/api/submissions/comments' -X POST \
    -H 'Content-Type: application/json; charset=utf-8' \
    --data-binary @- << EOF
    {
        "from_guid": "17e7a544-5b55-4c6c-8cfc-8fbe2f5ea747",
        "to_guid": "61f76f4d-ee87-4efc-b2b2-845125585bcf",
        "comments": "This definition seems to be incorrect."
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
|-----------|------------|------------------------------------------------------------------------------------------|
| `from_guid`      | `string`   | The `guid` of the main entry. Numerical IDs are not exposed in the public. |
| `to_guid`      | `string`   | The `guid` of the definition entry. Numerical IDs are not exposed in the public. |
| `comments`      | `string`   | Comments. |
