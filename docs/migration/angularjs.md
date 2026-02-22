# AngularJS (1.x) Legacy Project Migration Checklist

**Tool Pack**: `angularjs@1`
**Build Tools**: Grunt, Gulp, or npm scripts
**IDE**: VS Code
**Special Considerations**: Legacy project, may require older Node.js

---

## Pre-Migration

- [ ] Identify AngularJS version (1.x -- confirm not Angular 2+).
- [ ] Identify Node.js version required (check `.nvmrc`, `engines` in `package.json`). Often Node 14.x or 16.x.
- [ ] Identify build tool: Grunt, Gulp, or npm scripts.
- [ ] Identify test runner: Karma + Jasmine/Mocha, or Protractor for e2e.
- [ ] Check for Bower dependencies (legacy package manager).
- [ ] Verify all npm (and Bower) packages available through Nexus.

## Tool Pack Setup

```bash
aibox install angularjs@1
```

- [ ] `node --version` shows the correct legacy version (14.x or 16.x).
- [ ] `npm --version` compatible with the Node version.
- [ ] Grunt/Gulp CLI available: `grunt --version` or `gulp --version`.
- [ ] Bower available (if used): `bower --version`.
- [ ] Karma CLI available: `karma --version`.

## Dependency Installation

```bash
npm install
bower install                # if using Bower
```

- [ ] All npm packages resolve through Nexus.
- [ ] Bower packages resolve (if used) -- may need `.bowerrc` configured for Nexus.
- [ ] No deprecated package warnings that cause install failures.

## Build Validation

```bash
grunt build                  # or gulp build, or npm run build
```

- [ ] Build completes without errors.
- [ ] AngularJS templates compile.
- [ ] Minification/uglification succeeds (template annotation for DI).
- [ ] Build artifacts generated in expected output directory.

## Test Validation

```bash
npm test                     # or karma start
```

- [ ] Karma launches and connects to a headless browser.
- [ ] Headless Chrome works under gVisor (use `--no-sandbox` flag if needed).
- [ ] All unit tests pass.
- [ ] Test coverage report generates (if configured).

## IDE Validation

### VS Code

- [ ] JavaScript IntelliSense provides completions.
- [ ] ESLint/JSHint works with project config.
- [ ] AngularJS-specific extensions (if any) work.

## Known Issues and Limitations

- **Node.js version**: AngularJS 1.x projects often require Node 14.x or 16.x. The tool pack must include the correct version via nvm or a pinned binary.
- **Headless Chrome under gVisor**: Chrome's internal sandbox conflicts with gVisor. Karma config may need `--no-sandbox` flag for ChromeHeadless. This is safe because gVisor provides the outer sandbox.
- **Bower**: Deprecated package manager. If the project uses Bower, it must be available in the tool pack. Long-term: migrate off Bower.
- **Grunt/Gulp plugins**: Some Grunt/Gulp plugins may have native dependencies requiring build tools.
- **Protractor (e2e tests)**: Protractor is deprecated and requires a running browser. E2e tests may not work under gVisor. Consider skipping e2e in sandbox and running them in CI.
- **Windows-origin line endings**: Legacy projects may have CRLF line endings. Git autocrlf settings in the sandbox should handle this, but verify.
