#!/bin/bash
set -euo pipefail

PACK_DIR="/opt/toolpacks/java"
JAVA_VERSION="21"
GRADLE_VERSION="8.5"
MAVEN_VERSION="3.9.6"

echo "==> Installing Java tool pack (OpenJDK ${JAVA_VERSION}, Gradle ${GRADLE_VERSION}, Maven ${MAVEN_VERSION})"

# Install Eclipse Temurin JDK 21.
if ! command -v java &>/dev/null; then
    echo "  Installing OpenJDK ${JAVA_VERSION} (Temurin)..."
    apt-get update -qq
    apt-get install -y -qq wget apt-transport-https gnupg

    # Add Adoptium repository.
    wget -qO- https://packages.adoptium.net/artifactory/api/gpg/key/public | gpg --dearmor -o /usr/share/keyrings/adoptium.gpg
    echo "deb [signed-by=/usr/share/keyrings/adoptium.gpg] https://packages.adoptium.net/artifactory/deb $(. /etc/os-release && echo $VERSION_CODENAME) main" > /etc/apt/sources.list.d/adoptium.list
    apt-get update -qq
    apt-get install -y -qq temurin-${JAVA_VERSION}-jdk
    echo "  OpenJDK ${JAVA_VERSION} installed."
else
    echo "  Java already available: $(java --version 2>&1 | head -1)"
fi

# Install Gradle.
mkdir -p "${PACK_DIR}"
if [ ! -d "${PACK_DIR}/gradle" ]; then
    echo "  Installing Gradle ${GRADLE_VERSION}..."
    wget -q "https://services.gradle.org/distributions/gradle-${GRADLE_VERSION}-bin.zip" -O /tmp/gradle.zip
    unzip -q /tmp/gradle.zip -d "${PACK_DIR}"
    mv "${PACK_DIR}/gradle-${GRADLE_VERSION}" "${PACK_DIR}/gradle"
    rm /tmp/gradle.zip
    ln -sf "${PACK_DIR}/gradle/bin/gradle" /usr/local/bin/gradle
    echo "  Gradle ${GRADLE_VERSION} installed."
fi

# Install Maven.
if [ ! -d "${PACK_DIR}/maven" ]; then
    echo "  Installing Maven ${MAVEN_VERSION}..."
    wget -q "https://archive.apache.org/dist/maven/maven-3/${MAVEN_VERSION}/binaries/apache-maven-${MAVEN_VERSION}-bin.tar.gz" -O /tmp/maven.tar.gz
    tar xzf /tmp/maven.tar.gz -C "${PACK_DIR}"
    mv "${PACK_DIR}/apache-maven-${MAVEN_VERSION}" "${PACK_DIR}/maven"
    rm /tmp/maven.tar.gz
    ln -sf "${PACK_DIR}/maven/bin/mvn" /usr/local/bin/mvn
    echo "  Maven ${MAVEN_VERSION} installed."
fi

# Configure Maven to use Nexus mirror.
MAVEN_SETTINGS_DIR="${HOME}/.m2"
mkdir -p "${MAVEN_SETTINGS_DIR}"
if [ ! -f "${MAVEN_SETTINGS_DIR}/settings.xml" ]; then
    cat > "${MAVEN_SETTINGS_DIR}/settings.xml" << 'EOF'
<settings>
  <mirrors>
    <mirror>
      <id>nexus</id>
      <mirrorOf>*</mirrorOf>
      <url>https://nexus.internal/repository/maven-public/</url>
    </mirror>
  </mirrors>
</settings>
EOF
fi

# Ensure cache directories exist.
mkdir -p "${HOME}/.m2/repository"
mkdir -p "${HOME}/.gradle/caches"

echo "==> Java tool pack installed successfully."
