# Docker Overrides — Compose & Bake

## Le problème

Un même projet doit tourner dans des contextes différents : dev local, staging, prod. Les différences portent sur les ports, les volumes, les variables d'environnement, les commandes, les tags d'image. Dupliquer les fichiers Compose ou Dockerfile pour chaque environnement est une mauvaise idée — on finit avec des fichiers qui divergent et des bugs impossibles à reproduire.

Les **overrides** permettent de définir une base commune et de ne spécifier que les différences.

---

## Compose Overrides

### Principe

Docker Compose fusionne plusieurs fichiers dans l'ordre où ils sont passés. Les clés du second fichier écrasent celles du premier ; les listes sont **remplacées** (pas concaténées, sauf pour `volumes` et `ports` qui sont fusionnés).

```
docker-compose.yml        ← base (toujours chargé)
docker-compose.override.yml  ← override auto (si présent)
docker-compose.prod.yml   ← override manuel (-f)
```

### Fichier de base

```yaml
# docker-compose.yml
services:
  api:
    build: .
    ports:
      - "4000:4000"
    environment:
      - REDIS_URL=redis://redis:6379

  redis:
    image: redis:8-alpine
```

### Override automatique — dev

Le fichier `docker-compose.override.yml` est chargé **automatiquement** quand il existe dans le même répertoire. Pas besoin de le spécifier.

```yaml
# docker-compose.override.yml
services:
  api:
    volumes:
      - .:/app           # monte le code local pour le hot-reload
    command: air         # remplace le CMD du Dockerfile
    environment:
      - DEBUG=true       # s'ajoute aux variables existantes
```

```bash
# Charge docker-compose.yml + docker-compose.override.yml automatiquement
docker compose up
```

### Override manuel — prod

```yaml
# docker-compose.prod.yml
services:
  api:
    restart: always
    environment:
      - LOG_LEVEL=warn
```

```bash
# Charge uniquement la base + le fichier prod (ignore l'override dev)
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d
```

### Fusion des clés

| Type | Comportement |
|---|---|
| Scalaire (`image`, `command`) | Remplacé |
| Map (`environment`, `build`) | Fusionné (clé par clé) |
| Liste (`ports`, `volumes`) | Fusionné (union) |

```yaml
# base
environment:
  - FOO=1
  - BAR=2

# override
environment:
  - BAR=99   # écrase BAR
  - BAZ=3    # ajoute BAZ

# résultat
environment:
  - FOO=1
  - BAR=99
  - BAZ=3
```

### Plusieurs fichiers

On peut empiler autant de fichiers que nécessaire. Chaque fichier s'applique sur le résultat du précédent.

```bash
docker compose \
  -f docker-compose.yml \
  -f docker-compose.prod.yml \
  -f docker-compose.secrets.yml \
  up -d
```

### `extends` — réutiliser une définition

```yaml
# docker-compose.yml
services:
  api:
    build: .
    ports:
      - "4000:4000"

  api-worker:
    extends:
      service: api     # hérite de toute la config de "api"
    command: ./worker  # surcharge uniquement la commande
    ports: []          # pas de ports exposés pour le worker
```

---

## Bake Overrides

Docker Bake est le système de build de Docker (utilisé en interne par `docker compose build`). Il lit des fichiers HCL ou JSON et permet des overrides plus puissants que Compose, notamment pour la CI/CD.

### Principe

Même logique que Compose : un fichier de base, un ou plusieurs overrides chargés dans l'ordre.

```
docker-bake.hcl              ← base
docker-bake.override.hcl     ← override auto (si présent)
-f fichier.hcl               ← override manuel
variables d'environnement    ← surchargent les variables HCL
--set flag                   ← priorité maximale
```

### Fichier de base

```hcl
# docker-bake.hcl

variable "TAG" {
  default = "latest"
}

variable "REGISTRY" {
  default = "registry.example.com"
}

group "default" {
  targets = ["api", "front"]
}

target "api" {
  context    = "."
  dockerfile = "api/Dockerfile"
  tags       = ["${REGISTRY}/api:${TAG}"]
}

target "front" {
  context    = "."
  dockerfile = "front/Dockerfile"
  tags       = ["${REGISTRY}/front:${TAG}"]
}
```

### Override automatique

```hcl
# docker-bake.override.hcl — chargé automatiquement
variable "TAG" {
  default = "dev"   # écrase "latest" de la base
}

target "api" {
  target = "dev"    # cible un stage spécifique du Dockerfile
}
```

### Override manuel

```hcl
# prod.hcl
variable "TAG" {
  default = "1.4.2"
}

target "api" {
  platforms = ["linux/amd64", "linux/arm64"]
  cache-from = ["type=registry,ref=${REGISTRY}/api:cache"]
  cache-to   = ["type=registry,ref=${REGISTRY}/api:cache,mode=max"]
}

target "front" {
  platforms = ["linux/amd64", "linux/arm64"]
  args = {
    VITE_API_URL = "https://api.prod.example.com"
  }
}
```

```bash
docker buildx bake -f docker-bake.hcl -f prod.hcl --push
```

### Override via variables d'environnement

Les variables déclarées dans le HCL sont automatiquement surchargées par les variables d'environnement du même nom.

```bash
# TAG et REGISTRY lus depuis l'environnement
TAG=$(git rev-parse --short HEAD) \
REGISTRY=ghcr.io/mon-org \
docker buildx bake
```

### Override en ligne de commande avec `--set`

```bash
# Surcharger une propriété d'une target
docker buildx bake --set api.tags=monapp/api:hotfix

# Surcharger pour toutes les targets (wildcard)
docker buildx bake --set "*.platform=linux/arm64"

# Surcharger un build arg
docker buildx bake --set "front.args.VITE_API_URL=https://staging.example.com"

# Vérifier le résultat sans builder
docker buildx bake --print
```

### `--print` — inspecter le résultat final

Avant de lancer un build, tu peux inspecter la configuration fusionnée :

```bash
TAG=abc123 docker buildx bake -f docker-bake.hcl -f prod.hcl --print
```

```json
{
  "target": {
    "api": {
      "context": ".",
      "dockerfile": "api/Dockerfile",
      "tags": ["registry.example.com/api:abc123"],
      "platforms": ["linux/amd64", "linux/arm64"]
    }
  }
}
```

### Ordre de priorité

```
1. docker-bake.hcl             priorité la plus basse
2. docker-bake.override.hcl
3. -f fichier.hcl
4. Variables d'environnement
5. --set flag                  priorité la plus haute
```

---

## Compose vs Bake — quand utiliser quoi

| Besoin | Outil |
|---|---|
| Dev local avec volumes et hot-reload | Compose override |
| Différences de ports ou commandes entre envs | Compose override |
| Build multi-plateforme (`amd64` + `arm64`) | Bake |
| Tags dynamiques en CI/CD | Bake + variables d'env |
| Push vers un registry | Bake `--push` |
| Cache de build distribué | Bake `cache-from` / `cache-to` |

---

## Workflow concret

```bash
# Dev — override auto chargé, volumes montés, hot-reload actif
docker compose up

# Tests d'intégration — prod locale, pas d'override auto
docker compose -f docker-compose.yml up

# Déploiement prod
docker compose -f docker-compose.yml -f docker-compose.prod.yml up -d

# CI/CD — build + push multi-plateforme avec tag du commit
TAG=$(git rev-parse --short HEAD) \
docker buildx bake -f docker-bake.hcl -f prod.hcl --push
```
