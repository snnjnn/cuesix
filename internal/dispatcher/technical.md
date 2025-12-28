# Dispatcher - Technical

- Own the request queue (buffer size 1) and a worker loop.
- Track last dequeue time and apply cooldown based on time since the last dequeue.
- Provide the candidate config to the validator for validation.
- Return errors from the compile pipeline; the caller decides how to restart the loop.
- Provide a notifier-compatible interface to accept enqueue signals from listener.
