This project builds a web frontend for the operation of a configuration management service.

The API of the configuration management service is described in document /cmd/sixpack/docs/swagger.yaml of theis repository.

The application uses alpine.js for interactivity, session storage for shared state, and bulma css for layout and styling. The application favors builtin bulma styles and components and tries to keep the html structure clear and semantic, and the css customization to a minimum.

The application consists of static web assets (html, css, js). There is no server side in the application (no node, deno, bun , etc). For packaging module-based dependencies, the application uses esbuild.

The application and all of its components support dark mode automatically, based on the user preferences.

For displaying or editing highlighted yaml text, the application uses codemirror v6, with syntax highlighting and folding support.

The target user of the application is a devops engineer. So the application must be functional and straighforward, without unnecessary branding, embellishment or noise. Keep it straight to the point.

The application has two main modes:

1. Browse, where the user can select any of the configuration fragments listed by the /schema/sources api, and view the contents of the fragment in a read-only, syntax highlighted editor window.

2. Playground, where the user can write yaml into an editor window.

In both modes, the user must be able to validate the yaml it is seeing or editing, respectively, by using the corresponding validate endpoints of the API (GET /validate/path for sources, POST validate for playground).

Validation accepts an arbirtrary number of URL query parameters, so both modes would need a form to enter key / value pairs toi be submitted in the validation request. These parameters must be shared across the two modes.
