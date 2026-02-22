# Java Project Migration Checklist

**Tool Pack**: `java@21` (or `java@17`)
**Build Systems**: Gradle, Maven
**IDE**: IntelliJ IDEA, VS Code with Java Extension Pack

---

## Pre-Migration

- [ ] Identify Java version required (17 or 21).
- [ ] Identify build system (Gradle or Maven) and version.
- [ ] Verify Gradle wrapper (`gradlew`) or Maven wrapper (`mvnw`) is committed to the repo.
- [ ] Verify all dependencies are available through Nexus Maven/Gradle proxy.
- [ ] Check for proprietary dependencies that may need manual upload to Nexus.

## Tool Pack Setup

```bash
aibox install java@21        # or java@17
```

- [ ] `java -version` shows correct JDK version.
- [ ] `javac -version` matches.
- [ ] `JAVA_HOME` is set correctly.

## Build Validation

```bash
./gradlew build              # Gradle
./mvnw clean install         # Maven
```

- [ ] Build completes without errors.
- [ ] All dependencies resolved through Nexus proxy.
- [ ] Build time within 15% of local baseline.
- [ ] Gradle daemon starts and persists across builds (performance optimization).

## Test Validation

- [ ] `./gradlew test` or `./mvnw test` passes.
- [ ] Test reports generated correctly.
- [ ] No sandbox-specific test failures (flaky tests due to gVisor, timing, etc.).

## IDE Validation

### IntelliJ (via Gateway)

- [ ] Project imports and indexes successfully.
- [ ] Code completion and navigation work.
- [ ] Debugging works (set breakpoint, step through, inspect variables).
- [ ] Gradle/Maven tool window shows tasks correctly.

### VS Code

- [ ] Java Extension Pack provides IntelliSense.
- [ ] Build tasks run from VS Code terminal.

## Known Issues

- **Gradle daemon memory**: Gradle daemon can consume significant memory. On 16 GB machines, consider setting `org.gradle.jvmargs=-Xmx2g` in `gradle.properties`.
- **Nexus proxy**: Ensure `~/.gradle/gradle.properties` or `settings.xml` points to Nexus, not Maven Central directly.
