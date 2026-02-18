# Nexus Mirror URLs Reference

Base URL: `http://nexus.internal:8081`

This document lists the Nexus mirror URL for each package format and provides
client configuration examples. All group repositories aggregate the proxy
(upstream cache) and hosted (internal) repositories into a single endpoint.

---

## Mirror URLs

| Format         | Repository    | URL                                                                 | Type    |
|----------------|---------------|---------------------------------------------------------------------|---------|
| **npm**        | npm-group     | `http://nexus.internal:8081/repository/npm-group/`                  | group   |
|                | npm-proxy     | `http://nexus.internal:8081/repository/npm-proxy/`                  | proxy   |
|                | npm-hosted    | `http://nexus.internal:8081/repository/npm-hosted/`                 | hosted  |
| **Maven**      | maven-group   | `http://nexus.internal:8081/repository/maven-group/`                | group   |
|                | maven-central-proxy | `http://nexus.internal:8081/repository/maven-central-proxy/`  | proxy   |
|                | maven-hosted  | `http://nexus.internal:8081/repository/maven-hosted/`               | hosted  |
| **Gradle**     | gradle-plugins-proxy | `http://nexus.internal:8081/repository/gradle-plugins-proxy/` | proxy   |
| **PyPI**       | pypi-group    | `http://nexus.internal:8081/repository/pypi-group/`                 | group   |
|                | pypi-proxy    | `http://nexus.internal:8081/repository/pypi-proxy/`                 | proxy   |
|                | pypi-hosted   | `http://nexus.internal:8081/repository/pypi-hosted/`                | hosted  |
| **NuGet**      | nuget-group   | `http://nexus.internal:8081/repository/nuget-group/`                | group   |
|                | nuget-proxy   | `http://nexus.internal:8081/repository/nuget-proxy/`                | proxy   |
| **Go modules** | go-proxy      | `http://nexus.internal:8081/repository/go-proxy/`                   | proxy   |
| **Cargo**      | cargo-proxy   | `http://nexus.internal:8081/repository/cargo-proxy/` (limited)      | proxy   |

---

## Client Configuration Examples

### npm

Create or edit `~/.npmrc`:

```ini
registry=http://nexus.internal:8081/repository/npm-group/
strict-ssl=false
```

Or use per-project `.npmrc` in the project root with the same content.

Verify:

```bash
npm install --registry=http://nexus.internal:8081/repository/npm-group/ express
```

### yarn

Create or edit `~/.yarnrc.yml`:

```yaml
npmRegistryServer: "http://nexus.internal:8081/repository/npm-group/"
```

Or for Yarn Classic (`~/.yarnrc`):

```ini
registry "http://nexus.internal:8081/repository/npm-group/"
```

### Maven

Add to `~/.m2/settings.xml`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<settings xmlns="http://maven.apache.org/SETTINGS/1.2.0"
          xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
          xsi:schemaLocation="http://maven.apache.org/SETTINGS/1.2.0
                              http://maven.apache.org/xsd/settings-1.2.0.xsd">
  <mirrors>
    <mirror>
      <id>nexus-maven</id>
      <mirrorOf>central</mirrorOf>
      <url>http://nexus.internal:8081/repository/maven-group/</url>
    </mirror>
    <mirror>
      <id>nexus-gradle-plugins</id>
      <mirrorOf>gradle-plugins</mirrorOf>
      <url>http://nexus.internal:8081/repository/gradle-plugins-proxy/</url>
    </mirror>
  </mirrors>
</settings>
```

Verify:

```bash
mvn -s ~/.m2/settings.xml dependency:resolve
```

### Gradle

Add to `build.gradle` or `settings.gradle`:

```groovy
pluginManagement {
    repositories {
        maven { url 'http://nexus.internal:8081/repository/gradle-plugins-proxy/' }
        maven { url 'http://nexus.internal:8081/repository/maven-group/' }
    }
}

dependencyResolutionManagement {
    repositories {
        maven { url 'http://nexus.internal:8081/repository/maven-group/' }
    }
}
```

Or use `init.gradle` for global configuration:

```groovy
allprojects {
    repositories {
        maven {
            url 'http://nexus.internal:8081/repository/maven-group/'
            allowInsecureProtocol true
        }
    }
}
```

### PyPI / pip

Create or edit `~/.pip/pip.conf` (Linux/macOS) or `%APPDATA%\pip\pip.ini` (Windows):

```ini
[global]
index-url = http://nexus.internal:8081/repository/pypi-group/simple/
trusted-host = nexus.internal
```

Verify:

```bash
pip install --index-url=http://nexus.internal:8081/repository/pypi-group/simple/ \
  --trusted-host=nexus.internal requests
```

### NuGet / .NET

Add the Nexus source:

```bash
dotnet nuget add source http://nexus.internal:8081/repository/nuget-group/index.json \
  --name nexus-nuget
```

Or edit `nuget.config`:

```xml
<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources>
    <clear />
    <add key="nexus-nuget" value="http://nexus.internal:8081/repository/nuget-group/index.json" />
  </packageSources>
</configuration>
```

Verify:

```bash
dotnet add package Newtonsoft.Json --source http://nexus.internal:8081/repository/nuget-group/index.json
```

### Go modules

Set the `GOPROXY` environment variable:

```bash
export GOPROXY=http://nexus.internal:8081/repository/go-proxy/
export GONOSUMCHECK=*
export GONOSUMDB=*
```

Add to `~/.bashrc` or `~/.zshrc` for persistence.

Verify:

```bash
GOPROXY=http://nexus.internal:8081/repository/go-proxy/ go get golang.org/x/text
```

**Note**: The Go proxy uses a raw repository format. Full Go module proxy protocol
compliance depends on Nexus version. If the raw proxy does not satisfy the Go
module proxy protocol, consider deploying Athens or a dedicated Go proxy in front
of the raw cache.

### Cargo / crates.io (Limited Support)

Nexus 3.x does **not** natively support the Cargo sparse registry protocol.
The `cargo-proxy` repository is a raw proxy to `static.crates.io` for caching
`.crate` files, but it cannot serve as a full Cargo registry replacement.

**Workarounds**:
1. Use a dedicated Cargo registry proxy such as [Kellnr](https://kellnr.io/) or
   a crates.io mirror running alongside Nexus.
2. Pre-download crates and host them in a hosted raw repository.
3. If direct internet access is available to build containers, allow Cargo to
   use `crates.io` directly and rely on build caching.

This is a **documented gap** per the Phase 0 plan (R6).

---

## Container / Sandbox Integration

These mirror URLs will be injected into container configurations in Phase 2.
The typical approach is to mount configuration files into the container at
build or run time:

| Tool  | Config file      | Mount target                     |
|-------|------------------|----------------------------------|
| npm   | `.npmrc`         | `/home/dev/.npmrc`               |
| Maven | `settings.xml`   | `/home/dev/.m2/settings.xml`     |
| pip   | `pip.conf`       | `/home/dev/.pip/pip.conf`        |
| Go    | env vars         | Set via container environment    |
| NuGet | `nuget.config`   | `/home/dev/.nuget/NuGet/NuGet.Config` |

---

## Troubleshooting

### General

- **Check Nexus health**: `curl -sf http://nexus.internal:8081/service/rest/v1/status`
- **List repositories**: `curl -sf http://nexus.internal:8081/service/rest/v1/repositories | jq .`
- **Check proxy status**: In Nexus UI, navigate to Administration > Repositories and check the "Status" column for proxy repos. A green checkmark means the upstream is reachable.

### npm

- **SELF_SIGNED_CERT_IN_CHAIN**: Add `strict-ssl=false` to `.npmrc` or install the internal CA cert into Node's trust store.
- **404 on scoped packages**: Ensure the npm-group URL ends with `/` and that the proxy remote URL is `https://registry.npmjs.org` (no trailing slash).
- **Authentication errors**: If anonymous access is disabled, add auth token to `.npmrc`:
  ```
  //nexus.internal:8081/repository/npm-group/:_authToken=<token>
  ```

### Maven

- **Could not transfer artifact**: Check that `settings.xml` mirror URL matches the group repo URL exactly. Check Nexus logs for upstream connection errors.
- **SSL errors**: Add `-Dmaven.wagon.http.ssl.insecure=true` for testing, or install the internal CA cert into the JDK trust store.

### PyPI / pip

- **"Untrusted host"**: Add `--trusted-host nexus.internal` or set `trusted-host` in `pip.conf`.
- **404 errors**: Ensure the PyPI URL includes `/simple/` at the end.
- **HTML parse errors**: The PyPI simple API requires the `/simple/` suffix. Using the base repository URL without it will fail.

### Go

- **GONOSUMCHECK**: If the Go proxy does not serve checksum database, set `GONOSUMCHECK=*`.
- **Module not found**: The raw proxy may not fully support the Go module proxy protocol. Consider using a dedicated Go proxy (Athens) or falling back to `GOPROXY=direct`.

### NuGet

- **Source not found**: Ensure the NuGet URL includes `/index.json` at the end.
- **Package not found**: NuGet v3 protocol requires the `/index.json` suffix. Verify the proxy's remote URL is `https://api.nuget.org/v3/index.json`.

### Disk Space

- Cleanup policies are configured to evict artifacts unused for 90 days.
- To manually trigger cleanup: Nexus UI > Administration > System > Tasks > Run "Cleanup unused proxy artifacts".
- To check blob store usage: Nexus UI > Administration > Blob Stores.
