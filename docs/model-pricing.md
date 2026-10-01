# Trusted model pricing

Status: implemented as a local, server-owned usage projection.

TimeKeeper never trusts a model price supplied by an agent or usage snapshot.
Cost is `priced` only when the server starts with a validated catalogue at
`TIMEKEEPER_MODEL_PRICES_FILE`. If the variable is absent, or a session names a
model missing from the catalogue, the API reports `cost.status: "unpriced"` and
includes a bounded reason. It never estimates a fallback price.

## Catalogue format

```json
{
  "schema_version": "timekeeper-model-prices/v1",
  "source": "operator-file",
  "currency": "USD",
  "models": {
    "example-model": {
      "input_per_million_micros": 1000000,
      "output_per_million_micros": 2000000,
      "cache_creation_per_million_micros": 0,
      "cache_read_per_million_micros": 0
    }
  }
}
```

The file is bounded, rejects unknown fields and trailing JSON, requires a
three-letter currency, and rejects negative rates. Rates are integer
micro-units per million tokens. Cost is exposed as integer `amount_micros` to
avoid floating-point currency drift. Small fractional-token costs are rounded
up conservatively per counter bucket.

The catalogue is host configuration, not project or agent data. Changing it
requires restarting the server; historical usage counters remain unchanged,
but their read-only cost projection follows the currently loaded catalogue.
Use `cost.source` to identify the trusted source when a session is priced.

## Cost states

| State | Meaning |
|---|---|
| `priced` | Every usage session included in the aggregate has a matching server-owned rate. |
| `unpriced` | No trusted catalogue entry exists, counters are invalid, or an aggregate mixes priced and unpriced sessions. |

A zero-cost configured model is still `priced`; absence from the catalogue is
never interpreted as free.
