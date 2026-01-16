FROM mcr.microsoft.com/devcontainers/go:1.25-bookworm

# Installation de golang-migrate
RUN curl -L https://github.com/golang-migrate/migrate/releases/download/v4.18.1/migrate.linux-amd64.tar.gz | tar xvz \
    && mv migrate /usr/local/bin/migrate \
    && chmod +x /usr/local/bin/migrate

# [Optionnel] Vérification de l'installation
RUN migrate -version