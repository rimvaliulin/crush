# Skillstate

Merge a JSON patch into the explicit execution state of this session.

The runtime owns the state, not you: the patch is merged key by key, the
result is validated against the state schema, and the row is replaced inside
a transaction. A rejected patch leaves the state untouched; the reason is
returned so you can fix the patch and retry.

Rules:

- Objects merge key by key; set a key to `null` to delete it.
- Lists and strings replace what was there, so send the whole new value.
- `goal` is immutable: a patch that changes it is rejected.
- The state must stay under 4096 bytes; prune stale `verified`, `facts` and
  `errors` entries in the same patch.

Call this tool before your action so the next step sees the updated state.
