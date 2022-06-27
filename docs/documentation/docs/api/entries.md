# Entries

### GET /api/entries/:fromLang/:toLang/:searchQuery
Search the dictionary and retrieve paginated results. `:searchQuery` should be URL encoded.

This is identication to the public API `/api/dictionary/:fromLang/:toLang/:searchQuery` except that the public API does not return numerical database `id`s of entries.


#### Request
```bash
curl http://localhost:9000/api/dictionary/english/english/apple
```

**Response**

```json
{
  "data": {
    "entries": [
      {
        "id": 1,
        "guid": "17e7a544-5b55-4c6c-8cfc-8fbe2f5ea747",
        "weight": 0,
        "initial": "A",
        "lang": "english",
        "content": "Apple",
        "tokens": "",
        "tags": [
          "optional-tag1",
          "tag2"
        ],
        "phones": [
          "ˈæp.əl",
          "aapl"
        ],
        "notes": "Optional note",
        "status": "enabled",
        "relations": [
          {
            "id": 3,
            "guid": "61f76f4d-ee87-4efc-b2b2-845125585bcf",
            "weight": 0,
            "initial": "R",
            "lang": "english",
            "content": "round, red or yellow, edible fruit of a small tree",
            "tokens": "",
            "tags": [],
            "phones": [
              ""
            ],
            "notes": "",
            "status": "enabled",
            "created_at": "2022-06-26T08:33:34.842429Z",
            "updated_at": "2022-06-26T08:33:34.842429Z",
            "relation": {
              "id": 1,
              "types": [
                "noun"
              ],
              "tags": [
                ""
              ],
              "notes": "",
              "weight": 0,
              "status": "enabled",
              "created_at": "2022-06-26T08:33:34.844822Z",
              "updated_at": "2022-06-26T08:33:34.844822Z"
            }
          },
          {
            "id": 4,
            "guid": "72ee1c06-d3fc-4b5e-8fa7-ad868c12475d",
            "weight": 1,
            "initial": "T",
            "lang": "english",
            "content": "the tree, cultivated in most temperate regions.",
            "tokens": "",
            "tags": [],
            "phones": [
              ""
            ],
            "notes": "",
            "status": "enabled",
            "created_at": "2022-06-26T08:33:34.842429Z",
            "updated_at": "2022-06-26T08:33:34.842429Z",
            "relation": {
              "id": 2,
              "types": [
                "noun"
              ],
              "tags": [
                ""
              ],
              "notes": "",
              "weight": 1,
              "status": "enabled",
              "created_at": "2022-06-26T08:33:34.844822Z",
              "updated_at": "2022-06-26T08:33:34.844822Z"
            }
          },
          {
            "id": 5,
            "guid": "653fc521-f917-4049-99fc-5281b3e2e300",
            "weight": 2,
            "initial": "I",
            "lang": "italian",
            "content": "il pomo.",
            "tokens": "",
            "tags": [],
            "phones": [
              ""
            ],
            "notes": "",
            "status": "enabled",
            "created_at": "2022-06-26T08:33:34.842429Z",
            "updated_at": "2022-06-26T08:33:34.842429Z",
            "relation": {
              "id": 3,
              "types": [
                "noun"
              ],
              "tags": [
                ""
              ],
              "notes": "",
              "weight": 2,
              "status": "enabled",
              "created_at": "2022-06-26T08:33:34.844822Z",
              "updated_at": "2022-06-26T08:33:34.844822Z"
            }
          }
        ],
        "created_at": "2022-06-26T08:33:34.83976Z",
        "updated_at": "2022-06-26T08:33:34.83976Z"
      }
    ],
    "page": 1,
    "per_page": 10,
    "total_pages": 0,
    "total": 1
  }
}
```

#### Query params
| Param     | Type   |                                                                                                                                     |
|-----------|------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `type`      | `string`   | Filter results by the given type. eg: `noun`. |
| `tag`      | `string`   | Filter results by the given tag. eg: `my-tag`. |
| `per_page`      | `int`   | Number of results to return per page (query) |
| `page`      | `int`   | Page number for paginated results. |


### GET /api/entries/:id
Retrieve a single entry by its database ID.


#### Request
```bash
curl -u username:password http://localhost:9000/api/entries/1
```

**Response**
```json
{
  "data": {
    "id": 1,
    "guid": "17e7a544-5b55-4c6c-8cfc-8fbe2f5ea747",
    "weight": 0,
    "initial": "A",
    "lang": "english",
    "content": "Apple",
    "tokens": "",
    "tags": [
      "optional-tag1",
      "tag2"
    ],
    "phones": [
      "ˈæp.əl",
      "aapl"
    ],
    "notes": "Optional note",
    "status": "enabled",
    "relations": [
      {
        "id": 3,
        "guid": "61f76f4d-ee87-4efc-b2b2-845125585bcf",
        "weight": 0,
        "initial": "R",
        "lang": "english",
        "content": "round, red or yellow, edible fruit of a small tree",
        "tokens": "",
        "tags": [],
        "phones": [
          ""
        ],
        "notes": "",
        "status": "enabled",
        "created_at": "2022-06-26T08:33:34.842429Z",
        "updated_at": "2022-06-26T08:33:34.842429Z",
        "relation": {
          "id": 1,
          "types": [
            "noun"
          ],
          "tags": [
            ""
          ],
          "notes": "",
          "weight": 0,
          "status": "enabled",
          "created_at": "2022-06-26T08:33:34.844822Z",
          "updated_at": "2022-06-26T08:33:34.844822Z"
        }
      },
      {
        "id": 4,
        "guid": "72ee1c06-d3fc-4b5e-8fa7-ad868c12475d",
        "weight": 1,
        "initial": "T",
        "lang": "english",
        "content": "the tree, cultivated in most temperate regions.",
        "tokens": "",
        "tags": [],
        "phones": [
          ""
        ],
        "notes": "",
        "status": "enabled",
        "created_at": "2022-06-26T08:33:34.842429Z",
        "updated_at": "2022-06-26T08:33:34.842429Z",
        "relation": {
          "id": 2,
          "types": [
            "noun"
          ],
          "tags": [
            ""
          ],
          "notes": "",
          "weight": 1,
          "status": "enabled",
          "created_at": "2022-06-26T08:33:34.844822Z",
          "updated_at": "2022-06-26T08:33:34.844822Z"
        }
      },
      {
        "id": 5,
        "guid": "653fc521-f917-4049-99fc-5281b3e2e300",
        "weight": 2,
        "initial": "I",
        "lang": "italian",
        "content": "il pomo.",
        "tokens": "",
        "tags": [],
        "phones": [
          ""
        ],
        "notes": "",
        "status": "enabled",
        "created_at": "2022-06-26T08:33:34.842429Z",
        "updated_at": "2022-06-26T08:33:34.842429Z",
        "relation": {
          "id": 3,
          "types": [
            "noun"
          ],
          "tags": [
            ""
          ],
          "notes": "",
          "weight": 2,
          "status": "enabled",
          "created_at": "2022-06-26T08:33:34.844822Z",
          "updated_at": "2022-06-26T08:33:34.844822Z"
        }
      }
    ],
    "created_at": "2022-06-26T08:33:34.83976Z",
    "updated_at": "2022-06-26T08:33:34.83976Z"
  }
}

```



### GET /api/entries/:id/parents
Retrieve all parent entries of a definition entry.


#### Request
```bash
curl -u username:password http://localhost:9000/api/entries/3/parents
```

**Response**
```json
{
  "data": [
    {
      "id": 1,
      "guid": "17e7a544-5b55-4c6c-8cfc-8fbe2f5ea747",
      "weight": 0,
      "initial": "A",
      "lang": "english",
      "content": "Apple",
      "tokens": "",
      "tags": [
        "optional-tag1",
        "tag2"
      ],
      "phones": [
        "ˈæp.əl",
        "aapl"
      ],
      "notes": "Optional note",
      "status": "enabled",
      "created_at": "2022-06-26T08:33:34.83976Z",
      "updated_at": "2022-06-26T08:33:34.83976Z"
    }
  ]
}
```



### POST /api/entries
Create a new entry in the database. This can be a main entry or a definition entry which can be added
to another main entry later.

#### Request

```bash
curl -u username:password 'http://localhost:9000/api/entries' -X POST \
    -H 'Content-Type: application/json; charset=utf-8' \
    --data-binary @- << EOF
    {
        "content": "Apple",
        "initial": "A",
        "lang": "english",
        "phones": ["aapl"],
        "tags": ["my-tag"],
        "tokens": "my-tag",
        "notes": "Optional notes",
        "weight": 2,
        "status": "enabled"
    }
EOF

```

**Response**
```json
{
  "data": {
    "id": 8,
    "guid": "fa19911a-06a8-424b-8ca3-256e5511cd1f",
    "weight": 2,
    "initial": "A",
    "lang": "english",
    "content": "Apple",
    "tokens": "",
    "tags": [
      "my-tag"
    ],
    "phones": [
      "aapl"
    ],
    "notes": "Optional notes",
    "status": "enabled",
    "created_at": "2022-06-26T09:45:21.011192Z",
    "updated_at": "2022-06-26T09:45:21.011192Z"
  }
}
```

#### Params
| Param     | Type   |                                                                                                                                     |
|-----------|------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `content`      | `string`   | The entry content (word or phrase). |
| `initial`      | `string`   | The uppercase first character of the entry. Eg: `A` for Apple. If left empty, it is automatically picked up. |
| `lang`      | `string`   | Language of the entry as defined in the config. |
| `phones`      | `[]string`   | Optional phonetic notations representing the pronunciations of the entry. |
| `tokens`      | `string`   | Postgres fulltext search tokens for the entry (content). If this is left empty and the language config has `tokenizer_type` set as `Postgres`, the tokens are automatically created in the database using `TO_TSVECTOR($tsvector_language, $content)`. For languages without Postgres tokenizers, the [tsvector](https://www.postgresql.org/docs/10/datatype-textsearch.html#DATATYPE-TSVECTOR) token string should be computed externally and provided here. |
| `tags`      | `[]string`   | Optional tags describing the entry. |
| `notes`      | `string`   | Optional notes describing the entry. |
| `weight`      | `int`   | Optional numerical weight to order the entry in the glossary and search results. If left empty, it is automatically computed as the last entry by the initial in ascending order. |
| `status`      | `string`   | `enabled` = Visible in public search and APIs.<br />`pending` = Pending moderation in the admin UI.<br />`disabled` = Hidden from public search and APIs. |



### PUT /api/entries/:id
Update an entry.

#### Request

```bash
curl -u username:password 'http://localhost:9000/api/entries/8' -X PUT \
    -H 'Content-Type: application/json; charset=utf-8' \
    --data-binary @- << EOF
    {
        "content": "Apple",
        "initial": "A",
        "lang": "english",
        "phones": ["aapl"],
        "tags": ["my-tag"],
        "tokens": "my-tag",
        "notes": "Optional notes",
        "weight": 2,
        "status": "enabled"
    }
EOF
```

**Response**
```json
{
  "data": {
    "id": 8,
    "guid": "fa19911a-06a8-424b-8ca3-256e5511cd1f",
    "weight": 2,
    "initial": "A",
    "lang": "english",
    "content": "Apple",
    "tokens": "",
    "tags": [
      "my-tag"
    ],
    "phones": [
      "aapl"
    ],
    "notes": "Optional notes",
    "status": "enabled",
    "created_at": "2022-06-26T09:45:21.011192Z",
    "updated_at": "2022-06-26T09:45:21.011192Z"
  }
}
```

#### Params
| Param     | Type   |                                                                                                                                     |
|-----------|------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `content`      | `string`   | The entry content (word or phrase). |
| `initial`      | `string`   | The uppercase first character of the entry. Eg: `A` for Apple. If left empty, it is automatically picked up. |
| `lang`      | `string`   | Language of the entry as defined in the config. |
| `phones`      | `[]string`   | Optional phonetic notations representing the pronunciations of the entry. |
| `tokens`      | `string`   | Postgres fulltext search tokens for the entry (content). If this is left empty and the language config has `tokenizer_type` set as `Postgres`, the tokens are automatically created in the database using `TO_TSVECTOR($TSVectorLanguage, $content)`. For languages without Postgres tokenizers, the TSVectorToken strings should be computed externally and added here. |
| `tags`      | `[]string`   | Optional tags describing the entry. |
| `notes`      | `string`   | Optional notes describing the entry. |
| `weight`      | `int`   | Optional numerical weight to order the entry in the glossary and search results. |
| `status`      | `string`   | `enabled` = Visible in public search and APIs.<br />`pending` = Pending moderation in the admin UI.<br />`disabled` = Hidden from public search and APIs. |








### DELETE /api/entries/:id
Delete an entry. If this is a main entry, its definition entries are not removed, but merely unlinked from the `relations` table.

#### Request
```bash
curl -u username:password 'http://localhost:9000/api/entries/1' -X DELETE
```

**Response**
```json
{
    "data": true
}
```

