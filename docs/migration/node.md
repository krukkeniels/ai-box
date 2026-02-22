# Node.js Project Migration Checklist

**Tool Pack**: `node@20` (or `node@18`)
**Package Managers**: npm, yarn
**IDE**: VS Code, WebStorm (JetBrains)

---

## Pre-Migration

- [ ] Identify Node.js version required (check `.nvmrc`, `engines` in `package.json`).
- [ ] Identify package manager (npm or yarn) and version.
- [ ] Identify framework: React, Angular, Vue, Next.js, Express, etc.
- [ ] Verify all npm packages are available through Nexus npm proxy.
- [ ] Check for native modules (`node-gyp` dependencies) that may need build tools.

## Tool Pack Setup

```bash
aibox install node@20        # or node@18
```

- [ ] `node --version` shows correct version.
- [ ] `npm --version` shows installed version.
- [ ] npm registry configured: `npm config get registry` points to Nexus.

## Dependency Installation

```bash
npm install                  # or yarn install
```

- [ ] All dependencies resolve through Nexus npm proxy.
- [ ] No network errors (all requests go through proxy, not npmjs.org).
- [ ] `node_modules/` created successfully.
- [ ] Native modules compile (if any): `node-gyp` has build tools available.

## Build Validation

```bash
npm run build                # or yarn build
```

- [ ] Build completes without errors.
- [ ] Build artifacts generated correctly.
- [ ] Build time within 15% of local baseline.

## Test Validation

```bash
npm test                     # or yarn test
```

- [ ] All tests pass.
- [ ] Test runner (Jest, Mocha, Vitest) works correctly.
- [ ] No sandbox-specific failures.

## IDE Validation

### VS Code

- [ ] TypeScript/JavaScript IntelliSense works.
- [ ] ESLint/Prettier extensions work.
- [ ] Debugging (launch.json) works.

### WebStorm (via Gateway)

- [ ] Project indexes successfully.
- [ ] Code completion and navigation work.
- [ ] Debugging works.

## Known Issues

- **Native modules**: Packages with native C/C++ addons (`node-gyp`) need `build-essential`, `python3`. These should be in the base image.
- **File watchers**: Development servers with hot reload (webpack-dev-server, Vite) use file watchers. gVisor supports inotify but verify watcher limits: `fs.inotify.max_user_watches`.
- **Ports**: Dev servers binding to ports need `aibox port-forward` to be accessible from the host IDE.
