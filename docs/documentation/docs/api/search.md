# Search

### GET /api/dictionary/:fromLang/:toLang/:searchWords
Search the dictionary and retrieve paginated results. `:searchQuery` should be URL encoded.


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
