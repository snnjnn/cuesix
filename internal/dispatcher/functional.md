# Dispatcher - Functional

- Dequeue compile requests and execute the compile pipeline.
- Apply throttling: queue requests that arrive during a compile.
- Enforce a cooldown: minimum time between compilations is configurable.
- Collapse bursts so only one compile runs per cooldown window.
- Return errors to the caller so the run loop can be restarted by the application.
- The pipeline operates on plugin-processed configurations before cache/validate/reload.
