# Docker Multi-Stage & Bake Overrides — Go + React

## Le constat

Go compile en **binaire statique**. En dev, tu lances `go run ./cmd/` directement sur ta machine (ou avec `air` pour le hot-reload). Tu n'as pas besoin d'un stage `dev` dans le Dockerfile — le Dockerfile sert à produire l'image de prod. Les variations dev/prod se gèrent via **Docker Compose overrides** ou **Docker Bake overrides**, pas en multipliant les stages.

React c'est pareil : en dev tu fais `npm run dev` en local. Le Dockerfile sert à builder les fichiers statiques et les servir en prod.

---

## 1. Multi-stage : uniquement ce qui est nécessaire

### Go — 2 stages (build + prod)

```dockerfile
FROM golang:1.22-alpine AS build

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=0 go build -o /app/server ./cmd/

FROM scratch

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /app/server /server

CMD ["/server"]
```

Pas de stage `dev`. Le Dockerfile fait une chose : produire le binaire et le mettre dans une image minimale.

### React — 3 stages (deps + build + prod)

```dockerfile
FROM node:20-alpine AS deps

WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts

FROM node:20-alpine AS builder

WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .

ARG VITE_API_URL
ENV VITE_API_URL=${VITE_API_URL}

RUN npm run build && npm install -g serve@14.2.5

FROM gcr.io/distroless/nodejs20-debian12:nonroot AS prod

WORKDIR /app
COPY --from=builder /usr/local/lib/node_modules/serve /app/serve
COPY --from=builder /app/dist ./dist

EXPOSE 3000
USER nonroot:nonroot
CMD ["/app/serve/build/main.js", "-s", "dist", "-l", "3000"]
```

Le stage `deps` est séparé pour profiter du cache Docker : tant que `package.json` ne change pas, les dépendances ne sont pas réinstallées.

---

## 2. Comment fonctionne le multi-stage

Chaque `FROM` crée un **stage indépendant**. On copie entre stages avec `COPY --from=`.

```
Stage: build (golang:alpine ~300 Mo)
  ├── Télécharge les dépendances
  ├── Compile le binaire
  └── Produit : /app/server
         │
         │ COPY --from=build
         ▼
Stage: prod (scratch = 0 Mo)
  └── Contient uniquement le binaire (~10 Mo)
```

BuildKit (le builder par défaut) est intelligent : il ne construit que les stages nécessaires. Si tu cibles `build` avec `--target build`, le stage `prod` n'est jamais exécuté.

---

## 3. Le dev se fait en local, pas dans un stage

En Go, tu travailles directement sur ta machine :

```bash
# Hot-reload avec air
air

# Ou simplement
go run ./cmd/
```

En React :

```bash
npm run dev
```

Le Dockerfile n'intervient que pour construire l'image de production. La question est : comment gérer les **différences de configuration** entre dev et prod ? C'est là qu'interviennent les overrides.

---

## 4. Docker Compose Overrides

La méthode la plus simple. Un fichier de base + un fichier d'override par environnement.

### `docker-compose.yml` (base)

```yaml
services:
  api:
    build:
      context: .
      dockerfile: services/api/Dockerfile
    ports:
      - "8080:8080"
    environment:
      - DB_HOST=postgres

  front:
    build:
      context: .
      dockerfile: services/front/Dockerfile
      args:
        VITE_API_URL: http://localhost:8080
    ports:
      - "3000:3000"
```

### `docker-compose.override.yml` (dev — chargé automatiquement)

```yaml
services:
  api:
    build:
      target: build    # Arrête au stage build si besoin de debug
    volumes:
      - ./services/api:/app
    command: air        # Override le CMD du Dockerfile
    environment:
      - DEBUG=true

  front:
    volumes:
      - ./services/front:/app
      - /app/node_modules
    command: npm run dev -- --host
    ports:
      - "5173:5173"    # Port Vite dev server
```

### `docker-compose.prod.yml` (prod)

```yaml
services:
  api:
    restart: always
    environment:
      - GIN_MODE=release

  front:
    restart: always
```

### Utilisation

```bash
# Dev (charge automatiquement docker-compose.yml + docker-compose.override.yml)
docker compose up

# Prod (charge la base + le fichier prod, ignore l'override)
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

Le fichier `docker-compose.override.yml` est chargé **automatiquement** par Docker Compose quand il existe. Pas besoin de le spécifier.

---

## 5. Docker Bake Overrides

Docker Bake est un outil plus avancé pour gérer les builds. Il utilise des fichiers HCL (ou JSON) pour définir les cibles de build, avec un système d'overrides puissant.

### `docker-bake.hcl` (configuration de base)

```hcl
variable "TAG" {
  default = "latest"
}

variable "VITE_API_URL" {
  default = "http://localhost:8080"
}

group "default" {
  targets = ["api", "front"]
}

target "api" {
  context    = "."
  dockerfile = "services/api/Dockerfile"
  tags       = ["monapp/api:${TAG}"]
}

target "front" {
  context    = "."
  dockerfile = "services/front/Dockerfile"
  tags       = ["monapp/front:${TAG}"]
  args = {
    VITE_API_URL = VITE_API_URL
  }
}
```

### `docker-bake.override.hcl` (chargé automatiquement)

```hcl
variable "TAG" {
  default = "dev"
}
```

Ce fichier est détecté et mergé automatiquement. La variable `TAG` passe de `latest` à `dev`.

### Override manuel avec un fichier spécifique

```hcl
# prod.hcl
variable "TAG" {
  default = "1.0.0"
}

variable "VITE_API_URL" {
  default = "https://api.monapp.com"
}

target "api" {
  platforms = ["linux/amd64", "linux/arm64"]
}

target "front" {
  platforms = ["linux/amd64", "linux/arm64"]
}
```

```bash
# Charge la base + le fichier prod
docker buildx bake -f docker-bake.hcl -f prod.hcl
```

### Override en ligne de commande avec `--set`

```bash
# Override le tag pour une seule target
docker buildx bake --set api.tags=monapp/api:hotfix-123

# Override pour toutes les targets (wildcard)
docker buildx bake --set *.platform=linux/arm64

# Override un build arg
docker buildx bake --set front.args.VITE_API_URL=https://staging.monapp.com
```

### Override via variables d'environnement

Les variables déclarées dans le fichier Bake peuvent être surchargées par des variables d'environnement du même nom :

```bash
# Le TAG dans docker-bake.hcl sera remplacé par le hash du commit
export TAG=$(git rev-parse --short HEAD)
docker buildx bake --print
```

```json
{
  "target": {
    "api": {
      "tags": ["monapp/api:a1b2c3d"]
    },
    "front": {
      "tags": ["monapp/front:a1b2c3d"]
    }
  }
}
```

---

## 6. Ordre de priorité des overrides dans Bake

Du moins prioritaire au plus prioritaire :

```
1. docker-bake.hcl           (fichier de base)
2. docker-bake.override.hcl  (override automatique)
3. -f fichier.hcl            (override manuel)
4. Variables d'environnement  (export TAG=...)
5. --set flag                 (ligne de commande)
```

Le dernier gagne toujours. Si tu définis `TAG=latest` dans le fichier HCL et que tu fais `export TAG=prod`, c'est `prod` qui sera utilisé.

---

## 7. Bake vs Compose : quand utiliser quoi ?

| Critère | Compose overrides | Bake overrides |
|---|---|---|
| **Build + Run** | ✅ Gère les deux | ❌ Build seulement |
| **Multi-plateforme** | ❌ | ✅ `platforms = [...]` |
| **Wildcards** | ❌ | ✅ `--set *.platform=...` |
| **Variables typées** | ❌ (que des strings) | ✅ (int, bool, string) |
| **CI/CD** | Possible mais lourd | ✅ Conçu pour |
| **Dev local** | ✅ Idéal (volumes, ports) | ❌ Pas prévu pour |

**En pratique pour ton projet :** Compose pour le dev local (volumes, hot-reload, ports). Bake pour la CI/CD (multi-plateforme, tags dynamiques, override par environnement).

---

## 8. Exemple complet : workflow dev → prod

### Structure

```
project/
├── services/
│   ├── api/
│   │   ├── Dockerfile        # Multi-stage : build → scratch
│   │   ├── cmd/
│   │   └── go.mod
│   └── front/
│       ├── Dockerfile        # Multi-stage : deps → build → distroless
│       ├── src/
│       └── package.json
├── docker-compose.yml        # Base
├── docker-compose.override.yml  # Dev (auto-chargé)
├── docker-compose.prod.yml   # Prod
├── docker-bake.hcl           # CI/CD base
└── prod.hcl                  # CI/CD prod overrides
```

### Workflow quotidien

```bash
# Dev — code en local avec hot-reload
air                    # Go
npm run dev            # React

# Dev — si besoin de tester les conteneurs
docker compose up      # Charge automatiquement l'override dev

# Prod — déploiement
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# CI/CD — build multi-plateforme avec tag du commit
TAG=$(git rev-parse --short HEAD) docker buildx bake -f docker-bake.hcl -f prod.hcl
```

---

## 9. Résumé

| Concept | Quoi | Quand |
|---|---|---|
| **Multi-stage** | Plusieurs `FROM` dans un Dockerfile, `COPY --from=` entre stages | Toujours — séparer build et runtime |
| **Compose override** | `docker-compose.override.yml` chargé automatiquement | Dev local — volumes, ports, commandes |
| **Compose `-f`** | Charger plusieurs fichiers Compose manuellement | Prod — `docker-compose.yml` + `docker-compose.prod.yml` |
| **Bake override** | `docker-bake.override.hcl` chargé automatiquement | CI/CD — tags, plateformes |
| **Bake `--set`** | Override en ligne de commande | CI/CD — ponctuellement |
| **Bake env vars** | Variables d'env surchargent les variables HCL | CI/CD — `TAG=$(git rev-parse --short HEAD)` |

Le Dockerfile reste simple : il produit l'image de prod. Toute la flexibilité dev/prod passe par les overrides, pas par des stages supplémentaires.
