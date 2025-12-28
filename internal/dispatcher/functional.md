# Dispatcher - Functional

- Dequeue compile requests and execute the compile pipeline.
- Apply throttling: queue requests that arrive during a compile.
- Enforce a cooldown: minimum time between compilations is configurable.
- Collapse bursts so only one compile runs per cooldown window.
- Pass the candidate config to the validator for validation.
- Return errors to the caller so the run loop can be restarted by the application.
